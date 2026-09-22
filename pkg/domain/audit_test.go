package domain_test

import (
	"testing"
	"time"

	"github.com/dhawalhost/flagura/pkg/domain"
)

func TestComputeAuditEntryHash_Determinism(t *testing.T) {
	ts := time.Date(2026, 9, 22, 12, 0, 0, 0, time.UTC)
	entry := domain.AuditLogEntry{
		ID:          "audit_1",
		ProjectID:   "proj_default",
		Timestamp:   ts,
		Actor:       "admin@flagura.dev",
		Action:      "TOGGLE",
		FlagKey:     "checkout-redesign",
		Environment: domain.EnvProduction,
		Details:     "Enabled flag in production",
	}

	h1 := domain.ComputeAuditEntryHash(domain.AuditGenesisHash, entry)
	h2 := domain.ComputeAuditEntryHash(domain.AuditGenesisHash, entry)

	if h1 != h2 {
		t.Fatalf("expected identical hash for identical entry, got %s and %s", h1, h2)
	}

	if len(h1) != 64 {
		t.Fatalf("expected 64-character SHA-256 hex string, got %d chars: %s", len(h1), h1)
	}

	// Any field alteration must mutate the resulting hash
	alteredEntry := entry
	alteredEntry.Details = "Tampered details"
	hAltered := domain.ComputeAuditEntryHash(domain.AuditGenesisHash, alteredEntry)
	if hAltered == h1 {
		t.Fatalf("expected altered details to produce different hash, got collision: %s", h1)
	}

	// Altering prevHash must mutate the resulting hash
	hDifferentPrev := domain.ComputeAuditEntryHash("DIFFERENT_PREV_HASH", entry)
	if hDifferentPrev == h1 {
		t.Fatalf("expected different prev_hash to produce different hash, got collision: %s", h1)
	}

	// Defaulting empty prevHash to GENESIS
	hEmptyPrev := domain.ComputeAuditEntryHash("", entry)
	if hEmptyPrev != h1 {
		t.Fatalf("expected empty prevHash to default to GENESIS, got %s vs %s", hEmptyPrev, h1)
	}
}
