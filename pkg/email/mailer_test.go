package email

import (
	"os"
	"testing"
)

func TestConsoleMailerHTMLRendering(t *testing.T) {
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

	// 1. Test Password Reset HTML Email
	err := mailer.SendPasswordReset("dhawal@flagura.dev", "Dhawal Dyavanpalli", "http://localhost:3000/auth?mode=reset&token=test_token_123")
	if err != nil {
		t.Fatalf("Failed to render and send password reset HTML email: %v", err)
	}

	// 2. Test Welcome HTML Email
	err = mailer.SendWelcomeEmail("newuser@flagura.dev", "New Engineer", "http://localhost:3000/dashboard")
	if err != nil {
		t.Fatalf("Failed to render and send welcome HTML email: %v", err)
	}

	// 3. Test Change Request HTML Email
	err = mailer.SendChangeRequestNotification(
		"admin@flagura.dev",
		"Lead Architect",
		"Developer One",
		"new_checkout_flow",
		"production",
		"ENABLE_FLAG",
		"http://localhost:3000/dashboard?tab=governance",
	)
	if err != nil {
		t.Fatalf("Failed to render and send change request HTML email: %v", err)
	}
}

func TestConfigFromEnvironmentVariables(t *testing.T) {
	os.Setenv("SMTP_FROM", "notifier@corp.internal")
	os.Setenv("FLAGURA_SUPPORT_EMAIL", "support@corp.internal")
	os.Setenv("FLAGURA_BRAND_NAME", "Corp Flags")
	os.Setenv("FLAGURA_GOVERNANCE_EMAILS", "approver1@corp.internal, approver2@corp.internal")
	defer func() {
		os.Unsetenv("SMTP_FROM")
		os.Unsetenv("FLAGURA_SUPPORT_EMAIL")
		os.Unsetenv("FLAGURA_BRAND_NAME")
		os.Unsetenv("FLAGURA_GOVERNANCE_EMAILS")
	}()

	cfg := LoadConfigFromEnv()
	if cfg.FromEmail != "notifier@corp.internal" {
		t.Fatalf("Expected from email 'notifier@corp.internal', got: %s", cfg.FromEmail)
	}
	if cfg.SupportEmail != "support@corp.internal" {
		t.Fatalf("Expected support email 'support@corp.internal', got: %s", cfg.SupportEmail)
	}
	if cfg.BrandName != "Corp Flags" {
		t.Fatalf("Expected brand name 'Corp Flags', got: %s", cfg.BrandName)
	}
	if len(cfg.GovernanceEmails) != 2 || cfg.GovernanceEmails[0] != "approver1@corp.internal" || cfg.GovernanceEmails[1] != "approver2@corp.internal" {
		t.Fatalf("Expected 2 trimmed governance emails, got: %+v", cfg.GovernanceEmails)
	}
}

func TestSMTPMailerCreationFromEnv(t *testing.T) {
	// 1. By default, when SMTP_HOST is unset, email is disabled
	mailer := NewMailerFromEnv()
	if mailer == nil {
		t.Fatalf("Expected non-nil mailer from env")
	}
	if mailer.IsEnabled() {
		t.Fatalf("Expected email to be disabled by default when SMTP_HOST is unset")
	}
	if _, ok := mailer.(*DisabledMailer); !ok {
		t.Fatalf("Expected default mailer to be *DisabledMailer when SMTP_HOST is unset")
	}

	// 2. When ENABLE_CONSOLE_MAILER=true, console mailer is activated
	os.Setenv("ENABLE_CONSOLE_MAILER", "true")
	defer os.Unsetenv("ENABLE_CONSOLE_MAILER")

	devMailer := NewMailerFromEnv()
	if !devMailer.IsEnabled() {
		t.Fatalf("Expected email to be enabled when ENABLE_CONSOLE_MAILER=true")
	}
	if _, ok := devMailer.(*ConsoleMailer); !ok {
		t.Fatalf("Expected *ConsoleMailer when ENABLE_CONSOLE_MAILER=true")
	}
}
