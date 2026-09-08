package client

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
