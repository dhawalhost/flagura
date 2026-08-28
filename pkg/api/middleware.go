package api

import (
	"context"
	"encoding/json"
	"net/http"
	"os"

	"github.com/dhawalhost/flagura/pkg/domain"
)

type contextKey string

const userContextKey contextKey = "flagura_user"

// WithUserContext returns a new context with the authenticated user.
func WithUserContext(ctx context.Context, user *domain.User) context.Context {
	return context.WithValue(ctx, userContextKey, user)
}

// UserFromContext retrieves the authenticated user from the context if present.
func UserFromContext(ctx context.Context) *domain.User {
	if user, ok := ctx.Value(userContextKey).(*domain.User); ok {
		return user
	}
	return nil
}

// SecurityHeadersMiddleware adds industry-standard security headers to all responses.
func SecurityHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "SAMEORIGIN")
		w.Header().Set("X-XSS-Protection", "1; mode=block")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")

		// In production or HTTPS, enforce HSTS
		if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" || os.Getenv("ENVIRONMENT") == "production" {
			w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}

		// Content Security Policy allowing required CDNs and embedded resources
		csp := "default-src 'self'; " +
			"script-src 'self' 'unsafe-inline' 'unsafe-eval' https://cdn.tailwindcss.com https://cdn.jsdelivr.net https://unpkg.com https://cdnjs.cloudflare.com; " +
			"style-src 'self' 'unsafe-inline' https://fonts.googleapis.com; " +
			"font-src 'self' https://fonts.gstatic.com data:; " +
			"img-src 'self' data: https: blob:; " +
			"connect-src 'self' https: wss: ws:;"
		w.Header().Set("Content-Security-Policy", csp)

		next.ServeHTTP(w, r)
	})
}

// MaxBytesMiddleware limits the maximum request body size (default 1 MB) to prevent DoS.
func MaxBytesMiddleware(maxBytes int64, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Body != nil && (r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodPatch) {
			r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
		}
		next.ServeHTTP(w, r)
	})
}

// RequireAuth ensures that the request is authenticated via session cookie or Bearer token.
func (s *Server) RequireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, err := s.getUserFromRequest(r)
		if err != nil || user == nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"error":   "Unauthorized",
				"message": "Authentication required. Please provide a valid session cookie or Bearer token.",
			})
			return
		}

		ctx := WithUserContext(r.Context(), user)
		next(w, r.WithContext(ctx))
	}
}

// RequireRole ensures that the authenticated user has the required role (or is admin).
func (s *Server) RequireRole(role domain.UserRole, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := UserFromContext(r.Context())
		if user == nil {
			var err error
			user, err = s.getUserFromRequest(r)
			if err != nil || user == nil {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				_ = json.NewEncoder(w).Encode(map[string]interface{}{
					"error":   "Unauthorized",
					"message": "Authentication required.",
				})
				return
			}
			r = r.WithContext(WithUserContext(r.Context(), user))
		}

		// Admin has access to all actions; otherwise check matching role
		if user.Role != domain.RoleAdmin && user.Role != role {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"error":   "Forbidden",
				"message": "Insufficient permissions to perform this operation.",
			})
			return
		}

		next(w, r)
	}
}
