package api

import (
	"net/http"

	"github.com/dhawalhost/flagura/web/views"
)

func (s *Server) handleLanding(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	component := views.LandingPage()
	if err := component.Render(r.Context(), w); err != nil {
		http.Error(w, "Templ Render Error: "+err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) handleAuth(w http.ResponseWriter, r *http.Request) {
	// If already authenticated, redirect directly to dashboard
	if user, err := s.getUserFromRequest(r); err == nil && user != nil {
		http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	component := views.AuthPage()
	if err := component.Render(r.Context(), w); err != nil {
		http.Error(w, "Templ Render Error: "+err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	// Verify authenticated session
	user, err := s.getUserFromRequest(r)
	if err != nil || user == nil {
		http.Redirect(w, r, "/auth", http.StatusSeeOther)
		return
	}

	flags, err := s.store.ListFlags(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	logs, _ := s.store.ListAuditLogs(r.Context(), 20)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	component := views.Dashboard(user, flags, logs, s.store.DriverName())
	if err := component.Render(r.Context(), w); err != nil {
		http.Error(w, "Templ Render Error: "+err.Error(), http.StatusInternalServerError)
	}
}
