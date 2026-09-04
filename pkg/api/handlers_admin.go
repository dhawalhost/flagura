package api

import (
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/dhawalhost/flagura/pkg/domain"
)

func (s *Server) handleGetAuditLogs(w http.ResponseWriter, r *http.Request) {
	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if val, err := strconv.Atoi(l); err == nil {
			limit = val
		}
	}

	projectID := s.resolveProjectID(r)
	logs, err := s.store.ListAuditLogsByProject(r.Context(), projectID, limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"project_id": projectID,
		"logs":       logs,
		"total":      len(logs),
	})
}

func (s *Server) handleReset(w http.ResponseWriter, r *http.Request) {
	projectID := s.resolveProjectID(r)
	if err := s.store.Reset(r.Context()); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.broadcastCurrentFlags(r.Context(), projectID, "")
	flags, _ := s.store.ListFlagsByProject(r.Context(), projectID)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"message":     "Database reset to default seed flags",
		"flags_count": len(flags),
	})
}

// handleWebhookKillSwitch engages the kill-switch for a flag triggered by automated alerts (e.g. Datadog/Sentry).
func (s *Server) handleWebhookKillSwitch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	// Security: Webhook requests must be authenticated.
	// Check Authorization header (Bearer token), X-Webhook-Secret header, or ?token= query param.
	webhookSecret := os.Getenv("FLAGURA_WEBHOOK_SECRET")
	providedToken := ""

	authHeader := r.Header.Get("Authorization")
	if strings.HasPrefix(authHeader, "Bearer ") {
		providedToken = strings.TrimPrefix(authHeader, "Bearer ")
	}
	if providedToken == "" {
		providedToken = r.Header.Get("X-Webhook-Secret")
	}
	if providedToken == "" {
		providedToken = r.Header.Get("X-API-Key")
	}
	if providedToken == "" {
		providedToken = r.URL.Query().Get("token")
	}

	authenticated := false

	// Check if matching webhook secret
	if webhookSecret != "" && providedToken != "" && subtle.ConstantTimeCompare([]byte(providedToken), []byte(webhookSecret)) == 1 {
		authenticated = true
	} else if providedToken != "" && webhookSecret == "" {
		// If no secret configured in env, verify against a valid user session or API token
		if sess, err := s.store.GetSession(r.Context(), providedToken); err == nil && sess != nil && !sess.IsExpired() {
			authenticated = true
		}
	}

	// Also check if caller has an active authenticated session cookie
	if !authenticated {
		if user, err := s.getUserFromRequest(r); err == nil && user != nil {
			authenticated = true
		}
	}

	if !authenticated {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"error":   "Unauthorized",
			"message": "Webhook authentication failed. Provide a valid Bearer token, X-Webhook-Secret header, or active session.",
		})
		return
	}

	key := strings.TrimPrefix(r.URL.Path, "/api/v1/webhooks/kill-switch/")
	key = strings.TrimSpace(strings.Split(key, "/")[0])
	if key == "" {
		http.Error(w, "flag key is required in url path", http.StatusBadRequest)
		return
	}

	envStr := r.URL.Query().Get("env")
	if envStr == "" {
		envStr = "production"
	}
	env := domain.Environment(envStr)

	disabled := false
	actor := "webhook-alert-automation"

	flag, log, err := s.store.ToggleFlag(r.Context(), key, env, &disabled, actor)
	if err != nil {
		http.Error(w, "Flag not found or failed to disable: "+err.Error(), http.StatusNotFound)
		return
	}

	s.broadcastCurrentFlags(r.Context(), flag.ProjectID, env)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"status":      "kill_switch_engaged",
		"flag_key":    flag.Key,
		"environment": env,
		"enabled":     false,
		"audit":       log,
	})
}

