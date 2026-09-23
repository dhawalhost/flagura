package api

import (
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/dhawalhost/flagura/pkg/domain"
	"golang.org/x/time/rate"
)

// RateLimitTier represents the traffic allocation tier of a client, project, or token.
type RateLimitTier string

// TrafficTier and TenantTier are aliases for RateLimitTier.
type TrafficTier = RateLimitTier
type TenantTier = RateLimitTier

const (
	// Role-based operational tiers
	TierAnonymous     RateLimitTier = "anonymous"     // Unauthenticated / public callers (120 req/min, burst 30)
	TierAuthenticated RateLimitTier = "authenticated" // Authenticated apps, SDK clients & developers (1,200 req/min, burst 100)
	TierSystem        RateLimitTier = "system"        // High-throughput production services & admin tokens (12,000 req/min, burst 500)

	// Operational aliases
	TierStandard       RateLimitTier = "anonymous"
	TierElevated       RateLimitTier = "authenticated"
	TierHighThroughput RateLimitTier = "system"

	// Backward-compatible aliases
	TierFree       RateLimitTier = "anonymous"
	TierPro        RateLimitTier = "authenticated"
	TierEnterprise RateLimitTier = "system"
)

// TierConfig defines rate limit characteristics for a traffic tier.
type TierConfig struct {
	Rate           rate.Limit // tokens per second
	Burst          int
	QuotaPerMinute int
}

// DefaultTierConfigs provides default limits for the standard traffic tiers:
// - Anonymous: 120 req/min (2 req/s), burst 30
// - Authenticated: 1,200 req/min (20 req/s), burst 100
// - System: 12,000 req/min (200 req/s), burst 500
var DefaultTierConfigs = map[RateLimitTier]TierConfig{
	TierAnonymous: {
		Rate:           rate.Limit(2),
		Burst:          30,
		QuotaPerMinute: 120,
	},
	TierAuthenticated: {
		Rate:           rate.Limit(20),
		Burst:          100,
		QuotaPerMinute: 1200,
	},
	TierSystem: {
		Rate:           rate.Limit(200),
		Burst:          500,
		QuotaPerMinute: 12000,
	},
}

func getTierConfig(tier RateLimitTier) TierConfig {
	config, ok := DefaultTierConfigs[tier]
	if !ok {
		config = DefaultTierConfigs[TierAnonymous]
	}

	switch tier {
	case TierAnonymous:
		rpm := os.Getenv("FLAGURA_RATE_LIMIT_ANONYMOUS_RPM")
		if rpm == "" {
			rpm = os.Getenv("FLAGURA_RATE_LIMIT_STANDARD_RPM")
		}
		if rpm == "" {
			rpm = os.Getenv("FLAGURA_RATE_LIMIT_FREE_RPM")
		}
		if rpm != "" {
			if v, err := strconv.Atoi(rpm); err == nil && v > 0 {
				config.QuotaPerMinute = v
				config.Rate = rate.Limit(float64(v) / 60.0)
			}
		}

		burst := os.Getenv("FLAGURA_RATE_LIMIT_ANONYMOUS_BURST")
		if burst == "" {
			burst = os.Getenv("FLAGURA_RATE_LIMIT_STANDARD_BURST")
		}
		if burst == "" {
			burst = os.Getenv("FLAGURA_RATE_LIMIT_FREE_BURST")
		}
		if burst != "" {
			if v, err := strconv.Atoi(burst); err == nil && v > 0 {
				config.Burst = v
			}
		}

	case TierAuthenticated:
		rpm := os.Getenv("FLAGURA_RATE_LIMIT_AUTHENTICATED_RPM")
		if rpm == "" {
			rpm = os.Getenv("FLAGURA_RATE_LIMIT_ELEVATED_RPM")
		}
		if rpm == "" {
			rpm = os.Getenv("FLAGURA_RATE_LIMIT_PRO_RPM")
		}
		if rpm != "" {
			if v, err := strconv.Atoi(rpm); err == nil && v > 0 {
				config.QuotaPerMinute = v
				config.Rate = rate.Limit(float64(v) / 60.0)
			}
		}

		burst := os.Getenv("FLAGURA_RATE_LIMIT_AUTHENTICATED_BURST")
		if burst == "" {
			burst = os.Getenv("FLAGURA_RATE_LIMIT_ELEVATED_BURST")
		}
		if burst == "" {
			burst = os.Getenv("FLAGURA_RATE_LIMIT_PRO_BURST")
		}
		if burst != "" {
			if v, err := strconv.Atoi(burst); err == nil && v > 0 {
				config.Burst = v
			}
		}

	case TierSystem:
		rpm := os.Getenv("FLAGURA_RATE_LIMIT_SYSTEM_RPM")
		if rpm == "" {
			rpm = os.Getenv("FLAGURA_RATE_LIMIT_HIGH_THROUGHPUT_RPM")
		}
		if rpm == "" {
			rpm = os.Getenv("FLAGURA_RATE_LIMIT_ENTERPRISE_RPM")
		}
		if rpm != "" {
			if v, err := strconv.Atoi(rpm); err == nil && v > 0 {
				config.QuotaPerMinute = v
				config.Rate = rate.Limit(float64(v) / 60.0)
			}
		}

		burst := os.Getenv("FLAGURA_RATE_LIMIT_SYSTEM_BURST")
		if burst == "" {
			burst = os.Getenv("FLAGURA_RATE_LIMIT_HIGH_THROUGHPUT_BURST")
		}
		if burst == "" {
			burst = os.Getenv("FLAGURA_RATE_LIMIT_ENTERPRISE_BURST")
		}
		if burst != "" {
			if v, err := strconv.Atoi(burst); err == nil && v > 0 {
				config.Burst = v
			}
		}
	}

	return config
}

