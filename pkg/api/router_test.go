package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestChainMiddlewares(t *testing.T) {
	var trace []string

	m1 := func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			trace = append(trace, "m1_pre")
			next(w, r)
			trace = append(trace, "m1_post")
		}
	}

	m2 := func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			trace = append(trace, "m2_pre")
			next(w, r)
			trace = append(trace, "m2_post")
		}
	}

	handler := func(w http.ResponseWriter, r *http.Request) {
		trace = append(trace, "handler")
		w.WriteHeader(http.StatusOK)
	}

	chained := ChainMiddlewares(handler, m1, m2)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	chained(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	expected := []string{"m1_pre", "m2_pre", "handler", "m2_post", "m1_post"}
	if len(trace) != len(expected) {
		t.Fatalf("expected trace %v, got %v", expected, trace)
	}
	for i, v := range expected {
		if trace[i] != v {
			t.Errorf("at index %d: expected %q, got %q", i, v, trace[i])
		}
	}
}

func TestMethodRouter(t *testing.T) {
	mr := NewMethodRouter()
	mr.Handle(http.MethodGet, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("get_ok"))
	})
	mr.Handle(http.MethodPost, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte("post_ok"))
	})

	// 1. GET allowed
	reqGet := httptest.NewRequest(http.MethodGet, "/test", nil)
	recGet := httptest.NewRecorder()
	mr.ServeHTTP(recGet, reqGet)
	if recGet.Code != http.StatusOK || recGet.Body.String() != "get_ok" {
		t.Fatalf("expected 200 get_ok, got %d %q", recGet.Code, recGet.Body.String())
	}

	// 2. POST allowed
	reqPost := httptest.NewRequest(http.MethodPost, "/test", nil)
	recPost := httptest.NewRecorder()
	mr.ServeHTTP(recPost, reqPost)
	if recPost.Code != http.StatusCreated || recPost.Body.String() != "post_ok" {
		t.Fatalf("expected 201 post_ok, got %d %q", recPost.Code, recPost.Body.String())
	}

	// 3. DELETE not allowed -> 405 Method Not Allowed + Allow header
	reqDelete := httptest.NewRequest(http.MethodDelete, "/test", nil)
	recDelete := httptest.NewRecorder()
	mr.ServeHTTP(recDelete, reqDelete)
	if recDelete.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405 Method Not Allowed, got %d", recDelete.Code)
	}
	if recDelete.Header().Get("Allow") != "GET, POST" {
		t.Errorf("expected Allow header 'GET, POST', got %q", recDelete.Header().Get("Allow"))
	}
}
