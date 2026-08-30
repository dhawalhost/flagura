package canary

import (
	"context"
	"testing"
	"time"

	"github.com/dhawalhost/flagura/pkg/domain"
	"github.com/dhawalhost/flagura/pkg/store"
)

type mockBroadcaster struct {
	events []string
}

func (m *mockBroadcaster) Broadcast(event string, payload interface{}) {
	m.events = append(m.events, event)
}

func TestCanaryStageAdvancementHarness(t *testing.T) {
	memStore := store.NewMemoryStore()
	flagKey := "search-v2-canary"
	_, _ = memStore.SaveFlag(context.Background(), domain.FeatureFlag{
		ID:   "flag_canary_01",
		Key:  flagKey,
		Name: "Search V2 Canary",
		Type: "boolean",
		Environments: map[domain.Environment]domain.EnvironmentConfig{
			domain.EnvProduction: {
				Enabled:    true,
				Strategy:   domain.StrategyPercentage,
				Percentage: 0,
			},
		},
	}, "test")

	broadcaster := &mockBroadcaster{}
	scheduler := NewCanaryScheduler(memStore, broadcaster)
	defer scheduler.Close()

	ctx := context.Background()
	startTime := time.Now().UTC()

	// 1. Submit a 3-stage canary schedule:
	// Stage 0: 5% for 100 seconds
	// Stage 1: 25% for 200 seconds
	// Stage 2: 100% for 300 seconds
	sched := domain.CanarySchedule{
		FlagKey:     flagKey,
		Environment: domain.EnvProduction,
		Stages: []domain.CanaryStage{
			{Index: 0, TargetPercentage: 5.0, DurationSec: 100},
			{Index: 1, TargetPercentage: 25.0, DurationSec: 200},
			{Index: 2, TargetPercentage: 100.0, DurationSec: 300},
		},
		Guardrails: domain.CanaryGuardrails{
			MaxErrorRatePct: 1.0,
			AutoRollback:    true,
		},
	}

	submitted, err := scheduler.SubmitSchedule(ctx, sched)
	if err != nil {
		t.Fatalf("failed to submit canary schedule: %v", err)
	}

	// Verify stage 0 applied immediately (5%)
	flag, _ := memStore.GetFlag(ctx, flagKey)
	if flag.Environments[domain.EnvProduction].Percentage != 5.0 {
		t.Fatalf("expected initial 5%% rollout, got %f", flag.Environments[domain.EnvProduction].Percentage)
	}
	if submitted.CurrentStageIdx != 0 {
		t.Fatalf("expected stage 0, got %d", submitted.CurrentStageIdx)
	}

	// 2. Advance simulated time by +50s (Stage 0 still in progress)
	scheduler.EvaluateSchedules(startTime.Add(50 * time.Second))
	flag, _ = memStore.GetFlag(ctx, flagKey)
	if flag.Environments[domain.EnvProduction].Percentage != 5.0 {
		t.Fatalf("expected still 5%% at +50s, got %f", flag.Environments[domain.EnvProduction].Percentage)
	}

	// 3. Advance simulated time by +101s (Stage 0 elapsed, should advance to Stage 1 at 25%)
	advanced := scheduler.EvaluateSchedules(startTime.Add(101 * time.Second))
	if advanced != 1 {
		t.Fatalf("expected 1 schedule advanced, got %d", advanced)
	}

	flag, _ = memStore.GetFlag(ctx, flagKey)
	if flag.Environments[domain.EnvProduction].Percentage != 25.0 {
		t.Fatalf("expected 25%% rollout at Stage 1, got %f", flag.Environments[domain.EnvProduction].Percentage)
	}

	// 4. Advance simulated time past Stage 1 duration (+305s from start) ➔ Stage 2 at 100%
	scheduler.EvaluateSchedules(startTime.Add(305 * time.Second))
	flag, _ = memStore.GetFlag(ctx, flagKey)
	if flag.Environments[domain.EnvProduction].Percentage != 100.0 {
		t.Fatalf("expected 100%% rollout at Stage 2, got %f", flag.Environments[domain.EnvProduction].Percentage)
	}
}

func TestCanaryHealthRollbackTrigger(t *testing.T) {
	memStore := store.NewMemoryStore()
	flagKey := "checkout-v3"
	_, _ = memStore.SaveFlag(context.Background(), domain.FeatureFlag{
		ID:   "flag_canary_02",
		Key:  flagKey,
		Name: "Checkout V3",
		Type: "boolean",
		Environments: map[domain.Environment]domain.EnvironmentConfig{
			domain.EnvProduction: {
				Enabled:    true,
				Strategy:   domain.StrategyPercentage,
				Percentage: 0,
			},
		},
	}, "test")

	scheduler := NewCanaryScheduler(memStore, &mockBroadcaster{})
	defer scheduler.Close()

	ctx := context.Background()
	_, _ = scheduler.SubmitSchedule(ctx, domain.CanarySchedule{
		FlagKey:     flagKey,
		Environment: domain.EnvProduction,
		Stages: []domain.CanaryStage{
			{Index: 0, TargetPercentage: 20.0, DurationSec: 3600},
		},
	})

	// Trigger emergency health rollback
	err := scheduler.TriggerHealthRollback(ctx, flagKey, "APM error rate spiked to 3.2% (> 1.0% threshold)")
	if err != nil {
		t.Fatalf("failed to trigger health rollback: %v", err)
	}

	flag, _ := memStore.GetFlag(ctx, flagKey)
	if flag.Environments[domain.EnvProduction].Percentage != 0.0 {
		t.Fatalf("expected percentage 0.0 after rollback, got %f", flag.Environments[domain.EnvProduction].Percentage)
	}

	sched, ok := scheduler.GetSchedule(flagKey)
	if !ok || sched.Status != domain.CanaryStatusRolledBack {
		t.Fatalf("expected schedule status %s, got %s", domain.CanaryStatusRolledBack, sched.Status)
	}
}
