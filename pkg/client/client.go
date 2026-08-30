// Package client provides the official Go SDK for Flagura.
//
// It supports both instant remote HTTP evaluation and local in-memory
// synchronized evaluation for sub-microsecond deterministic flag resolution.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/dhawalhost/flagura/pkg/domain"
	"github.com/dhawalhost/flagura/pkg/engine"
)

// Environment aliases for convenience
const (
	EnvProduction  = domain.EnvProduction
	EnvStaging     = domain.EnvStaging
	EnvDevelopment = domain.EnvDevelopment
)

// Context represents the user or request context for flag evaluation.
type Context struct {
	UserID      string                 `json:"user_id"`
	Email       string                 `json:"email,omitempty"`
	Country     string                 `json:"country,omitempty"`
	Role        string                 `json:"role,omitempty"`
	Tier        string                 `json:"tier,omitempty"`
	Environment domain.Environment     `json:"environment,omitempty"`
	Custom      map[string]interface{} `json:"custom,omitempty"`
}

// EvaluationResult represents the computed result of evaluating a feature flag.
type EvaluationResult struct {
	FlagKey             string      `json:"flag_key"`
	Enabled             bool        `json:"enabled"`
	Variant             string      `json:"variant"`
	Value               interface{} `json:"value"`
	Reason              string      `json:"reason"`
	Bucket              float64     `json:"bucket"`
	EvaluationLatencyNs int64       `json:"latency_ns"`
	EvaluationLatencyUs float64     `json:"latency_us"`
}

// Config holds client configuration settings.
type Config struct {
	// Endpoint is the base URL of the Flagura server (e.g. "http://localhost:3000" or "https://flagura.yourdomain.com")
	Endpoint string

	// APIKey is the optional authorization bearer key
	APIKey string

	// DefaultEnvironment to use if not specified in individual evaluation contexts (default: "production")
	DefaultEnvironment domain.Environment

	// HTTPClient to use for network requests (default: http.DefaultClient with 5s timeout)
	HTTPClient *http.Client

	// LocalEvaluation enables background flag caching and in-process local evaluation
	LocalEvaluation bool

	// SyncInterval is the refresh rate for local evaluation cache (default: 30s)
	SyncInterval time.Duration

	// SnapshotFile is an optional path to persist and load local flag cache snapshots
	SnapshotFile string

	// CircuitBreakerThreshold is the consecutive failure limit before opening the circuit (default: 5)
	CircuitBreakerThreshold int

	// CircuitBreakerCooldown is the cooldown period before testing recovery (default: 10s)
	CircuitBreakerCooldown time.Duration

	// DisableCircuitBreaker disables client-side circuit breaking
	DisableCircuitBreaker bool

	// DisableStreaming disables real-time SSE streaming updates (falls back to polling only)
	DisableStreaming bool

	// TelemetryFlushInterval is the interval to push evaluation counts to the server (default: 60s)
	TelemetryFlushInterval time.Duration

	// DisableTelemetry disables client-side evaluation telemetry push
	DisableTelemetry bool
}

// Option configures a Flagura Client.
type Option func(*Config)

// WithAPIKey sets the API key for authenticated requests.
func WithAPIKey(key string) Option {
	return func(c *Config) {
		c.APIKey = key
	}
}

// WithEnvironment sets the default environment.
func WithEnvironment(env domain.Environment) Option {
	return func(c *Config) {
		c.DefaultEnvironment = env
	}
}

// WithHTTPClient sets a custom HTTP client.
func WithHTTPClient(client *http.Client) Option {
	return func(c *Config) {
		c.HTTPClient = client
	}
}

// WithLocalEvaluation enables in-memory evaluation cache for sub-microsecond local resolution.
func WithLocalEvaluation(syncInterval time.Duration) Option {
	return func(c *Config) {
		c.LocalEvaluation = true
		c.SyncInterval = syncInterval
	}
}

// WithSnapshotFile enables offline disk caching and bootstrap from a local snapshot file.
func WithSnapshotFile(path string) Option {
	return func(c *Config) {
		c.SnapshotFile = path
	}
}

