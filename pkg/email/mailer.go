package email

import (
	"bytes"
	"crypto/tls"
	"embed"
	"fmt"
	"html/template"
	"log"
	"net/smtp"
	"os"
	"strconv"
	"strings"
)

//go:embed templates/*.html
var templatesFS embed.FS

var emailTemplates = template.Must(template.ParseFS(templatesFS, "templates/*.html"))

// Config encapsulates email service settings, branding, and notification targets.
type Config struct {
	Host             string
	Port             int
	Username         string
	Password         string
	FromEmail        string
	SupportEmail     string
	BrandName        string
	AppURL           string
	GovernanceEmails []string
	UseTLS           bool
}

// DefaultConfig returns safe, privacy-preserving defaults for self-hosted instances.
func DefaultConfig() Config {
	return Config{
		Host:             "",
		Port:             587,
		FromEmail:        "no-reply@localhost",
		SupportEmail:     "", // Empty by default so self-hosted users see their workspace admin notice
		BrandName:        "Flagura",
		AppURL:           "http://localhost:3000",
		GovernanceEmails: []string{}, // Auto-resolved dynamically from local database admins
		UseTLS:           false,
	}
}

// LoadConfigFromEnv populates email configuration from environment variables.
func LoadConfigFromEnv() Config {
	cfg := DefaultConfig()

	if host := os.Getenv("SMTP_HOST"); host != "" {
		cfg.Host = host
	}
	if pStr := os.Getenv("SMTP_PORT"); pStr != "" {
		if p, err := strconv.Atoi(pStr); err == nil {
			cfg.Port = p
		}
	}
	if cfg.Port == 465 || os.Getenv("SMTP_USE_TLS") == "true" {
		cfg.UseTLS = true
	}
	if u := os.Getenv("SMTP_USERNAME"); u != "" {
		cfg.Username = u
	}
	if p := os.Getenv("SMTP_PASSWORD"); p != "" {
		cfg.Password = p
	}
	if from := os.Getenv("SMTP_FROM"); from != "" {
		cfg.FromEmail = from
	} else if from := os.Getenv("FLAGURA_FROM_EMAIL"); from != "" {
		cfg.FromEmail = from
	}
	if sup := os.Getenv("FLAGURA_SUPPORT_EMAIL"); sup != "" {
		cfg.SupportEmail = sup
	}
	if b := os.Getenv("FLAGURA_BRAND_NAME"); b != "" {
		cfg.BrandName = b
	}
	if u := os.Getenv("FLAGURA_APP_URL"); u != "" {
		cfg.AppURL = strings.TrimRight(u, "/")
	}

	if rawGov := os.Getenv("FLAGURA_GOVERNANCE_EMAILS"); rawGov != "" {
		var list []string
		for _, item := range strings.Split(rawGov, ",") {
			if trimmed := strings.TrimSpace(item); trimmed != "" {
				list = append(list, trimmed)
			}
		}
		if len(list) > 0 {
			cfg.GovernanceEmails = list
		}
	}

	return cfg
}

// Mailer defines the interface for dispatching HTML transactional emails.
type Mailer interface {
	IsEnabled() bool
	GetConfig() Config
	GetGovernanceEmails() []string
	GetSupportEmail() string
	SendPasswordReset(toEmail, recipientName, resetURL string) error
	SendWelcomeEmail(toEmail, recipientName, dashboardURL string) error
	SendChangeRequestNotification(toEmail, recipientName, requesterName, flagKey, environment, actionType, reviewURL string) error
}

// PasswordResetData represents template parameters for password reset.
type PasswordResetData struct {
	ToEmail       string
	RecipientName string
	ResetURL      string
	BrandName     string
	SupportEmail  string
}

// WelcomeData represents template parameters for onboarding email.
type WelcomeData struct {
	ToEmail       string
	RecipientName string
	DashboardURL  string
	BrandName     string
	SupportEmail  string
}

// ChangeRequestData represents template parameters for governance review emails.
type ChangeRequestData struct {
	ToEmail       string
	RecipientName string
	RequesterName string
	FlagKey       string
	Environment   string
	ActionType    string
	ReviewURL     string
	BrandName     string
	SupportEmail  string
}

// DisabledMailer is a no-op mailer used when SMTP is not configured.
type DisabledMailer struct {
	Config Config
}

func NewDisabledMailer(cfg ...Config) *DisabledMailer {
	c := DefaultConfig()
	if len(cfg) > 0 {
		c = cfg[0]
	}
	return &DisabledMailer{Config: c}
}

func (m *DisabledMailer) IsEnabled() bool { return false }
func (m *DisabledMailer) GetConfig() Config { return m.Config }
func (m *DisabledMailer) GetGovernanceEmails() []string { return m.Config.GovernanceEmails }
func (m *DisabledMailer) GetSupportEmail() string { return m.Config.SupportEmail }
func (m *DisabledMailer) SendPasswordReset(toEmail, recipientName, resetURL string) error { return nil }
func (m *DisabledMailer) SendWelcomeEmail(toEmail, recipientName, dashboardURL string) error { return nil }
func (m *DisabledMailer) SendChangeRequestNotification(toEmail, recipientName, requesterName, flagKey, environment, actionType, reviewURL string) error { return nil }

