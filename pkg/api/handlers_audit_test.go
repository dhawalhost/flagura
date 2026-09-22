package api

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/dhawalhost/flagura/pkg/domain"
	"github.com/dhawalhost/flagura/pkg/store"
)

func getViewerAuthCookie(t *testing.T, memStore store.Store) *http.Cookie {
	ctx := context.Background()
	user := domain.NewUser("viewer.suite@flagura.dev", "Viewer Tester", "hashed_pwd", domain.RoleViewer)
	createdUser, err := memStore.CreateUser(ctx, user)
	if err != nil {
		createdUser, _ = memStore.GetUserByEmail(ctx, "viewer.suite@flagura.dev")
	}

	token, _ := generateSessionToken()
	session := domain.Session{
		Token:     token,
		UserID:    createdUser.ID,
		ExpiresAt: time.Now().Add(24 * time.Hour),
		CreatedAt: time.Now(),
	}
	if err := memStore.CreateSession(ctx, session); err != nil {
		t.Fatalf("failed to create viewer session: %v", err)
	}

	return &http.Cookie{
		Name:     "flagura_session",
		Value:    token,
		Path:     "/",
		HttpOnly: true,
	}
}

func TestAuditHandlers_VerifyIntegrity(t *testing.T) {
	memStore := store.NewMemoryStore()
	srv, err := NewServer(memStore)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	authCookie := getAdminAuthCookie(t, memStore)
	ctx := context.Background()

	// 1. Clean verification with seeded flags
	req := httptest.NewRequest(http.MethodGet, "/api/v1/audit/verify", nil)
	req.AddCookie(authCookie)
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", w.Code, w.Body.String())
	}

	var res domain.AuditIntegrityResult
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	if !res.Valid {
		t.Fatalf("expected chain to be valid, got: %+v", res)
	}

	// 2. Perform a flag mutation to create an audit entry
	flag := domain.FeatureFlag{
		ID:        "flg_audit_api_1",
		ProjectID: domain.DefaultProjectID,
		Key:       "test-audit-verify-flag",
		Name:      "Test Audit Verify Flag",
		Type:      "boolean",
		Environments: map[domain.Environment]domain.EnvironmentConfig{
			domain.EnvProduction: {Enabled: false},
		},
	}
	log, err := memStore.SaveFlag(ctx, flag, "admin.suite@flagura.dev")
	if err != nil {
		t.Fatalf("SaveFlag failed: %v", err)
	}
	if log.EntryHash == "" {
		t.Fatal("expected non-empty EntryHash")
	}

	// Re-verify after mutation
	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/audit/verify", nil)
	req2.AddCookie(authCookie)
	w2 := httptest.NewRecorder()
	srv.mux.ServeHTTP(w2, req2)

	if w2.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", w2.Code, w2.Body.String())
	}
	var res2 domain.AuditIntegrityResult
	if err := json.Unmarshal(w2.Body.Bytes(), &res2); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if !res2.Valid || res2.TotalVerified == 0 {
		t.Fatalf("expected valid verification with >0 entries, got %+v", res2)
	}
}