// WithCircuitBreaker configures the 3-state failure circuit breaker on the HTTP client.
func WithCircuitBreaker(threshold int, cooldown time.Duration) Option {
	return func(c *Config) {
		c.CircuitBreakerThreshold = threshold
		c.CircuitBreakerCooldown = cooldown
	}
}

// WithDisabledCircuitBreaker disables the client-side circuit breaker.
func WithDisabledCircuitBreaker() Option {
	return func(c *Config) {
		c.DisableCircuitBreaker = true
	}
}

// WithStreaming configures real-time SSE streaming for instant flag updates.
func WithStreaming(enabled bool) Option {
	return func(c *Config) {
		c.DisableStreaming = !enabled
	}
}

// WithTelemetryFlushInterval sets the cadence for pushing local evaluation counts to Flagura.
func WithTelemetryFlushInterval(d time.Duration) Option {
	return func(c *Config) {
		c.TelemetryFlushInterval = d
	}
}

// WithDisabledTelemetry disables local evaluation count pushback.
func WithDisabledTelemetry() Option {
	return func(c *Config) {
		c.DisableTelemetry = true
	}
}

// Client is the Flagura evaluation SDK client.
type Client struct {
	config Config

	// Local cache state
	mu        sync.RWMutex
	flags     map[string]domain.FeatureFlag
	listeners []func(flags map[string]domain.FeatureFlag, changedKeys []string)
	stopCh    chan struct{}
	closeOnce sync.Once
	cb        *CircuitBreaker
	telemetry *TelemetryBuffer
}

// RegisterUpdateListener registers a callback invoked when feature flags are synchronized or updated.
func (c *Client) RegisterUpdateListener(fn func(flags map[string]domain.FeatureFlag, changedKeys []string)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.listeners = append(c.listeners, fn)
}

// New creates a new Flagura client with functional options.
func New(endpoint string, opts ...Option) *Client {
	cfg := Config{
		Endpoint:           strings.TrimRight(endpoint, "/"),
		DefaultEnvironment: domain.EnvProduction,
		HTTPClient: &http.Client{
			Timeout: 5 * time.Second,
		},
		SyncInterval:            30 * time.Second,
		CircuitBreakerThreshold: 5,
		CircuitBreakerCooldown:  10 * time.Second,
		TelemetryFlushInterval: 60 * time.Second,
	}

	for _, opt := range opts {
		opt(&cfg)
	}

	var cb *CircuitBreaker
	if !cfg.DisableCircuitBreaker {
		cb = NewCircuitBreaker(cfg.CircuitBreakerThreshold, cfg.CircuitBreakerCooldown)
	}

	c := &Client{
		config: cfg,
		flags:  make(map[string]domain.FeatureFlag),
		stopCh: make(chan struct{}),
		cb:     cb,
	}

	if !cfg.DisableTelemetry {
		c.telemetry = NewTelemetryBuffer(cfg.Endpoint, cfg.APIKey, cfg.HTTPClient)
		go c.telemetry.StartBackgroundLoop(c.stopCh, cfg.TelemetryFlushInterval)
	}

	if c.config.SnapshotFile != "" {
		_ = c.loadSnapshot()
	}

	if c.config.LocalEvaluation {
		// Attempt initial sync. If server unreachable, fallback to snapshot
		if err := c.syncFlags(context.Background()); err != nil && c.config.SnapshotFile != "" {
			_ = c.loadSnapshot()
		}
		if !c.config.DisableStreaming {
			go c.startSSEStream()
		}
		go c.startBackgroundSync()
	}

	return c
}

// CircuitBreakerState returns the current state of the client's circuit breaker.
func (c *Client) CircuitBreakerState() CircuitState {
	if c.cb == nil {
		return StateClosed
	}
	return c.cb.State()
}

// IsEnabled evaluates a single boolean flag and returns true if enabled.
// If an error occurs, it safely returns false.
func (c *Client) IsEnabled(ctx context.Context, flagKey string, evalCtx Context) bool {
	res, err := c.Evaluate(ctx, flagKey, evalCtx)
	if err != nil {
		return false
	}
	return res.Enabled
}

