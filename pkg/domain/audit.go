package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"
)

// AuditGenesisHash is the predecessor hash for the initial entry in a project chain.
const AuditGenesisHash = "GENESIS"

// AuditIntegrityResult represents the outcome of a cryptographic chain verification.
type AuditIntegrityResult struct {
	Valid         bool      `json:"valid"`
	TotalVerified int       `json:"totalVerified"`
	HeadHash      string    `json:"headHash,omitempty"`
	VerifiedAt    time.Time `json:"verifiedAt"`
	BrokenEntryID string    `json:"brokenEntryId,omitempty"`
	ErrorMessage  string    `json:"errorMessage,omitempty"`
}

// ComputeAuditEntryHash deterministically generates a SHA-256 hash over an audit entry
// and its predecessor's hash, forming an immutable tamper-evident chain.
func ComputeAuditEntryHash(prevHash string, entry AuditLogEntry) string {
	if prevHash == "" {
		prevHash = AuditGenesisHash
	}

	payload := fmt.Sprintf("%s|%s|%s|%s|%s|%s|%s|%s|%s",
		prevHash,
		entry.ID,
		entry.ProjectID,
		entry.FlagKey,
		entry.Action,
		string(entry.Environment),
		entry.Actor,
		entry.Details,
		entry.Timestamp.UTC().Format(time.RFC3339Nano),
	)

	sum := sha256.Sum256([]byte(payload))
	return hex.EncodeToString(sum[:])
}
