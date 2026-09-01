package flagura

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
)

// Logger interface for structured client logging.
type Logger interface {
	Debugf(format string, args ...interface{})
	Infof(format string, args ...interface{})
	Warnf(format string, args ...interface{})
	Errorf(format string, args ...interface{})
}

type nopLogger struct{}

func (n *nopLogger) Debugf(format string, args ...interface{}) {}
func (n *nopLogger) Infof(format string, args ...interface{})  {}
func (n *nopLogger) Warnf(format string, args ...interface{})  {}
func (n *nopLogger) Errorf(format string, args ...interface{}) {}

// Config holds client configuration settings.
type Config struct {
	Endpoint                string
	APIKey                  string
	ProjectID               string
	DefaultEnvironment      Environment
	HTTPClient              *http.Client
	Logger                  Logger
	LocalEvaluation         bool
	SyncInterval            time.Duration
	SnapshotFile            string
	CircuitBreakerThreshold int
	CircuitBreakerCooldown  time.Duration
	DisableCircuitBreaker   bool
}

// Option configures client settings.
type Option func(*Config)

func WithAPIKey(key string) Option {
	return func(c *Config) {
		c.APIKey = key
	}
}

func WithProjectID(projectID string) Option {
	return func(c *Config) {
		c.ProjectID = projectID
	}
}

func WithEnvironment(env Environment) Option {
	return func(c *Config) {
		c.DefaultEnvironment = env
	}
}

func WithLocalEvaluation(enabled bool) Option {
	return func(c *Config) {
		c.LocalEvaluation = enabled
	}
}

func WithSyncInterval(d time.Duration) Option {
	return func(c *Config) {
		c.SyncInterval = d
	}
}

func WithSnapshotFile(path string) Option {
	return func(c *Config) {
		c.SnapshotFile = path
	}
}

func WithCircuitBreaker(threshold int, cooldown time.Duration) Option {
	return func(c *Config) {
		c.CircuitBreakerThreshold = threshold
		c.CircuitBreakerCooldown = cooldown
	}
}

func WithDisableCircuitBreaker(disabled bool) Option {
	return func(c *Config) {
		c.DisableCircuitBreaker = disabled
	}
}

func WithLogger(logger Logger) Option {
	return func(c *Config) {
		c.Logger = logger
	}
}

func WithHTTPClient(httpClient *http.Client) Option {
	return func(c *Config) {
		c.HTTPClient = httpClient
	}
}

// UpdateListener receives notifications when flags are synchronized.
type UpdateListener func(flags map[string]FeatureFlag, changedKeys []string)

// Client is the official Flagura Go SDK client.
type Client struct {
	config         Config
	circuitBreaker *CircuitBreaker
	telemetry      *telemetryBuffer

	mu        sync.RWMutex
	flags     map[string]FeatureFlag
	listeners []UpdateListener

	cancelFunc context.CancelFunc
}

// NewClient initializes a new Flagura SDK client.
func NewClient(endpoint string, apiKey string, opts ...Option) *Client {
	cfg := Config{
		Endpoint:                endpoint,
		APIKey:                  apiKey,
		DefaultEnvironment:      EnvProduction,
		HTTPClient:              &http.Client{Timeout: 5 * time.Second},
		Logger:                  &nopLogger{},
		LocalEvaluation:         false,
		SyncInterval:            30 * time.Second,
		CircuitBreakerThreshold: 5,
		CircuitBreakerCooldown:  10 * time.Second,
	}

	for _, opt := range opts {
		opt(&cfg)
	}

	ctx, cancel := context.WithCancel(context.Background())

	c := &Client{
		config:     cfg,
		flags:      make(map[string]FeatureFlag),
		cancelFunc: cancel,
	}

	if !cfg.DisableCircuitBreaker {
		c.circuitBreaker = NewCircuitBreaker(cfg.CircuitBreakerThreshold, cfg.CircuitBreakerCooldown)
	}

	c.telemetry = newTelemetryBuffer(c)

	if cfg.LocalEvaluation {
		if cfg.SnapshotFile != "" {
			c.loadSnapshot(cfg.SnapshotFile)
		}
		_ = c.syncFlags(ctx)
		go c.startSyncWorker(ctx)
		go c.startStreaming(ctx)
	}

	return c
}

