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

	if submitted.CurrentStageIdx != 0 {
		t.Fatalf("expected stage 0, got %d", submitted.CurrentStageIdx)
	}

	timelineSteps := []struct {
		name               string
		elapsedOffset      time.Duration
		expectedPercentage float64
		expectedStageIdx   int
	}{
		{
			name:               "Immediate Stage 0 initialization",
			elapsedOffset:      0,
			expectedPercentage: 5.0,
			expectedStageIdx:   0,
		},
		{
			name:               "Mid-flight Stage 0 duration (+50s)",
			elapsedOffset:      50 * time.Second,
			expectedPercentage: 5.0,
			expectedStageIdx:   0,
		},
		{
			name:               "Elapsed Stage 0 (+101s) -> Advance to Stage 1 (25%)",
			elapsedOffset:      101 * time.Second,
			expectedPercentage: 25.0,
			expectedStageIdx:   1,
		},
		{
			name:               "Elapsed Stage 1 (+305s) -> Advance to Stage 2 (100%)",
			elapsedOffset:      305 * time.Second,
			expectedPercentage: 100.0,
			expectedStageIdx:   2,
		},
	}

	for _, step := range timelineSteps {
		t.Run(step.name, func(t *testing.T) {
			if step.elapsedOffset > 0 {
				scheduler.EvaluateSchedules(startTime.Add(step.elapsedOffset))
			}
			flag, _ := memStore.GetFlag(ctx, flagKey)
			actualPct := flag.Environments[domain.EnvProduction].Percentage
			if actualPct != step.expectedPercentage {
				t.Errorf("Percentage = %f, expected %f", actualPct, step.expectedPercentage)
			}
		})
	}
}

func TestCanaryHealthRollbackTrigger(t *testing.T) {
	tests := []struct {
		name           string
		flagKey        string
		initialPct     float64
		reason         string
		expectedPct    float64
		expectedStatus domain.CanaryStatus
	}{
		{
			name:           "APM error spike triggers emergency 0% rollback",
			flagKey:        "checkout-v3",
			initialPct:     20.0,
			reason:         "APM error rate spiked to 3.2% (> 1.0% threshold)",
			expectedPct:    0.0,
			expectedStatus: domain.CanaryStatusRolledBack,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			memStore := store.NewMemoryStore()
			_, _ = memStore.SaveFlag(context.Background(), domain.FeatureFlag{
				ID:   "flag_canary_02",
				Key:  tt.flagKey,
				Name: "Checkout V3",
				Type: "boolean",
				Environments: map[domain.Environment]domain.EnvironmentConfig{
					domain.EnvProduction: {
						Enabled:    true,
						Strategy:   domain.StrategyPercentage,
						Percentage: tt.initialPct,
					},
				},
			}, "test")

			scheduler := NewCanaryScheduler(memStore, &mockBroadcaster{})
			defer scheduler.Close()

			ctx := context.Background()
			_, _ = scheduler.SubmitSchedule(ctx, domain.CanarySchedule{
				FlagKey:     tt.flagKey,
				Environment: domain.EnvProduction,
				Stages: []domain.CanaryStage{
					{Index: 0, TargetPercentage: tt.initialPct, DurationSec: 3600},
				},
			})

			err := scheduler.TriggerHealthRollback(ctx, tt.flagKey, tt.reason)
			if err != nil {
				t.Fatalf("failed to trigger health rollback: %v", err)
			}

			flag, _ := memStore.GetFlag(ctx, tt.flagKey)
			if flag.Environments[domain.EnvProduction].Percentage != tt.expectedPct {
				t.Errorf("expected percentage %f after rollback, got %f", tt.expectedPct, flag.Environments[domain.EnvProduction].Percentage)
			}

			sched, ok := scheduler.GetSchedule(tt.flagKey)
			if !ok || sched.Status != tt.expectedStatus {
				t.Errorf("expected schedule status %s, got %s", tt.expectedStatus, sched.Status)
			}
		})
	}
}
