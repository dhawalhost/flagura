package api

import (
	"bufio"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/dhawalhost/flagura/pkg/store"
)

func TestSSEStreamingEndpoint(t *testing.T) {
	st := store.NewMemoryStore()
	server, err := NewServer(st)
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	ts := httptest.NewServer(server)
	defer ts.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL+"/api/v1/flags/stream?project_id=proj_default", nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("X-Project-ID", "proj_default")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("failed to connect to stream: %v", err)
	}
	defer resp.Body.Close()

	if resp.Header.Get("Content-Type") != "text/event-stream" {
		t.Fatalf("expected Content-Type 'text/event-stream', got '%s'", resp.Header.Get("Content-Type"))
	}

	scanner := bufio.NewScanner(resp.Body)
	receivedInit := false

	// Read initial SSE message
	for scanner.Scan() {
		line := scanner.Text()
		if strings.Contains(line, "flags_init") {
			receivedInit = true
			break
		}
	}

	if !receivedInit {
		t.Fatalf("expected flags_init event on initial connection")
	}
}
