package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/dhawalhost/flagura/pkg/domain"
)

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPatch || r.Method == http.MethodPut {
		s.handleUpdateProfile(w, r)
		return
	}
	user, err := s.getUserFromRequest(r)
	if err != nil {
		http.Error(w, "Unauthorized: "+err.Error(), http.StatusUnauthorized)
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(user)
}

func (s *Server) handleUpdateProfile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPatch && r.Method != http.MethodPut && r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	user, err := s.getUserFromRequest(r)
	if err != nil {
		http.Error(w, "Unauthorized: "+err.Error(), http.StatusUnauthorized)
		return
	}

	var req domain.UpdateProfileRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request payload: "+err.Error(), http.StatusBadRequest)
		return
	}

	trimmedName := strings.TrimSpace(req.Name)
	if trimmedName == "" {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"error":   "Validation Error",
			"message": "Full Name cannot be empty.",
		})
		return
	}

	updatedUser, err := s.store.UpdateUser(r.Context(), domain.User{
		ID:        user.ID,
		Name:      trimmedName,
		AvatarURL: strings.TrimSpace(req.AvatarURL),
	})
	if err != nil {
		http.Error(w, "Failed to update profile: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "Profile updated successfully",
		"user":    updatedUser,
	})
}
