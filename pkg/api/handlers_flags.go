package api

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/dhawalhost/flagura/pkg/domain"
)

// authorizeProjectAccess checks if the caller in r (User or API Key) is authorized to access projectID.
func (s *Server) authorizeProjectAccess(r *http.Request, projectID string) error {
	ctx := r.Context()

	// 1. API Key
	if apiKey := s.getAPIKeyFromRequest(r); apiKey != nil && apiKey.ProjectID != "" {
		if apiKey.ProjectID != projectID {
			slog.WarnContext(ctx, "security_event",
				slog.String("event_type", "project_access_denied"),
				slog.String("ip", GetClientIP(r)),
				slog.String("path", r.URL.Path),
				slog.String("api_key_id", apiKey.ID),
				slog.String("api_key_project", apiKey.ProjectID),
				slog.String("target_project", projectID),
				slog.String("reason", "api_key_cross_project_access"),
			)
			return domain.NewAppError(
				domain.ErrCodeProjectAccessDenied,
				fmt.Sprintf("API key scoped to project '%s' is not authorized to access project '%s'", apiKey.ProjectID, projectID),
				http.StatusForbidden,
				domain.ErrForbidden,
			)
		}
		return nil
	}

	// 2. User Session
	user := UserFromContext(ctx)
	if user == nil {
		user, _ = s.getUserFromRequest(r)
	}

	if user != nil {
		if user.Role == domain.RoleAdmin {
			if _, err := s.store.GetProject(ctx, projectID); err != nil {
				return domain.NewAppError(domain.ErrCodeProjectNotFound, "project not found: "+projectID, http.StatusNotFound, domain.ErrProjectNotFound)
			}
			return nil
		}

		userOrgs, err := s.store.ListUserOrganizations(ctx, user.ID)
		if err != nil {
			return domain.NewAppError(domain.ErrCodeDatabaseQuery, err.Error(), http.StatusInternalServerError, err)
		}
		userOrgMap := make(map[string]bool)
		for _, o := range userOrgs {
			userOrgMap[o.ID] = true
		}

		proj, err := s.store.GetProject(ctx, projectID)
		if err != nil {
			return domain.NewAppError(domain.ErrCodeProjectNotFound, "project not found: "+projectID, http.StatusNotFound, domain.ErrProjectNotFound)
		}
		if !userOrgMap[proj.OrganizationID] {
			slog.WarnContext(ctx, "security_event",
				slog.String("event_type", "project_access_denied"),
				slog.String("ip", GetClientIP(r)),
				slog.String("path", r.URL.Path),
				slog.String("user_id", user.ID),
				slog.String("user_email", user.Email),
				slog.String("target_project", projectID),
				slog.String("project_org", proj.OrganizationID),
				slog.String("reason", "user_not_org_member"),
			)
			return domain.NewAppError(
				domain.ErrCodeProjectAccessDenied,
				fmt.Sprintf("user '%s' is not authorized to access project '%s' (not a member of organization '%s')", user.Email, projectID, proj.OrganizationID),
				http.StatusForbidden,
				domain.ErrForbidden,
			)
		}
		return nil
	}

	// Fail-closed defensive default: If neither an authorized API key nor a valid user session
	// is present on the request, deny access explicitly.
	slog.WarnContext(ctx, "security_event",
		slog.String("event_type", "project_access_denied"),
		slog.String("ip", GetClientIP(r)),
		slog.String("path", r.URL.Path),
		slog.String("target_project", projectID),
		slog.String("reason", "anonymous_project_access_denied"),
	)
	return domain.NewAppError(
		domain.ErrCodeUnauthorized,
		"authentication required to access project",
		http.StatusUnauthorized,
		domain.ErrUnauthorized,
	)
}

