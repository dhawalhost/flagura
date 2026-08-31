package config

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/dhawalhost/flagura/pkg/domain"
)

// Config represents the complete runtime configuration for Flagura.
type Config struct {
	// Server Networking & Endpoints
	ServerPort  string             `json:"server_port"`
	Host        string             `json:"host"`
	BaseURL     string             `json:"base_url"`
	Environment domain.Environment `json:"environment"`

	// Storage & Persistence
	DatabaseURL string `json:"database_url"`

	// Structured Logging
	LogLevel  slog.Level `json:"log_level"`
	LogFormat string     `json:"log_format"` // "json" or "text"

	// HTTP Server Timeouts & Limits
	ReadHeaderTimeout time.Duration `json:"read_header_timeout"`
	ReadTimeout       time.Duration `json:"read_timeout"`
	WriteTimeout      time.Duration `json:"write_timeout"`
	IdleTimeout       time.Duration `json:"idle_timeout"`
	MaxHeaderBytes    int           `json:"max_header_bytes"`

	// Rate Limiting
	RateLimitRPS   float64 `json:"rate_limit_rps"`
	RateLimitBurst int     `json:"rate_limit_burst"`

	// Security & CORS
	CORSAllowedOrigins []string `json:"cors_allowed_origins"`
	SessionSecret      string   `json:"-"`
}

// Load populates Config from environment variables with production defaults.
func Load() (*Config, error) {
	cfg := &Config{
		ServerPort:         getEnv("PORT", "3000"),
		Host:               getEnv("HOST", "0.0.0.0"),
		BaseURL:            getEnv("FLAGURA_BASE_URL", "http://localhost:3000"),
		Environment:        domain.Environment(strings.ToLower(getEnv("FLAGURA_ENV", string(domain.EnvProduction)))),
		DatabaseURL:        os.Getenv("DATABASE_URL"),
		LogFormat:          strings.ToLower(getEnv("FLAGURA_LOG_FORMAT", "json")),
		ReadHeaderTimeout:  5 * time.Second,
		ReadTimeout:        15 * time.Second,
		WriteTimeout:       15 * time.Second,
		IdleTimeout:        60 * time.Second,
		MaxHeaderBytes:     1 << 20, // 1 MB
		RateLimitRPS:       100.0,
		RateLimitBurst:     200,
		CORSAllowedOrigins: []string{"*"},
		SessionSecret:      getEnv("SESSION_SECRET", "flagura-enterprise-secret-key"),
	}

	// Parse LogLevel
	levelStr := strings.ToUpper(getEnv("FLAGURA_LOG_LEVEL", "INFO"))
	switch levelStr {
	case "DEBUG":
		cfg.LogLevel = slog.LevelDebug
	case "WARN", "WARNING":
		cfg.LogLevel = slog.LevelWarn
	case "ERROR":
		cfg.LogLevel = slog.LevelError
	default:
		cfg.LogLevel = slog.LevelInfo
	}

	// Parse RateLimitRPS if configured
	if rpsStr := os.Getenv("FLAGURA_RATE_LIMIT_RPS"); rpsStr != "" {
		if rps, err := strconv.ParseFloat(rpsStr, 64); err == nil && rps > 0 {
			cfg.RateLimitRPS = rps
		}
	}

	// Parse RateLimitBurst if configured
	if burstStr := os.Getenv("FLAGURA_RATE_LIMIT_BURST"); burstStr != "" {
		if burst, err := strconv.Atoi(burstStr); err == nil && burst > 0 {
			cfg.RateLimitBurst = burst
		}
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	return cfg, nil
}

// Validate ensures runtime bounds and invariant constraints are met.
func (c *Config) Validate() error {
	if c.ServerPort == "" {
		return fmt.Errorf("server_port cannot be empty")
	}
	if c.RateLimitRPS <= 0 {
		return fmt.Errorf("rate_limit_rps must be greater than 0")
	}
	if c.RateLimitBurst <= 0 {
		return fmt.Errorf("rate_limit_burst must be greater than 0")
	}
	return nil
}

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}