// GetVariant evaluates a multivariate flag and returns the assigned variant string.
// If evaluation fails or flag is not found, fallback is returned.
func (c *Client) GetVariant(ctx context.Context, flagKey string, evalCtx Context, fallback string) string {
	res, err := c.Evaluate(ctx, flagKey, evalCtx)
	if err != nil || res.Variant == "" || res.Variant == "off" {
		return fallback
	}
	return res.Variant
}

// Evaluate evaluates a single feature flag.
func (c *Client) Evaluate(ctx context.Context, flagKey string, evalCtx Context) (EvaluationResult, error) {
	if c.config.LocalEvaluation {
		return c.evaluateLocal(flagKey, evalCtx)
	}
	return c.evaluateRemote(ctx, flagKey, evalCtx)
}

// EvaluateBatch evaluates multiple feature flags in a single call.
func (c *Client) EvaluateBatch(ctx context.Context, flagKeys []string, evalCtx Context) (map[string]EvaluationResult, error) {
	if c.config.LocalEvaluation {
		results := make(map[string]EvaluationResult, len(flagKeys))
		for _, key := range flagKeys {
			res, _ := c.evaluateLocal(key, evalCtx)
			results[key] = res
		}
		return results, nil
	}
	return c.evaluateBatchRemote(ctx, flagKeys, evalCtx)
}

// evaluateLocal evaluates a flag using the synchronized in-memory cache and FNV-1a sticky hashing.
func (c *Client) evaluateLocal(flagKey string, evalCtx Context) (EvaluationResult, error) {
	c.mu.RLock()
	flag, exists := c.flags[flagKey]
	c.mu.RUnlock()

	domainCtx := c.toDomainContext(evalCtx)

	if !exists {
		return EvaluationResult{
			FlagKey: flagKey,
			Enabled: false,
			Variant: "off",
			Value:   false,
			Reason:  string(domain.ReasonFlagNotFound),
		}, fmt.Errorf("flag %q not found in local cache", flagKey)
	}

	domRes := engine.EvaluateFlag(flag, domainCtx)
	var bucket float64
	if domRes.BucketVal != nil {
		bucket = *domRes.BucketVal
	}

	if c.telemetry != nil {
		c.telemetry.Record(flagKey, domRes.Variant)
	}

	return EvaluationResult{
		FlagKey:             domRes.FlagKey,
		Enabled:             domRes.Enabled,
		Variant:             domRes.Variant,
		Value:               domRes.Value,
		Reason:              string(domRes.Reason),
		Bucket:              bucket,
		EvaluationLatencyNs: domRes.EvaluationLatencyNs,
		EvaluationLatencyUs: domRes.EvaluationLatencyUs,
	}, nil
}

// evaluateRemote evaluates a flag via HTTP request to the Flagura server.
func (c *Client) evaluateRemote(ctx context.Context, flagKey string, evalCtx Context) (EvaluationResult, error) {
	results, err := c.evaluateBatchRemote(ctx, []string{flagKey}, evalCtx)
	if err != nil {
		if res, ok := results[flagKey]; ok {
			if c.telemetry != nil {
				c.telemetry.Record(flagKey, res.Variant)
			}
			return res, err
		}
		return EvaluationResult{
			FlagKey: flagKey,
			Enabled: false,
			Variant: "off",
			Value:   false,
			Reason:  "client_request_error",
		}, err
	}

	res, ok := results[flagKey]
	if !ok {
		return EvaluationResult{
			FlagKey: flagKey,
			Enabled: false,
			Variant: "off",
			Value:   false,
			Reason:  string(domain.ReasonFlagNotFound),
		}, fmt.Errorf("flag %q missing from response", flagKey)
	}

	if c.telemetry != nil {
		c.telemetry.Record(flagKey, res.Variant)
	}

	return res, nil
}

