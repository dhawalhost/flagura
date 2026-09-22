package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

const (
	// DefaultMaxBufferedFlags is the maximum distinct feature flags tracked in memory.
	DefaultMaxBufferedFlags = 1000
	// DefaultMaxVariantsPerFlag is the maximum distinct variants tracked per flag.
	DefaultMaxVariantsPerFlag = 50
)

// FlagMetric tracks aggregated evaluation counts and variant distributions for a single flag.
type FlagMetric struct {
	Evaluations uint64            `json:"evaluations"`
	Variants    map[string]uint64 `json:"variants"`
}

// TelemetryPayload represents the batch telemetry payload flushed to the server.
type TelemetryPayload struct {
	Timestamp     int64                 `json:"timestamp"`
	Events        map[string]FlagMetric `json:"events"`
	DroppedEvents uint64                `json:"dropped_events,omitempty"`
}

// TelemetryBuffer aggregates in-memory evaluations and periodically flushes them to Flagura.
type TelemetryBuffer struct {
	mu            sync.Mutex
	endpoint      string
	apiKey        string
	httpClient    *http.Client
	metrics       map[string]*FlagMetric
	maxFlags      int
	maxVariants   int
	droppedEvents uint64
}

// NewTelemetryBuffer creates a new TelemetryBuffer.
func NewTelemetryBuffer(endpoint string, apiKey string, httpClient *http.Client) *TelemetryBuffer {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 5 * time.Second}
	}
	return &TelemetryBuffer{
		endpoint:    endpoint,
		apiKey:      apiKey,
		httpClient:  httpClient,
		metrics:     make(map[string]*FlagMetric),
		maxFlags:    DefaultMaxBufferedFlags,
		maxVariants: DefaultMaxVariantsPerFlag,
	}
}

// SetCapacityBounds allows configuring custom limits for in-memory flag and variant telemetry buffers.
func (tb *TelemetryBuffer) SetCapacityBounds(maxFlags, maxVariants int) {
	if tb == nil {
		return
	}
	tb.mu.Lock()
	defer tb.mu.Unlock()
	if maxFlags > 0 {
		tb.maxFlags = maxFlags
	}
	if maxVariants > 0 {
		tb.maxVariants = maxVariants
	}
}

// DroppedEvents returns the count of dropped evaluation events due to buffer capacity bounds.
func (tb *TelemetryBuffer) DroppedEvents() uint64 {
	if tb == nil {
		return 0
	}
	return atomic.LoadUint64(&tb.droppedEvents)
}

// Record atomically increments the evaluation count and variant counter for a given flag.
func (tb *TelemetryBuffer) Record(flagKey string, variant string) {
	if tb == nil || flagKey == "" {
		return
	}

	if variant == "" {
		variant = "off"
	}

	tb.mu.Lock()
	defer tb.mu.Unlock()

	m, exists := tb.metrics[flagKey]
	if !exists {
		if len(tb.metrics) >= tb.maxFlags {
			atomic.AddUint64(&tb.droppedEvents, 1)
			return
		}
		m = &FlagMetric{
			Variants: make(map[string]uint64),
		}
		tb.metrics[flagKey] = m
	}

	if _, vExists := m.Variants[variant]; !vExists {
		if len(m.Variants) >= tb.maxVariants {
			atomic.AddUint64(&tb.droppedEvents, 1)
			return
		}
	}

	m.Evaluations++
	m.Variants[variant]++
}

// Flush sends the currently buffered metrics to the Flagura control plane and resets counters.
func (tb *TelemetryBuffer) Flush(ctx context.Context) error {
	if tb == nil {
		return nil
	}

	tb.mu.Lock()
	dropped := atomic.SwapUint64(&tb.droppedEvents, 0)
	if len(tb.metrics) == 0 && dropped == 0 {
		tb.mu.Unlock()
		return nil
	}

	snapshot := make(map[string]FlagMetric, len(tb.metrics))
	for k, v := range tb.metrics {
		varCopy := make(map[string]uint64, len(v.Variants))
		for vk, vv := range v.Variants {
			varCopy[vk] = vv
		}
		snapshot[k] = FlagMetric{
			Evaluations: v.Evaluations,
			Variants:    varCopy,
		}
	}
	// Reset local buffer
	tb.metrics = make(map[string]*FlagMetric)
	tb.mu.Unlock()

	payload := TelemetryPayload{
		Timestamp:     time.Now().UTC().UnixMilli(),
		Events:        snapshot,
		DroppedEvents: dropped,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal telemetry payload: %w", err)
	}

	url := fmt.Sprintf("%s/api/v1/telemetry/events", tb.endpoint)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("failed to create telemetry request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if tb.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+tb.apiKey)
	}

	resp, err := tb.httpClient.Do(req)
	if err != nil {
		// Restore dropped counter on network failure
		atomic.AddUint64(&tb.droppedEvents, dropped)
		return fmt.Errorf("telemetry flush failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		atomic.AddUint64(&tb.droppedEvents, dropped)
		return fmt.Errorf("telemetry flush returned status %d", resp.StatusCode)
	}

	return nil
}

// StartBackgroundLoop runs the periodic flush worker.
func (tb *TelemetryBuffer) StartBackgroundLoop(stopCh chan struct{}, interval time.Duration) {
	if interval <= 0 {
		interval = 60 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			_ = tb.Flush(ctx)
			cancel()
		case <-stopCh:
			// Flush remaining on shutdown
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			_ = tb.Flush(ctx)
			cancel()
			return
		}
	}
}
