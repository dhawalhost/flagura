package domain

import (
	"strings"
	"time"
)

// OIDCConfig represents an enterprise OpenID Connect identity provider configuration
// scoped to a specific organization.
type OIDCConfig struct {
	OrganizationID string    `json:"organization_id"`
	Enabled        bool      `json:"enabled"`
	IssuerURL      string    `json:"issuer_url"`
	ClientID       string    `json:"client_id"`
	ClientSecret   string    `json:"client_secret,omitempty"` // Masked in public/GET responses
	AllowedDomains string    `json:"allowed_domains"`         // Comma-separated domains e.g. "acme.com, acme.co"
	DefaultRole    string    `json:"default_role"`            // Default role for JIT provisioning: "developer", "viewer", "admin"
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// IsDomainAllowed checks whether the provided email address matches one of the allowed domains.
// If AllowedDomains is empty, any domain is permitted.
func (c *OIDCConfig) IsDomainAllowed(email string) bool {
	if c == nil || strings.TrimSpace(c.AllowedDomains) == "" {
		return true
	}
	parts := strings.Split(email, "@")
	if len(parts) != 2 {
		return false
	}
	domain := strings.ToLower(strings.TrimSpace(parts[1]))
	allowed := strings.Split(c.AllowedDomains, ",")
	for _, a := range allowed {
		clean := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(a), "@"))
		if clean == domain {
			return true
		}
	}
	return false
}

// OIDCStateClaims stores ephemeral CSRF state for an in-flight OIDC authentication request.
type OIDCStateClaims struct {
	State          string    `json:"state"`
	Nonce          string    `json:"nonce"`
	OrganizationID string    `json:"organization_id"`
	RedirectURI    string    `json:"redirect_uri,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
}