// resolveAndAuthorizeProjectID resolves the project ID for the request and validates
// that the authenticated caller (API key or User session) is strictly authorized to access it.
func (s *Server) resolveAndAuthorizeProjectID(r *http.Request) (string, error) {
	ctx := r.Context()

	// Candidate project from request
	targetID := ""
	if p := r.Header.Get(domain.HeaderProjectID); p != "" {
		targetID = p
	} else if p := r.URL.Query().Get("project_id"); p != "" {
		targetID = p
	} else if p := r.URL.Query().Get("projectId"); p != "" {
		targetID = p
	} else if c, err := r.Cookie(domain.CookieProjectName); err == nil && c.Value != "" {
		targetID = c.Value
	}

	// 1. If request is authenticated via API Key
	if apiKey := s.getAPIKeyFromRequest(r); apiKey != nil && apiKey.ProjectID != "" {
		if targetID != "" && targetID != apiKey.ProjectID {
			return "", domain.NewAppError(
				domain.ErrCodeProjectAccessDenied,
				fmt.Sprintf("API key scoped to project '%s' is not authorized to access project '%s'", apiKey.ProjectID, targetID),
				http.StatusForbidden,
				domain.ErrForbidden,
			)
		}
		return apiKey.ProjectID, nil
	}

	// 2. If request is authenticated via User Session
	user := UserFromContext(ctx)
	if user == nil {
		user, _ = s.getUserFromRequest(r)
	}

	if user != nil {
		if targetID != "" {
			if err := s.authorizeProjectAccess(r, targetID); err != nil {
				return "", err
			}
			return targetID, nil
		}

		// Platform Admin has access to all projects
		if user.Role == domain.RoleAdmin {
			// Admin without targetID: first project from user's orgs, or DefaultProjectID
			if orgs, err := s.store.ListUserOrganizations(ctx, user.ID); err == nil && len(orgs) > 0 {
				for _, org := range orgs {
					if projs, err := s.store.ListProjects(ctx, org.ID); err == nil && len(projs) > 0 {
						return projs[0].ID, nil
					}
				}
			}
			return domain.DefaultProjectID, nil
		}

		// Non-admin user: Fetch user's organizations
		userOrgs, err := s.store.ListUserOrganizations(ctx, user.ID)
		if err != nil {
			return "", domain.NewAppError(domain.ErrCodeDatabaseQuery, err.Error(), http.StatusInternalServerError, err)
		}

		// If no candidate project specified, fallback to user's first project in their organizations
		if len(userOrgs) == 0 {
			slog.WarnContext(ctx, "security_event",
				slog.String("event_type", "project_access_denied"),
				slog.String("ip", GetClientIP(r)),
				slog.String("path", r.URL.Path),
				slog.String("user_id", user.ID),
				slog.String("reason", "user_has_no_organizations"),
			)
			return "", domain.NewAppError(domain.ErrCodeProjectAccessDenied, "user has no assigned organizations", http.StatusForbidden, domain.ErrForbidden)
		}
		for _, org := range userOrgs {
			if projs, err := s.store.ListProjects(ctx, org.ID); err == nil && len(projs) > 0 {
				return projs[0].ID, nil
			}
		}

		return "", domain.NewAppError(domain.ErrCodeProjectRequired, "no active project found for user", http.StatusBadRequest, domain.ErrInvalidInput)
	}

	// 3. Unauthenticated requests
	if targetID != "" {
		if _, err := s.store.GetProject(ctx, targetID); err == nil {
			return targetID, nil
		}
		return "", domain.NewAppError(domain.ErrCodeProjectNotFound, "project not found: "+targetID, http.StatusNotFound, domain.ErrProjectNotFound)
	}
	// In strict multi-tenancy, project_id is required; no silent fallback to default tenant
	return "", domain.NewAppError(domain.ErrCodeProjectRequired, "project_id is required via X-Project-ID header or query parameter", http.StatusBadRequest, domain.ErrInvalidInput)
}

func (s *Server) resolveProjectID(r *http.Request) string {
	pid, _ := s.resolveAndAuthorizeProjectID(r)
	return pid
}

func (s *Server) getActorFromRequest(r *http.Request, fallback string) string {
	if u := UserFromContext(r.Context()); u != nil && u.Email != "" {
		return u.Email
	}
	if a := r.Header.Get(domain.HeaderActor); a != "" {
		return a
	}
	if a := r.URL.Query().Get("actor"); a != "" {
		return a
	}
	if fallback != "" {
		return fallback
	}
	return "developer@flagura.dev"
}

