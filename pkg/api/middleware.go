package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"runtime"
	"time"

	"github.com/dhawalhost/flagura/pkg/domain"
)

type contextKey struct {
	name string
}

var (
	userCtxKey      = &contextKey{name: "flagura_user"}
	apiKeyCtxKey    = &contextKey{name: "flagura_api_key"}
	projectCtxKey   = &contextKey{name: "flagura_project_id"}
	requestIDCtxKey = &contextKey{name: "flagura_request_id"}
)

// WithRequestIDContext returns a new context with the request ID.
func WithRequestIDContext(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, requestIDCtxKey, requestID)
}

// RequestIDFromContext retrieves the request ID from the context if present.
func RequestIDFromContext(ctx context.Context) string {
	if reqID, ok := ctx.Value(requestIDCtxKey).(string); ok {
		return reqID
	}
	return ""
}

// WithUserContext returns a new context with the authenticated user.
func WithUserContext(ctx context.Context, user *domain.User) context.Context {
	return context.WithValue(ctx, userCtxKey, user)
}

// UserFromContext retrieves the authenticated user from the context if present.
func UserFromContext(ctx context.Context) *domain.User {
	if user, ok := ctx.Value(userCtxKey).(*domain.User); ok {
		return user
	}
	return nil
}

// WithAPIKeyContext returns a new context with the authenticated API key.
func WithAPIKeyContext(ctx context.Context, key *domain.APIKey) context.Context {
	return context.WithValue(ctx, apiKeyCtxKey, key)
}

// APIKeyFromContext retrieves the authenticated API key from the context if present.
func APIKeyFromContext(ctx context.Context) *domain.APIKey {
	if key, ok := ctx.Value(apiKeyCtxKey).(*domain.APIKey); ok {
		return key
	}
	return nil
}

// WithProjectContext returns a new context with the active project ID.
func WithProjectContext(ctx context.Context, projectID string) context.Context {
	return context.WithValue(ctx, projectCtxKey, projectID)
}

// ProjectFromContext retrieves the active project ID from the context if present.
func ProjectFromContext(ctx context.Context) string {
	if proj, ok := ctx.Value(projectCtxKey).(string); ok {
		return proj
	}
	return ""
}

// RequestIDMiddleware ensures each request has a correlation ID.
func RequestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqID := r.Header.Get(domain.HeaderRequestID)
		if reqID == "" {
			reqID = domain.NewID("req")
		}
		w.Header().Set(domain.HeaderRequestID, reqID)
		ctx := WithRequestIDContext(r.Context(), reqID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// responseWriter wraps http.ResponseWriter to capture status code and bytes written.
type responseWriter struct {
	http.ResponseWriter
	statusCode   int
	bytesWritten int64
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	if rw.statusCode == 0 {
		rw.statusCode = http.StatusOK
	}
	n, err := rw.ResponseWriter.Write(b)
	rw.bytesWritten += int64(n)
	return n, err
}

func (rw *responseWriter) Flush() {
	if f, ok := rw.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// StructuredLoggerMiddleware logs every request with slog structured attributes.
func StructuredLoggerMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rw := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

		next.ServeHTTP(rw, r)

		duration := time.Since(start)
		reqID := RequestIDFromContext(r.Context())

		// Skip high-frequency health probes in log if successful
		if (r.URL.Path == "/livez" || r.URL.Path == "/readyz" || r.URL.Path == "/healthz") && rw.statusCode == http.StatusOK {
			return
		}

		var userID string
		if u := UserFromContext(r.Context()); u != nil {
			userID = u.ID
		}

		slog.InfoContext(r.Context(), "http_request",
			slog.String("method", r.Method),
			slog.String("path", r.URL.Path),
			slog.Int("status", rw.statusCode),
			slog.Int64("bytes", rw.bytesWritten),
			slog.Float64("duration_ms", float64(duration.Microseconds())/1000.0),
			slog.String("request_id", reqID),
			slog.String("user_id", userID),
			slog.String("remote_addr", r.RemoteAddr),
		)
	})
}

// PanicRecoveryMiddleware recovers from handler panics and writes a safe 500 JSON envelope.
func (s *Server) PanicRecoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				reqID := RequestIDFromContext(r.Context())
				stack := make([]byte, 4096)
				stackLen := runtime.Stack(stack, false)

				slog.ErrorContext(r.Context(), "panic_recovered",
					slog.Any("panic", rec),
					slog.String("request_id", reqID),
					slog.String("stack", string(stack[:stackLen])),
				)

				s.writeError(w, r, domain.NewAppError(
					domain.ErrCodeInternal,
					"An unexpected internal error occurred. Please contact support.",
					http.StatusInternalServerError,
					domain.ErrInternal,
				))
			}
		}()
		next.ServeHTTP(w, r)
	})
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
