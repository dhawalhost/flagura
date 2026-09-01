package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dhawalhost/flagura/pkg/domain"
	"github.com/dhawalhost/flagura/pkg/store"
)

func TestRequestIDMiddleware(t *testing.T) {
	var capturedContextID string

	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedContextID = RequestIDFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	handler := RequestIDMiddleware(testHandler)

	// Case 1: Header provided
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set(domain.HeaderRequestID, "req_custom_trace_123")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Header().Get(domain.HeaderRequestID) != "req_custom_trace_123" {
		t.Errorf("expected response header to contain req_custom_trace_123, got %s", rec.Header().Get(domain.HeaderRequestID))
	}
	if capturedContextID != "req_custom_trace_123" {
		t.Errorf("expected context to contain req_custom_trace_123, got %s", capturedContextID)
	}

	// Case 2: Header omitted (auto-generation)
	req2 := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec2 := httptest.NewRecorder()

	handler.ServeHTTP(rec2, req2)

	generatedID := rec2.Header().Get(domain.HeaderRequestID)
	if generatedID == "" || !strings.HasPrefix(generatedID, "req_") {
		t.Errorf("expected generated ID starting with req_, got %s", generatedID)
	}
	if capturedContextID != generatedID {
		t.Errorf("expected context ID to match response header %s, got %s", generatedID, capturedContextID)
	}
}

func TestPanicRecoveryMiddleware(t *testing.T) {
	memStore := store.NewMemoryStore()
	server, err := NewServer(memStore)
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	panickingHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("database connection pointer is nil")
	})

	// Wrap panickingHandler with RequestID and PanicRecovery
	pipeline := RequestIDMiddleware(server.PanicRecoveryMiddleware(panickingHandler))

	req := httptest.NewRequest(http.MethodGet, "/panic", nil)
	req.Header.Set(domain.HeaderRequestID, "req_panic_trace_999")
	rec := httptest.NewRecorder()

	pipeline.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected HTTP 500 on panic, got %d", rec.Code)
	}

	var errResp struct {
		Error struct {
			Code      int    `json:"code"`
			Type      string `json:"type"`
			Layer     string `json:"layer"`
			Message   string `json:"message"`
			Status    int    `json:"status"`
			RequestID string `json:"request_id"`
		} `json:"error"`
	}

	if err := json.Unmarshal(rec.Body.Bytes(), &errResp); err != nil {
		t.Fatalf("failed to decode error JSON: %v, body: %s", err, rec.Body.String())
	}

	if errResp.Error.Code != int(domain.ErrCodeInternal) {
		t.Errorf("expected code %d, got %d", domain.ErrCodeInternal, errResp.Error.Code)
	}
	if errResp.Error.RequestID != "req_panic_trace_999" {
		t.Errorf("expected request_id req_panic_trace_999, got %s", errResp.Error.RequestID)
	}
}

func TestLivenessAndReadinessProbes(t *testing.T) {
	memStore := store.NewMemoryStore()
	server, err := NewServer(memStore)
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	// 1. Test /livez
	reqLive := httptest.NewRequest(http.MethodGet, "/livez", nil)
	recLive := httptest.NewRecorder()
	server.ServeHTTP(recLive, reqLive)

	if recLive.Code != http.StatusOK {
		t.Fatalf("expected 200 OK from /livez, got %d", recLive.Code)
	}

	// 2. Test /readyz
	reqReady := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	recReady := httptest.NewRecorder()
	server.ServeHTTP(recReady, reqReady)

	if recReady.Code != http.StatusOK {
		t.Fatalf("expected 200 OK from /readyz, got %d", recReady.Code)
	}
}
