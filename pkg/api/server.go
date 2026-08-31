package api

import (
	"encoding/json"
	"io/fs"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/dhawalhost/flagura/pkg/canary"
	"github.com/dhawalhost/flagura/pkg/domain"
	"github.com/dhawalhost/flagura/pkg/email"
	"github.com/dhawalhost/flagura/pkg/store"
	"github.com/dhawalhost/flagura/web"
)

type Server struct {
	store       store.Store
	mux         *http.ServeMux
	handler     http.Handler
	startTime   time.Time
	evalCount   uint64
	authLimiter *IPRateLimiter
	apiLimiter  *IPRateLimiter
	streamHub   *StreamHub
	telemetry   *TelemetryAggregator
	canary      *canary.CanaryScheduler
	mailer      email.Mailer
}

func NewServer(st store.Store) (*Server, error) {
	hub := NewStreamHub()
	go hub.Run()

	canarySched := canary.NewCanaryScheduler(st, hub)
	canarySched.StartBackgroundLoop(15 * time.Second)

	s := &Server{
		store:       st,
		mux:         http.NewServeMux(),
		startTime:   time.Now().UTC(),
		authLimiter: NewIPRateLimiter(5, 10, 1*time.Minute),
		apiLimiter:  NewIPRateLimiter(200, 400, 1*time.Minute),
		streamHub:   hub,
		telemetry:   NewTelemetryAggregator(),
		canary:      canarySched,
		mailer:      email.NewMailerFromEnv(),
	}
	s.routes()
	s.handler = s.PanicRecoveryMiddleware(
		RequestIDMiddleware(
			StructuredLoggerMiddleware(
				SecurityHeadersMiddleware(
					MaxBytesMiddleware(1<<20, s.mux),
				),
			),
		),
	)
	return s, nil
}

func (s *Server) SetMailer(m email.Mailer) {
	s.mailer = m
}

