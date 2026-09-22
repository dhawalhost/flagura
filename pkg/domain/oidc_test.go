package domain

import (
	"testing"
)

func TestOIDCConfig_IsDomainAllowed(t *testing.T) {
	tests := []struct {
		name           string
		allowedDomains string
		email          string
		expected       bool
	}{
		{
			name:           "empty allowed domains permits any email",
			allowedDomains: "",
			email:          "user@example.com",
			expected:       true,
		},
		{
			name:           "whitespace allowed domains permits any email",
			allowedDomains: "  ",
			email:          "user@example.com",
			expected:       true,
		},
		{
			name:           "exact domain match without @",
			allowedDomains: "acme.com,stark.corp",
			email:          "alice@acme.com",
			expected:       true,
		},
		{
			name:           "exact domain match with @",
			allowedDomains: "@acme.com, @stark.corp",
			email:          "tony@stark.corp",
			expected:       true,
		},
		{
			name:           "case insensitive match",
			allowedDomains: "ACME.COM",
			email:          "bob@Acme.Com",
			expected:       true,
		},
		{
			name:           "unauthorized domain rejected",
			allowedDomains: "acme.com",
			email:          "attacker@evil.com",
			expected:       false,
		},
		{
			name:           "subdomain not automatically permitted",
			allowedDomains: "acme.com",
			email:          "user@sub.acme.com",
			expected:       false,
		},
		{
			name:           "malformed email rejected",
			allowedDomains: "acme.com",
			email:          "invalid-email-no-at",
			expected:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &OIDCConfig{AllowedDomains: tt.allowedDomains}
			if got := cfg.IsDomainAllowed(tt.email); got != tt.expected {
				t.Fatalf("IsDomainAllowed(%q) with allowed %q: got %v, want %v", tt.email, tt.allowedDomains, got, tt.expected)
			}
		})
	}
}