func TestAuditHandlers_ExportFormatsAndFormulaInjection(t *testing.T) {
	memStore := store.NewMemoryStore()
	srv, err := NewServer(memStore)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	authCookie := getAdminAuthCookie(t, memStore)
	ctx := context.Background()

	// Insert an audit log with potentially malicious formula injection patterns in details
	maliciousEntry := domain.AuditLogEntry{
		ID:          "=cmd|' /C calc'!A0",
		ProjectID:   domain.DefaultProjectID,
		FlagKey:     "+dangerous_flag",
		Action:      "-FLAG_EXPLOIT",
		Environment: domain.EnvProduction,
		Actor:       "@admin",
		Details:     "=1+1; EXEC xp_cmdshell('dir');",
		Timestamp:   time.Now().UTC(),
	}
	_, err = memStore.AppendAuditLog(ctx, maliciousEntry)
	if err != nil {
		t.Fatalf("AppendAuditLog failed: %v", err)
	}

	// 1. Export as CSV (Default)
	reqCSV := httptest.NewRequest(http.MethodGet, "/api/v1/audit/export?format=csv", nil)
	reqCSV.AddCookie(authCookie)
	wCSV := httptest.NewRecorder()
	srv.mux.ServeHTTP(wCSV, reqCSV)

	if wCSV.Code != http.StatusOK {
		t.Fatalf("CSV export expected 200 OK, got %d: %s", wCSV.Code, wCSV.Body.String())
	}
	if !strings.Contains(wCSV.Header().Get("Content-Type"), "text/csv") {
		t.Errorf("expected text/csv Content-Type, got %s", wCSV.Header().Get("Content-Type"))
	}
	if !strings.Contains(wCSV.Header().Get("Content-Disposition"), "attachment; filename=") {
		t.Errorf("expected Content-Disposition header, got %s", wCSV.Header().Get("Content-Disposition"))
	}
	if wCSV.Header().Get("Cache-Control") != "no-store" {
		t.Errorf("expected Cache-Control: no-store, got %s", wCSV.Header().Get("Cache-Control"))
	}

	// Parse CSV and verify formula injection sanitation
	reader := csv.NewReader(bytes.NewReader(wCSV.Body.Bytes()))
	records, err := reader.ReadAll()
	if err != nil {
		t.Fatalf("failed to parse returned CSV: %v", err)
	}
	if len(records) < 2 {
		t.Fatalf("expected at least header and 1 data row, got %d rows", len(records))
	}

	foundSanitized := false
	for _, row := range records[1:] {
		// row: [id, project_id, timestamp, actor, action, flag_key, environment, details, prev_hash, entry_hash]
		if strings.Contains(row[7], "1+1") {
			foundSanitized = true
			if !strings.HasPrefix(row[7], "'=") {
				t.Fatalf("formula injection defense failed: details cell did not start with single quote, got: %s", row[7])
			}
			if !strings.HasPrefix(row[0], "'=") {
				t.Fatalf("formula injection defense failed for ID: got %s", row[0])
			}
			if !strings.HasPrefix(row[3], "'@") {
				t.Fatalf("formula injection defense failed for Actor: got %s", row[3])
			}
		}
	}
	if !foundSanitized {
		t.Fatal("could not find the malicious entry in exported CSV")
	}

	// 2. Export as JSON
	reqJSON := httptest.NewRequest(http.MethodGet, "/api/v1/audit/export?format=json", nil)
	reqJSON.AddCookie(authCookie)
	wJSON := httptest.NewRecorder()
	srv.mux.ServeHTTP(wJSON, reqJSON)

	if wJSON.Code != http.StatusOK {
		t.Fatalf("JSON export expected 200 OK, got %d: %s", wJSON.Code, wJSON.Body.String())
	}
	if !strings.Contains(wJSON.Header().Get("Content-Type"), "application/json") {
		t.Errorf("expected application/json, got %s", wJSON.Header().Get("Content-Type"))
	}
	var jsonLogs []domain.AuditLogEntry
	if err := json.Unmarshal(wJSON.Body.Bytes(), &jsonLogs); err != nil {
		t.Fatalf("failed to decode JSON export: %v", err)
	}
	if len(jsonLogs) == 0 {
		t.Fatal("expected non-empty JSON logs")
	}

	// 3. Export as NDJSON
	reqNDJSON := httptest.NewRequest(http.MethodGet, "/api/v1/audit/export?format=ndjson", nil)
	reqNDJSON.AddCookie(authCookie)
	wNDJSON := httptest.NewRecorder()
	srv.mux.ServeHTTP(wNDJSON, reqNDJSON)

	if wNDJSON.Code != http.StatusOK {
		t.Fatalf("NDJSON export expected 200 OK, got %d: %s", wNDJSON.Code, wNDJSON.Body.String())
	}
	if !strings.Contains(wNDJSON.Header().Get("Content-Type"), "application/x-ndjson") {
		t.Errorf("expected application/x-ndjson, got %s", wNDJSON.Header().Get("Content-Type"))
	}
	lines := strings.Split(strings.TrimSpace(wNDJSON.Body.String()), "\n")
	if len(lines) == 0 {
		t.Fatal("expected NDJSON lines")
	}
	var firstLine domain.AuditLogEntry
	if err := json.Unmarshal([]byte(lines[0]), &firstLine); err != nil {
		t.Fatalf("failed to parse NDJSON line: %v", err)
	}
}

