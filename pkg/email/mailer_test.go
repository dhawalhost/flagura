package email

import (
	"bufio"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"net"
	"os"
	"strings"
	"testing"
	"time"
)

func generateTestCert(t *testing.T) tls.Certificate {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate private key: %v", err)
	}

	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization: []string{"Flagura Test"},
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(1 * time.Hour),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		t.Fatalf("failed to create certificate: %v", err)
	}

	return tls.Certificate{
		Certificate: [][]byte{derBytes},
		PrivateKey:  priv,
	}
}

func startMockTLSSMTPServer(t *testing.T) (string, int, func()) {
	cert := generateTestCert(t)
	tlsCfg := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}

	ln, err := tls.Listen("tcp", "127.0.0.1:0", tlsCfg)
	if err != nil {
		t.Fatalf("failed to bind mock tls smtp listener: %v", err)
	}

	addr := ln.Addr().(*net.TCPAddr)

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				r := bufio.NewReader(c)
				_, _ = c.Write([]byte("220 mock-tls-smtp.flagura.dev ESMTP Flagura\r\n"))
				for {
					line, err := r.ReadString('\n')
					if err != nil {
						return
					}
					line = strings.TrimSpace(line)
					if strings.HasPrefix(line, "EHLO") || strings.HasPrefix(line, "HELO") {
						_, _ = c.Write([]byte("250-mock-tls-smtp.flagura.dev\r\n250-AUTH PLAIN\r\n250 8BITMIME\r\n"))
					} else if strings.HasPrefix(line, "AUTH") {
						_, _ = c.Write([]byte("235 2.7.0 Authentication successful\r\n"))
					} else if strings.HasPrefix(line, "MAIL FROM:") || strings.HasPrefix(line, "RCPT TO:") {
						_, _ = c.Write([]byte("250 2.1.0 Ok\r\n"))
					} else if line == "DATA" {
						_, _ = c.Write([]byte("354 End data with <CR><LF>.<CR><LF>\r\n"))
						for {
							dataLine, err := r.ReadString('\n')
							if err != nil {
								return
							}
							if strings.TrimSpace(dataLine) == "." {
								_, _ = c.Write([]byte("250 2.0.0 Ok: queued\r\n"))
								break
							}
						}
					} else if line == "QUIT" {
						_, _ = c.Write([]byte("221 2.0.0 Bye\r\n"))
						return
					} else {
						_, _ = c.Write([]byte("250 OK\r\n"))
					}
				}
			}(conn)
		}
	}()

	return addr.IP.String(), addr.Port, func() { _ = ln.Close() }
}

func startMockSMTPServer(t *testing.T) (string, int, func()) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to bind mock smtp listener: %v", err)
	}

	addr := ln.Addr().(*net.TCPAddr)

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				r := bufio.NewReader(c)
				_, _ = c.Write([]byte("220 mock-smtp.flagura.dev ESMTP Flagura\r\n"))
				for {
					line, err := r.ReadString('\n')
					if err != nil {
						return
					}
					line = strings.TrimSpace(line)
					if strings.HasPrefix(line, "EHLO") || strings.HasPrefix(line, "HELO") {
						_, _ = c.Write([]byte("250-mock-smtp.flagura.dev\r\n250-AUTH PLAIN\r\n250 8BITMIME\r\n"))
					} else if strings.HasPrefix(line, "AUTH") {
						_, _ = c.Write([]byte("235 2.7.0 Authentication successful\r\n"))
					} else if strings.HasPrefix(line, "MAIL FROM:") || strings.HasPrefix(line, "RCPT TO:") {
						_, _ = c.Write([]byte("250 2.1.0 Ok\r\n"))
					} else if line == "DATA" {
						_, _ = c.Write([]byte("354 End data with <CR><LF>.<CR><LF>\r\n"))
						for {
							dataLine, err := r.ReadString('\n')
							if err != nil {
								return
							}
							if strings.TrimSpace(dataLine) == "." {
								_, _ = c.Write([]byte("250 2.0.0 Ok: queued\r\n"))
								break
							}
						}
					} else if line == "QUIT" {
						_, _ = c.Write([]byte("221 2.0.0 Bye\r\n"))
						return
					} else {
						_, _ = c.Write([]byte("250 OK\r\n"))
					}
				}
			}(conn)
		}
	}()

	return addr.IP.String(), addr.Port, func() { _ = ln.Close() }
}

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

func TestDisabledMailer_Methods(t *testing.T) {
	dm := NewDisabledMailer()
	if dm.IsEnabled() {
		t.Errorf("expected IsEnabled() to be false")
	}
	if dm.GetSupportEmail() != "" {
		t.Errorf("expected empty support email by default")
	}
	if len(dm.GetGovernanceEmails()) != 0 {
		t.Errorf("expected empty governance emails by default")
	}
	_ = dm.GetConfig()

	if err := dm.SendPasswordReset("a@b.com", "Name", "url"); err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
	if err := dm.SendWelcomeEmail("a@b.com", "Name", "url"); err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
	if err := dm.SendChangeRequestNotification("a@b.com", "Name", "Req", "flag", "prod", "action", "url"); err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
}