func (c *Client) evaluateBatchRemote(ctx context.Context, flagKeys []string, evalCtx Context) (map[string]EvaluationResult, error) {
	if c.cb != nil && !c.cb.Allow() {
		results := make(map[string]EvaluationResult, len(flagKeys))
		for _, k := range flagKeys {
			results[k] = EvaluationResult{
				FlagKey: k,
				Enabled: false,
				Variant: "off",
				Value:   false,
				Reason:  "circuit_breaker_open",
			}
		}
		return results, fmt.Errorf("circuit breaker is OPEN: fast-failing remote evaluation")
	}

	if evalCtx.Environment == "" {
		evalCtx.Environment = c.config.DefaultEnvironment
	}

	payload := map[string]interface{}{
		"flags":   flagKeys,
		"context": evalCtx,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/api/v1/evaluate", c.config.Endpoint)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if c.config.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.config.APIKey)
	}

	resp, err := c.config.HTTPClient.Do(req)
	if err != nil {
		if c.cb != nil {
			c.cb.OnFailure()
		}
		return nil, fmt.Errorf("evaluation request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		if c.cb != nil {
			c.cb.OnFailure()
		}
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("evaluation returned status %d: %s", resp.StatusCode, string(body))
	}

	if c.cb != nil {
		c.cb.OnSuccess()
	}

	var evalResp struct {
		Results map[string]EvaluationResult `json:"results"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&evalResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return evalResp.Results, nil
}

func (c *Client) syncFlags(ctx context.Context) error {
	url := fmt.Sprintf("%s/api/v1/flags", c.config.Endpoint)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}

	if c.config.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.config.APIKey)
	}

	resp, err := c.config.HTTPClient.Do(req)
	if err != nil {
		if c.cb != nil {
			c.cb.OnFailure()
		}
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		if c.cb != nil {
			c.cb.OnFailure()
		}
		return fmt.Errorf("fetch flags returned status %d", resp.StatusCode)
	}

	if c.cb != nil {
		c.cb.OnSuccess()
	}

	var flagsResp struct {
		Flags []domain.FeatureFlag `json:"flags"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&flagsResp); err != nil {
		return err
	}

	c.updateFlags(flagsResp.Flags)
	return nil
}

func (c *Client) loadSnapshot() error {
	if c.config.SnapshotFile == "" {
		return nil
	}

	data, err := os.ReadFile(c.config.SnapshotFile)
	if err != nil {
		return err
	}

	var flags []domain.FeatureFlag
	if err := json.Unmarshal(data, &flags); err != nil {
		return err
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	newMap := make(map[string]domain.FeatureFlag, len(flags))
	for _, f := range flags {
		newMap[f.Key] = f
		newMap[f.ID] = f
	}
	c.flags = newMap
	return nil
}

func (c *Client) saveSnapshot(flags []domain.FeatureFlag) error {
	if c.config.SnapshotFile == "" {
		return nil
	}

	data, err := json.MarshalIndent(flags, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(c.config.SnapshotFile, data, 0600)
}

func (c *Client) startBackgroundSync() {
	ticker := time.NewTicker(c.config.SyncInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			_ = c.syncFlags(ctx)
			cancel()
		case <-c.stopCh:
			return
		}
	}
}

func (c *Client) toDomainContext(ctx Context) domain.EvaluationContext {
	env := ctx.Environment
	if env == "" {
		env = c.config.DefaultEnvironment
	}
	return domain.EvaluationContext{
		UserID:      ctx.UserID,
		Email:       ctx.Email,
		Country:     ctx.Country,
		Role:        ctx.Role,
		Tier:        ctx.Tier,
		Environment: env,
		Attributes:  ctx.Custom,
	}
}

// Track sends an experiment event/conversion observation to the Flagura control plane.
func (c *Client) Track(ctx context.Context, flagKey, variant, metricName string, value float64, userID string) error {
	payload := map[string]interface{}{
		"event": domain.ExperimentEvent{
			FlagKey:     flagKey,
			Variant:     variant,
			MetricName:  metricName,
			EventType:   domain.EventTypeConversion,
			Value:       value,
			UserID:      userID,
			Environment: c.config.DefaultEnvironment,
			Timestamp:   time.Now().UTC(),
		},
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	url := fmt.Sprintf("%s/api/v1/events", strings.TrimRight(c.config.Endpoint, "/"))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.config.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.config.APIKey)
	}

	resp, err := c.config.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("track request failed with status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// Close gracefully stops any background synchronization goroutines.
func (c *Client) Close() {
	c.closeOnce.Do(func() {
		close(c.stopCh)
	})
}