func (s *Server) handleInvitations(w http.ResponseWriter, r *http.Request) {
	user, err := s.getUserFromRequest(r)
	if err != nil {
		s.writeError(w, r, domain.NewAppError(domain.ErrCodeUnauthorized, "Unauthorized", http.StatusUnauthorized, err))
		return
	}

	switch r.Method {
	case http.MethodGet:
		orgID := r.URL.Query().Get("organization_id")
		if orgID == "" {
			orgs, _ := s.store.ListUserOrganizations(r.Context(), user.ID)
			if len(orgs) > 0 {
				orgID = orgs[0].ID
			}
		}
		invitations, err := s.store.ListOrgInvitations(r.Context(), orgID)
		if err != nil {
			s.writeError(w, r, domain.NewAppError(domain.ErrCodeInternal, err.Error(), http.StatusInternalServerError, err))
			return
		}
		s.writeJSON(w, http.StatusOK, map[string]interface{}{
			"invitations": invitations,
		})

	case http.MethodPost:
		var req struct {
			OrganizationID string `json:"organization_id"`
			Email          string `json:"email"`
			Role           string `json:"role"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			s.writeError(w, r, domain.NewAppError(domain.ErrCodeMalformedPayload, err.Error(), http.StatusBadRequest, err))
			return
		}

		if req.OrganizationID == "" {
			orgs, _ := s.store.ListUserOrganizations(r.Context(), user.ID)
			if len(orgs) > 0 {
				req.OrganizationID = orgs[0].ID
			}
		}
		if req.Role == "" {
			req.Role = "developer"
		}

		org, err := s.store.GetOrganization(r.Context(), req.OrganizationID)
		if err != nil {
			s.writeError(w, r, domain.NewAppError(domain.ErrCodeNotFound, "Organization not found", http.StatusNotFound, err))
			return
		}

		inv := domain.OrgInvitation{
			OrganizationID: org.ID,
			OrgName:        org.Name,
			Email:          strings.TrimSpace(strings.ToLower(req.Email)),
			Role:           req.Role,
			InvitedBy:      user.Email,
		}

		createdInv, err := s.store.CreateOrgInvitation(r.Context(), inv)
		if err != nil {
			s.writeError(w, r, domain.NewAppError(domain.ErrCodeInternal, err.Error(), http.StatusInternalServerError, err))
			return
		}

		s.writeJSON(w, http.StatusCreated, map[string]interface{}{
			"invitation": createdInv,
			"invite_url": fmt.Sprintf("/auth?invite=%s", createdInv.Token),
		})

	default:
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleGetInvitationByToken(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) < 4 {
		http.Error(w, "invalid invitation token path", http.StatusBadRequest)
		return
	}
	token := parts[len(parts)-1]
	inv, err := s.store.GetOrgInvitation(r.Context(), token)
	if err != nil {
		s.writeError(w, r, domain.NewAppError(domain.ErrCodeNotFound, err.Error(), http.StatusNotFound, err))
		return
	}

	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"invitation": inv,
	})
}

func (s *Server) handleAcceptInvitation(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	user, err := s.getUserFromRequest(r)
	if err != nil {
		s.writeError(w, r, domain.NewAppError(domain.ErrCodeUnauthorized, "Unauthorized", http.StatusUnauthorized, err))
		return
	}

	var req struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, r, domain.NewAppError(domain.ErrCodeMalformedPayload, err.Error(), http.StatusBadRequest, err))
		return
	}

	member, err := s.store.AcceptOrgInvitation(r.Context(), req.Token, user.ID)
	if err != nil {
		s.writeError(w, r, domain.NewAppError(domain.ErrCodeNotFound, err.Error(), http.StatusNotFound, err))
		return
	}

	// Switch active project to accepted org's project
	if projs, err := s.store.ListProjects(r.Context(), member.OrganizationID); err == nil && len(projs) > 0 {
		s.setProjectCookie(w, r, projs[0].ID, time.Now().Add(7*24*time.Hour))
	}

	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"member":  member,
		"message": "Invitation accepted successfully",
	})
}