func (s *Server) handleGetFlags(w http.ResponseWriter, r *http.Request) {
	projectID, err := s.resolveAndAuthorizeProjectID(r)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	flags, err := s.store.ListFlagsByProject(r.Context(), projectID)
	if err != nil {
		s.writeError(w, r, domain.NewAppError(domain.ErrCodeDatabaseQuery, err.Error(), http.StatusInternalServerError, err))
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"project_id": projectID,
		"flags":      flags,
		"count":      len(flags),
		"timestamp":  time.Now().UTC(),
	})
}

func (s *Server) handleCreateFlag(w http.ResponseWriter, r *http.Request) {
	var flag domain.FeatureFlag
	if err := json.NewDecoder(r.Body).Decode(&flag); err != nil {
		s.writeError(w, r, domain.NewAppError(domain.ErrCodeMalformedPayload, err.Error(), http.StatusBadRequest, err))
		return
	}
	if flag.Key == "" {
		s.writeError(w, r, domain.NewAppError(domain.ErrCodeMalformedPayload, "flag key is required", http.StatusBadRequest, domain.ErrInvalidInput))
		return
	}
	if !domain.IsValidFlagKey(flag.Key) {
		s.writeError(w, r, domain.NewAppError(domain.ErrCodeMalformedPayload, "flag key must contain only alphanumeric characters, underscores, and hyphens (1-64 chars)", http.StatusBadRequest, domain.ErrInvalidInput))
		return
	}
	projectID, err := s.resolveAndAuthorizeProjectID(r)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	if flag.ProjectID != "" && flag.ProjectID != projectID {
		s.writeError(w, r, domain.NewAppError(domain.ErrCodeProjectAccessDenied, fmt.Sprintf("cannot create flag in unauthorized project '%s'", flag.ProjectID), http.StatusForbidden, domain.ErrForbidden))
		return
	}
	flag.ProjectID = projectID

	if err := domain.ValidateFeatureFlag(flag); err != nil {
		s.writeError(w, r, domain.NewAppError(domain.ErrCodeMalformedPayload, err.Error(), http.StatusBadRequest, domain.ErrInvalidInput))
		return
	}

	actor := s.getActorFromRequest(r, "developer@flagura.dev")

	log, err := s.store.SaveFlag(r.Context(), flag, actor)
	if err != nil {
		s.writeError(w, r, domain.NewAppError(domain.ErrCodeDatabaseQuery, err.Error(), http.StatusInternalServerError, err))
		return
	}

	s.broadcastCurrentFlags(r.Context(), flag.ProjectID, "")

	s.writeJSON(w, http.StatusCreated, map[string]interface{}{
		"flag":  flag,
		"audit": log,
	})
}

func (s *Server) handleUpdateFlag(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/v1/flags/")
	id = strings.Split(id, "/")[0]

	var flag domain.FeatureFlag
	if err := json.NewDecoder(r.Body).Decode(&flag); err != nil {
		s.writeError(w, r, domain.NewAppError(domain.ErrCodeMalformedPayload, err.Error(), http.StatusBadRequest, err))
		return
	}

	if flag.Key == "" {
		flag.Key = id
	}
	if !domain.IsValidFlagKey(flag.Key) {
		s.writeError(w, r, domain.NewAppError(domain.ErrCodeMalformedPayload, "flag key must contain only alphanumeric characters, underscores, and hyphens (1-64 chars)", http.StatusBadRequest, domain.ErrInvalidInput))
		return
	}
	projectID, err := s.resolveAndAuthorizeProjectID(r)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	if flag.ProjectID != "" && flag.ProjectID != projectID {
		s.writeError(w, r, domain.NewAppError(domain.ErrCodeProjectAccessDenied, fmt.Sprintf("cannot update flag in unauthorized project '%s'", flag.ProjectID), http.StatusForbidden, domain.ErrForbidden))
		return
	}
	flag.ProjectID = projectID

	existing, err := s.store.GetFlagByProject(r.Context(), projectID, flag.Key)
	if err != nil {
		s.writeError(w, r, domain.NewAppError(domain.ErrCodeFlagNotFound, err.Error(), http.StatusNotFound, domain.ErrFlagNotFound))
		return
	}
	if existing != nil && flag.ID == "" {
		flag.ID = existing.ID
	}

	if err := domain.ValidateFeatureFlag(flag); err != nil {
		s.writeError(w, r, domain.NewAppError(domain.ErrCodeMalformedPayload, err.Error(), http.StatusBadRequest, domain.ErrInvalidInput))
		return
	}

	actor := s.getActorFromRequest(r, "developer@flagura.dev")

	log, err := s.store.SaveFlag(r.Context(), flag, actor)
	if err != nil {
		s.writeError(w, r, domain.NewAppError(domain.ErrCodeDatabaseQuery, err.Error(), http.StatusInternalServerError, err))
		return
	}

	s.broadcastCurrentFlags(r.Context(), flag.ProjectID, "")

	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"flag":  flag,
		"audit": log,
	})
}