type visitor struct {
	limiter  *rate.Limiter
	tier     RateLimitTier
	lastSeen time.Time
}

// IPRateLimiter manages per-identity and per-tenant rate limiters with automatic memory cleanup.
type IPRateLimiter struct {
	mu              sync.RWMutex
	visitors        map[string]*visitor
	rate            rate.Limit
	burst           int
	cleanupInterval time.Duration
	enableTiers     bool
	stopCh          chan struct{}
}

// NewIPRateLimiter creates a new rate limiter per IP/identity address.
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

// EnableTiers enables or disables tier-based quota allocation (Anonymous/Authenticated/System).
func (i *IPRateLimiter) EnableTiers(enable bool) *IPRateLimiter {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.enableTiers = enable
	return i
}

// GetLimiter returns or creates the rate.Limiter for a given identity with anonymous tier.
func (i *IPRateLimiter) GetLimiter(identity string) *rate.Limiter {
	l, _ := i.GetLimiterForTier(identity, TierAnonymous)
	return l
}

// GetLimiterForTier returns or creates the rate.Limiter for a given identity and tier.
func (i *IPRateLimiter) GetLimiterForTier(identity string, tier RateLimitTier) (*rate.Limiter, TierConfig) {
	config := getTierConfig(tier)

	// If tiers are not enabled and a custom non-zero rate was provided at creation, use it.
	if !i.enableTiers && i.rate > 0 {
		config = TierConfig{
			Rate:           i.rate,
			Burst:          i.burst,
			QuotaPerMinute: int(float64(i.rate) * 60),
		}
	}

	i.mu.Lock()
	defer i.mu.Unlock()

	v, exists := i.visitors[identity]
	if !exists || (i.enableTiers && v.tier != tier) {
		limiter := rate.NewLimiter(config.Rate, config.Burst)
		i.visitors[identity] = &visitor{
			limiter:  limiter,
			tier:     tier,
			lastSeen: time.Now(),
		}
		return limiter, config
	}

	v.lastSeen = time.Now()
	return v.limiter, config
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

// LimitHandler wraps an http.HandlerFunc with identity- and tier-aware rate limiting,
// injecting standard rate limit response headers into every response.
func (i *IPRateLimiter) LimitHandler(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		identity, tier := ResolveTenantIdentityAndTier(r)
		limiter, config := i.GetLimiterForTier(identity, tier)

		allowed := limiter.Allow()
		remaining := int(limiter.Tokens())
		if remaining < 0 {
			remaining = 0
		}

		limitQuota := config.QuotaPerMinute
		if limitQuota <= 0 {
			limitQuota = config.Burst
		}
		if remaining > limitQuota {
			remaining = limitQuota
		}

		var secondsToReset int64 = 1
		if config.Rate > 0 {
			missing := float64(config.Burst - remaining)
			if missing > 0 {
				secondsToReset = int64(missing/float64(config.Rate) + 0.999)
			}
		}
		if secondsToReset < 1 {
			secondsToReset = 1
		}
		resetEpoch := time.Now().Unix() + secondsToReset

		var retryAfter int64 = 1
		if config.Rate > 0 {
			timeForOne := 1.0 / float64(config.Rate)
			if timeForOne > 1 {
				retryAfter = int64(timeForOne + 0.999)
			}
		}

		w.Header().Set("X-RateLimit-Limit", strconv.Itoa(limitQuota))
		w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(remaining))
		w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(resetEpoch, 10))

		if !allowed {
			slog.WarnContext(r.Context(), "security_event",
				slog.String("event_type", "rate_limit_exceeded"),
				slog.String("identity", identity),
				slog.String("tier", string(tier)),
				slog.String("ip", GetClientIP(r)),
				slog.String("path", r.URL.Path),
				slog.String("method", r.Method),
				slog.String("user_agent", r.UserAgent()),
				slog.String("request_id", RequestIDFromContext(r.Context())),
			)
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Retry-After", strconv.FormatInt(retryAfter, 10))
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
				"retry_after_seconds": retryAfter,
			})
			return
		}

		next(w, r)
	}
}

