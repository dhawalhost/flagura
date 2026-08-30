package api

import (
	"io/fs"
	"net/http"
	"strings"
	"time"

	"github.com/dhawalhost/flagura/pkg/canary"
	"github.com/dhawalhost/flagura/pkg/domain"
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
	}
	s.routes()
	s.handler = SecurityHeadersMiddleware(MaxBytesMiddleware(1<<20, s.mux))
	return s, nil
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
	s.mux.HandleFunc("/api/v1/auth/logout", s.handleLogout)
	s.mux.HandleFunc("/api/v1/auth/me", s.handleMe)

	// Public Observability & Webhook Routes
	s.mux.HandleFunc("/api/health", s.handleHealthz)
	s.mux.HandleFunc("/healthz", s.handleHealthz)
	s.mux.HandleFunc("/livez", s.handleHealthz)
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
			s.handleGetFlags(w, r)
		case http.MethodPost:
			s.RequireAuth(s.handleCreateFlag)(w, r)
		default:
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		}
	}))

	s.mux.HandleFunc("/api/v1/flags/", s.apiLimiter.LimitHandler(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if strings.Contains(path, "/canary") {
			s.handleCanaryRoutes(w, r)
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
				s.handleGetExperimentReport(w, r)
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

	s.mux.HandleFunc("/api/v1/events", s.apiLimiter.LimitHandler(s.handleIngestEvents))
	s.mux.HandleFunc("/api/v1/experiments/", s.apiLimiter.LimitHandler(s.handleGetExperimentReport))
	s.mux.HandleFunc("/api/v1/change-requests", s.apiLimiter.LimitHandler(s.handleListOrCreateChangeRequests))
	s.mux.HandleFunc("/api/v1/change-requests/", s.apiLimiter.LimitHandler(s.handleChangeRequestItem))

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

	s.mux.HandleFunc("/api/v1/audit-logs", s.handleGetAuditLogs)
	s.mux.HandleFunc("/api/v1/reset", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
			return
		}
		s.RequireAuth(s.RequireRole(domain.RoleAdmin, s.handleReset))(w, r)
	})
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Enable CORS for API requests
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Actor")

	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	s.handler.ServeHTTP(w, r)
}