func (s *Server) handleDeleteFlag(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/v1/flags/")
	id = strings.Split(id, "/")[0]

	actor := s.getActorFromRequest(r, "admin@flagura.dev")
	projectID, err := s.resolveAndAuthorizeProjectID(r)
	if err != nil {
		s.writeError(w, r, err)
		return
	}

	log, err := s.store.DeleteFlagByProject(r.Context(), projectID, id, actor)
	if err != nil {
		s.writeError(w, r, domain.NewAppError(domain.ErrCodeFlagNotFound, err.Error(), http.StatusNotFound, domain.ErrFlagNotFound))
		return
	}

	s.broadcastCurrentFlags(r.Context(), projectID, "")

	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"deleted": id,
		"audit":   log,
	})
}

func (s *Server) handleToggleFlag(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/v1/flags/")
	id = strings.TrimSuffix(id, "/toggle")

	var req struct {
		Environment domain.Environment `json:"environment"`
		Enabled     *bool              `json:"enabled"`
		Actor       string             `json:"actor"`
	}
	if r.Body != nil && r.ContentLength > 0 {
		_ = json.NewDecoder(r.Body).Decode(&req)
	}

	if req.Environment == "" {
		req.Environment = domain.EnvProduction
	}

	projectID, err := s.resolveAndAuthorizeProjectID(r)
	if err != nil {
		s.writeError(w, r, err)
		return
	}

	if apiKey := s.getAPIKeyFromRequest(r); apiKey != nil {
		if !apiKey.AllowsEnvironment(req.Environment) {
			s.writeError(w, r, domain.NewAppError(
				domain.ErrCodeEnvironmentRestricted,
				fmt.Sprintf("API key is scoped to environment '%s' and cannot modify '%s'", apiKey.Environment, req.Environment),
				http.StatusForbidden,
				domain.ErrEnvironmentRestricted,
			))
			return
		}
	}

	actor := s.getActorFromRequest(r, req.Actor)

	flag, log, err := s.store.ToggleFlagByProject(r.Context(), projectID, id, req.Environment, req.Enabled, actor)
	if err != nil {
		s.writeError(w, r, domain.NewAppError(domain.ErrCodeFlagNotFound, err.Error(), http.StatusNotFound, domain.ErrFlagNotFound))
		return
	}

	s.broadcastCurrentFlags(r.Context(), flag.ProjectID, req.Environment)

	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"flag_key":    flag.Key,
		"environment": req.Environment,
		"enabled":     flag.Environments[req.Environment].Enabled,
		"flag":        flag,
		"audit":       log,
	})
}