func TestAuditHandlers_PurgeLifecycleAndRBAC(t *testing.T) {
	memStore := store.NewMemoryStore()
	srv, err := NewServer(memStore)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	adminCookie := getAdminAuthCookie(t, memStore)
	viewerCookie := getViewerAuthCookie(t, memStore)
	ctx := context.Background()

	// Seed 2 audit logs
	now := time.Now().UTC()
	oldEntry := domain.AuditLogEntry{
		ID:          "old_log_1",
		ProjectID:   domain.DefaultProjectID,
		FlagKey:     "flag-old",
		Action:      "FLAG_CREATED",
		Environment: domain.EnvProduction,
		Actor:       "admin@flagura.dev",
		Details:     "old flag",
		Timestamp:   now.Add(-100 * 24 * time.Hour),
	}
	_, _ = memStore.AppendAuditLog(ctx, oldEntry)

	newEntry := domain.AuditLogEntry{
		ID:          "new_log_1",
		ProjectID:   domain.DefaultProjectID,
		FlagKey:     "flag-new",
		Action:      "FLAG_CREATED",
		Environment: domain.EnvProduction,
		Actor:       "admin@flagura.dev",
		Details:     "new flag",
		Timestamp:   now.Add(-1 * time.Hour),
	}
	_, _ = memStore.AppendAuditLog(ctx, newEntry)

	// 1. Viewer attempt to purge must be rejected with 403 Forbidden
	purgePayload := `{"retention_days": 30}`
	reqViewer := httptest.NewRequest(http.MethodPost, "/api/v1/audit/purge", strings.NewReader(purgePayload))
	reqViewer.Header.Set("Content-Type", "application/json")
	reqViewer.AddCookie(viewerCookie)
	wViewer := httptest.NewRecorder()
	srv.mux.ServeHTTP(wViewer, reqViewer)

	if wViewer.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden for viewer purge, got %d: %s", wViewer.Code, wViewer.Body.String())
	}

	// 2. Future timestamp must be rejected with 400 Bad Request
	futurePayload := fmt.Sprintf(`{"before": "%s"}`, now.Add(24*time.Hour).Format(time.RFC3339))
	reqFuture := httptest.NewRequest(http.MethodPost, "/api/v1/audit/purge", strings.NewReader(futurePayload))
	reqFuture.Header.Set("Content-Type", "application/json")
	reqFuture.AddCookie(adminCookie)
	wFuture := httptest.NewRecorder()
	srv.mux.ServeHTTP(wFuture, reqFuture)

	if wFuture.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request for future purge date, got %d: %s", wFuture.Code, wFuture.Body.String())
	}

	// 3. Admin successfully purges logs older than 30 days
	reqAdmin := httptest.NewRequest(http.MethodPost, "/api/v1/audit/purge", strings.NewReader(purgePayload))
	reqAdmin.Header.Set("Content-Type", "application/json")
	reqAdmin.AddCookie(adminCookie)
	wAdmin := httptest.NewRecorder()
	srv.mux.ServeHTTP(wAdmin, reqAdmin)

	if wAdmin.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for admin purge, got %d: %s", wAdmin.Code, wAdmin.Body.String())
	}

	var purgeResp map[string]interface{}
	if err := json.Unmarshal(wAdmin.Body.Bytes(), &purgeResp); err != nil {
		t.Fatalf("failed to decode purge response: %v", err)
	}
	purgedCount, ok := purgeResp["purged_count"].(float64)
	if !ok || purgedCount < 1 {
		t.Fatalf("expected at least 1 purged log, got %v", purgeResp["purged_count"])
	}

	// 4. Verify that the surviving chain still verifies cleanly
	reqVerify := httptest.NewRequest(http.MethodGet, "/api/v1/audit/verify", nil)
	reqVerify.AddCookie(adminCookie)
	wVerify := httptest.NewRecorder()
	srv.mux.ServeHTTP(wVerify, reqVerify)

	if wVerify.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for verify after purge, got %d: %s", wVerify.Code, wVerify.Body.String())
	}
	var vRes domain.AuditIntegrityResult
	if err := json.Unmarshal(wVerify.Body.Bytes(), &vRes); err != nil {
		t.Fatalf("failed to decode verify response: %v", err)
	}
	if !vRes.Valid {
		t.Fatalf("expected surviving audit chain to verify valid, got error: %s", vRes.ErrorMessage)
	}

	// 5. Verify that AUDIT_LOGS_PURGED action was logged
	reqLogs := httptest.NewRequest(http.MethodGet, "/api/v1/audit-logs", nil)
	reqLogs.AddCookie(adminCookie)
	wLogs := httptest.NewRecorder()
	srv.mux.ServeHTTP(wLogs, reqLogs)

	if !strings.Contains(wLogs.Body.String(), "AUDIT_LOGS_PURGED") {
		t.Fatal("expected audit log of action AUDIT_LOGS_PURGED to be recorded")
	}
}
