package api

import (
	"net/http"
	"os"

	"github.com/dhawalhost/flagura/web/views"
)

func (s *Server) handleLanding(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" && r.URL.Path != "/api" && r.URL.Path != "/api/" {
		http.NotFound(w, r)
		return
	}

	// 1. If user is already authenticated, redirect directly to dashboard
	if user, err := s.getUserFromRequest(r); err == nil && user != nil {
		http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
		return
	}

	// 2. Default: Landing page is disabled by default for self-hosted instances (redirect to /auth)
	// Only enabled when explicitly configured (e.g. ENABLE_LANDING_PAGE=true on our official cloud/demo deployment)
	if os.Getenv("ENABLE_LANDING_PAGE") != "true" && os.Getenv("SHOW_LANDING_PAGE") != "true" {
		http.Redirect(w, r, "/auth", http.StatusSeeOther)
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

	showLanding := os.Getenv("ENABLE_LANDING_PAGE") == "true" || os.Getenv("SHOW_LANDING_PAGE") == "true"

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	component := views.AuthPage(showLanding)
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

	projectID := s.resolveProjectID(r)

	// Fetch projects and orgs for selector
	orgs, _ := s.store.ListOrganizations(r.Context())
	projects, _ := s.store.ListProjects(r.Context(), "")

	flags, err := s.store.ListFlagsByProject(r.Context(), projectID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	logs, _ := s.store.ListAuditLogsByProject(r.Context(), projectID, 20)
	changeRequests, _ := s.store.ListChangeRequestsByProject(r.Context(), projectID, "")

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	component := views.Dashboard(user, flags, logs, changeRequests, s.store.DriverName(), orgs, projects, projectID)
	if err := component.Render(r.Context(), w); err != nil {
		http.Error(w, "Templ Render Error: "+err.Error(), http.StatusInternalServerError)
	}
}
