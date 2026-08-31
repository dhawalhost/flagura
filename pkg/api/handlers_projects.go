package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/dhawalhost/flagura/pkg/domain"
)

func (s *Server) handleListOrganizations(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	orgs, err := s.store.ListOrganizations(r.Context())
	if err != nil {
		s.writeError(w, r, domain.NewAppError(domain.ErrCodeDatabaseQuery, err.Error(), http.StatusInternalServerError, err))
		return
	}

	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"organizations": orgs,
		"count":         len(orgs),
	})
}

func (s *Server) handleCreateOrganization(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Name        string `json:"name"`
		Slug        string `json:"slug"`
		Description string `json:"description"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, r, domain.NewAppError(domain.ErrCodeMalformedPayload, err.Error(), http.StatusBadRequest, err))
		return
	}

	if strings.TrimSpace(req.Name) == "" {
		s.writeError(w, r, domain.NewAppError(domain.ErrCodeMalformedPayload, "organization name is required", http.StatusBadRequest, domain.ErrInvalidInput))
		return
	}

	org := domain.NewOrganization(req.Name, req.Slug, req.Description)

	created, err := s.store.CreateOrganization(r.Context(), org)
	if err != nil {
		s.writeError(w, r, domain.NewAppError(domain.ErrCodeOrgConflict, err.Error(), http.StatusBadRequest, err))
		return
	}

	s.writeJSON(w, http.StatusCreated, created)
}

func (s *Server) handleListProjects(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	orgID := r.URL.Query().Get("organization_id")
	if orgID == "" {
		orgID = r.URL.Query().Get("org_id")
	}

	projects, err := s.store.ListProjects(r.Context(), orgID)
	if err != nil {
		s.writeError(w, r, domain.NewAppError(domain.ErrCodeDatabaseQuery, err.Error(), http.StatusInternalServerError, err))
		return
	}

	activeProjectID := s.resolveProjectID(r)

	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"projects":          projects,
		"count":             len(projects),
		"active_project_id": activeProjectID,
	})
}

func (s *Server) handleCreateProject(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		OrganizationID string `json:"organization_id"`
		Name           string `json:"name"`
		Slug           string `json:"slug"`
		Description    string `json:"description"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, r, domain.NewAppError(domain.ErrCodeMalformedPayload, err.Error(), http.StatusBadRequest, err))
		return
	}

	if strings.TrimSpace(req.Name) == "" {
		s.writeError(w, r, domain.NewAppError(domain.ErrCodeMalformedPayload, "project name is required", http.StatusBadRequest, domain.ErrInvalidInput))
		return
	}
	if req.OrganizationID == "" {
		req.OrganizationID = domain.DefaultOrgID
	}

	proj := domain.NewProject(req.OrganizationID, req.Name, req.Slug, req.Description)

	created, err := s.store.CreateProject(r.Context(), proj)
	if err != nil {
		s.writeError(w, r, domain.NewAppError(domain.ErrCodeProjectConflict, err.Error(), http.StatusBadRequest, err))
		return
	}

	// Set active project cookie upon creation for seamless UX
	http.SetCookie(w, &http.Cookie{
		Name:     domain.CookieProjectName,
		Value:    created.ID,
		Path:     "/",
		Expires:  time.Now().Add(30 * 24 * time.Hour),
		HttpOnly: false,
		SameSite: http.SameSiteLaxMode,
	})

	s.writeJSON(w, http.StatusCreated, created)
}

func (s *Server) handleGetProject(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	id := strings.TrimPrefix(r.URL.Path, "/api/v1/projects/")
	id = strings.TrimSpace(strings.Split(id, "/")[0])
	if id == "" {
		id = s.resolveProjectID(r)
	}

	proj, err := s.store.GetProject(r.Context(), id)
	if err != nil {
		s.writeError(w, r, domain.NewAppError(domain.ErrCodeProjectNotFound, err.Error(), http.StatusNotFound, domain.ErrProjectNotFound))
		return
	}

	s.writeJSON(w, http.StatusOK, proj)
}

func (s *Server) handleSwitchActiveProject(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		ProjectID string `json:"project_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, r, domain.NewAppError(domain.ErrCodeMalformedPayload, err.Error(), http.StatusBadRequest, err))
		return
	}

	if req.ProjectID == "" {
		req.ProjectID = domain.DefaultProjectID
	}

	// Verify project exists
	proj, err := s.store.GetProject(r.Context(), req.ProjectID)
	if err != nil {
		s.writeError(w, r, domain.NewAppError(domain.ErrCodeProjectNotFound, "project not found: "+req.ProjectID, http.StatusNotFound, domain.ErrProjectNotFound))
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     domain.CookieProjectName,
		Value:    proj.ID,
		Path:     "/",
		Expires:  time.Now().Add(30 * 24 * time.Hour),
		HttpOnly: false,
		SameSite: http.SameSiteLaxMode,
	})

	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"success":           true,
		"active_project_id": proj.ID,
		"active_project":    proj,
	})
}
