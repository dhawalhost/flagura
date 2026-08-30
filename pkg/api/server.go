package api

import (
	"io/fs"
	"net/http"
	"strings"
	"time"

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
}

func NewServer(st store.Store) (*Server, error) {
	s := &Server{
		store:       st,
		mux:         http.NewServeMux(),
		startTime:   time.Now().UTC(),
		authLimiter: NewIPRateLimiter(5, 10, 1*time.Minute),
		apiLimiter:  NewIPRateLimiter(200, 400, 1*time.Minute),
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

	// REST API Routes & Observability Probes
	s.mux.HandleFunc("/api/health", s.handleHealth)
	s.mux.HandleFunc("/healthz", s.handleHealthz)
	s.mux.HandleFunc("/livez", s.handleHealthz)
	s.mux.HandleFunc("/readyz", s.handleReadyz)
	s.mux.HandleFunc("/metrics", s.handleMetrics)
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

		switch r.Method {
		case http.MethodPut, http.MethodPatch, http.MethodPost:
			s.RequireAuth(s.handleUpdateFlag)(w, r)
		case http.MethodDelete:
			s.RequireAuth(s.RequireRole(domain.RoleAdmin, s.handleDeleteFlag))(w, r)
		default:
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		}
	}))

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
