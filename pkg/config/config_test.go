package config

import (
	"log/slog"
	"os"
	"testing"
)

func TestConfigLoadAndValidation(t *testing.T) {
	// Test default values
	os.Unsetenv("PORT")
	os.Unsetenv("FLAGURA_LOG_LEVEL")
	os.Unsetenv("FLAGURA_RATE_LIMIT_RPS")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error loading config: %v", err)
	}

	if cfg.ServerPort != "3000" {
		t.Errorf("expected port 3000, got %s", cfg.ServerPort)
	}

	if cfg.LogLevel != slog.LevelInfo {
		t.Errorf("expected log level INFO, got %v", cfg.LogLevel)
	}

	if cfg.RateLimitRPS != 100.0 {
		t.Errorf("expected rate limit RPS 100.0, got %f", cfg.RateLimitRPS)
	}

	// Test overrides
	os.Setenv("PORT", "8080")
	os.Setenv("FLAGURA_LOG_LEVEL", "DEBUG")
	os.Setenv("FLAGURA_RATE_LIMIT_RPS", "250.5")

	cfg2, err := Load()
	if err != nil {
		t.Fatalf("unexpected error loading config with overrides: %v", err)
	}

	if cfg2.ServerPort != "8080" {
		t.Errorf("expected port 8080, got %s", cfg2.ServerPort)
	}

	if cfg2.LogLevel != slog.LevelDebug {
		t.Errorf("expected log level DEBUG, got %v", cfg2.LogLevel)
	}

	if cfg2.RateLimitRPS != 250.5 {
		t.Errorf("expected rate limit RPS 250.5, got %f", cfg2.RateLimitRPS)
	}

	// Test invalid bounds
	invalidCfg := &Config{
		ServerPort:   "",
		RateLimitRPS: 10,
	}
	if err := invalidCfg.Validate(); err == nil {
		t.Errorf("expected validation failure on empty port")
	}
}
