package flagura

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

// TelemetryEvent represents a client-side evaluation or experiment conversion event.
type TelemetryEvent struct {
	FlagKey     string      `json:"flag_key"`
	ProjectID   string      `json:"project_id,omitempty"`
	Environment Environment `json:"environment"`
	Variant     string      `json:"variant"`
	Enabled     bool        `json:"enabled"`
	EventType   string      `json:"event_type"` // "evaluation", "conversion"
	MetricName  string      `json:"metric_name,omitempty"`
	Value       float64     `json:"value,omitempty"`
	UserID      string      `json:"user_id,omitempty"`
	Timestamp   time.Time   `json:"timestamp"`
}

type telemetryBuffer struct {
	mu          sync.Mutex
	events      []TelemetryEvent
	client      *Client
	batchSize   int
	flushPeriod time.Duration
	stopChan    chan struct{}
	closeOnce   sync.Once
}

func newTelemetryBuffer(c *Client) *telemetryBuffer {
	tb := &telemetryBuffer{
		client:      c,
		events:      make([]TelemetryEvent, 0, 100),
		batchSize:   50,
		flushPeriod: 5 * time.Second,
		stopChan:    make(chan struct{}),
	}
	go tb.flushLoop()
	return tb
}

func (tb *telemetryBuffer) Record(e TelemetryEvent) {
	if tb == nil {
		return
	}
	tb.mu.Lock()
	defer tb.mu.Unlock()

	tb.events = append(tb.events, e)
	if len(tb.events) >= tb.batchSize {
		go tb.Flush()
	}
}

func (tb *telemetryBuffer) Flush() {
	if tb == nil {
		return
	}
	tb.mu.Lock()
	if len(tb.events) == 0 {
		tb.mu.Unlock()
		return
	}
	toSend := tb.events
	tb.events = make([]TelemetryEvent, 0, 100)
	tb.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	payload := map[string]interface{}{
		"events": toSend,
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return
	}

	url := fmt.Sprintf("%s/api/v1/telemetry/events", strings.TrimRight(tb.client.config.Endpoint, "/"))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(b))
	if err != nil {
		return
	}

	req.Header.Set("Content-Type", "application/json")
	if tb.client.config.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+tb.client.config.APIKey)
	}
	if tb.client.config.ProjectID != "" {
		req.Header.Set(HeaderProjectID, tb.client.config.ProjectID)
	}

	resp, err := tb.client.config.HTTPClient.Do(req)
	if err == nil && resp != nil {
		_ = resp.Body.Close()
	}
}

func (tb *telemetryBuffer) flushLoop() {
	ticker := time.NewTicker(tb.flushPeriod)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			tb.Flush()
		case <-tb.stopChan:
			tb.Flush()
			return
		}
	}
}

func (tb *telemetryBuffer) Close() {
	if tb == nil {
		return
	}
	tb.closeOnce.Do(func() {
		close(tb.stopChan)
	})
}
