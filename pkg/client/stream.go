package client

import (
	"bufio"
	"context"
	crand "crypto/rand"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/dhawalhost/flagura/pkg/domain"
)

const (
	// InitialStreamBufferSize is the initial scanner buffer size (64 KB).
	InitialStreamBufferSize = 64 * 1024
	// MaxStreamPayloadSize is the maximum token size for incoming SSE events (10 MB).
	MaxStreamPayloadSize = 10 * 1024 * 1024
)

func (c *Client) updateFlags(flags []domain.FeatureFlag) {
	c.mu.Lock()
	var changedKeys []string
	newMap := make(map[string]domain.FeatureFlag, len(flags))
	for _, f := range flags {
		newMap[f.Key] = f
		newMap[f.ID] = f
		if old, exists := c.flags[f.Key]; !exists || old.UpdatedAt != f.UpdatedAt {
			changedKeys = append(changedKeys, f.Key)
		}
	}
	c.flags = newMap
	currentListeners := make([]func(flags map[string]domain.FeatureFlag, changedKeys []string), len(c.listeners))
	copy(currentListeners, c.listeners)
	c.mu.Unlock()

	if c.config.SnapshotFile != "" {
		_ = c.saveSnapshot(flags)
	}

	if len(changedKeys) > 0 {
		for _, listener := range currentListeners {
			listener(newMap, changedKeys)
		}
	}
}

// computeJitteredBackoff computes exponential backoff with full jitter to avoid thundering herds.
func computeJitteredBackoff(attempt int, base, max time.Duration) time.Duration {
	if base <= 0 {
		base = 500 * time.Millisecond
	}
	if max <= 0 {
		max = 30 * time.Second
	}
	multiplier := 1 << attempt
	if multiplier <= 0 || multiplier > 1024 {
		multiplier = 1024
	}
	backoff := base * time.Duration(multiplier)
	if backoff > max {
		backoff = max
	}

	// Cryptographically secure jitter in [0, base)
	var jitter time.Duration
	if n, err := crand.Int(crand.Reader, big.NewInt(int64(base))); err == nil {
		jitter = time.Duration(n.Int64())
	}
	total := backoff + jitter
	if total > max {
		return max
	}
	return total
}

// startSSEStream maintains an active Server-Sent Events stream for instant flag updates,
// falling back to periodic HTTP polling if the SSE connection drops or is blocked.
func (c *Client) startSSEStream() {
	baseBackoff := 500 * time.Millisecond
	maxBackoff := 30 * time.Second
	attempt := 0

	var fallbackCancel context.CancelFunc
	var fallbackMu sync.Mutex

	startFallback := func() {
		fallbackMu.Lock()
		defer fallbackMu.Unlock()
		if fallbackCancel != nil {
			return // already running
		}
		c.setConnectionState(StateConnectedPolling)
		// #nosec G118 -- cancel function is stored in fallbackCancel and invoked via stopFallback()
		ctx, cancel := context.WithCancel(context.Background())
		fallbackCancel = cancel

		interval := c.config.FallbackPollInterval
		if interval <= 0 {
			interval = 5 * time.Second
		}

		go func() {
			ticker := time.NewTicker(interval)
			defer ticker.Stop()

			// Immediate initial poll on fallback
			pollCtx, pCancel := context.WithTimeout(ctx, 5*time.Second)
			_ = c.syncFlags(pollCtx)
			pCancel()

			for {
				select {
				case <-ticker.C:
					pCtx, pCancel := context.WithTimeout(ctx, 5*time.Second)
					_ = c.syncFlags(pCtx)
					pCancel()
				case <-ctx.Done():
					return
				case <-c.stopCh:
					return
				}
			}
		}()
	}

	stopFallback := func() {
		fallbackMu.Lock()
		defer fallbackMu.Unlock()
		if fallbackCancel != nil {
			fallbackCancel()
			fallbackCancel = nil
		}
	}

	defer stopFallback()

	for {
		select {
		case <-c.stopCh:
			return
		default:
		}

		err := c.listenSSEStream(func() {
			stopFallback()
			c.setConnectionState(StateConnectedSSE)
			attempt = 0
		})

		if err != nil {
			startFallback()
			backoffDuration := computeJitteredBackoff(attempt, baseBackoff, maxBackoff)
			attempt++

			select {
			case <-c.stopCh:
				return
			case <-time.After(backoffDuration):
			}
		} else {
			startFallback()
			attempt = 0
		}
	}
}

func (c *Client) listenSSEStream(onConnected func()) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		select {
		case <-c.stopCh:
			cancel()
		case <-ctx.Done():
		}
	}()

	url := fmt.Sprintf("%s/api/v1/flags/stream", c.config.Endpoint)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}

	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Cache-Control", "no-cache")
	if c.config.APIKey != "" {
		req.Header.Set(domain.HeaderAuthorization, "Bearer "+c.config.APIKey)
	}
	if c.config.ProjectID != "" {
		req.Header.Set(domain.HeaderProjectID, c.config.ProjectID)
	}

	// Use stream transport with 0 timeout for persistent HTTP streaming
	streamClient := &http.Client{
		Timeout: 0,
	}

	resp, err := streamClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected SSE response status: %d", resp.StatusCode)
	}

	var connectedOnce sync.Once
	signalConnected := func() {
		connectedOnce.Do(func() {
			if onConnected != nil {
				onConnected()
			}
		})
	}
	signalConnected()

	scanner := bufio.NewScanner(resp.Body)
	buf := make([]byte, InitialStreamBufferSize)
	scanner.Buffer(buf, MaxStreamPayloadSize)

	var currentEvent string
	var currentData strings.Builder

	for scanner.Scan() {
		line := scanner.Text()

		// Heartbeat ping comment or empty line
		if strings.HasPrefix(line, ":") {
			continue
		}

		if strings.HasPrefix(line, "event:") {
			currentEvent = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
			continue
		}

		if strings.HasPrefix(line, "data:") {
			dataContent := strings.TrimPrefix(line, "data:")
			if currentData.Len()+len(dataContent) > MaxStreamPayloadSize {
				c.config.Logger.Warnf("flagura: stream event payload exceeded max size (%d bytes), discarding", currentData.Len())
				currentEvent = ""
				currentData.Reset()
				continue
			}
			currentData.WriteString(strings.TrimSpace(dataContent))
			continue
		}

		// Empty line indicates event dispatch in SSE protocol
		if line == "" && currentData.Len() > 0 {
			if currentEvent == "flags_init" || currentEvent == "flags_update" {
				var payload struct {
					Flags []domain.FeatureFlag `json:"flags"`
				}
				if err := json.Unmarshal([]byte(currentData.String()), &payload); err == nil && payload.Flags != nil {
					c.updateFlags(payload.Flags)
				}
			}
			currentEvent = ""
			currentData.Reset()
		}
	}

	return scanner.Err()
}