// ConsoleMailer logs HTML emails to stdout (useful in local dev or testing when explicitly enabled).
type ConsoleMailer struct {
	Config Config
}

func NewConsoleMailer(cfg ...Config) *ConsoleMailer {
	c := DefaultConfig()
	if len(cfg) > 0 {
		c = cfg[0]
	}
	return &ConsoleMailer{Config: c}
}

func (m *ConsoleMailer) IsEnabled() bool {
	return true
}

func (m *ConsoleMailer) GetConfig() Config {
	return m.Config
}

func (m *ConsoleMailer) GetGovernanceEmails() []string {
	return m.Config.GovernanceEmails
}

func (m *ConsoleMailer) GetSupportEmail() string {
	return m.Config.SupportEmail
}

func (m *ConsoleMailer) SendPasswordReset(toEmail, recipientName, resetURL string) error {
	data := PasswordResetData{
		ToEmail:       toEmail,
		RecipientName: recipientName,
		ResetURL:      resetURL,
		BrandName:     m.Config.BrandName,
		SupportEmail:  m.Config.SupportEmail,
	}
	var buf bytes.Buffer
	if err := emailTemplates.ExecuteTemplate(&buf, "password_reset.html", data); err != nil {
		return fmt.Errorf("failed to render password_reset.html: %w", err)
	}

	log.Printf("\n📧 [HTML EMAIL DISPATCH] Password Reset Request\n"+
		"   From:      %s\n"+
		"   To:        %s (%s)\n"+
		"   Subject:   Reset Your %s Console Password\n"+
		"   Template:  pkg/email/templates/password_reset.html\n"+
		"   Reset URL: %s\n"+
		"   Support:   %s\n"+
		"   HTML Size: %d bytes\n", m.Config.FromEmail, toEmail, recipientName, m.Config.BrandName, resetURL, m.Config.SupportEmail, buf.Len())
	return nil
}

func (m *ConsoleMailer) SendWelcomeEmail(toEmail, recipientName, dashboardURL string) error {
	data := WelcomeData{
		ToEmail:       toEmail,
		RecipientName: recipientName,
		DashboardURL:  dashboardURL,
		BrandName:     m.Config.BrandName,
		SupportEmail:  m.Config.SupportEmail,
	}
	var buf bytes.Buffer
	if err := emailTemplates.ExecuteTemplate(&buf, "welcome.html", data); err != nil {
		return fmt.Errorf("failed to render welcome.html: %w", err)
	}

	log.Printf("\n📧 [HTML EMAIL DISPATCH] Welcome to %s\n"+
		"   From:      %s\n"+
		"   To:        %s (%s)\n"+
		"   Subject:   Welcome to %s Feature Flag Platform!\n"+
		"   Template:  pkg/email/templates/welcome.html\n"+
		"   Dashboard: %s\n"+
		"   Support:   %s\n"+
		"   HTML Size: %d bytes\n", m.Config.BrandName, m.Config.FromEmail, toEmail, recipientName, m.Config.BrandName, dashboardURL, m.Config.SupportEmail, buf.Len())
	return nil
}

func (m *ConsoleMailer) SendChangeRequestNotification(toEmail, recipientName, requesterName, flagKey, environment, actionType, reviewURL string) error {
	data := ChangeRequestData{
		ToEmail:       toEmail,
		RecipientName: recipientName,
		RequesterName: requesterName,
		FlagKey:       flagKey,
		Environment:   environment,
		ActionType:    actionType,
		ReviewURL:     reviewURL,
		BrandName:     m.Config.BrandName,
		SupportEmail:  m.Config.SupportEmail,
	}
	var buf bytes.Buffer
	if err := emailTemplates.ExecuteTemplate(&buf, "change_request.html", data); err != nil {
		return fmt.Errorf("failed to render change_request.html: %w", err)
	}

	log.Printf("\n📧 [HTML EMAIL DISPATCH] Change Request Review Required\n"+
		"   From:      %s\n"+
		"   To:        %s (%s)\n"+
		"   Subject:   Four-Eyes Governance Approval Required\n"+
		"   Flag:      %s (%s)\n"+
		"   Template:  pkg/email/templates/change_request.html\n"+
		"   Support:   %s\n"+
		"   HTML Size: %d bytes\n", m.Config.FromEmail, toEmail, recipientName, flagKey, environment, m.Config.SupportEmail, buf.Len())
	return nil
}

// SMTPMailer dispatches rich HTML emails through a standard SMTP server.
type SMTPMailer struct {
	Config Config
}

func NewSMTPMailer(cfg Config) *SMTPMailer {
	return &SMTPMailer{Config: cfg}
}

func (m *SMTPMailer) IsEnabled() bool {
	return true
}

func (m *SMTPMailer) GetConfig() Config {
	return m.Config
}

func (m *SMTPMailer) GetGovernanceEmails() []string {
	return m.Config.GovernanceEmails
}

func (m *SMTPMailer) GetSupportEmail() string {
	return m.Config.SupportEmail
}

