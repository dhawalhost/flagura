package api

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/dhawalhost/flagura/pkg/domain"
)

// sanitizeCSVCell sanitizes a string cell to prevent CSV / Excel formula injection (CSV Injection).
// Prepends a single quote if the first non-whitespace character is =, +, -, or @.
func sanitizeCSVCell(val string) string {
	trimmed := strings.TrimSpace(val)
	if len(trimmed) > 0 {
		switch trimmed[0] {
		case '=', '+', '-', '@':
			return "'" + val
		}
	}
	return val
}

// handleVerifyAuditChain verifies the SHA-256 cryptographic hash chain integrity for the project's audit logs.
func (s *Server) handleVerifyAuditChain(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	projectID, err := s.resolveAndAuthorizeProjectID(r)
	if err != nil {
		s.writeError(w, r, err)
		return
	}

	result, err := s.store.VerifyAuditLogIntegrity(r.Context(), projectID)
	if err != nil {
		s.writeError(w, r, domain.NewAppError(domain.ErrCodeDatabaseQuery, err.Error(), http.StatusInternalServerError, err))
		return
	}

	s.writeJSON(w, http.StatusOK, result)
}

// handleExportAuditLogs exports audit logs in CSV, JSON, or NDJSON format with formula injection defenses.
func (s *Server) handleExportAuditLogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	projectID, err := s.resolveAndAuthorizeProjectID(r)
	if err != nil {
		s.writeError(w, r, err)
		return
	}

	q := r.URL.Query()
	format := strings.ToLower(strings.TrimSpace(q.Get("format")))
	if format == "" {
		format = "csv"
	}
	if format != "csv" && format != "json" && format != "ndjson" {
		s.writeError(w, r, domain.NewAppError(domain.ErrCodeBadRequest, "unsupported export format: must be 'csv', 'json', or 'ndjson'", http.StatusBadRequest, nil))
		return
	}

	var from, to *time.Time
	if fromStr := q.Get("from"); fromStr != "" {
		t, err := time.Parse(time.RFC3339, fromStr)
		if err != nil {
			t, err = time.Parse(time.RFC3339Nano, fromStr)
		}
		if err != nil {
			s.writeError(w, r, domain.NewAppError(domain.ErrCodeBadRequest, "invalid 'from' timestamp: must be RFC3339", http.StatusBadRequest, err))
			return
		}
		from = &t
	}
	if toStr := q.Get("to"); toStr != "" {
		t, err := time.Parse(time.RFC3339, toStr)
		if err != nil {
			t, err = time.Parse(time.RFC3339Nano, toStr)
		}
		if err != nil {
			s.writeError(w, r, domain.NewAppError(domain.ErrCodeBadRequest, "invalid 'to' timestamp: must be RFC3339", http.StatusBadRequest, err))
			return
		}
		to = &t
	}

	limit := 1000
	if lStr := q.Get("limit"); lStr != "" {
		if val, err := strconv.Atoi(lStr); err == nil && val > 0 {
			limit = val
		}
	}
	if limit > 10000 {
		limit = 10000
	}

	logs, err := s.store.ExportAuditLogs(r.Context(), projectID, from, to, limit)
	if err != nil {
		s.writeError(w, r, domain.NewAppError(domain.ErrCodeDatabaseQuery, err.Error(), http.StatusInternalServerError, err))
		return
	}

	ts := time.Now().Unix()
	w.Header().Set("Cache-Control", "no-store")

	switch format {
	case "csv":
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="audit_logs_%s_%d.csv"`, projectID, ts))

		writer := csv.NewWriter(w)
		header := []string{
			"id", "project_id", "timestamp", "actor", "action", "flag_key", "environment", "details", "prev_hash", "entry_hash",
		}
		if err := writer.Write(header); err != nil {
			return
		}

		for _, l := range logs {
			row := []string{
				sanitizeCSVCell(l.ID),
				sanitizeCSVCell(l.ProjectID),
				l.Timestamp.Format(time.RFC3339Nano),
				sanitizeCSVCell(l.Actor),
				sanitizeCSVCell(l.Action),
				sanitizeCSVCell(l.FlagKey),
				sanitizeCSVCell(string(l.Environment)),
				sanitizeCSVCell(l.Details),
				sanitizeCSVCell(l.PrevHash),
				sanitizeCSVCell(l.EntryHash),
			}
			if err := writer.Write(row); err != nil {
				return
			}
		}
		writer.Flush()

	case "json":
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="audit_logs_%s_%d.json"`, projectID, ts))
		if logs == nil {
			logs = []domain.AuditLogEntry{}
		}
		_ = json.NewEncoder(w).Encode(logs)

	case "ndjson":
		w.Header().Set("Content-Type", "application/x-ndjson; charset=utf-8")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="audit_logs_%s_%d.ndjson"`, projectID, ts))
		for _, l := range logs {
			line, err := json.Marshal(l)
			if err != nil {
				continue
			}
			_, _ = w.Write(line)
			_, _ = w.Write([]byte("\n"))
		}
	}
}

// handlePurgeAuditLogs removes audit logs prior to a specified timestamp or retention cutoff,
// strictly verifying Admin or Owner permissions and writing an audit trail entry for the purge action.
func (s *Server) handlePurgeAuditLogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	projectID, err := s.resolveAndAuthorizeProjectID(r)
	if err != nil {
		s.writeError(w, r, err)
		return
	}

	// Strict RBAC: Caller must have management privileges (Admin or Owner)
	u := UserFromContext(r.Context())
	if u != nil && !u.IsPrivileged() {
		s.writeError(w, r, domain.NewAppError(domain.ErrCodeForbidden, "Admin or Owner role required to purge audit logs", http.StatusForbidden, nil))
		return
	}

	actor := s.getActorFromRequest(r, "admin@flagura.dev")

	var req struct {
		Before        *time.Time `json:"before"`
		RetentionDays int        `json:"retention_days"`
	}
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&req)
	}

	// Support query parameter overrides
	if req.RetentionDays <= 0 {
		if retStr := r.URL.Query().Get("retention_days"); retStr != "" {
			if val, err := strconv.Atoi(retStr); err == nil && val > 0 {
				req.RetentionDays = val
			}
		}
	}
	if req.Before == nil {
		if beforeStr := r.URL.Query().Get("before"); beforeStr != "" {
			if t, err := time.Parse(time.RFC3339, beforeStr); err == nil {
				req.Before = &t
			}
		}
	}

	var before time.Time
	if req.RetentionDays > 0 {
		before = time.Now().UTC().AddDate(0, 0, -req.RetentionDays)
	} else if req.Before != nil {
		before = req.Before.UTC()
	} else if envVal := os.Getenv("FLAGURA_AUDIT_RETENTION_DAYS"); envVal != "" {
		if days, err := strconv.Atoi(envVal); err == nil && days > 0 {
			before = time.Now().UTC().AddDate(0, 0, -days)
		}
	}

	if before.IsZero() {
		s.writeError(w, r, domain.NewAppError(domain.ErrCodeBadRequest, "'retention_days' or 'before' timestamp is required", http.StatusBadRequest, nil))
		return
	}

	if before.After(time.Now().UTC()) {
		s.writeError(w, r, domain.NewAppError(domain.ErrCodeBadRequest, "cannot purge audit logs with a future timestamp", http.StatusBadRequest, nil))
		return
	}

	purgedCount, err := s.store.PurgeAuditLogs(r.Context(), projectID, before)
	if err != nil {
		s.writeError(w, r, domain.NewAppError(domain.ErrCodeDatabaseQuery, err.Error(), http.StatusInternalServerError, err))
		return
	}

	// Record audit entry for the purge action itself
	purgeAudit := domain.AuditLogEntry{
		ProjectID:   projectID,
		FlagKey:     "audit-logs",
		Action:      "AUDIT_LOGS_PURGED",
		Environment: "all",
		Actor:       actor,
		Timestamp:   time.Now().UTC(),
		Details:     fmt.Sprintf("Purged %d audit logs before %s", purgedCount, before.Format(time.RFC3339)),
	}
	_, _ = s.store.AppendAuditLog(r.Context(), purgeAudit)

	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"purged_count": purgedCount,
		"project_id":   projectID,
		"before":       before.Format(time.RFC3339),
		"purged_at":    time.Now().UTC().Format(time.RFC3339),
	})
}
