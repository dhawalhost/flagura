package api

import (
	"io/fs"
	"net/http"
	"strings"

	"github.com/dhawalhost/flagura/internal/store"
	"github.com/dhawalhost/flagura/web"
)

type Server struct {
	store store.Store
	mux   *http.ServeMux
}

func NewServer(st store.Store) (*Server, error) {
	s := &Server{
		store: st,
		mux:   http.NewServeMux(),
	}
	s.routes()
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

	// Auth API Routes
	s.mux.HandleFunc("/api/v1/auth/signup", s.handleSignUp)
	s.mux.HandleFunc("/api/v1/auth/login", s.handleLogin)
	s.mux.HandleFunc("/api/v1/auth/logout", s.handleLogout)
	s.mux.HandleFunc("/api/v1/auth/me", s.handleMe)

	// REST API Routes
	s.mux.HandleFunc("/api/health", s.handleHealth)
	s.mux.HandleFunc("/api/v1/flags", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			s.handleGetFlags(w, r)
		case http.MethodPost:
			s.handleCreateFlag(w, r)
		default:
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		}
	})

	s.mux.HandleFunc("/api/v1/flags/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if strings.HasSuffix(path, "/toggle") {
			if r.Method == http.MethodPatch || r.Method == http.MethodPost {
				s.handleToggleFlag(w, r)
				return
			}
		}
		if strings.HasSuffix(path, "/rollout") {
			if r.Method == http.MethodPatch || r.Method == http.MethodPost {
				s.handleUpdateRollout(w, r)
				return
			}
		}

		switch r.Method {
		case http.MethodPut, http.MethodPatch, http.MethodPost:
			s.handleUpdateFlag(w, r)
		case http.MethodDelete:
			s.handleDeleteFlag(w, r)
		default:
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		}
	})

	s.mux.HandleFunc("/api/v1/evaluate", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			s.handleEvaluate(w, r)
			return
		}
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
	})

	s.mux.HandleFunc("/api/v1/benchmark", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			s.handleBenchmark(w, r)
			return
		}
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
	})

	s.mux.HandleFunc("/api/v1/audit-logs", s.handleGetAuditLogs)
	s.mux.HandleFunc("/api/v1/reset", s.handleReset)
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

	s.mux.ServeHTTP(w, r)
}
