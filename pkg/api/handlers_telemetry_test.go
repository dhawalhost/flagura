package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dhawalhost/flagura/pkg/api"
	"github.com/dhawalhost/flagura/pkg/store"
)

func TestTelemetryIngestionAndStats(t *testing.T) {
	memStore := store.NewMemoryStore()
	server, err := api.NewServer(memStore)
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	ts := httptest.NewServer(server)
	defer ts.Close()

	// 1. Ingest telemetry
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

	resp, err := http.Post(ts.URL+"/api/v1/telemetry/events", "application/json", bytes.NewReader(body))
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK for telemetry ingest, got: %v (code %v)", err, resp.StatusCode)
	}
	resp.Body.Close()

	// 2. Query telemetry stats
	statsResp, err := http.Get(ts.URL + "/api/v1/telemetry/stats?flag=ai-smart-search")
	if err != nil || statsResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK for stats, got: %v (code %v)", err, statsResp.StatusCode)
	}
	defer statsResp.Body.Close()

	var stats map[string]interface{}
	if err := json.NewDecoder(statsResp.Body).Decode(&stats); err != nil {
		t.Fatalf("failed to decode stats response: %v", err)
	}

	if stats["total_evaluations"] != float64(120) {
		t.Fatalf("expected total_evaluations 120, got %v", stats["total_evaluations"])
	}
}
