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
	Name string          `json:"name"`
	Role domain.UserRole `json:"role"`
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
		keys, err := s.store.ListAPIKeys(r.Context())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"api_keys": keys,
		})

	case http.MethodPost:
		var req CreateAPIKeyRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request payload: "+err.Error(), http.StatusBadRequest)
			return
		}

		if strings.TrimSpace(req.Name) == "" {
			http.Error(w, "Name is required for API Key", http.StatusBadRequest)
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

		rawKey, prefix, hash, err := generateRawAPIKey()
		if err != nil {
			http.Error(w, "Failed to generate API Key: "+err.Error(), http.StatusInternalServerError)
			return
		}

		apiKey := domain.APIKey{
			Key:        rawKey,
			KeyPrefix:  prefix,
			KeyHash:    hash,
			Name:       strings.TrimSpace(req.Name),
			Role:       role,
			CreatedBy:  createdBy,
			CreatedAt:  time.Now().UTC(),
			Revoked:    false,
		}

		created, err := s.store.CreateAPIKey(r.Context(), apiKey)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
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
		http.Error(w, "API Key ID required", http.StatusBadRequest)
		return
	}

	actor := s.getActorFromRequest(r, "admin@flagura.dev")

	if err := s.store.RevokeAPIKey(r.Context(), id, actor); err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"revoked": id,
	})
}
