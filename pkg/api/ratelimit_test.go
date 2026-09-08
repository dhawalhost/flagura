package api

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

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

	// Request from different IP should be allowed immediately
	reqOtherIP := httptest.NewRequest(http.MethodGet, "/test", nil)
	reqOtherIP.RemoteAddr = "10.0.0.1:5678"
	wOther := httptest.NewRecorder()
	handler(wOther, reqOtherIP)
	if wOther.Code != http.StatusOK {
		t.Fatalf("expected req from other IP to pass, got %d", wOther.Code)
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
