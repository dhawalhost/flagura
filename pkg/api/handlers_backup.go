package api

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/dhawalhost/flagura/pkg/domain"
	"github.com/dhawalhost/flagura/pkg/store"
)

func (s *Server) isBackupAuthorized(r *http.Request) bool {
	if user := UserFromContext(r.Context()); user != nil && user.IsPrivileged() {
		return true
	}
	if apiKey := APIKeyFromContext(r.Context()); apiKey != nil && apiKey.Role == domain.RoleAdmin {
		return true
	}
	return false
}

// handleExportBackup streams an atomic relational snapshot of the database.
func (s *Server) handleExportBackup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	if !s.isBackupAuthorized(r) {
		s.writeError(w, r, domain.NewAppError(domain.ErrCodeForbidden, "Admin or Organization Owner privileges required for backup operations", http.StatusForbidden, nil))
		return
	}

	compress := strings.EqualFold(r.URL.Query().Get("compress"), "true") || strings.EqualFold(r.URL.Query().Get("gzip"), "true")

	timestamp := time.Now().UTC().Format("20060102-150405")
	var filename string
	if compress {
		w.Header().Set("Content-Type", "application/gzip")
		filename = fmt.Sprintf("flagura-backup-%s.json.gz", timestamp)
	} else {
		w.Header().Set("Content-Type", "application/json")
		filename = fmt.Sprintf("flagura-backup-%s.json", timestamp)
	}

	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	w.WriteHeader(http.StatusOK)

	_ = store.CreateSnapshot(r.Context(), s.store, w, compress)
}

// handleImportBackup restores entities from an uploaded snapshot into the active store.
func (s *Server) handleImportBackup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	if !s.isBackupAuthorized(r) {
		s.writeError(w, r, domain.NewAppError(domain.ErrCodeForbidden, "Admin or Organization Owner privileges required for restore operations", http.StatusForbidden, nil))
		return
	}

	snap, err := store.RestoreSnapshot(r.Context(), s.store, r.Body)
	if err != nil {
		s.writeError(w, r, domain.NewAppError(domain.ErrCodeMalformedPayload, fmt.Sprintf("failed to restore snapshot: %v", err), http.StatusBadRequest, err))
		return
	}

	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":      "success",
		"message":     "Snapshot restored successfully",
		"version":     snap.Version,
		"exported_at": snap.ExportedAt,
		"counts": map[string]int{
			"organizations": len(snap.Organizations),
			"projects":      len(snap.Projects),
			"flags":         len(snap.Flags),
			"users":         len(snap.Users),
			"api_keys":      len(snap.APIKeys),
			"audit_logs":    len(snap.AuditLogs),
		},
	})
}
