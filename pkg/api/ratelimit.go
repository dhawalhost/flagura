package api

import (
	"encoding/json"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/dhawalhost/flagura/pkg/domain"
	"golang.org/x/time/rate"
)

type visitor struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// IPRateLimiter manages per-IP rate limiters with automatic memory cleanup.
type IPRateLimiter struct {
	mu              sync.RWMutex
	visitors        map[string]*visitor
	rate            rate.Limit
	burst           int
	cleanupInterval time.Duration
	stopCh          chan struct{}
}

// NewIPRateLimiter creates a new rate limiter per IP address.
func NewIPRateLimiter(r rate.Limit, burst int, cleanupInterval time.Duration) *IPRateLimiter {
	limiter := &IPRateLimiter{
		visitors:        make(map[string]*visitor),
		rate:            r,
		burst:           burst,
		cleanupInterval: cleanupInterval,
		stopCh:          make(chan struct{}),
	}

	if cleanupInterval > 0 {
		go limiter.cleanupLoop()
	}

	return limiter
}

// GetLimiter returns or creates the rate.Limiter for a given IP.
func (i *IPRateLimiter) GetLimiter(ip string) *rate.Limiter {
	i.mu.Lock()
	defer i.mu.Unlock()

	v, exists := i.visitors[ip]
	if !exists {
		limiter := rate.NewLimiter(i.rate, i.burst)
		i.visitors[ip] = &visitor{
			limiter:  limiter,
			lastSeen: time.Now(),
		}
		return limiter
	}

	v.lastSeen = time.Now()
	return v.limiter
}

func (i *IPRateLimiter) cleanupLoop() {
	ticker := time.NewTicker(i.cleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			i.mu.Lock()
			threshold := time.Now().Add(-3 * time.Minute)
			for ip, v := range i.visitors {
				if v.lastSeen.Before(threshold) {
					delete(i.visitors, ip)
				}
			}
			i.mu.Unlock()
		case <-i.stopCh:
			return
		}
	}
}

// Close stops the background cleanup goroutine.
func (i *IPRateLimiter) Close() {
	if i.stopCh != nil {
		select {
		case <-i.stopCh:
		default:
			close(i.stopCh)
		}
	}
}

// LimitHandler wraps an http.HandlerFunc with identity-aware rate limiting.
func (i *IPRateLimiter) LimitHandler(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		identity := GetClientIdentity(r)
		limiter := i.GetLimiter(identity)

		if !limiter.Allow() {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"error": map[string]interface{}{
					"code":       domain.ErrCodeRateLimitExceeded,
					"type":       "RATE_LIMIT_EXCEEDED",
					"layer":      "TransportLayer",
					"message":    "Rate limit exceeded. Please retry after a brief pause.",
					"status":     http.StatusTooManyRequests,
					"request_id": RequestIDFromContext(r.Context()),
				},
				"retry_after_seconds": 1,
			})
			return
		}

		next(w, r)
	}
}

// GetClientIdentity extracts a unique rate limiting key by resolving:
// 1. Authenticated API Key: "apikey:<key_id>"
// 2. Authenticated User: "user:<user_id>"
// 3. Remote Client IP: "ip:<client_ip>"
func GetClientIdentity(r *http.Request) string {
	if apiKey := APIKeyFromContext(r.Context()); apiKey != nil && apiKey.ID != "" {
		return "apikey:" + apiKey.ID
	}
	if user := UserFromContext(r.Context()); user != nil && user.ID != "" {
		return "user:" + user.ID
	}
	return "ip:" + GetClientIP(r)
}

// GetClientIP extracts the real client IP address from headers or remote connection.
func GetClientIP(r *http.Request) string {
	// Check X-Forwarded-For (proxy/load balancer)
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		if len(parts) > 0 {
			ip := strings.TrimSpace(parts[0])
			if ip != "" {
				return ip
			}
		}
	}

	// Check X-Real-IP
	if xrip := r.Header.Get("X-Real-IP"); xrip != "" {
		ip := strings.TrimSpace(xrip)
		if ip != "" {
			return ip
		}
	}

	// Fallback to RemoteAddr
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil && host != "" {
		return host
	}

	return r.RemoteAddr
}
