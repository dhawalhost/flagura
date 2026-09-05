package api

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
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

func (s *Server) setProjectCookie(w http.ResponseWriter, r *http.Request, projectID string, expiresAt time.Time) {
	isSecure := false
	if r != nil && (r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https") {
		isSecure = true
	}
	if os.Getenv("ENVIRONMENT") == string(domain.EnvProduction) || os.Getenv("SECURE_COOKIE") == "true" {
		isSecure = true
	}

	// #nosec G124 -- active project selection cookie configured with SameSite and dynamic TLS
	http.SetCookie(w, &http.Cookie{
		Name:     domain.CookieProjectName,
		Value:    projectID,
		Path:     "/",
		Expires:  expiresAt,
		HttpOnly: false,
		SameSite: http.SameSiteLaxMode,
		Secure:   isSecure,
	})
}

func (s *Server) setSessionCookie(w http.ResponseWriter, r *http.Request, token string, expiresAt time.Time) {
	isSecure := false
	if r != nil && (r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https") {
		isSecure = true
	}
	if os.Getenv("ENVIRONMENT") == string(domain.EnvProduction) || os.Getenv("SECURE_COOKIE") == "true" {
		isSecure = true
	}

	// #nosec G124 -- dynamic secure flag based on TLS and environment
	http.SetCookie(w, &http.Cookie{
		Name:     domain.CookieSessionName,
		Value:    token,
		Path:     "/",
		Expires:  expiresAt,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   isSecure,
	})
}

func (s *Server) clearSessionCookie(w http.ResponseWriter, r *http.Request) {
	isSecure := false
	if r != nil && (r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https") {
		isSecure = true
	}
	if os.Getenv("ENVIRONMENT") == string(domain.EnvProduction) || os.Getenv("SECURE_COOKIE") == "true" {
		isSecure = true
	}

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
}

func (s *Server) getUserFromRequest(r *http.Request) (*domain.User, error) {
	var token string

	// 1. Check HTTP Cookie
	if cookie, err := r.Cookie(domain.CookieSessionName); err == nil && cookie.Value != "" {
		token = cookie.Value
	}

	// 2. Fallback to Authorization Header (Bearer token)
	if token == "" {
		authHeader := r.Header.Get(domain.HeaderAuthorization)
		if strings.HasPrefix(authHeader, "Bearer ") {
			token = strings.TrimPrefix(authHeader, "Bearer ")
		}
	}

	// 3. Fallback to X-API-Key Header
	if token == "" {
		token = r.Header.Get(domain.HeaderAPIKey)
	}

	if token == "" {
		return nil, domain.ErrUnauthorized
	}

	// Check master API key if configured
	apiKeyEnv := os.Getenv("FLAGURA_API_KEY")
	if apiKeyEnv != "" && subtle.ConstantTimeCompare([]byte(token), []byte(apiKeyEnv)) == 1 {
		return &domain.User{
			ID:    "usr_api_key_service",
			Email: "api-service@flagura.dev",
			Name:  "API Service Account",
			Role:  domain.RoleAdmin,
		}, nil
	}

	// Check stored dynamic API Keys via SHA-256 hash lookup
	h := sha256.Sum256([]byte(token))
	keyHash := hex.EncodeToString(h[:])
	if apiKey, err := s.store.GetAPIKeyByHash(r.Context(), keyHash); err == nil && apiKey != nil && !apiKey.Revoked {
		return &domain.User{
			ID:    apiKey.ID,
			Email: apiKey.CreatedBy,
			Name:  apiKey.Name,
			Role:  apiKey.Role,
		}, nil
	}

	sess, err := s.store.GetSession(r.Context(), token)
	if err != nil {
		return nil, domain.ErrUnauthorized
	}

	if sess.User == nil {
		return nil, domain.ErrUserNotFound
	}

	return sess.User, nil
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
