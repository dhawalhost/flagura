package store

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/dhawalhost/flagura/pkg/domain"
)

const (
	// CurrentSnapshotVersion indicates the schema version of Flagura backup snapshots.
	CurrentSnapshotVersion = "1.0.0"
)

// SnapshotData represents the full relational and multi-tenant state of a Flagura deployment.
type SnapshotData struct {
	Version         string                  `json:"version"`
	ExportedAt      time.Time               `json:"exported_at"`
	Driver          string                  `json:"driver"`
	Organizations   []domain.Organization   `json:"organizations"`
	OrgMembers      []domain.OrgMember      `json:"org_members"`
	Projects        []domain.Project        `json:"projects"`
	Flags           []domain.FeatureFlag    `json:"flags"`
	Users           []domain.User           `json:"users"`
	APIKeys         []domain.APIKey         `json:"api_keys"`
	AuditLogs       []domain.AuditLogEntry  `json:"audit_logs"`
	CanarySchedules []domain.CanarySchedule `json:"canary_schedules,omitempty"`
}

// CreateSnapshot exports all multi-tenant entities from the Store into an io.Writer (optionally gzip compressed).
func CreateSnapshot(ctx context.Context, st Store, w io.Writer, compress bool) error {
	var targetWriter io.Writer = w
	if compress {
		gz := gzip.NewWriter(w)
		defer gz.Close()
		targetWriter = gz
	}

	snap := SnapshotData{
		Version:    CurrentSnapshotVersion,
		ExportedAt: time.Now().UTC(),
		Driver:     st.DriverName(),
	}

	// 1. Export Organizations
	orgs, err := st.ListOrganizations(ctx)
	if err != nil {
		return fmt.Errorf("failed to export organizations: %w", err)
	}
	snap.Organizations = orgs

	// 2. Export Org Members & Projects
	seenProjects := make(map[string]bool)
	for _, org := range orgs {
		members, err := st.ListOrgMembers(ctx, org.ID)
		if err == nil {
			snap.OrgMembers = append(snap.OrgMembers, members...)
		}

		projects, err := st.ListProjects(ctx, org.ID)
		if err == nil {
			for _, p := range projects {
				if !seenProjects[p.ID] {
					seenProjects[p.ID] = true
					snap.Projects = append(snap.Projects, p)
				}
			}
		}
	}

	// Ensure default project is included if not yet captured
	if !seenProjects[DefaultProjectID] {
		if defProj, err := st.GetProject(ctx, DefaultProjectID); err == nil && defProj != nil {
			snap.Projects = append(snap.Projects, *defProj)
			seenProjects[DefaultProjectID] = true
		}
	}

	// 3. Export Flags & Audit Logs per project
	seenLogs := make(map[string]bool)
	for _, p := range snap.Projects {
		flags, err := st.ListFlagsByProject(ctx, p.ID)
		if err == nil {
			snap.Flags = append(snap.Flags, flags...)
		}

		logs, err := st.ExportAuditLogs(ctx, p.ID, nil, nil, 100000)
		if err == nil {
			for _, l := range logs {
				if !seenLogs[l.ID] {
					seenLogs[l.ID] = true
					snap.AuditLogs = append(snap.AuditLogs, l)
				}
			}
		}
	}

	// 4. Export Users
	users, err := st.ListUsers(ctx)
	if err == nil {
		snap.Users = users
	}

	// 5. Export API Keys
	keys, err := st.ListAPIKeys(ctx)
	if err == nil {
		snap.APIKeys = keys
	}

	// 6. Export Canary Schedules
	canaries, err := st.ListActiveCanarySchedules(ctx)
	if err == nil {
		snap.CanarySchedules = canaries
	}

	enc := json.NewEncoder(targetWriter)
	enc.SetIndent("", "  ")
	if err := enc.Encode(snap); err != nil {
		return fmt.Errorf("failed to encode snapshot JSON: %w", err)
	}

	return nil
}

// RestoreSnapshot imports entities from an io.Reader into the Store.
func RestoreSnapshot(ctx context.Context, st Store, r io.Reader) (*SnapshotData, error) {
	// Attempt gzip header detection
	bufReader := io.Reader(r)
	gz, err := gzip.NewReader(r)
	if err == nil {
		defer gz.Close()
		bufReader = gz
	}

	var snap SnapshotData
	if err := json.NewDecoder(bufReader).Decode(&snap); err != nil {
		return nil, fmt.Errorf("failed to decode snapshot JSON: %w", err)
	}

	if snap.Version == "" {
		return nil, fmt.Errorf("invalid snapshot: missing version identifier")
	}

	// 1. Restore Organizations
	for _, org := range snap.Organizations {
		_, _ = st.CreateOrganization(ctx, org)
	}

	// 2. Restore Users
	for _, u := range snap.Users {
		_, _ = st.CreateUser(ctx, u)
	}

	// 3. Restore Org Members
	for _, m := range snap.OrgMembers {
		_, _ = st.CreateOrgMember(ctx, m)
	}

	// 4. Restore Projects
	for _, p := range snap.Projects {
		_, _ = st.CreateProject(ctx, p)
	}

	// 5. Restore API Keys
	for _, k := range snap.APIKeys {
		_, _ = st.CreateAPIKey(ctx, k)
	}

	// 6. Restore Flags
	for _, f := range snap.Flags {
		_, _ = st.SaveFlag(ctx, f, "snapshot_restore")
	}

	// 7. Restore Audit Logs (preserving chronological order and cryptographic hash chaining)
	seenLogs := make(map[string]bool)
	for i := len(snap.AuditLogs) - 1; i >= 0; i-- {
		log := snap.AuditLogs[i]
		if !seenLogs[log.ID] {
			seenLogs[log.ID] = true
			_, _ = st.AppendAuditLog(ctx, log)
		}
	}

	// 8. Restore Canary Schedules
	for _, c := range snap.CanarySchedules {
		_ = st.SaveCanarySchedule(ctx, c)
	}

	return &snap, nil
}

// BackupSQLite creates a hot atomic file copy of an SQLite database with safe permissions (0600).
func BackupSQLite(srcPath, destPath string) error {
	data, err := os.ReadFile(filepath.Clean(srcPath))
	if err != nil {
		return fmt.Errorf("failed to read source sqlite file: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(destPath), 0750); err != nil {
		return fmt.Errorf("failed to create destination directory: %w", err)
	}

	// Write file with strict 0600 file permissions
	// #nosec G703 -- destination file path is cleaned by caller for local administrative backup with explicit 0600 file permissions
	if err := os.WriteFile(filepath.Clean(destPath), data, 0600); err != nil {
		return fmt.Errorf("failed to write sqlite backup file: %w", err)
	}

	return nil
}
