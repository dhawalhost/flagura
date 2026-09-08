package api

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/dhawalhost/flagura/pkg/domain"
)

type CreateAPIKeyRequest struct {
	Name        string          `json:"name"`
	Role        domain.UserRole `json:"role"`
	Environment string          `json:"environment"`
}

func generateRawAPIKey() (string, string, string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", "", "", err
	}
	hexStr := hex.EncodeToString(b)
	rawKey := fmt.Sprintf("flg_live_%s", hexStr)

	h := sha256.Sum256([]byte(rawKey))
	keyHash := hex.EncodeToString(h[:])

	keyPrefix := fmt.Sprintf("flg_live_%s...****", hexStr[:8])

	return rawKey, keyPrefix, keyHash, nil
}

func (s *Server) handleListOrCreateAPIKeys(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		projectID, err := s.resolveAndAuthorizeProjectID(r)
		if err != nil {
			s.writeError(w, r, err)
			return
		}
		keys, err := s.store.ListAPIKeysByProject(r.Context(), projectID)
		if err != nil {
			s.writeError(w, r, domain.NewAppError(domain.ErrCodeDatabaseQuery, err.Error(), http.StatusInternalServerError, err))
			return
		}
		s.writeJSON(w, http.StatusOK, map[string]interface{}{
			"project_id": projectID,
			"api_keys":   keys,
		})

	case http.MethodPost:
		var req CreateAPIKeyRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			s.writeError(w, r, domain.NewAppError(domain.ErrCodeMalformedPayload, "Invalid request payload: "+err.Error(), http.StatusBadRequest, err))
			return
		}

		if strings.TrimSpace(req.Name) == "" {
			s.writeError(w, r, domain.NewAppError(domain.ErrCodeMalformedPayload, "Name is required for API Key", http.StatusBadRequest, domain.ErrInvalidInput))
			return
		}

		user, _ := s.getUserFromRequest(r)
		createdBy := "system"
		if user != nil && user.Email != "" {
			createdBy = user.Email
		}

		role := req.Role
		if role == "" {
			role = domain.RoleDeveloper
		}
		// If requesting admin role, ensure caller is admin
		if role == domain.RoleAdmin && (user == nil || user.Role != domain.RoleAdmin) {
			role = domain.RoleDeveloper
		}

		env := strings.ToLower(strings.TrimSpace(req.Environment))
		if env == "" {
			env = string(domain.EnvProduction)
		}
		// Only admins can create 'all' environment tokens
		if env == string(domain.EnvAll) || env == "*" {
			if user == nil || user.Role != domain.RoleAdmin {
				env = string(domain.EnvProduction)
			}
		} else if env != string(domain.EnvProduction) && env != string(domain.EnvStaging) && env != string(domain.EnvDevelopment) {
			s.writeError(w, r, domain.NewAppError(domain.ErrCodeInvalidEnvironment, "Invalid environment: must be 'production', 'staging', 'development', or 'all'", http.StatusBadRequest, domain.ErrInvalidEnvironment))
			return
		}

		rawKey, prefix, hash, err := generateRawAPIKey()
		if err != nil {
			s.writeError(w, r, domain.NewAppError(domain.ErrCodeInternal, "Failed to generate API Key: "+err.Error(), http.StatusInternalServerError, err))
			return
		}

		projectID, err := s.resolveAndAuthorizeProjectID(r)
		if err != nil {
			s.writeError(w, r, err)
			return
		}
		apiKey := domain.APIKey{
			ID:          domain.NewID("key"),
			ProjectID:   projectID,
			Environment: env,
			Key:         rawKey,
			KeyPrefix:   prefix,
			KeyHash:     hash,
			Name:        strings.TrimSpace(req.Name),
			Role:        role,
			CreatedBy:   createdBy,
			CreatedAt:   time.Now().UTC(),
			Revoked:     false,
		}

		created, err := s.store.CreateAPIKey(r.Context(), apiKey)
		if err != nil {
			s.writeError(w, r, domain.NewAppError(domain.ErrCodeDatabaseQuery, err.Error(), http.StatusInternalServerError, err))
			return
		}

		s.writeJSON(w, http.StatusCreated, map[string]interface{}{
			"api_key": created,
			"message": "API key generated successfully. Copy this secret key now; it will not be shown again.",
		})

	default:
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleRevokeAPIKey(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete && r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	id := strings.TrimPrefix(r.URL.Path, "/api/v1/api-keys/")
	id = strings.TrimSpace(strings.Split(id, "/")[0])
	if id == "" {
		s.writeError(w, r, domain.NewAppError(domain.ErrCodeMalformedPayload, "API Key ID required", http.StatusBadRequest, domain.ErrInvalidInput))
		return
	}

	// 1. Fetch the target key to identify its project
	key, err := s.store.GetAPIKeyByID(r.Context(), id)
	if err != nil || key == nil {
		s.writeError(w, r, domain.NewAppError(domain.ErrCodeAPIKeyNotFound, fmt.Sprintf("API key not found: %s", id), http.StatusNotFound, domain.ErrKeyNotFound))
		return
	}

	// 2. Security: Verify that caller is authorized to administer this project's keys
	if err := s.authorizeProjectAccess(r, key.ProjectID); err != nil {
		s.writeError(w, r, err)
		return
	}

	actor := s.getActorFromRequest(r, "admin@flagura.dev")

	// 3. Revoke with project scoping
	if err := s.store.RevokeAPIKeyByProject(r.Context(), key.ProjectID, id, actor); err != nil {
		s.writeError(w, r, domain.NewAppError(domain.ErrCodeAPIKeyNotFound, err.Error(), http.StatusNotFound, domain.ErrKeyNotFound))
		return
	}

	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"revoked": id,
	})
}