func (s *Server) routes() {
	// Static asset server from embedded filesystem
	staticFS, err := fs.Sub(web.Files, "static")
	if err == nil {
		s.mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticFS))))
	}

	// UI Web Routes
	s.mux.HandleFunc("/", s.handleLanding)
	s.mux.HandleFunc("/auth", s.handleAuth)
	s.mux.HandleFunc("/dashboard", s.handleDashboard)

	// Auth API Routes (Rate limited for brute-force protection)
	s.mux.HandleFunc("/api/v1/auth/signup", s.authLimiter.LimitHandler(s.handleSignUp))
	s.mux.HandleFunc("/api/v1/auth/login", s.authLimiter.LimitHandler(s.handleLogin))
	s.mux.HandleFunc("/api/v1/auth/forgot-password", s.authLimiter.LimitHandler(s.handleForgotPassword))
	s.mux.HandleFunc("/api/v1/auth/reset-password", s.authLimiter.LimitHandler(s.handleResetPassword))
	s.mux.HandleFunc("/api/v1/auth/logout", s.handleLogout)
	s.mux.HandleFunc("/api/v1/auth/me", s.handleMe)

	// Public Observability & Webhook Routes
	s.mux.HandleFunc("/api/health", s.handleHealthz)
	s.mux.HandleFunc("/healthz", s.handleHealthz)
	s.mux.HandleFunc("/livez", s.handleLivez)
	s.mux.HandleFunc("/readyz", s.handleReadyz)
	s.mux.HandleFunc("/metrics", s.handleMetrics)
	s.mux.HandleFunc("/api/v1/flags/stream", s.handleFlagsStream)
	s.mux.HandleFunc("/api/v1/telemetry/events", s.apiLimiter.LimitHandler(s.handleIngestTelemetry))
	s.mux.HandleFunc("/api/v1/telemetry/stats", s.handleGetTelemetryStats)
	s.mux.HandleFunc("/api/v1/webhooks/kill-switch/", s.apiLimiter.LimitHandler(s.handleWebhookKillSwitch))

	// Flag Management API Routes
	s.mux.HandleFunc("/api/v1/flags", s.apiLimiter.LimitHandler(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			s.RequireAuth(s.handleGetFlags)(w, r)
		case http.MethodPost:
			s.RequireAuth(s.handleCreateFlag)(w, r)
		default:
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		}
	}))

	s.mux.HandleFunc("/api/v1/flags/", s.apiLimiter.LimitHandler(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if strings.Contains(path, "/canary") {
			s.RequireAuth(s.handleCanaryRoutes)(w, r)
			return
		}
		if strings.HasSuffix(path, "/toggle") {
			if r.Method == http.MethodPatch || r.Method == http.MethodPost {
				s.RequireAuth(s.handleToggleFlag)(w, r)
				return
			}
		}
		if strings.HasSuffix(path, "/rollout") {
			if r.Method == http.MethodPatch || r.Method == http.MethodPost {
				s.RequireAuth(s.handleUpdateRollout)(w, r)
				return
			}
		}
		if strings.HasSuffix(path, "/promote") {
			if r.Method == http.MethodPost {
				s.RequireAuth(s.handlePromoteEnvironment)(w, r)
				return
			}
		}
		if strings.HasSuffix(path, "/experiment") {
			if r.Method == http.MethodGet {
				s.RequireAuth(s.handleGetExperimentReport)(w, r)
				return
			}
		}

		switch r.Method {
		case http.MethodPut, http.MethodPatch, http.MethodPost:
			s.RequireAuth(s.handleUpdateFlag)(w, r)
		case http.MethodDelete:
			s.RequireAuth(s.RequireRole(domain.RoleAdmin, s.handleDeleteFlag))(w, r)
		default:
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		}
	}))

	// Organizations & Projects API Routes
	s.mux.HandleFunc("/api/v1/organizations", s.apiLimiter.LimitHandler(s.RequireAuth(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			s.handleListOrganizations(w, r)
		} else if r.Method == http.MethodPost {
			s.RequireRole(domain.RoleAdmin, s.handleCreateOrganization)(w, r)
		} else {
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		}
	})))

	s.mux.HandleFunc("/api/v1/projects", s.apiLimiter.LimitHandler(s.RequireAuth(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			s.handleListProjects(w, r)
		} else if r.Method == http.MethodPost {
			s.handleCreateProject(w, r)
		} else {
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		}
	})))
	s.mux.HandleFunc("/api/v1/projects/active", s.apiLimiter.LimitHandler(s.RequireAuth(s.handleSwitchActiveProject)))
	s.mux.HandleFunc("/api/v1/projects/", s.apiLimiter.LimitHandler(s.RequireAuth(s.handleGetProject)))

	s.mux.HandleFunc("/api/v1/events", s.apiLimiter.LimitHandler(s.handleIngestEvents))
	s.mux.HandleFunc("/api/v1/experiments/", s.apiLimiter.LimitHandler(s.RequireAuth(s.handleGetExperimentReport)))
	s.mux.HandleFunc("/api/v1/change-requests", s.apiLimiter.LimitHandler(s.RequireAuth(s.handleListOrCreateChangeRequests)))
	s.mux.HandleFunc("/api/v1/change-requests/", s.apiLimiter.LimitHandler(s.RequireAuth(s.handleChangeRequestItem)))
	s.mux.HandleFunc("/api/v1/api-keys", s.apiLimiter.LimitHandler(s.RequireAuth(s.handleListOrCreateAPIKeys)))
	s.mux.HandleFunc("/api/v1/api-keys/", s.apiLimiter.LimitHandler(s.RequireAuth(s.handleRevokeAPIKey)))

	s.mux.HandleFunc("/api/v1/evaluate", s.apiLimiter.LimitHandler(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			s.handleEvaluate(w, r)
			return
		}
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
	}))

	s.mux.HandleFunc("/api/v1/benchmark", s.apiLimiter.LimitHandler(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			s.handleBenchmark(w, r)
			return
		}
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
	}))

	s.mux.HandleFunc("/api/v1/audit-logs", s.RequireAuth(s.handleGetAuditLogs))
	s.mux.HandleFunc("/api/v1/reset", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
			return
		}
		s.RequireAuth(s.RequireRole(domain.RoleAdmin, s.handleReset))(w, r)
	})
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	origin := r.Header.Get("Origin")
	allowedOrigin := os.Getenv("FLAGURA_ALLOWED_ORIGIN")
	if allowedOrigin != "" {
		w.Header().Set("Access-Control-Allow-Origin", allowedOrigin)
		w.Header().Set("Access-Control-Allow-Credentials", "true")
	} else if origin != "" {
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Access-Control-Allow-Credentials", "true")
	} else {
		w.Header().Set("Access-Control-Allow-Origin", "*")
	}

	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-API-Key, X-Webhook-Secret, X-Actor, X-Project-ID, X-Organization-ID")

	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	s.handler.ServeHTTP(w, r)
}

// writeError outputs a structured application error response adhering to Google/Stripe API standards.
func (s *Server) writeError(w http.ResponseWriter, r *http.Request, err error) {
	if err == nil {
		return
	}
	appErr := domain.MapSentinelToAppError(err)
	reqID := RequestIDFromContext(r.Context())

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(appErr.HTTPStatus)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"error": map[string]interface{}{
			"code":       appErr.Code,
			"type":       appErr.Type,
			"layer":      appErr.Layer,
			"message":    appErr.Message,
			"status":     appErr.HTTPStatus,
			"request_id": reqID,
		},
	})
}

// writeJSON serializes data to JSON with standard Content-Type header.
func (s *Server) writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}
