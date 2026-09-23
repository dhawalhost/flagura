package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/dhawalhost/flagura/pkg/api"
	"github.com/dhawalhost/flagura/pkg/domain"
	"github.com/dhawalhost/flagura/pkg/store"
)

func TestBackupAndRestoreDrill(t *testing.T) {
	ctx := context.Background()
	st := store.NewMemoryStore()

	// 1. Seed Multi-Tenant Data
	org, err := st.CreateOrganization(ctx, domain.Organization{
		ID:   "org_enterprise_drill",
		Name: "Enterprise Recovery Org",
		Slug: "enterprise-recovery",
	})
	if err != nil {
		t.Fatalf("failed to create organization: %v", err)
	}

	adminUser, err := st.CreateUser(ctx, domain.User{
		Email: "admin@recovery.corp",
		Name:  "Recovery Admin",
		Role:  domain.RoleAdmin,
	})
	if err != nil {
		t.Fatalf("failed to create admin user: %v", err)
	}

	_, _ = st.CreateOrgMember(ctx, domain.OrgMember{
		OrganizationID: org.ID,
		UserID:         adminUser.ID,
		Role:           string(domain.RoleAdmin),
	})

	proj, err := st.CreateProject(ctx, domain.Project{
		ID:             "proj_recovery_drill",
		OrganizationID: org.ID,
		Name:           "Disaster Recovery Drill Project",
		Slug:           "dr-drill",
	})
	if err != nil {
		t.Fatalf("failed to create project: %v", err)
	}

	// Seed Flags and Audit Chain
	flag1 := domain.FeatureFlag{
		ID:        "flag_critical_checkout",
		ProjectID: proj.ID,
		Key:       "critical-checkout-v3",
		Name:      "Critical Checkout V3",
		Type:      "boolean",
		Environments: map[domain.Environment]domain.EnvironmentConfig{
			domain.EnvProduction: {
				Enabled:  true,
				Strategy: domain.StrategyBoolean,
			},
		},
	}
	_, err = st.SaveFlag(ctx, flag1, adminUser.Email)
	if err != nil {
		t.Fatalf("failed to save flag: %v", err)
	}

	// Mutate flag to build hash-chain
	_, _, err = st.ToggleFlagByProject(ctx, proj.ID, flag1.Key, domain.EnvProduction, nil, adminUser.Email)
	if err != nil {
		t.Fatalf("failed to toggle flag: %v", err)
	}

	// Verify pre-backup cryptographic integrity
	preCheck, err := st.VerifyAuditLogIntegrity(ctx, proj.ID)
	if err != nil || !preCheck.Valid {
		t.Fatalf("pre-backup audit log integrity check failed: %v, result: %+v", err, preCheck)
	}
	if preCheck.TotalVerified < 2 {
		t.Fatalf("expected at least 2 verified audit entries, got %d", preCheck.TotalVerified)
	}

	// 2. Export Backup Snapshot via Store Engine (both Gzip and raw JSON)
	var backupBuffer bytes.Buffer
	err = store.CreateSnapshot(ctx, st, &backupBuffer, true)
	if err != nil {
		t.Fatalf("CreateSnapshot failed: %v", err)
	}

	if backupBuffer.Len() == 0 {
		t.Fatalf("expected non-empty snapshot buffer")
	}

	// 3. Test HTTP API Endpoints & RBAC (/api/v1/backup/export & /api/v1/backup/import)
	srv, err := api.NewServer(st)
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}
	ts := httptest.NewServer(srv)
	defer ts.Close()

	sessionToken := "session_dr_admin_token"
	_ = st.CreateSession(ctx, domain.Session{
		Token:     sessionToken,
		UserID:    adminUser.ID,
		ExpiresAt: time.Now().Add(2 * time.Hour),
	})

	// 3a. Unauthorized export should fail
	unauthReq, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/backup/export", nil)
	unauthResp, err := http.DefaultClient.Do(unauthReq)
	if err != nil || unauthResp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized, got: %v (code: %v)", err, unauthResp.StatusCode)
	}
	unauthResp.Body.Close()

	// 3b. Authenticated export should succeed
	authReq, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/backup/export?compress=true", nil)
	authReq.Header.Set("Authorization", "Bearer "+sessionToken)
	authResp, err := http.DefaultClient.Do(authReq)
	if err != nil || authResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK from backup export, got: %v (code: %v)", err, authResp.StatusCode)
	}
	apiSnapshotData, err := io.ReadAll(authResp.Body)
	authResp.Body.Close()
	if err != nil || len(apiSnapshotData) == 0 {
		t.Fatalf("failed to read API snapshot data: %v", err)
	}

	// 4. Simulate Disaster: Reset/Wipe database completely
	_ = st.Reset(ctx)

	// Verify database is empty
	flagsAfterReset, _ := st.ListFlagsByProject(ctx, proj.ID)
	if len(flagsAfterReset) != 0 {
		t.Fatalf("expected 0 flags after disaster wipe, got %d", len(flagsAfterReset))
	}

	// 5. Restore Database from Snapshot
	restoredSnap, err := store.RestoreSnapshot(ctx, st, bytes.NewReader(backupBuffer.Bytes()))
	if err != nil {
		t.Fatalf("RestoreSnapshot failed: %v", err)
	}
	if restoredSnap == nil {
		t.Fatalf("expected non-nil restored snapshot")
	}

	// 6. Assert Full Data Recovery & Cryptographic Verification
	// 6a. Organizations and Projects
	restoredOrg, err := st.GetOrganization(ctx, org.ID)
	if err != nil || restoredOrg == nil || restoredOrg.Name != org.Name {
		t.Fatalf("organization not restored properly: %v", err)
	}
	restoredProj, err := st.GetProject(ctx, proj.ID)
	if err != nil || restoredProj == nil || restoredProj.Name != proj.Name {
		t.Fatalf("project not restored properly: %v", err)
	}

	// 6b. Flags
	restoredFlags, err := st.ListFlagsByProject(ctx, proj.ID)
	if err != nil || len(restoredFlags) == 0 {
		t.Fatalf("expected flags restored, got %d (err: %v)", len(restoredFlags), err)
	}

	// 6c. Users
	restoredUser, err := st.GetUserByEmail(ctx, adminUser.Email)
	if err != nil || restoredUser == nil {
		t.Fatalf("user not restored properly: %v", err)
	}

	// 6d. Audit Log Cryptographic Tamper-Evidence Verification Drill
	postCheck, err := st.VerifyAuditLogIntegrity(ctx, proj.ID)
	if err != nil {
		t.Fatalf("post-recovery audit log verification error: %v", err)
	}
	if !postCheck.Valid {
		t.Fatalf("post-recovery audit log integrity broke! Details: %+v", postCheck)
	}
	postLogs, _ := st.ListAuditLogsByProject(ctx, proj.ID, 100)
	for i, l := range postLogs {
		t.Logf("postLog[%d]: ID=%s FlagKey=%s Action=%s Actor=%s", i, l.ID, l.FlagKey, l.Action, l.Actor)
	}
	if postCheck.TotalVerified != preCheck.TotalVerified {
		t.Fatalf("expected %d verified entries, got %d", preCheck.TotalVerified, postCheck.TotalVerified)
	}

	// 7. Test API Import Endpoint (/api/v1/backup/import)
	importReq, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/backup/import", bytes.NewReader(apiSnapshotData))
	importReq.Header.Set("Authorization", "Bearer "+sessionToken)
	importReq.Header.Set("Content-Type", "application/gzip")
	importResp, err := http.DefaultClient.Do(importReq)
	if err != nil || importResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(importResp.Body)
		t.Fatalf("expected 200 OK from backup import endpoint, got %d: %s", importResp.StatusCode, string(body))
	}
	var importResult map[string]interface{}
	_ = json.NewDecoder(importResp.Body).Decode(&importResult)
	importResp.Body.Close()

	if importResult["status"] != "success" {
		t.Fatalf("expected success status from import endpoint, got: %v", importResult)
	}
}

func TestSQLiteFileHotBackup(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "flagura_sqlite_backup_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	srcDb := filepath.Join(tempDir, "source.db")
	destDb := filepath.Join(tempDir, "backup.db")

	// Create dummy sqlite content
	err = os.WriteFile(srcDb, []byte("SQLite format 3\x00mocked-binary-content-42"), 0600)
	if err != nil {
		t.Fatalf("failed to create source db: %v", err)
	}

	err = store.BackupSQLite(srcDb, destDb)
	if err != nil {
		t.Fatalf("BackupSQLite failed: %v", err)
	}

	destData, err := os.ReadFile(destDb)
	if err != nil {
		t.Fatalf("failed to read dest db: %v", err)
	}
	if string(destData) != "SQLite format 3\x00mocked-binary-content-42" {
		t.Fatalf("backup content does not match source content")
	}

	// Verify file permissions 0600
	info, err := os.Stat(destDb)
	if err != nil {
		t.Fatalf("failed to stat dest db: %v", err)
	}
	if info.Mode().Perm() != 0600 {
		t.Errorf("expected 0600 file permissions, got %v", info.Mode().Perm())
	}
}