// RegisterUpdateListener registers a callback invoked when flags update in local cache.
func (c *Client) RegisterUpdateListener(listener UpdateListener) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.listeners = append(c.listeners, listener)
}

// IsEnabled checks if a boolean feature flag is active for the given context.
func (c *Client) IsEnabled(ctx context.Context, flagKey string, evalCtx Context) (bool, error) {
	res, err := c.Evaluate(ctx, flagKey, evalCtx)
	if err != nil {
		return false, err
	}
	return res.Enabled, nil
}

// GetVariant returns the active multivariate variant key for the given flag.
func (c *Client) GetVariant(ctx context.Context, flagKey string, evalCtx Context) (string, error) {
	res, err := c.Evaluate(ctx, flagKey, evalCtx)
	if err != nil {
		return "", err
	}
	return res.Variant, nil
}

// Evaluate evaluates a single feature flag and records telemetry.
func (c *Client) Evaluate(ctx context.Context, flagKey string, evalCtx Context) (EvaluationResult, error) {
	if evalCtx.Environment == "" {
		evalCtx.Environment = c.config.DefaultEnvironment
	}

	var res EvaluationResult
	var err error

	if c.config.LocalEvaluation {
		res, err = c.evaluateLocal(flagKey, evalCtx)
	} else {
		res, err = c.evaluateRemote(ctx, flagKey, evalCtx)
	}

	if err == nil && c.telemetry != nil {
		c.telemetry.Record(TelemetryEvent{
			FlagKey:     flagKey,
			ProjectID:   c.config.ProjectID,
			Environment: evalCtx.Environment,
			Variant:     res.Variant,
			Enabled:     res.Enabled,
			EventType:   "evaluation",
			UserID:      evalCtx.UserID,
			Timestamp:   time.Now(),
		})
	}

	return res, err
}

