package engine_test

import (
	"context"
	"os"
	"testing"

	"github.com/dhawalhost/flagura/pkg/domain"
	"github.com/dhawalhost/flagura/pkg/engine"
	"github.com/dhawalhost/flagura/pkg/store"
)

func createBenchmarkSampleFlag(key string) domain.FeatureFlag {
	return domain.FeatureFlag{
		ID:        "flg_bench_" + key,
		ProjectID: store.DefaultProjectID,
		Key:       key,
		Name:      "Benchmark Feature Flag",
		Type:      "boolean",
		Environments: map[domain.Environment]domain.EnvironmentConfig{
			domain.EnvProduction: {
				Enabled:    true,
				Strategy:   domain.StrategyPercentage,
				Percentage: 50.0,
				Rules: []domain.TargetingRule{
					{
						ID:        "rule_1",
						Name:      "Beta User Rule",
						Attribute: domain.AttrRole,
						Operator:  domain.OpEquals,
						Values:    []string{"beta-tester", "qa"},
						Action:    domain.ActionForceEnabled,
					},
				},
			},
		},
	}
}

// BenchmarkInProcessEvaluation_DirectEngine benchmarks pure in-memory deterministic evaluation.
func BenchmarkInProcessEvaluation_DirectEngine(b *testing.B) {
	flag := createBenchmarkSampleFlag("ai-smart-search")
	evalCtx := domain.EvaluationContext{
		UserID:      "usr_bench_12345",
		Email:       "tester@flagura.dev",
		Role:        "developer",
		Environment: domain.EnvProduction,
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		res := engine.EvaluateFlag(flag, evalCtx)
		if !res.Enabled && res.Variant == "" {
			b.Fatal("unexpected evaluation result")
		}
	}
}

// BenchmarkInProcessEvaluation_FromMemoryStore benchmarks flag cached from MemoryStore.
func BenchmarkInProcessEvaluation_FromMemoryStore(b *testing.B) {
	ctx := context.Background()
	memStore := store.NewMemoryStore()
	flag := createBenchmarkSampleFlag("search-v2")
	_, _ = memStore.SaveFlag(ctx, flag, "bench")

	// In Data-Plane SDK architecture, flags are synced into memory cache:
	cachedFlag, err := memStore.GetFlag(ctx, "search-v2")
	if err != nil {
		b.Fatalf("failed to retrieve flag: %v", err)
	}

	evalCtx := domain.EvaluationContext{
		UserID:      "usr_user_789",
		Email:       "alex@example.com",
		Environment: domain.EnvProduction,
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		res := engine.EvaluateFlag(*cachedFlag, evalCtx)
		if !res.Enabled && res.Variant == "" {
			b.Fatal("unexpected evaluation result")
		}
	}
}

// BenchmarkInProcessEvaluation_FromSQLiteStore benchmarks flag synced from Embedded SQLite.
func BenchmarkInProcessEvaluation_FromSQLiteStore(b *testing.B) {
	ctx := context.Background()
	tmpFile := "test_bench_sqlite.db"
	defer os.Remove(tmpFile)
	defer os.Remove(tmpFile + "-wal")
	defer os.Remove(tmpFile + "-shm")

	sqliteStore, err := store.NewSQLiteStore(tmpFile)
	if err != nil {
		b.Fatalf("failed to init sqlite store: %v", err)
	}
	defer sqliteStore.Close()

	flag := createBenchmarkSampleFlag("sqlite-flag-v1")
	_, _ = sqliteStore.SaveFlag(ctx, flag, "bench")

	// In Data-Plane SDK architecture, flags are synced from SQLite into memory cache:
	cachedFlag, err := sqliteStore.GetFlag(ctx, "sqlite-flag-v1")
	if err != nil {
		b.Fatalf("failed to retrieve flag: %v", err)
	}

	evalCtx := domain.EvaluationContext{
		UserID:      "usr_sqlite_42",
		Email:       "sqlite@flagura.dev",
		Environment: domain.EnvProduction,
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		res := engine.EvaluateFlag(*cachedFlag, evalCtx)
		if !res.Enabled && res.Variant == "" {
			b.Fatal("unexpected evaluation result")
		}
	}
}

// BenchmarkStorageDirectRead_MemoryStore measures raw retrieval latency from MemoryStore.
func BenchmarkStorageDirectRead_MemoryStore(b *testing.B) {
	ctx := context.Background()
	memStore := store.NewMemoryStore()
	flag := createBenchmarkSampleFlag("direct-mem-flag")
	_, _ = memStore.SaveFlag(ctx, flag, "bench")

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = memStore.GetFlag(ctx, "direct-mem-flag")
	}
}

// BenchmarkStorageDirectRead_SQLiteStore measures raw disk retrieval latency from SQLiteStore with WAL mode.
func BenchmarkStorageDirectRead_SQLiteStore(b *testing.B) {
	ctx := context.Background()
	tmpFile := "test_bench_sqlite_direct.db"
	defer os.Remove(tmpFile)
	defer os.Remove(tmpFile + "-wal")
	defer os.Remove(tmpFile + "-shm")

	sqliteStore, err := store.NewSQLiteStore(tmpFile)
	if err != nil {
		b.Fatalf("failed to init sqlite store: %v", err)
	}
	defer sqliteStore.Close()

	flag := createBenchmarkSampleFlag("direct-sqlite-flag")
	_, _ = sqliteStore.SaveFlag(ctx, flag, "bench")

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = sqliteStore.GetFlag(ctx, "direct-sqlite-flag")
	}
}
