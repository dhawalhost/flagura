package config

import (
	"log/slog"
	"os"
	"testing"

	"github.com/dhawalhost/flagura/pkg/domain"
)

func TestLoad(t *testing.T) {
	tests := []struct {
		name         string
		envSetup     map[string]string
		expectedPort string
		expectedLog  slog.Level
		expectedRPS  float64
		expectedEnv  domain.Environment
	}{
		{
			name: "Default configuration values",
			envSetup: map[string]string{
				"PORT":                  "",
				"FLAGURA_LOG_LEVEL":     "",
				"FLAGURA_RATE_LIMIT_RPS": "",
				"FLAGURA_ENV":           "",
			},
			expectedPort: "3000",
			expectedLog:  slog.LevelInfo,
			expectedRPS:  100.0,
			expectedEnv:  domain.EnvProduction,
		},
		{
			name: "Custom environment overrides",
			envSetup: map[string]string{
				"PORT":                  "8080",
				"FLAGURA_LOG_LEVEL":     "DEBUG",
				"FLAGURA_RATE_LIMIT_RPS": "250.5",
				"FLAGURA_ENV":           "staging",
			},
			expectedPort: "8080",
			expectedLog:  slog.LevelDebug,
			expectedRPS:  250.5,
			expectedEnv:  domain.EnvStaging,
		},
		{
			name: "Warn log level and development environment",
			envSetup: map[string]string{
				"PORT":                  "4000",
				"FLAGURA_LOG_LEVEL":     "WARN",
				"FLAGURA_RATE_LIMIT_RPS": "",
				"FLAGURA_ENV":           "development",
			},
			expectedPort: "4000",
			expectedLog:  slog.LevelWarn,
			expectedRPS:  100.0,
			expectedEnv:  domain.EnvDevelopment,
		},
		{
			name: "Error log level, custom burst, and text format",
			envSetup: map[string]string{
				"PORT":                    "5000",
				"FLAGURA_LOG_LEVEL":       "ERROR",
				"FLAGURA_RATE_LIMIT_BURST": "500",
				"FLAGURA_LOG_FORMAT":      "text",
				"HOST":                    "127.0.0.1",
			},
			expectedPort: "5000",
			expectedLog:  slog.LevelError,
			expectedRPS:  100.0,
			expectedEnv:  domain.EnvProduction,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for k, v := range tt.envSetup {
				if v == "" {
					os.Unsetenv(k)
				} else {
					os.Setenv(k, v)
				}
			}
			defer func() {
				for k := range tt.envSetup {
					os.Unsetenv(k)
				}
			}()

			cfg, err := Load()
			if err != nil {
				t.Fatalf("unexpected error loading config: %v", err)
			}

			if cfg.ServerPort != tt.expectedPort {
				t.Errorf("expected port %s, got %s", tt.expectedPort, cfg.ServerPort)
			}
			if cfg.LogLevel != tt.expectedLog {
				t.Errorf("expected log level %v, got %v", tt.expectedLog, cfg.LogLevel)
			}
			if cfg.RateLimitRPS != tt.expectedRPS {
				t.Errorf("expected rate limit RPS %f, got %f", tt.expectedRPS, cfg.RateLimitRPS)
			}
			if cfg.Environment != tt.expectedEnv {
				t.Errorf("expected environment %v, got %v", tt.expectedEnv, cfg.Environment)
			}
		})
	}
}

func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name        string
		cfg         Config
		expectError bool
	}{
		{
			name: "Valid configuration",
			cfg: Config{
				ServerPort:     "3000",
				RateLimitRPS:   100,
				RateLimitBurst: 200,
			},
			expectError: false,
		},
		{
			name: "Missing server port",
			cfg: Config{
				ServerPort:     "",
				RateLimitRPS:   100,
				RateLimitBurst: 200,
			},
			expectError: true,
		},
		{
			name: "Zero or negative rate limit RPS",
			cfg: Config{
				ServerPort:     "3000",
				RateLimitRPS:   0,
				RateLimitBurst: 200,
			},
			expectError: true,
		},
		{
			name: "Zero or negative rate limit Burst",
			cfg: Config{
				ServerPort:     "3000",
				RateLimitRPS:   100,
				RateLimitBurst: 0,
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if (err != nil) != tt.expectError {
				t.Errorf("Validate() error = %v, expectError = %v", err, tt.expectError)
			}
		})
	}
}
