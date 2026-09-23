package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/dhawalhost/flagura/pkg/domain"
	"golang.org/x/time/rate"
)

func TestIPRateLimiter(t *testing.T) {
	// 2 requests per second, burst 2
	limiter := NewIPRateLimiter(rate.Limit(2), 2, 0)
	defer limiter.Close()

	handler := limiter.LimitHandler(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	req1 := httptest.NewRequest(http.MethodGet, "/test", nil)
	req1.RemoteAddr = "192.168.1.100:1234"
	w1 := httptest.NewRecorder()
	handler(w1, req1)
	if w1.Code != http.StatusOK {
		t.Fatalf("expected req1 to pass (200 OK), got %d", w1.Code)
	}
	if w1.Header().Get("X-RateLimit-Limit") == "" {
		t.Errorf("expected X-RateLimit-Limit header to be set")
	}

	req2 := httptest.NewRequest(http.MethodGet, "/test", nil)
	req2.RemoteAddr = "192.168.1.100:1234"
	w2 := httptest.NewRecorder()
	handler(w2, req2)
	if w2.Code != http.StatusOK {
		t.Fatalf("expected req2 to pass (200 OK), got %d", w2.Code)
	}

	// 3rd rapid request from same IP should be blocked (429)
	req3 := httptest.NewRequest(http.MethodGet, "/test", nil)
	req3.RemoteAddr = "192.168.1.100:1234"
	w3 := httptest.NewRecorder()
	handler(w3, req3)
	if w3.Code != http.StatusTooManyRequests {
		t.Fatalf("expected req3 to be rate-limited (429 Too Many Requests), got %d", w3.Code)
	}
	if w3.Header().Get("Retry-After") != "1" {
		t.Errorf("expected Retry-After header '1', got '%s'", w3.Header().Get("Retry-After"))
	}
	if w3.Header().Get("X-RateLimit-Remaining") != "0" {
		t.Errorf("expected X-RateLimit-Remaining '0', got '%s'", w3.Header().Get("X-RateLimit-Remaining"))
	}

	// Request from different IP should be allowed immediately
	reqOtherIP := httptest.NewRequest(http.MethodGet, "/test", nil)
	reqOtherIP.RemoteAddr = "10.0.0.1:5678"
	wOther := httptest.NewRecorder()
	handler(wOther, reqOtherIP)
	if wOther.Code != http.StatusOK {
		t.Fatalf("expected req from other IP to pass, got %d", wOther.Code)
	}
}

func TestTenantRateLimiter_Tiers(t *testing.T) {
	limiter := NewIPRateLimiter(0, 0, 0).EnableTiers(true)
	defer limiter.Close()

	handler := limiter.LimitHandler(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	tests := []struct {
		name          string
		headerKey     string
		tier          string
		expectedLimit string
	}{
		{"Anonymous Tier", "X-RateLimit-Tier", "anonymous", "120"},
		{"Authenticated Tier", "X-RateLimit-Tier", "authenticated", "1200"},
		{"System Tier", "X-RateLimit-Tier", "system", "12000"},
		{"Standard Alias", "X-Traffic-Tier", "standard", "120"},
		{"Elevated Alias", "X-Traffic-Tier", "elevated", "1200"},
		{"High-Throughput Alias", "X-Traffic-Tier", "high-throughput", "12000"},
		{"Free Alias", "X-Tenant-Tier", "free", "120"},
		{"Pro Alias", "X-Tenant-Tier", "pro", "1200"},
		{"Enterprise Alias", "X-Tenant-Tier", "enterprise", "12000"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/evaluate", nil)
			req.Header.Set(tt.headerKey, tt.tier)
			req.RemoteAddr = "192.168.10.1:1234"
			w := httptest.NewRecorder()

			handler(w, req)
			if w.Code != http.StatusOK {
				t.Fatalf("expected status 200, got %d", w.Code)
			}
			if limit := w.Header().Get("X-RateLimit-Limit"); limit != tt.expectedLimit {
				t.Errorf("expected X-RateLimit-Limit %q, got %q", tt.expectedLimit, limit)
			}
			if rem := w.Header().Get("X-RateLimit-Remaining"); rem == "" {
				t.Errorf("expected non-empty X-RateLimit-Remaining")
			}
			if reset := w.Header().Get("X-RateLimit-Reset"); reset == "" {
				t.Errorf("expected non-empty X-RateLimit-Reset")
			}
		})
	}
}

func TestTenantRateLimiter_TenantIsolation(t *testing.T) {
	limiter := NewIPRateLimiter(rate.Limit(1), 1, 0)
	defer limiter.Close()

	handler := limiter.LimitHandler(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Tenant 1 API key
	key1 := &domain.APIKey{
		ID:        "key-1",
		ProjectID: "proj-1",
		Role:      domain.RoleDeveloper,
	}
	ctx1 := WithAPIKeyContext(context.Background(), key1)
	req1 := httptest.NewRequest(http.MethodGet, "/api/v1/flags", nil).WithContext(ctx1)
	w1 := httptest.NewRecorder()
	handler(w1, req1)
	if w1.Code != http.StatusOK {
		t.Fatalf("expected tenant 1 request 1 to pass, got %d", w1.Code)
	}

	// Tenant 1 request 2 should be rate limited (burst 1 exhausted)
	req1Repeat := httptest.NewRequest(http.MethodGet, "/api/v1/flags", nil).WithContext(ctx1)
	w1Repeat := httptest.NewRecorder()
	handler(w1Repeat, req1Repeat)
	if w1Repeat.Code != http.StatusTooManyRequests {
		t.Fatalf("expected tenant 1 request 2 to be blocked, got %d", w1Repeat.Code)
	}

	// Tenant 2 API key should be completely unaffected
	key2 := &domain.APIKey{
		ID:        "key-2",
		ProjectID: "proj-2",
		Role:      domain.RoleDeveloper,
	}
	ctx2 := WithAPIKeyContext(context.Background(), key2)
	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/flags", nil).WithContext(ctx2)
	w2 := httptest.NewRecorder()
	handler(w2, req2)
	if w2.Code != http.StatusOK {
		t.Fatalf("expected tenant 2 to pass despite tenant 1 being blocked, got %d", w2.Code)
	}
}

func TestGetClientIP(t *testing.T) {
	// 1. Without proxy trust (default secure behavior)
	_ = os.Setenv("FLAGURA_TRUST_PROXY", "false")
	reqSpoofed := httptest.NewRequest(http.MethodGet, "/", nil)
	reqSpoofed.RemoteAddr = "192.0.2.1:45678"
	reqSpoofed.Header.Set("X-Forwarded-For", "203.0.113.195, 70.41.3.18")
	reqSpoofed.Header.Set("X-Real-IP", "198.51.100.1")
	if ip := GetClientIP(reqSpoofed); ip != "192.0.2.1" {
		t.Errorf("expected remote addr '192.0.2.1' when proxy trust is disabled, got '%s'", ip)
	}

	// 2. With proxy trust enabled
	_ = os.Setenv("FLAGURA_TRUST_PROXY", "true")
	defer os.Unsetenv("FLAGURA_TRUST_PROXY")

	// 2a. X-Forwarded-For
	reqXFF := httptest.NewRequest(http.MethodGet, "/", nil)
	reqXFF.RemoteAddr = "192.0.2.1:45678"
	reqXFF.Header.Set("X-Forwarded-For", "203.0.113.195, 70.41.3.18")
	if ip := GetClientIP(reqXFF); ip != "203.0.113.195" {
		t.Errorf("expected IP '203.0.113.195', got '%s'", ip)
	}

	// 2b. X-Real-IP
	reqReal := httptest.NewRequest(http.MethodGet, "/", nil)
	reqReal.RemoteAddr = "192.0.2.1:45678"
	reqReal.Header.Set("X-Real-IP", "198.51.100.1")
	if ip := GetClientIP(reqReal); ip != "198.51.100.1" {
		t.Errorf("expected IP '198.51.100.1', got '%s'", ip)
	}

	// 2c. Fallback to RemoteAddr with port
	reqRemote := httptest.NewRequest(http.MethodGet, "/", nil)
	reqRemote.RemoteAddr = "192.0.2.1:45678"
	if ip := GetClientIP(reqRemote); ip != "192.0.2.1" {
		t.Errorf("expected IP '192.0.2.1', got '%s'", ip)
	}
}
