package client

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/dhawalhost/flagura/pkg/domain"
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

// startSSEStream maintains an active Server-Sent Events stream for instant flag updates.
func (c *Client) startSSEStream() {
	backoff := 500 * time.Millisecond
	maxBackoff := 15 * time.Second

	for {
		select {
		case <-c.stopCh:
			return
		default:
		}

		err := c.listenSSEStream()
		if err != nil {
			select {
			case <-c.stopCh:
				return
			case <-time.After(backoff):
				backoff *= 2
				if backoff > maxBackoff {
					backoff = maxBackoff
				}
			}
		} else {
			backoff = 500 * time.Millisecond
		}
	}
}

func (c *Client) listenSSEStream() error {
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
		req.Header.Set("Authorization", "Bearer "+c.config.APIKey)
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

	scanner := bufio.NewScanner(resp.Body)
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