// ResolveTenantIdentityAndTier extracts the identity key and traffic rate limiting tier:
// 1. Explicit X-RateLimit-Tier, X-Traffic-Tier, or X-Tenant-Tier header
// 2. API Key context: Admin=TierSystem, Developer=TierAuthenticated
// 3. User session context: Admin=TierSystem, User=TierAuthenticated
// 4. Authorization Header (Bearer token)
// 5. Session Cookie
// 6. Project Scope (Header, Query parameter, or Project Cookie)
// 7. Remote Client IP: "ip:<ip>" (TierAnonymous)
func ResolveTenantIdentityAndTier(r *http.Request) (string, RateLimitTier) {
	identity := "ip:" + GetClientIP(r)
	tier := TierAnonymous

	if apiKey := APIKeyFromContext(r.Context()); apiKey != nil && apiKey.ID != "" {
		tier = TierAuthenticated
		if apiKey.Role == domain.RoleAdmin {
			tier = TierSystem
		}
		if apiKey.ProjectID != "" {
			identity = "tenant:" + apiKey.ProjectID + ":apikey:" + apiKey.ID
		} else {
			identity = "apikey:" + apiKey.ID
		}
	} else if user := UserFromContext(r.Context()); user != nil && user.ID != "" {
		tier = TierAuthenticated
		if user.Role == domain.RoleAdmin {
			tier = TierSystem
		}
		identity = "user:" + user.ID
	} else if authHeader := r.Header.Get("Authorization"); strings.HasPrefix(authHeader, "Bearer ") {
		token := strings.TrimSpace(strings.TrimPrefix(authHeader, "Bearer "))
		if token != "" {
			if strings.HasPrefix(token, "flg_") {
				identity = "apikey:" + token
			} else {
				identity = "user:session:" + token
			}
			tier = TierAuthenticated
		}
	} else if sessionCookie, err := r.Cookie(domain.CookieSessionName); err == nil && sessionCookie.Value != "" {
		identity = "user:session:" + sessionCookie.Value
		tier = TierAuthenticated
	} else {
		projectID := r.Header.Get(domain.HeaderProjectID)
		if projectID == "" {
			projectID = r.URL.Query().Get("project_id")
		}
		if projectID == "" {
			projectID = r.URL.Query().Get("project")
		}
		if projectID == "" {
			if c, err := r.Cookie(domain.CookieProjectName); err == nil && c.Value != "" {
				projectID = c.Value
			}
		}
		if projectID != "" {
			identity = "tenant:" + projectID + ":ip:" + GetClientIP(r)
		}
	}

	tierHeader := r.Header.Get("X-RateLimit-Tier")
	if tierHeader == "" {
		tierHeader = r.Header.Get("X-Traffic-Tier")
	}
	if tierHeader == "" {
		tierHeader = r.Header.Get("X-Tenant-Tier")
	}
	if tierHeader != "" {
		normalized := strings.ToLower(strings.TrimSpace(tierHeader))
		switch normalized {
		case "anonymous", "public", "standard", "free", "default":
			tier = TierAnonymous
		case "authenticated", "client", "elevated", "pro", "developer":
			tier = TierAuthenticated
		case "system", "admin", "cluster", "high-throughput", "enterprise", "production":
			tier = TierSystem
		}
	}

	return identity, tier
}

// GetClientIdentity extracts a unique rate limiting key by resolving:
// 1. Authenticated API Key: "apikey:<key_id>"
// 2. Authenticated User: "user:<user_id>"
// 3. Remote Client IP: "ip:<client_ip>"
func GetClientIdentity(r *http.Request) string {
	id, _ := ResolveTenantIdentityAndTier(r)
	return id
}

// GetClientIP extracts the real client IP address. By default, it extracts directly
// from r.RemoteAddr to prevent header spoofing. If FLAGURA_TRUST_PROXY=true or
// TRUST_PROXY=true, it parses and validates X-Forwarded-For / X-Real-IP headers.
func GetClientIP(r *http.Request) string {
	trustProxy := strings.EqualFold(os.Getenv("FLAGURA_TRUST_PROXY"), "true") || strings.EqualFold(os.Getenv("TRUST_PROXY"), "true")

	if trustProxy {
		// Check X-Forwarded-For (proxy/load balancer)
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			parts := strings.Split(xff, ",")
			if len(parts) > 0 {
				ip := strings.TrimSpace(parts[0])
				if net.ParseIP(ip) != nil {
					return ip
				}
			}
		}

		// Check X-Real-IP
		if xrip := r.Header.Get("X-Real-IP"); xrip != "" {
			ip := strings.TrimSpace(xrip)
			if net.ParseIP(ip) != nil {
				return ip
			}
		}
	}

	// RemoteAddr
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil && host != "" {
		return host
	}

	return r.RemoteAddr
}
