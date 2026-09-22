package api

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/dhawalhost/flagura/pkg/domain"
)

const SessionCookieName = domain.CookieSessionName

func generateSessionToken() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

func isRequestHTTPS(r *http.Request) bool {
	if r == nil {
		return false
	}
	if r.TLS != nil {
		return true
	}
	// Check X-Forwarded-Proto (standard reverse proxy header)
	proto := r.Header.Get("X-Forwarded-Proto")
	if proto != "" {
		// In multi-hop setups, take the first hop (the client-to-proxy connection)
		parts := strings.Split(proto, ",")
		if strings.EqualFold(strings.TrimSpace(parts[0]), "https") {
			return true
		}
	}
	// Cloudflare, AWS ALB, Nginx, Front-End headers
	if strings.EqualFold(r.Header.Get("X-Forwarded-Ssl"), "on") ||
		strings.EqualFold(r.Header.Get("Front-End-Https"), "on") ||
		strings.EqualFold(r.Header.Get("X-Url-Scheme"), "https") ||
		strings.Contains(r.Header.Get("CF-Visitor"), `"https"`) {
		return true
	}
	return false
}

func resolveCookieDomain(r *http.Request) string {
	if domainEnv := os.Getenv("COOKIE_DOMAIN"); domainEnv != "" {
		return domainEnv
	}
	if r == nil {
		return ""
	}
	host := r.Host
	if host == "" {
		return ""
	}
	// Strip port if present
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	// Avoid domain cookie for localhost or raw IP
	if host == "localhost" || host == "127.0.0.1" || host == "::1" || strings.HasPrefix(host, "[") || net.ParseIP(host) != nil {
		return ""
	}

	// For flagura.dev / subdomains, share across subdomains by returning apex "flagura.dev"
	if strings.HasSuffix(host, "flagura.dev") {
		return "flagura.dev"
	}

	return host
}

func isCookieSecure(r *http.Request) bool {
	if strings.EqualFold(os.Getenv("ALLOW_INSECURE_COOKIES"), "true") || strings.EqualFold(os.Getenv("SECURE_COOKIE"), "false") {
		return false
	}
	if strings.EqualFold(os.Getenv("SECURE_COOKIE"), "true") {
		return true
	}
	if r == nil {
		return true
	}
	if isRequestHTTPS(r) {
		return true
	}
	if strings.EqualFold(os.Getenv("FLAGURA_ENV"), "development") || strings.EqualFold(os.Getenv("ENV"), "development") {
		return false
	}
	host := r.Host
	if strings.HasPrefix(host, "localhost") || strings.HasPrefix(host, "127.0.0.1") || strings.HasPrefix(host, "[::1]") {
		return false
	}
	// Fallback: If connection is not HTTPS, setting Secure=true causes modern browsers
	// to drop the cookie completely (RFC 6265bis). Only return true if request is HTTPS.
	return isRequestHTTPS(r)
}

func (s *Server) setProjectCookie(w http.ResponseWriter, r *http.Request, projectID string, expiresAt time.Time) {
	isSecure := isCookieSecure(r)
	maxAge := int(time.Until(expiresAt).Seconds())
	if maxAge < 0 {
		maxAge = 0
	}
	cookieDomain := resolveCookieDomain(r)

	// #nosec G124 -- active project selection cookie configured with SameSite and dynamic TLS
	http.SetCookie(w, &http.Cookie{
		Name:     domain.CookieProjectName,
		Value:    projectID,
		Path:     "/",
		Domain:   cookieDomain,
		Expires:  expiresAt,
		MaxAge:   maxAge,
		HttpOnly: false,
		SameSite: http.SameSiteLaxMode,
		Secure:   isSecure,
	})
}

func (s *Server) clearProjectCookie(w http.ResponseWriter, r *http.Request) {
	isSecure := isCookieSecure(r)
	cookieDomain := resolveCookieDomain(r)

	// 1. Clear host-only cookie
	// #nosec G124 -- active project selection cookie configured with SameSite and dynamic TLS
	http.SetCookie(w, &http.Cookie{
		Name:     domain.CookieProjectName,
		Value:    "",
		Path:     "/",
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
		HttpOnly: false,
		SameSite: http.SameSiteLaxMode,
		Secure:   isSecure,
	})

	// 2. Clear domain-scoped cookie if domain resolved
	if cookieDomain != "" {
		// #nosec G124 -- active project selection cookie configured with SameSite and dynamic TLS
		http.SetCookie(w, &http.Cookie{
			Name:     domain.CookieProjectName,
			Value:    "",
			Path:     "/",
			Domain:   cookieDomain,
			Expires:  time.Unix(0, 0),
			MaxAge:   -1,
			HttpOnly: false,
			SameSite: http.SameSiteLaxMode,
			Secure:   isSecure,
		})
	}
}

func (s *Server) setSessionCookie(w http.ResponseWriter, r *http.Request, token string, expiresAt time.Time) {
	isSecure := isCookieSecure(r)
	maxAge := int(time.Until(expiresAt).Seconds())
	if maxAge < 0 {
		maxAge = 0
	}
	cookieDomain := resolveCookieDomain(r)

	// #nosec G124 -- dynamic secure flag based on TLS and environment
	http.SetCookie(w, &http.Cookie{
		Name:     domain.CookieSessionName,
		Value:    token,
		Path:     "/",
		Domain:   cookieDomain,
		Expires:  expiresAt,
		MaxAge:   maxAge,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   isSecure,
	})
}

