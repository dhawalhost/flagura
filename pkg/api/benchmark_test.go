package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dhawalhost/flagura/pkg/api"
	"github.com/dhawalhost/flagura/pkg/domain"
	"github.com/dhawalhost/flagura/pkg/store"
)

func BenchmarkHTTPEvaluateEndpoint(b *testing.B) {
	memStore := store.NewMemoryStore()
	_, _ = memStore.SaveFlag(context.Background(), domain.FeatureFlag{
		ID:   "flag_bench",
		Key:  "ai-smart-search",
		Name: "AI Smart Search",
		Type: "boolean",
		Environments: map[domain.Environment]domain.EnvironmentConfig{
			domain.EnvProduction: {
				Enabled:    true,
				Strategy:   domain.StrategyPercentage,
				Percentage: 50,
			},
		},
	}, "system")

	server, err := api.NewServer(memStore)
	if err != nil {
		b.Fatalf("failed to create server: %v", err)
	}

	ts := httptest.NewServer(server)
	defer ts.Close()

	payload, _ := json.Marshal(map[string]interface{}{
		"flags": []string{"ai-smart-search"},
		"context": map[string]interface{}{
			"user_id":     "usr_bench_01",
			"email":       "alice@flagura.dev",
			"environment": "production",
		},
	})

	client := ts.Client()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/evaluate", bytes.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		resp, err := client.Do(req)
		if err != nil {
			b.Fatalf("HTTP request failed: %v", err)
		}
		_ = resp.Body.Close()
	}
}
