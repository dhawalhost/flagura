package client

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dhawalhost/flagura/pkg/api"
	"github.com/dhawalhost/flagura/pkg/domain"
	"github.com/dhawalhost/flagura/pkg/store"
)

func TestRealTimeStreamingSync(t *testing.T) {
	st := store.NewMemoryStore()
	server, err := api.NewServer(st)
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	ts := httptest.NewServer(server)
	defer ts.Close()

	ctx := context.Background()

	// Seed org, project and target flag
	_, _ = st.CreateOrganization(ctx, domain.Organization{
		ID:   domain.DefaultOrgID,
		Name: "Default Org",
	})
	_, _ = st.CreateProject(ctx, domain.Project{
		ID:             "proj_test",
		OrganizationID: domain.DefaultOrgID,
		Name:           "Test Project",
	})
	_, _ = st.SaveFlag(ctx, domain.FeatureFlag{
		ID:        "flag_rate_limiter",
		ProjectID: "proj_test",
		Key:       "rate-limiter-v2",
		Name:      "Rate Limiter V2",
		Type:      "boolean",
		Environments: map[domain.Environment]domain.EnvironmentConfig{
			domain.EnvProduction: {
				Enabled:  true,
				Strategy: domain.StrategyBoolean,
			},
		},
	}, "test")

	apiKeyVal := "flg_live_test_stream_key_secret_12345"
	h := sha256.Sum256([]byte(apiKeyVal))
	keyHash := hex.EncodeToString(h[:])
	_, _ = st.CreateAPIKey(ctx, domain.APIKey{
		ID:          "key_stream_test",
		ProjectID:   "proj_test",
		Key:         apiKeyVal,
		KeyHash:     keyHash,
		Name:        "Stream Test Key",
		Role:        domain.RoleDeveloper,
		Environment: "production",
	})

	// Initialize client with long 60s polling interval so we know updates come via SSE stream
	c := New(ts.URL,
		WithAPIKey(apiKeyVal),
		WithProjectID("proj_test"),
		WithLocalEvaluation(60*time.Second),
		WithStreaming(true),
	)
	defer c.Close()

	// Give SSE connection 50ms to establish
	time.Sleep(50 * time.Millisecond)

	// Create test authenticated developer user & session
	devUser, _ := st.CreateUser(ctx, domain.User{
		Email: "tester@flagura.dev",
		Name:  "Tester",
		Role:  domain.RoleDeveloper,
	})
	_, _ = st.CreateOrgMember(ctx, domain.OrgMember{
		OrganizationID: domain.DefaultOrgID,
		UserID:         devUser.ID,
		Role:           string(domain.RoleDeveloper),
	})
	sessionToken := "test_stream_session_token"
	_ = st.CreateSession(ctx, domain.Session{
		Token:     sessionToken,
		UserID:    devUser.ID,
		ExpiresAt: time.Now().Add(1 * time.Hour),
	})

	evalCtx := Context{UserID: "usr_stream_test"}

	// 1. Initial evaluation
	val1 := c.IsEnabled(ctx, "rate-limiter-v2", evalCtx)

	// 2. Toggle the flag on the server via authenticated HTTP mutation
	toggleBody, _ := json.Marshal(map[string]interface{}{
		"environment": "production",
		"enabled":     !val1,
		"actor":       "tester@flagura.dev",
	})
	req, _ := http.NewRequest(http.MethodPatch, ts.URL+"/api/v1/flags/rate-limiter-v2/toggle", bytes.NewReader(toggleBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+sessionToken)
	req.Header.Set("X-Project-ID", "proj_test")
	resp, err := http.DefaultClient.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("failed to toggle flag on server: %v (code: %v)", err, resp.StatusCode)
	}
	resp.Body.Close()

	// 3. Assert that within < 200ms the client in-memory cache has the updated value via SSE
	var val2 bool
	updated := false
	start := time.Now()

	for time.Since(start) < 500*time.Millisecond {
		val2 = c.IsEnabled(ctx, "rate-limiter-v2", evalCtx)
		if val2 != val1 {
			updated = true
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if !updated {
		t.Fatalf("expected real-time SSE streaming update from %v to %v within 500ms, got %v", val1, !val1, val2)
	}
}

func TestComputeJitteredBackoff(t *testing.T) {
	base := 100 * time.Millisecond
	max := 2 * time.Second

	// Verify attempt 0: [base, base + base]
	d0 := computeJitteredBackoff(0, base, max)
	if d0 < base || d0 > 2*base {
		t.Errorf("expected attempt 0 in [%v, %v], got %v", base, 2*base, d0)
	}

	// Verify max bound
	dMax := computeJitteredBackoff(10, base, max)
	if dMax > max {
		t.Errorf("expected capped backoff <= %v, got %v", max, dMax)
	}

	// Verify jitter spread across multiple iterations (non-deterministic)
	seen := make(map[time.Duration]bool)
	for i := 0; i < 50; i++ {
		d := computeJitteredBackoff(1, base, max)
		seen[d] = true
	}
	if len(seen) <= 1 {
		t.Errorf("expected jitter to produce varied backoff durations, got %d distinct values", len(seen))
	}
}

func TestStreamLargePayloadScannerBuffer(t *testing.T) {
	// Generate a flags payload > 128KB (exceeds default 64KB bufio.Scanner buffer)
	var largeFlags []domain.FeatureFlag
	for i := 0; i < 500; i++ {
		largeFlags = append(largeFlags, domain.FeatureFlag{
			ID:          fmt.Sprintf("flag_large_%d", i),
			Key:         fmt.Sprintf("enterprise-flag-key-large-payload-test-%d", i),
			Name:        fmt.Sprintf("Enterprise Feature Flag Number %d with Descriptive Long Name", i),
			Description: fmt.Sprintf("Detailed enterprise description explaining the feature rollout for tenant customer partition %d with extensive rule metadata", i),
			Type:        "boolean",
			ProjectID:   "proj_test",
			Environments: map[domain.Environment]domain.EnvironmentConfig{
				domain.EnvProduction: {
					Enabled:  true,
					Strategy: domain.StrategyBoolean,
				},
			},
		})
	}

	payloadBytes, err := json.Marshal(map[string]interface{}{
		"flags": largeFlags,
	})
	if err != nil {
		t.Fatalf("failed to marshal payload: %v", err)
	}

	if len(payloadBytes) < 65536 {
		t.Fatalf("test payload size %d is not > 64KB", len(payloadBytes))
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/flags/stream" {
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
			flusher, ok := w.(http.Flusher)
			if !ok {
				return
			}
			fmt.Fprintf(w, "event: flags_init\ndata: %s\n\n", string(payloadBytes))
			flusher.Flush()

			// Keep connection open until request context done
			<-r.Context().Done()
			return
		}
		http.NotFound(w, r)
	}))
	defer ts.Close()

	c := New(ts.URL,
		WithDisabledTelemetry(),
		WithLocalEvaluation(10*time.Second),
		WithStreaming(true),
	)
	defer c.Close()

	// Wait up to 2 seconds for stream scanner to parse the > 64KB payload
	start := time.Now()
	var flag domain.FeatureFlag
	var found bool
	for time.Since(start) < 2*time.Second {
		c.mu.RLock()
		flag, found = c.flags["enterprise-flag-key-large-payload-test-499"]
		c.mu.RUnlock()
		if found {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	if !found || flag.Key != "enterprise-flag-key-large-payload-test-499" {
		t.Fatalf("expected client to scan and parse >64KB enterprise payload (size %d bytes) without bufio.ErrTooLong", len(payloadBytes))
	}
}

func TestStreamPollingFallbackOnDisconnect(t *testing.T) {
	var mu sync.Mutex
	flagsData := []domain.FeatureFlag{
		{
			ID:   "flag_fallback_1",
			Key:  "feature-fallback",
			Type: "boolean",
			Environments: map[domain.Environment]domain.EnvironmentConfig{
				domain.EnvProduction: {
					Enabled:  false,
					Strategy: domain.StrategyBoolean,
				},
			},
		},
	}

	var streamDisconnect atomic.Bool

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/flags/stream":
			if streamDisconnect.Load() {
				http.Error(w, "stream disabled by proxy/middlebox", http.StatusBadGateway)
				return
			}
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
			flusher, ok := w.(http.Flusher)
			if !ok {
				return
			}
			mu.Lock()
			data, _ := json.Marshal(map[string]interface{}{"flags": flagsData})
			mu.Unlock()
			fmt.Fprintf(w, "event: flags_init\ndata: %s\n\n", string(data))
			flusher.Flush()

			// Disconnect when triggered
			for {
				if streamDisconnect.Load() {
					return
				}
				select {
				case <-r.Context().Done():
					return
				case <-time.After(10 * time.Millisecond):
				}
			}
		case "/api/v1/flags":
			w.Header().Set("Content-Type", "application/json")
			mu.Lock()
			data, _ := json.Marshal(map[string]interface{}{"flags": flagsData})
			mu.Unlock()
			_, _ = w.Write(data)
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	c := New(ts.URL,
		WithDisabledTelemetry(),
		WithLocalEvaluation(30*time.Second),          // Normal sync interval is 30s
		WithFallbackPollInterval(50*time.Millisecond), // Fast fallback polling
		WithStreaming(true),
	)
	defer c.Close()

	// 1. Give SSE stream time to connect
	start := time.Now()
	for time.Since(start) < 1*time.Second {
		if c.ConnectionState() == StateConnectedSSE {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if c.ConnectionState() != StateConnectedSSE {
		t.Fatalf("expected client to be CONNECTED_SSE, got %s", c.ConnectionState())
	}

	// 2. Trigger SSE disconnection (simulate corporate proxy dropping stream)
	streamDisconnect.Store(true)

	// Mutate flag on the REST API endpoint
	mu.Lock()
	flagsData[0].Environments[domain.EnvProduction] = domain.EnvironmentConfig{
		Enabled:  true,
		Strategy: domain.StrategyBoolean,
	}
	mu.Unlock()

	// 3. Verify client enters CONNECTED_POLLING and receives updated flag via fallback polling
	updated := false
	start = time.Now()
	for time.Since(start) < 3*time.Second {
		if c.IsEnabled(context.Background(), "feature-fallback", Context{UserID: "usr_fallback"}) {
			updated = true
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	if !updated {
		t.Fatalf("expected flag to be updated via fallback polling within 3s, but remained disabled")
	}

	if c.ConnectionState() != StateConnectedPolling {
		t.Logf("connection state during fallback: %s", c.ConnectionState())
	}
}