// EvaluateBatch evaluates multiple flags in a single batch operation.
func (c *Client) EvaluateBatch(ctx context.Context, flagKeys []string, evalCtx Context) (map[string]EvaluationResult, error) {
	if evalCtx.Environment == "" {
		evalCtx.Environment = c.config.DefaultEnvironment
	}

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

// Track records a custom experiment conversion metric.
func (c *Client) Track(flagKey string, metricName string, value float64, userID string) {
	if c.telemetry != nil {
		c.telemetry.Record(TelemetryEvent{
			FlagKey:     flagKey,
			ProjectID:   c.config.ProjectID,
			Environment: c.config.DefaultEnvironment,
			EventType:   "conversion",
			MetricName:  metricName,
			Value:       value,
			UserID:      userID,
			Timestamp:   time.Now(),
		})
	}
}

// Close gracefully closes background workers and flushes telemetry.
func (c *Client) Close() {
	if c.cancelFunc != nil {
		c.cancelFunc()
	}
	if c.telemetry != nil {
		c.telemetry.Close()
	}
	if c.config.SnapshotFile != "" {
		c.saveSnapshot(c.config.SnapshotFile)
	}
}

func (c *Client) evaluateLocal(flagKey string, evalCtx Context) (EvaluationResult, error) {
	c.mu.RLock()
	flag, exists := c.flags[flagKey]
	c.mu.RUnlock()

	if !exists {
		return EvaluationResult{
			FlagKey: flagKey,
			Enabled: false,
			Reason:  "FLAG_NOT_FOUND_IN_CACHE",
		}, fmt.Errorf("flag %q not found in local cache", flagKey)
	}

	return Evaluate(flag, evalCtx), nil
}

func (c *Client) evaluateRemote(ctx context.Context, flagKey string, evalCtx Context) (EvaluationResult, error) {
	if c.circuitBreaker != nil && !c.circuitBreaker.Allow() {
		return EvaluationResult{
			FlagKey: flagKey,
			Enabled: false,
			Reason:  "CIRCUIT_BREAKER_OPEN",
		}, ErrCircuitBreakerOpen
	}

	batchRes, err := c.evaluateBatchRemote(ctx, []string{flagKey}, evalCtx)
	if err != nil {
		if c.circuitBreaker != nil {
			c.circuitBreaker.RecordFailure()
		}
		return EvaluationResult{
			FlagKey: flagKey,
			Enabled: false,
			Reason:  "REMOTE_EVALUATION_ERROR: " + err.Error(),
		}, err
	}

	if c.circuitBreaker != nil {
		c.circuitBreaker.RecordSuccess()
	}

	res, ok := batchRes[flagKey]
	if !ok {
		return EvaluationResult{
			FlagKey: flagKey,
			Enabled: false,
			Reason:  "FLAG_NOT_FOUND_IN_RESPONSE",
		}, fmt.Errorf("flag %q not found in response", flagKey)
	}

	return res, nil
}

func (c *Client) evaluateBatchRemote(ctx context.Context, flagKeys []string, evalCtx Context) (map[string]EvaluationResult, error) {
	reqPayload := map[string]interface{}{
		"flags":   flagKeys,
		"context": evalCtx,
	}

	b, err := json.Marshal(reqPayload)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/api/v1/evaluate", strings.TrimRight(c.config.Endpoint, "/"))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(b))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	if c.config.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.config.APIKey)
	}
	if c.config.ProjectID != "" {
		req.Header.Set(HeaderProjectID, c.config.ProjectID)
	}

	resp, err := c.config.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("server returned status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var evalResp BatchEvaluationResponse
	if err := json.NewDecoder(resp.Body).Decode(&evalResp); err != nil {
		return nil, err
	}

	return evalResp.Results, nil
}

func (c *Client) syncFlags(ctx context.Context) error {
	url := fmt.Sprintf("%s/api/v1/flags", strings.TrimRight(c.config.Endpoint, "/"))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}

	if c.config.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.config.APIKey)
	}
	if c.config.ProjectID != "" {
		req.Header.Set(HeaderProjectID, c.config.ProjectID)
	}

	resp, err := c.config.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to fetch flags: status %d", resp.StatusCode)
	}

	var flagList struct {
		Flags []FeatureFlag `json:"flags"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&flagList); err != nil {
		return err
	}

	newMap := make(map[string]FeatureFlag, len(flagList.Flags))
	for _, f := range flagList.Flags {
		newMap[f.Key] = f
	}

	c.updateLocalFlags(newMap, nil)
	return nil
}

func (c *Client) startSyncWorker(ctx context.Context) {
	ticker := time.NewTicker(c.config.SyncInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := c.syncFlags(ctx); err != nil {
				c.config.Logger.Warnf("Flagura background flag sync failed: %v", err)
			}
		case <-ctx.Done():
			return
		}
	}
}

func (c *Client) updateLocalFlags(newFlags map[string]FeatureFlag, changedKeys []string) {
	c.mu.Lock()
	c.flags = newFlags
	listeners := make([]UpdateListener, len(c.listeners))
	copy(listeners, c.listeners)
	c.mu.Unlock()

	if c.config.SnapshotFile != "" {
		c.saveSnapshot(c.config.SnapshotFile)
	}

	for _, l := range listeners {
		l(newFlags, changedKeys)
	}
}

func (c *Client) saveSnapshot(path string) {
	c.mu.RLock()
	b, err := json.MarshalIndent(c.flags, "", "  ")
	c.mu.RUnlock()
	if err == nil {
		_ = os.WriteFile(path, b, 0600)
	}
}

func (c *Client) loadSnapshot(path string) {
	b, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var flags map[string]FeatureFlag
	if err := json.Unmarshal(b, &flags); err == nil && len(flags) > 0 {
		c.mu.Lock()
		c.flags = flags
		c.mu.Unlock()
	}
}
