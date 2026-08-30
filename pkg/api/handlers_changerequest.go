package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/dhawalhost/flagura/pkg/domain"
)

// handleListOrCreateChangeRequests handles GET and POST on /api/v1/change-requests
func (s *Server) handleListOrCreateChangeRequests(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		status := domain.ChangeRequestStatus(r.URL.Query().Get("status"))
		projectID := s.resolveProjectID(r)
		crs, err := s.store.ListChangeRequestsByProject(r.Context(), projectID, status)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"project_id":      projectID,
			"change_requests": crs,
		})
	case http.MethodPost:
		s.RequireAuth(s.handleCreateChangeRequest)(w, r)
	default:
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
	}
}

// handleCreateChangeRequest creates a new 4-Eyes ChangeRequest.
func (s *Server) handleCreateChangeRequest(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req domain.ChangeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON payload: "+err.Error(), http.StatusBadRequest)
		return
	}

	if req.FlagKey == "" {
		http.Error(w, "flag_key is required", http.StatusBadRequest)
		return
	}
	if req.Environment == "" {
		req.Environment = domain.EnvProduction
	}
	if req.ProjectID == "" {
		req.ProjectID = s.resolveProjectID(r)
	}

	req.AuthorUserID = user.ID
	req.AuthorEmail = user.Email
	req.AuthorName = user.Name

	created, err := s.store.CreateChangeRequest(r.Context(), req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if s.streamHub != nil {
		s.streamHub.Broadcast("change_request_created", created)
	}

	if s.mailer != nil && s.mailer.IsEnabled() {
		scheme := "http"
		if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
			scheme = "https"
		}
		host := r.Host
		if host == "" {
			host = "localhost:3000"
		}
		reviewURL := fmt.Sprintf("%s://%s/dashboard", scheme, host)
		govEmails := s.mailer.GetGovernanceEmails()
		if len(govEmails) == 0 {
			if users, err := s.store.ListUsers(r.Context()); err == nil {
				for _, u := range users {
					if u.Role == domain.RoleAdmin && u.ID != req.AuthorUserID {
						govEmails = append(govEmails, u.Email)
					}
				}
			}
		}

		if len(govEmails) > 0 {
			go func(gov []string, req domain.ChangeRequest, revURL string) {
				actionType := req.Title
				if actionType == "" {
					actionType = "Config Modification"
				}
				for _, recipient := range gov {
					_ = s.mailer.SendChangeRequestNotification(recipient, "Governance Reviewer", req.AuthorName, req.FlagKey, string(req.Environment), actionType, revURL)
				}
			}(govEmails, *created, reviewURL)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(created)
}

// handleChangeRequestItem handles routes under /api/v1/change-requests/:id
func (s *Server) handleChangeRequestItem(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/change-requests/")
	parts := strings.Split(strings.Trim(path, "/"), "/")

	if len(parts) == 0 || parts[0] == "" {
		http.Error(w, "Change request ID required", http.StatusBadRequest)
		return
	}
	id := parts[0]

	if len(parts) == 1 {
		if r.Method == http.MethodGet {
			cr, err := s.store.GetChangeRequest(r.Context(), id)
			if err != nil {
				http.Error(w, err.Error(), http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(cr)
			return
		}
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	action := parts[1]
	switch action {
	case "review":
		if r.Method != http.MethodPost {
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
			return
		}
		s.RequireAuth(func(w http.ResponseWriter, r *http.Request) {
			s.handleReviewChangeRequest(w, r, id)
		})(w, r)
	case "apply":
		if r.Method != http.MethodPost {
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
			return
		}
		s.RequireAuth(s.RequireRole(domain.RoleAdmin, func(w http.ResponseWriter, r *http.Request) {
			s.handleApplyChangeRequest(w, r, id)
		}))(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) handleReviewChangeRequest(w http.ResponseWriter, r *http.Request, id string) {
	user := UserFromContext(r.Context())
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req struct {
		Approved bool   `json:"approved"`
		Comments string `json:"comments"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON payload: "+err.Error(), http.StatusBadRequest)
		return
	}

	updated, err := s.store.ReviewChangeRequest(r.Context(), id, user.ID, user.Email, user.Name, req.Approved, req.Comments)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if s.streamHub != nil {
		s.streamHub.Broadcast("change_request_reviewed", updated)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(updated)
}

func (s *Server) handleApplyChangeRequest(w http.ResponseWriter, r *http.Request, id string) {
	user := UserFromContext(r.Context())
	actor := "system"
	if user != nil {
		actor = user.Email
	}

	flag, cr, audit, err := s.store.ApplyChangeRequest(r.Context(), id, actor)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if s.streamHub != nil {
		flags, _ := s.store.ListFlags(r.Context())
		s.streamHub.BroadcastFlags(flags)
		s.streamHub.Broadcast("change_request_applied", cr)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"flag":           flag,
		"change_request": cr,
		"audit":          audit,
	})
}
