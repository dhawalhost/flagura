package flagura

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// StreamEvent represents an SSE payload from the Flagura control plane.
type StreamEvent struct {
	Type      string                 `json:"type"`
	Timestamp string                 `json:"timestamp"`
	Flags     map[string]FeatureFlag `json:"flags,omitempty"`
	Changed   []string               `json:"changed_flags,omitempty"`
}

func (c *Client) startStreaming(ctx context.Context) {
	backoff := 1 * time.Second
	maxBackoff := 30 * time.Second

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		err := c.connectSSE(ctx)
		if err != nil && ctx.Err() == nil {
			c.config.Logger.Warnf("Flagura SSE streaming connection closed: %v. Reconnecting in %v...", err, backoff)
			select {
			case <-time.After(backoff):
				backoff *= 2
				if backoff > maxBackoff {
					backoff = maxBackoff
				}
			case <-ctx.Done():
				return
			}
		} else {
			backoff = 1 * time.Second
		}
	}
}

func (c *Client) connectSSE(ctx context.Context) error {
	streamURL := fmt.Sprintf("%s/api/v1/flags/stream", strings.TrimRight(c.config.Endpoint, "/"))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, streamURL, nil)
	if err != nil {
		return err
	}

	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Cache-Control", "no-cache")
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
		return fmt.Errorf("unexpected status code from stream endpoint: %d", resp.StatusCode)
	}

	c.config.Logger.Infof("Connected to Flagura real-time SSE streaming control plane")
	reader := bufio.NewReader(resp.Body)

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF || ctx.Err() != nil {
				return nil
			}
			return err
		}

		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "data:") {
			data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if data == "" || data == "ping" {
				continue
			}

			var raw map[string]interface{}
			if err := json.Unmarshal([]byte(data), &raw); err == nil {
				if flagsRaw, ok := raw["flags"]; ok {
					flagsBytes, _ := json.Marshal(flagsRaw)
					// Try slice of FeatureFlag
					var flagList []FeatureFlag
					if err := json.Unmarshal(flagsBytes, &flagList); err == nil && len(flagList) > 0 {
						flagMap := make(map[string]FeatureFlag, len(flagList))
						for _, f := range flagList {
							flagMap[f.Key] = f
						}
						c.updateLocalFlags(flagMap, nil)
					} else {
						// Try map of FeatureFlag
						var flagMap map[string]FeatureFlag
						if err := json.Unmarshal(flagsBytes, &flagMap); err == nil && len(flagMap) > 0 {
							c.updateLocalFlags(flagMap, nil)
						}
					}
				}
			}
		}
	}
}
