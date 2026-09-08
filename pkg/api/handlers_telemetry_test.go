package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/dhawalhost/flagura/pkg/domain"
	"github.com/dhawalhost/flagura/pkg/store"
)

func TestTelemetryIngestionAndStats(t *testing.T) {
	memStore := store.NewMemoryStore()
	server, err := NewServer(memStore)
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	ts := httptest.NewServer(server)
	defer ts.Close()

	ctx := context.Background()
	projA := "proj_telem_a"
	projB := "proj_telem_b"
	keyA, prefixA, hashA, _ := generateRawAPIKey()
	_, _ = memStore.CreateAPIKey(ctx, domain.APIKey{
		ID:          "k_telem_a",
		Key:         keyA,
		KeyHash:     hashA,
		KeyPrefix:   prefixA,
		ProjectID:   projA,
		Name:        "Test Key A",
		Role:        domain.RoleDeveloper,
		CreatedAt:   time.Now(),
	})
	keyB, prefixB, hashB, _ := generateRawAPIKey()
	_, _ = memStore.CreateAPIKey(ctx, domain.APIKey{
		ID:          "k_telem_b",
		Key:         keyB,
		KeyHash:     hashB,
		KeyPrefix:   prefixB,
		ProjectID:   projB,
		Name:        "Test Key B",
		Role:        domain.RoleDeveloper,
		CreatedAt:   time.Now(),
	})

	// 1. Ingest telemetry unauthenticated (should fail)
	payload := map[string]interface{}{
		"timestamp": 1724999999000,
		"events": map[string]interface{}{
			"ai-smart-search": map[string]interface{}{
				"evaluations": 120,
				"variants": map[string]interface{}{
					"treatment": 60,
					"control":   60,
				},
			},
		},
	}
	body, _ := json.Marshal(payload)

	unauthReq, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/telemetry/events", bytes.NewReader(body))
	unauthReq.Header.Set("Content-Type", "application/json")
	unauthResp, err := http.DefaultClient.Do(unauthReq)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	unauthResp.Body.Close()
	if unauthResp.StatusCode == http.StatusOK {
		t.Fatalf("expected unauthenticated telemetry ingest to fail, got %d", unauthResp.StatusCode)
	}

	// 2. Ingest telemetry authenticated for projA
	authReqA, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/telemetry/events", bytes.NewReader(body))
	authReqA.Header.Set("Content-Type", "application/json")
	authReqA.Header.Set(domain.HeaderAPIKey, keyA)
	authRespA, err := http.DefaultClient.Do(authReqA)
	if err != nil || authRespA.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK for authenticated telemetry ingest in projA, got: %v (code %v)", err, authRespA.StatusCode)
	}
	authRespA.Body.Close()

	// 3. Query telemetry stats in projA
	statsReqA, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/telemetry/stats?flag=ai-smart-search", nil)
	statsReqA.Header.Set(domain.HeaderAPIKey, keyA)
	statsRespA, err := http.DefaultClient.Do(statsReqA)
	if err != nil || statsRespA.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK for stats in projA, got: %v (code %v)", err, statsRespA.StatusCode)
	}
	defer statsRespA.Body.Close()

	var statsA map[string]interface{}
	if err := json.NewDecoder(statsRespA.Body).Decode(&statsA); err != nil {
		t.Fatalf("failed to decode stats response: %v", err)
	}
	if statsA["total_evaluations"] != float64(120) {
		t.Fatalf("expected total_evaluations 120, got %v", statsA["total_evaluations"])
	}

	// 4. Multi-Tenant isolation: projB must NOT see projA's telemetry
	statsReqB, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/telemetry/stats?flag=ai-smart-search", nil)
	statsReqB.Header.Set(domain.HeaderAPIKey, keyB)
	statsRespB, err := http.DefaultClient.Do(statsReqB)
	if err != nil || statsRespB.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK for stats in projB, got: %v (code %v)", err, statsRespB.StatusCode)
	}
	defer statsRespB.Body.Close()

	var statsB map[string]interface{}
	if err := json.NewDecoder(statsRespB.Body).Decode(&statsB); err != nil {
		t.Fatalf("failed to decode stats response: %v", err)
	}
	if statsB["total_evaluations"] != float64(0) {
		t.Fatalf("expected 0 evaluations in projB (cross-tenant leakage!), got %v", statsB["total_evaluations"])
	}
}

func TestTelemetryBoundedCapacity(t *testing.T) {
	memStore := store.NewMemoryStore()
	server, err := NewServer(memStore)
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	ts := httptest.NewServer(server)
	defer ts.Close()

	ctx := context.Background()
	proj := "proj_capacity_test"
	key, prefix, hash, _ := generateRawAPIKey()
	_, _ = memStore.CreateAPIKey(ctx, domain.APIKey{
		ID:        "k_cap",
		Key:       key,
		KeyHash:   hash,
		KeyPrefix: prefix,
		ProjectID: proj,
		Name:      "Cap Key",
		Role:      domain.RoleDeveloper,
		CreatedAt: time.Now(),
	})

	// Ingest 1,050 distinct flags
	events := make(map[string]interface{})
	for i := 0; i < 1050; i++ {
		flagName := fmt.Sprintf("flag-%04d", i)
		events[flagName] = map[string]interface{}{
			"evaluations": 1,
		}
	}
	payload := map[string]interface{}{
		"timestamp": time.Now().UnixMilli(),
		"events":    events,
	}
	body, _ := json.Marshal(payload)

	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/telemetry/events", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(domain.HeaderAPIKey, key)
	resp, err := http.DefaultClient.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("failed to ingest telemetry: %v (code %v)", err, resp.StatusCode)
	}
	resp.Body.Close()

	// Query telemetry summary for project - flags count should be capped at maxProjectFlags (1000)
	summary := server.telemetry.Stats(proj, "all")
	flagsMap, ok := summary["flags"].(map[string]FlagAggregatedMetric)
	if !ok {
		t.Fatalf("expected flags map in summary, got: %T", summary["flags"])
	}
	if len(flagsMap) > 1000 {
		t.Fatalf("expected at most 1000 flags in memory, got %d", len(flagsMap))
	}
}
