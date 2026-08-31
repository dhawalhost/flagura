package email

import (
	"os"
	"testing"
)

func TestConsoleMailer_HTMLDispatch(t *testing.T) {
	customCfg := Config{
		FromEmail:        "alerts@enterprise.com",
		SupportEmail:     "helpdesk@enterprise.com",
		BrandName:        "Enterprise Flags",
		AppURL:           "https://flags.enterprise.com",
		GovernanceEmails: []string{"security@enterprise.com", "arch@enterprise.com"},
	}
	mailer := NewConsoleMailer(customCfg)

	if mailer.GetSupportEmail() != "helpdesk@enterprise.com" {
		t.Fatalf("Expected support email helpdesk@enterprise.com, got: %s", mailer.GetSupportEmail())
	}
	if len(mailer.GetGovernanceEmails()) != 2 {
		t.Fatalf("Expected 2 governance emails, got: %d", len(mailer.GetGovernanceEmails()))
	}

	tests := []struct {
		name     string
		sendFunc func() error
	}{
		{
			name: "Send Password Reset HTML Email",
			sendFunc: func() error {
				return mailer.SendPasswordReset("dhawal@flagura.dev", "Dhawal Dyavanpalli", "http://localhost:3000/auth?mode=reset&token=test_token_123")
			},
		},
		{
			name: "Send Welcome HTML Email",
			sendFunc: func() error {
				return mailer.SendWelcomeEmail("newuser@flagura.dev", "New Engineer", "http://localhost:3000/dashboard")
			},
		},
		{
			name: "Send Change Request Notification HTML Email",
			sendFunc: func() error {
				return mailer.SendChangeRequestNotification(
					"admin@flagura.dev",
					"Lead Architect",
					"Developer One",
					"new_checkout_flow",
					"production",
					"ENABLE_FLAG",
					"http://localhost:3000/dashboard?tab=governance",
				)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.sendFunc(); err != nil {
				t.Fatalf("sendFunc failed: %v", err)
			}
		})
	}
}

func TestLoadConfigFromEnv(t *testing.T) {
	tests := []struct {
		name             string
		envVars          map[string]string
		expectedFrom     string
		expectedSupport  string
		expectedBrand    string
		expectedGovCount int
	}{
		{
			name: "Config parsed with custom environment variables",
			envVars: map[string]string{
				"SMTP_FROM":                 "notifier@corp.internal",
				"FLAGURA_SUPPORT_EMAIL":     "support@corp.internal",
				"FLAGURA_BRAND_NAME":        "Corp Flags",
				"FLAGURA_GOVERNANCE_EMAILS": "approver1@corp.internal, approver2@corp.internal",
			},
			expectedFrom:     "notifier@corp.internal",
			expectedSupport:  "support@corp.internal",
			expectedBrand:    "Corp Flags",
			expectedGovCount: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for k, v := range tt.envVars {
				os.Setenv(k, v)
			}
			defer func() {
				for k := range tt.envVars {
					os.Unsetenv(k)
				}
			}()

			cfg := LoadConfigFromEnv()
			if cfg.FromEmail != tt.expectedFrom {
				t.Errorf("expected from %q, got %q", tt.expectedFrom, cfg.FromEmail)
			}
			if cfg.SupportEmail != tt.expectedSupport {
				t.Errorf("expected support %q, got %q", tt.expectedSupport, cfg.SupportEmail)
			}
			if cfg.BrandName != tt.expectedBrand {
				t.Errorf("expected brand %q, got %q", tt.expectedBrand, cfg.BrandName)
			}
			if len(cfg.GovernanceEmails) != tt.expectedGovCount {
				t.Errorf("expected %d governance emails, got %d", tt.expectedGovCount, len(cfg.GovernanceEmails))
			}
		})
	}
}

func TestNewMailerFromEnv(t *testing.T) {
	tests := []struct {
		name            string
		envVars         map[string]string
		expectedEnabled bool
	}{
		{
			name: "Default state when SMTP is not configured -> DisabledMailer",
			envVars: map[string]string{
				"ENABLE_CONSOLE_MAILER": "",
				"SMTP_HOST":             "",
			},
			expectedEnabled: false,
		},
		{
			name: "Console mailer activated via ENABLE_CONSOLE_MAILER=true",
			envVars: map[string]string{
				"ENABLE_CONSOLE_MAILER": "true",
			},
			expectedEnabled: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for k, v := range tt.envVars {
				if v == "" {
					os.Unsetenv(k)
				} else {
					os.Setenv(k, v)
				}
			}
			defer func() {
				for k := range tt.envVars {
					os.Unsetenv(k)
				}
			}()

			mailer := NewMailerFromEnv()
			if mailer == nil {
				t.Fatalf("expected non-nil mailer")
			}
			if mailer.IsEnabled() != tt.expectedEnabled {
				t.Errorf("mailer.IsEnabled() = %v, expected %v", mailer.IsEnabled(), tt.expectedEnabled)
			}
		})
	}
}
