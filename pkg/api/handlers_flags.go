package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/dhawalhost/flagura/pkg/domain"
)

func (s *Server) resolveProjectID(r *http.Request) string {
	if p := r.Header.Get(domain.HeaderProjectID); p != "" {
		return p
	}
	if p := r.URL.Query().Get("project_id"); p != "" {
		return p
	}
	if p := r.URL.Query().Get("projectId"); p != "" {
		return p
	}
	if apiKey := s.getAPIKeyFromRequest(r); apiKey != nil && apiKey.ProjectID != "" {
		return apiKey.ProjectID
	}
	if c, err := r.Cookie(domain.CookieProjectName); err == nil && c.Value != "" {
		return c.Value
	}
	if u := UserFromContext(r.Context()); u != nil {
		if orgs, err := s.store.ListUserOrganizations(r.Context(), u.ID); err == nil && len(orgs) > 0 {
			for _, org := range orgs {
				if projs, err := s.store.ListProjects(r.Context(), org.ID); err == nil && len(projs) > 0 {
					return projs[0].ID
				}
			}
		}
	}
	return ""
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
	projectID := s.resolveProjectID(r)
	if projectID == "" {
		s.writeError(w, r, domain.NewAppError(domain.ErrCodeProjectRequired, "project_id is required via X-Project-ID header, project_id query parameter, or active session", http.StatusBadRequest, domain.ErrInvalidInput))
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
	if flag.ProjectID == "" {
		flag.ProjectID = s.resolveProjectID(r)
	}
	if flag.ProjectID == "" {
		s.writeError(w, r, domain.NewAppError(domain.ErrCodeProjectRequired, "project_id is required to create a feature flag", http.StatusBadRequest, domain.ErrInvalidInput))
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
	if flag.ProjectID == "" {
		flag.ProjectID = s.resolveProjectID(r)
	}
	if flag.ProjectID == "" {
		s.writeError(w, r, domain.NewAppError(domain.ErrCodeProjectRequired, "project_id is required to update a feature flag", http.StatusBadRequest, domain.ErrInvalidInput))
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
	projectID := s.resolveProjectID(r)

	log, err := s.store.DeleteFlag(r.Context(), id, actor)
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

	flag, log, err := s.store.ToggleFlag(r.Context(), id, req.Environment, req.Enabled, actor)
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

	flag, log, err := s.store.UpdateRollout(r.Context(), id, req.Environment, req.Percentage, actor)
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

	projectID := s.resolveProjectID(r)
	flag, err := s.store.GetFlagByProject(r.Context(), projectID, key)
	if err != nil {
		flag, err = s.store.GetFlag(r.Context(), key)
		if err != nil {
			http.Error(w, "flag not found: "+err.Error(), http.StatusNotFound)
			return
		}
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
