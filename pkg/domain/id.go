package domain

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

// NewID generates a cryptographically secure, unique identifier with a given prefix.
// Example: NewID("usr") -> "usr_7f8a9b0c1d2e3f4a"
func NewID(prefix string) string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	timestamp := time.Now().UTC().UnixNano()
	return fmt.Sprintf("%s_%d_%s", prefix, timestamp, hex.EncodeToString(b)[:8])
}

// Slugify generates a clean URL-friendly slug from a name.
func Slugify(name string) string {
	s := strings.ToLower(strings.TrimSpace(name))
	var result strings.Builder
	for _, ch := range s {
		switch {
		case (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9'):
			result.WriteRune(ch)
		case ch == ' ' || ch == '-' || ch == '_':
			if result.Len() > 0 && !strings.HasSuffix(result.String(), "-") {
				result.WriteRune('-')
			}
		}
	}
	res := strings.Trim(result.String(), "-")
	if res == "" {
		return "default"
	}
	return res
}

// NewOrganization creates an Organization entity with defaults.
func NewOrganization(name, slug, description string) Organization {
	now := time.Now().UTC()
	if slug == "" {
		slug = Slugify(name)
	}
	return Organization{
		ID:          NewID("org"),
		Name:        strings.TrimSpace(name),
		Slug:        slug,
		Description: strings.TrimSpace(description),
		CreatedAt:   now,
		UpdatedAt:   now,
	}
}

// NewProject creates a Project entity under an Organization.
func NewProject(orgID, name, slug, description string) Project {
	now := time.Now().UTC()
	if orgID == "" {
		orgID = DefaultOrgID
	}
	if slug == "" {
		slug = Slugify(name)
	}
	return Project{
		ID:             NewID("proj"),
		OrganizationID: orgID,
		Name:           strings.TrimSpace(name),
		Slug:           slug,
		Description:    strings.TrimSpace(description),
		CreatedAt:      now,
		UpdatedAt:      now,
	}
}

// NewUser creates a User entity with defaults.
func NewUser(email, name, passwordHash string, role UserRole) User {
	now := time.Now().UTC()
	if role == "" {
		role = RoleDeveloper
	}
	return User{
		ID:           NewID("usr"),
		Email:        strings.TrimSpace(strings.ToLower(email)),
		Name:         strings.TrimSpace(name),
		PasswordHash: passwordHash,
		Role:         role,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
}