func (s *Server) clearSessionCookie(w http.ResponseWriter, r *http.Request) {
	isSecure := isCookieSecure(r)
	cookieDomain := resolveCookieDomain(r)

	// 1. Clear host-only cookie
	// #nosec G124 -- dynamic secure flag based on TLS and environment
	http.SetCookie(w, &http.Cookie{
		Name:     domain.CookieSessionName,
		Value:    "",
		Path:     "/",
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   isSecure,
	})

	// 2. Clear domain-scoped cookie if domain resolved
	if cookieDomain != "" {
		// #nosec G124 -- dynamic secure flag based on TLS and environment
		http.SetCookie(w, &http.Cookie{
			Name:     domain.CookieSessionName,
			Value:    "",
			Path:     "/",
			Domain:   cookieDomain,
			Expires:  time.Unix(0, 0),
			MaxAge:   -1,
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
			Secure:   isSecure,
		})
	}
}

func (s *Server) getUserFromRequest(r *http.Request) (*domain.User, error) {
	// Collect session cookie candidate tokens (in order, to handle shadowed/multiple cookies)
	var candidateCookies []string
	for _, c := range r.Cookies() {
		if c.Name == domain.CookieSessionName && c.Value != "" {
			clean := strings.Trim(strings.TrimSpace(c.Value), "\"")
			if clean != "" {
				candidateCookies = append(candidateCookies, clean)
			}
		}
	}

	// Authorization Header (Bearer token)
	var bearerToken string
	authHeader := r.Header.Get(domain.HeaderAuthorization)
	if strings.HasPrefix(authHeader, "Bearer ") {
		bearerToken = strings.Trim(strings.TrimSpace(strings.TrimPrefix(authHeader, "Bearer ")), "\"")
	}

	// X-API-Key Header
	var apiKeyHeader string
	if val := r.Header.Get(domain.HeaderAPIKey); val != "" {
		apiKeyHeader = strings.Trim(strings.TrimSpace(val), "\"")
	}

	// 1. Check master API key if configured
	apiKeyEnv := os.Getenv("FLAGURA_API_KEY")
	for _, tok := range []string{apiKeyHeader, bearerToken} {
		if tok != "" && apiKeyEnv != "" && subtle.ConstantTimeCompare([]byte(tok), []byte(apiKeyEnv)) == 1 {
			return &domain.User{
				ID:    "usr_api_key_service",
				Email: "api-service@flagura.dev",
				Name:  "API Service Account",
				Role:  domain.RoleAdmin,
			}, nil
		}
		if tok != "" {
			h := sha256.Sum256([]byte(tok))
			keyHash := hex.EncodeToString(h[:])
			if apiKey, err := s.store.GetAPIKeyByHash(r.Context(), keyHash); err == nil && apiKey != nil && !apiKey.Revoked {
				return &domain.User{
					ID:    apiKey.ID,
					Email: apiKey.CreatedBy,
					Name:  apiKey.Name,
					Role:  apiKey.Role,
				}, nil
			}
		}
	}

	// 2. Evaluate candidate session tokens (evaluate all candidates to handle cookie shadowing)
	var candidates []string
	candidates = append(candidates, candidateCookies...)
	if bearerToken != "" {
		candidates = append(candidates, bearerToken)
	}

	if len(candidates) == 0 {
		return nil, domain.ErrUnauthorized
	}

	for _, token := range candidates {
		sess, err := s.store.GetSession(r.Context(), token)
		if err != nil || sess == nil {
			continue
		}

		if sess.User != nil {
			return sess.User, nil
		}
		if sess.UserID != "" {
			if u, err := s.store.GetUserByID(r.Context(), sess.UserID); err == nil && u != nil {
				return u, nil
			}
		}
	}

	return nil, domain.ErrUnauthorized
}

func (s *Server) getAPIKeyFromRequest(r *http.Request) *domain.APIKey {
	var token string
	authHeader := r.Header.Get(domain.HeaderAuthorization)
	if strings.HasPrefix(authHeader, "Bearer ") {
		token = strings.TrimPrefix(authHeader, "Bearer ")
	}
	if token == "" {
		token = r.Header.Get(domain.HeaderAPIKey)
	}
	if token == "" {
		token = r.URL.Query().Get("api_key")
	}
	if token == "" {
		token = r.URL.Query().Get("token")
	}
	if token == "" {
		return nil
	}

	h := sha256.Sum256([]byte(token))
	keyHash := hex.EncodeToString(h[:])
	if apiKey, err := s.store.GetAPIKeyByHash(r.Context(), keyHash); err == nil && apiKey != nil && !apiKey.Revoked {
		return apiKey
	}
	return nil
}

func validatePasswordComplexity(password string) error {
	if len(password) < 8 {
		return fmt.Errorf("password must be at least 8 characters long")
	}

	var hasUpper, hasLower, hasDigit, hasSpecial bool
	for _, ch := range password {
		switch {
		case ch >= 'A' && ch <= 'Z':
			hasUpper = true
		case ch >= 'a' && ch <= 'z':
			hasLower = true
		case ch >= '0' && ch <= '9':
			hasDigit = true
		case strings.ContainsRune("!@#$%^&*()_+-=[]{}|;:,.<>/?~", ch):
			hasSpecial = true
		}
	}

	if !hasUpper {
		return fmt.Errorf("password must contain at least one uppercase letter (A-Z)")
	}
	if !hasLower {
		return fmt.Errorf("password must contain at least one lowercase letter (a-z)")
	}
	if !hasDigit {
		return fmt.Errorf("password must contain at least one number (0-9)")
	}
	if !hasSpecial {
		return fmt.Errorf("password must contain at least one special symbol (!@#$%%^&*)")
	}

	return nil
}