func (m *SMTPMailer) SendPasswordReset(toEmail, recipientName, resetURL string) error {
	data := PasswordResetData{
		ToEmail:       toEmail,
		RecipientName: recipientName,
		ResetURL:      resetURL,
		BrandName:     m.Config.BrandName,
		SupportEmail:  m.Config.SupportEmail,
	}
	var buf bytes.Buffer
	if err := emailTemplates.ExecuteTemplate(&buf, "password_reset.html", data); err != nil {
		return fmt.Errorf("failed to render password_reset.html: %w", err)
	}

	subject := fmt.Sprintf("Reset Your %s Console Password", m.Config.BrandName)
	return m.sendHTMLEmail(toEmail, subject, buf.String())
}

func (m *SMTPMailer) SendWelcomeEmail(toEmail, recipientName, dashboardURL string) error {
	data := WelcomeData{
		ToEmail:       toEmail,
		RecipientName: recipientName,
		DashboardURL:  dashboardURL,
		BrandName:     m.Config.BrandName,
		SupportEmail:  m.Config.SupportEmail,
	}
	var buf bytes.Buffer
	if err := emailTemplates.ExecuteTemplate(&buf, "welcome.html", data); err != nil {
		return fmt.Errorf("failed to render welcome.html: %w", err)
	}

	subject := fmt.Sprintf("Welcome to %s Feature Flag Platform", m.Config.BrandName)
	return m.sendHTMLEmail(toEmail, subject, buf.String())
}

func (m *SMTPMailer) SendChangeRequestNotification(toEmail, recipientName, requesterName, flagKey, environment, actionType, reviewURL string) error {
	data := ChangeRequestData{
		ToEmail:       toEmail,
		RecipientName: recipientName,
		RequesterName: requesterName,
		FlagKey:       flagKey,
		Environment:   environment,
		ActionType:    actionType,
		ReviewURL:     reviewURL,
		BrandName:     m.Config.BrandName,
		SupportEmail:  m.Config.SupportEmail,
	}
	var buf bytes.Buffer
	if err := emailTemplates.ExecuteTemplate(&buf, "change_request.html", data); err != nil {
		return fmt.Errorf("failed to render change_request.html: %w", err)
	}

	subject := fmt.Sprintf("Action Required: Review Flag Change for %s (%s)", flagKey, environment)
	return m.sendHTMLEmail(toEmail, subject, buf.String())
}

func (m *SMTPMailer) sendHTMLEmail(toEmail, subject, htmlBody string) error {
	addr := fmt.Sprintf("%s:%d", m.Config.Host, m.Config.Port)
	msg := []byte(fmt.Sprintf(
		"From: %s\r\n"+
			"To: %s\r\n"+
			"Subject: %s\r\n"+
			"MIME-Version: 1.0\r\n"+
			"Content-Type: text/html; charset=UTF-8\r\n\r\n"+
			"%s\r\n",
		m.Config.FromEmail, toEmail, subject, htmlBody,
	))

	var auth smtp.Auth
	if m.Config.Username != "" {
		auth = smtp.PlainAuth("", m.Config.Username, m.Config.Password, m.Config.Host)
	}

	// For port 465 (SMTPS) or explicit TLS, use TLS dialer
	if m.Config.Port == 465 || m.Config.UseTLS {
		insecure := m.Config.Host == "127.0.0.1" || m.Config.Host == "localhost"
		tlsConfig := &tls.Config{
			ServerName:         m.Config.Host,
			MinVersion:         tls.VersionTLS12,
			InsecureSkipVerify: insecure, // #nosec G402 -- test certificate support on local address
		}
		conn, err := tls.Dial("tcp", addr, tlsConfig)
		if err != nil {
			return fmt.Errorf("tls dial error: %w", err)
		}
		defer conn.Close()

		client, err := smtp.NewClient(conn, m.Config.Host)
		if err != nil {
			return fmt.Errorf("smtp client error: %w", err)
		}
		defer client.Quit()

		if auth != nil {
			if ok, _ := client.Extension("AUTH"); ok {
				if err = client.Auth(auth); err != nil {
					return fmt.Errorf("smtp auth error: %w", err)
				}
			}
		}

		if err = client.Mail(m.Config.FromEmail); err != nil {
			return err
		}
		if err = client.Rcpt(toEmail); err != nil {
			return err
		}
		w, err := client.Data()
		if err != nil {
			return err
		}
		_, err = w.Write(msg)
		if err != nil {
			return err
		}
		return w.Close()
	}

	return smtp.SendMail(addr, auth, m.Config.FromEmail, []string{toEmail}, msg)
}

// NewMailerFromEnv initializes a Mailer configured via environment variables.
// If SMTP is not configured (SMTP_HOST is empty), email delivery is disabled by default.
func NewMailerFromEnv() Mailer {
	cfg := LoadConfigFromEnv()
	if cfg.Host == "" {
		// Opt-in developer terminal logging if explicitly set
		if os.Getenv("ENABLE_CONSOLE_MAILER") == "true" {
			return NewConsoleMailer(cfg)
		}
		return NewDisabledMailer(cfg)
	}
	return NewSMTPMailer(cfg)
}