func (s *Server) handleUpdateRollout(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/v1/flags/")
	id = strings.TrimSuffix(id, "/rollout")

	var req struct {
		Environment domain.Environment `json:"environment"`
		Percentage  float64            `json:"percentage"`
		Actor       string             `json:"actor"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, r, domain.NewAppError(domain.ErrCodeMalformedPayload, err.Error(), http.StatusBadRequest, err))
		return
	}

	if req.Environment == "" {
		req.Environment = domain.EnvProduction
	}

	projectID, err := s.resolveAndAuthorizeProjectID(r)
	if err != nil {
		s.writeError(w, r, err)
		return
	}

	if apiKey := s.getAPIKeyFromRequest(r); apiKey != nil {
		if !apiKey.AllowsEnvironment(req.Environment) {
			s.writeError(w, r, domain.NewAppError(
				domain.ErrCodeEnvironmentRestricted,
				fmt.Sprintf("API key is scoped to environment '%s' and cannot modify '%s'", apiKey.Environment, req.Environment),
				http.StatusForbidden,
				domain.ErrEnvironmentRestricted,
			))
			return
		}
	}

	actor := s.getActorFromRequest(r, req.Actor)

	flag, log, err := s.store.UpdateRolloutByProject(r.Context(), projectID, id, req.Environment, req.Percentage, actor)
	if err != nil {
		s.writeError(w, r, domain.NewAppError(domain.ErrCodeFlagNotFound, err.Error(), http.StatusNotFound, domain.ErrFlagNotFound))
		return
	}

	s.broadcastCurrentFlags(r.Context(), flag.ProjectID, req.Environment)

	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"flag_key":    flag.Key,
		"environment": req.Environment,
		"percentage":  req.Percentage,
		"flag":        flag,
		"audit":       log,
	})
}

// handlePromoteEnvironment copies flag rules and configuration from one environment to another.
func (s *Server) handlePromoteEnvironment(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	key := strings.TrimPrefix(r.URL.Path, "/api/v1/flags/")
	key = strings.TrimSuffix(key, "/promote")
	key = strings.TrimSpace(key)
	if key == "" {
		http.Error(w, "flag key is required", http.StatusBadRequest)
		return
	}

	fromEnv := domain.Environment(r.URL.Query().Get("from"))
	toEnv := domain.Environment(r.URL.Query().Get("to"))
	if fromEnv == "" || toEnv == "" {
		http.Error(w, "query parameters 'from' and 'to' are required", http.StatusBadRequest)
		return
	}

	projectID, err := s.resolveAndAuthorizeProjectID(r)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	flag, err := s.store.GetFlagByProject(r.Context(), projectID, key)
	if err != nil {
		s.writeError(w, r, domain.NewAppError(domain.ErrCodeFlagNotFound, "flag not found in project: "+err.Error(), http.StatusNotFound, domain.ErrFlagNotFound))
		return
	}

	srcConfig, ok := flag.Environments[fromEnv]
	if !ok {
		http.Error(w, fmt.Sprintf("source environment %q does not exist on flag", fromEnv), http.StatusBadRequest)
		return
	}

	// Clone source environment configuration
	rulesCopy := make([]domain.TargetingRule, len(srcConfig.Rules))
	for i, r := range srcConfig.Rules {
		vals := make([]string, len(r.Values))
		copy(vals, r.Values)
		rulesCopy[i] = domain.TargetingRule{
			ID:           r.ID,
			Name:         r.Name,
			Attribute:    r.Attribute,
			CustomKey:    r.CustomKey,
			Operator:     r.Operator,
			Values:       vals,
			Action:       r.Action,
			ServeVariant: r.ServeVariant,
		}
	}

	variantsCopy := make([]domain.FlagVariant, len(srcConfig.Variants))
	for i, v := range srcConfig.Variants {
		variantsCopy[i] = domain.FlagVariant{
			Key:         v.Key,
			Name:        v.Name,
			Value:       v.Value,
			Weight:      v.Weight,
			Description: v.Description,
		}
	}

	flag.Environments[toEnv] = domain.EnvironmentConfig{
		Enabled:        srcConfig.Enabled,
		Strategy:       srcConfig.Strategy,
		Percentage:     srcConfig.Percentage,
		Rules:          rulesCopy,
		Variants:       variantsCopy,
		DefaultVariant: srcConfig.DefaultVariant,
		OffVariant:     srcConfig.OffVariant,
	}
	flag.UpdatedAt = time.Now().UTC()

	actor := s.getActorFromRequest(r, "system")

	auditLog, err := s.store.SaveFlag(r.Context(), *flag, actor)
	if err != nil {
		http.Error(w, "failed to promote environment: "+err.Error(), http.StatusInternalServerError)
		return
	}

	s.broadcastCurrentFlags(r.Context(), flag.ProjectID, toEnv)

	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":   "promoted",
		"flag_key": flag.Key,
		"from":     fromEnv,
		"to":       toEnv,
		"audit":    auditLog,
	})
}