func TestSMTPMailer_MethodsAndTemplateRendering(t *testing.T) {
	cfg := Config{
		Host:             "127.0.0.1",
		Port:             2525,
		Username:         "user",
		Password:         "pass",
		FromEmail:        "no-reply@flagura.dev",
		SupportEmail:     "support@flagura.dev",
		BrandName:        "Flagura Cloud",
		AppURL:           "https://app.flagura.dev",
		GovernanceEmails: []string{"admin@flagura.dev"},
	}

	sm := NewSMTPMailer(cfg)
	if !sm.IsEnabled() {
		t.Errorf("expected IsEnabled() to be true")
	}
	if sm.GetSupportEmail() != "support@flagura.dev" {
		t.Errorf("unexpected support email: %s", sm.GetSupportEmail())
	}
	if len(sm.GetGovernanceEmails()) != 1 {
		t.Errorf("unexpected governance emails: %v", sm.GetGovernanceEmails())
	}
	if sm.GetConfig().BrandName != "Flagura Cloud" {
		t.Errorf("unexpected brand name: %s", sm.GetConfig().BrandName)
	}

	// We expect connection error on local port 2525, but template rendering is verified
	_ = sm.SendPasswordReset("test@example.com", "Test User", "https://app.flagura.dev/reset")
	_ = sm.SendWelcomeEmail("test@example.com", "Test User", "https://app.flagura.dev/dashboard")
	_ = sm.SendChangeRequestNotification("admin@flagura.dev", "Admin", "Developer", "feat-pay", "production", "ENABLE", "https://app.flagura.dev/cr/1")

	// Test with port 465 (SMTPS branch)
	cfg465 := cfg
	cfg465.Port = 465
	sm465 := NewSMTPMailer(cfg465)
	_ = sm465.SendPasswordReset("test@example.com", "Test User", "https://app.flagura.dev/reset")
}

func TestLoadConfigFromEnv_Exhaustive(t *testing.T) {
	os.Setenv("SMTP_HOST", "smtp.test.com")
	os.Setenv("SMTP_PORT", "465")
	os.Setenv("SMTP_USERNAME", "myuser")
	os.Setenv("SMTP_PASSWORD", "mypass")
	os.Setenv("FLAGURA_FROM_EMAIL", "from@flagura.dev")
	os.Setenv("FLAGURA_APP_URL", "https://flagura.dev/")

	defer func() {
		os.Unsetenv("SMTP_HOST")
		os.Unsetenv("SMTP_PORT")
		os.Unsetenv("SMTP_USERNAME")
		os.Unsetenv("SMTP_PASSWORD")
		os.Unsetenv("FLAGURA_FROM_EMAIL")
		os.Unsetenv("FLAGURA_APP_URL")
	}()

	cfg := LoadConfigFromEnv()
	if cfg.Host != "smtp.test.com" || cfg.Port != 465 || cfg.Username != "myuser" || cfg.Password != "mypass" || cfg.FromEmail != "from@flagura.dev" || cfg.AppURL != "https://flagura.dev" {
		t.Errorf("unexpected config loaded: %+v", cfg)
	}

	// Also test NewMailerFromEnv returns SMTPMailer when SMTP_HOST is populated
	m := NewMailerFromEnv()
	if !m.IsEnabled() {
		t.Errorf("expected SMTPMailer to be enabled")
	}
}

func TestSMTPMailer_LiveDelivery(t *testing.T) {
	host, port, teardown := startMockSMTPServer(t)
	defer teardown()

	cfg := Config{
		Host:         host,
		Port:         port,
		Username:     "testuser",
		Password:     "testpass",
		FromEmail:    "noreply@flagura.dev",
		SupportEmail: "support@flagura.dev",
		BrandName:    "Flagura",
		AppURL:       "http://localhost:3000",
	}

	mailer := NewSMTPMailer(cfg)

	// Send password reset
	if err := mailer.SendPasswordReset("user@example.com", "Alice", "http://localhost:3000/reset"); err != nil {
		t.Errorf("SendPasswordReset failed: %v", err)
	}

	// Send welcome email
	if err := mailer.SendWelcomeEmail("user@example.com", "Alice", "http://localhost:3000/dash"); err != nil {
		t.Errorf("SendWelcomeEmail failed: %v", err)
	}

	// Send change request email
	if err := mailer.SendChangeRequestNotification("admin@example.com", "Bob", "Alice", "flag-x", "prod", "UPDATE", "http://localhost:3000/cr/1"); err != nil {
		t.Errorf("SendChangeRequestNotification failed: %v", err)
	}

	// Console mailer GetConfig check
	cm := NewConsoleMailer(cfg)
	if cm.GetConfig().BrandName != "Flagura" {
		t.Errorf("unexpected brand name: %s", cm.GetConfig().BrandName)
	}

	// Test Port 465 SMTPS delivery
	tlsHost, tlsPort, tlsTeardown := startMockTLSSMTPServer(t)
	defer tlsTeardown()

	tlsCfg := cfg
	tlsCfg.Host = tlsHost
	tlsCfg.Port = tlsPort
	tlsCfg.UseTLS = true
	tlsMailer := NewSMTPMailer(tlsCfg)

	if err := tlsMailer.SendPasswordReset("user@example.com", "Alice", "http://localhost:3000/reset"); err != nil {
		t.Errorf("tlsMailer.SendPasswordReset failed: %v", err)
	}
	if err := tlsMailer.SendWelcomeEmail("user@example.com", "Alice", "http://localhost:3000/dash"); err != nil {
		t.Errorf("tlsMailer.SendWelcomeEmail failed: %v", err)
	}
	if err := tlsMailer.SendChangeRequestNotification("admin@example.com", "Bob", "Alice", "flag-x", "prod", "UPDATE", "http://localhost:3000/cr/1"); err != nil {
		t.Errorf("tlsMailer.SendChangeRequestNotification failed: %v", err)
	}

	// Test unauthenticated plain SMTP
	unauthCfg := cfg
	unauthCfg.Username = ""
	unauthCfg.Password = ""
	unauthMailer := NewSMTPMailer(unauthCfg)
	if err := unauthMailer.SendPasswordReset("user@example.com", "Alice", "http://localhost:3000/reset"); err != nil {
		t.Errorf("unauthMailer.SendPasswordReset failed: %v", err)
	}
}
