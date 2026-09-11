package canary

import (
	"context"
	"fmt"
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
		ID:        "flag_canary_01",
		ProjectID: store.DefaultProjectID,
		Key:       flagKey,
		Name:      "Search V2 Canary",
		Type:      "boolean",
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
				ID:        "flag_canary_02",
				ProjectID: store.DefaultProjectID,
				Key:       tt.flagKey,
				Name:      "Checkout V3",
				Type:      "boolean",
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

			err := scheduler.TriggerHealthRollback(ctx, store.DefaultProjectID, tt.flagKey, tt.reason)
			if err != nil {
				t.Fatalf("failed to trigger health rollback: %v", err)
			}

			flag, _ := memStore.GetFlag(ctx, tt.flagKey)
			if flag.Environments[domain.EnvProduction].Percentage != tt.expectedPct {
				t.Errorf("expected percentage %f after rollback, got %f", tt.expectedPct, flag.Environments[domain.EnvProduction].Percentage)
			}

			sched, ok := scheduler.GetSchedule(store.DefaultProjectID, tt.flagKey)
			if !ok || sched.Status != tt.expectedStatus {
				t.Errorf("expected schedule status %s, got %s", tt.expectedStatus, sched.Status)
			}
		})
	}
}

func TestCanaryScheduler_EdgeCases(t *testing.T) {
	memStore := store.NewMemoryStore()
	scheduler := NewCanaryScheduler(memStore, &mockBroadcaster{})
	defer scheduler.Close()
	ctx := context.Background()

	// 1. Submit empty stages error
	_, err := scheduler.SubmitSchedule(ctx, domain.CanarySchedule{
		FlagKey: "empty-stages",
		Stages:  nil,
	})
	if err == nil {
		t.Errorf("expected error submitting schedule without stages")
	}

	// 2. Get non-existent schedule
	if s, ok := scheduler.GetSchedule(store.DefaultProjectID, "non-existent"); ok || s != nil {
		t.Errorf("expected nil for non-existent schedule")
	}

	// 3. Cancel non-existent schedule
	if ok := scheduler.CancelSchedule(store.DefaultProjectID, "non-existent"); ok {
		t.Errorf("expected false when cancelling non-existent schedule")
	}

	// 4. Trigger rollback on non-existent schedule
	if err := scheduler.TriggerHealthRollback(ctx, store.DefaultProjectID, "non-existent", "test"); err == nil {
		t.Errorf("expected error rolling back non-existent schedule")
	}

	// 5. Submit valid schedule and cancel it
	_, _ = scheduler.SubmitSchedule(ctx, domain.CanarySchedule{
		FlagKey: "test-cancel",
		Stages: []domain.CanaryStage{
			{Index: 0, TargetPercentage: 10, DurationSec: 100},
		},
	})
	if ok := scheduler.CancelSchedule(store.DefaultProjectID, "test-cancel"); !ok {
		t.Errorf("expected true when cancelling existing schedule")
	}

	// 6. Test StartBackgroundLoop
	scheduler.StartBackgroundLoop(5 * time.Millisecond)
	time.Sleep(15 * time.Millisecond)
}

func TestCanaryScheduler_MultiTenantIsolation(t *testing.T) {
	memStore := store.NewMemoryStore()
	scheduler := NewCanaryScheduler(memStore, &mockBroadcaster{})
	defer scheduler.Close()
	ctx := context.Background()

	projA := "proj_alpha"
	projB := "proj_beta"
	flagKey := "shared-flag-name"

	// Create flag in both projects
	_, _ = memStore.SaveFlag(ctx, domain.FeatureFlag{
		ID:        "f_alpha",
		ProjectID: projA,
		Key:       flagKey,
		Environments: map[domain.Environment]domain.EnvironmentConfig{
			domain.EnvProduction: {Enabled: true, Percentage: 0},
		},
	}, "test")
	_, _ = memStore.SaveFlag(ctx, domain.FeatureFlag{
		ID:        "f_beta",
		ProjectID: projB,
		Key:       flagKey,
		Environments: map[domain.Environment]domain.EnvironmentConfig{
			domain.EnvProduction: {Enabled: true, Percentage: 0},
		},
	}, "test")

	// Submit schedule for projA only
	_, err := scheduler.SubmitSchedule(ctx, domain.CanarySchedule{
		ProjectID:   projA,
		FlagKey:     flagKey,
		Environment: domain.EnvProduction,
		Stages: []domain.CanaryStage{
			{Index: 0, TargetPercentage: 20, DurationSec: 100},
		},
	})
	if err != nil {
		t.Fatalf("SubmitSchedule failed: %v", err)
	}

	// projA should have active schedule, projB should have none
	schedA, okA := scheduler.GetSchedule(projA, flagKey)
	if !okA || schedA == nil {
		t.Errorf("expected schedule in projA")
	}
	schedB, okB := scheduler.GetSchedule(projB, flagKey)
	if okB || schedB != nil {
		t.Errorf("expected NO schedule in projB")
	}

	// Verify projA flag was updated, projB flag untouched
	flagA, _ := memStore.GetFlagByProject(ctx, projA, flagKey)
	if flagA.Environments[domain.EnvProduction].Percentage != 20 {
		t.Errorf("expected projA percentage 20, got %f", flagA.Environments[domain.EnvProduction].Percentage)
	}
	flagB, _ := memStore.GetFlagByProject(ctx, projB, flagKey)
	if flagB.Environments[domain.EnvProduction].Percentage != 0 {
		t.Errorf("expected projB percentage 0, got %f", flagB.Environments[domain.EnvProduction].Percentage)
	}
}

type faultyRolloutStore struct {
	store.Store
	failRollout bool
}

func (f *faultyRolloutStore) UpdateRolloutByProject(ctx context.Context, projectID, key string, env domain.Environment, percentage float64, actor string) (*domain.FeatureFlag, *domain.AuditLogEntry, error) {
	if f.failRollout {
		return nil, nil, fmt.Errorf("simulated database connection error during rollout write")
	}
	return f.Store.UpdateRolloutByProject(ctx, projectID, key, env, percentage, actor)
}

func TestCanaryScheduler_PersistenceFailures(t *testing.T) {
	ctx := context.Background()
	memStore := store.NewMemoryStore()
	faulty := &faultyRolloutStore{Store: memStore}
	broadcaster := &mockBroadcaster{}
	scheduler := NewCanaryScheduler(faulty, broadcaster)
	defer scheduler.Close()

	flagKey := "canary-fault-test"
	now := time.Now().UTC()

	// Seed flag
	_, err := memStore.SaveFlag(ctx, domain.FeatureFlag{
		Key:       flagKey,
		ProjectID: store.DefaultProjectID,
		Environments: map[domain.Environment]domain.EnvironmentConfig{
			domain.EnvProduction: {Enabled: true, Percentage: 0},
		},
	}, "test")
	if err != nil {
		t.Fatalf("SaveFlag failed: %v", err)
	}

	// 1. Submit valid schedule while store is healthy
	sched, err := scheduler.SubmitSchedule(ctx, domain.CanarySchedule{
		FlagKey:     flagKey,
		ProjectID:   store.DefaultProjectID,
		Environment: domain.EnvProduction,
		Stages: []domain.CanaryStage{
			{Index: 0, TargetPercentage: 10, DurationSec: 100, StartedAt: now},
			{Index: 1, TargetPercentage: 50, DurationSec: 100},
		},
	})
	if err != nil {
		t.Fatalf("SubmitSchedule failed: %v", err)
	}
	if sched.Status != domain.CanaryStatusActive {
		t.Fatalf("expected ACTIVE status, got %s", sched.Status)
	}

	// 2. Simulate database failure during TriggerHealthRollback
	faulty.failRollout = true
	rollbackErr := scheduler.TriggerHealthRollback(ctx, store.DefaultProjectID, flagKey, "APM breach")
	if rollbackErr == nil {
		t.Fatalf("expected error from TriggerHealthRollback when store write fails")
	}

	// The in-memory schedule MUST NOT be marked as ROLLED_BACK!
	currentSched, ok := scheduler.GetSchedule(store.DefaultProjectID, flagKey)
	if !ok || currentSched == nil {
		t.Fatalf("schedule missing")
	}
	if currentSched.Status == domain.CanaryStatusRolledBack {
		t.Fatalf("BUG: schedule was marked ROLLED_BACK even though database write failed!")
	}
	if currentSched.Status != domain.CanaryStatusNeedsAttention {
		t.Fatalf("expected status NEEDS_ATTENTION, got: %s", currentSched.Status)
	}

	// 3. Reset schedule to ACTIVE and test EvaluateSchedules with store failure
	scheduler.mu.Lock()
	scheduler.schedules[scheduleKey(store.DefaultProjectID, flagKey)].Status = domain.CanaryStatusActive
	scheduler.schedules[scheduleKey(store.DefaultProjectID, flagKey)].Stages[0].StartedAt = now.Add(-150 * time.Second)
	scheduler.mu.Unlock()

	// Evaluate schedules when stage duration has elapsed but store write fails
	adv := scheduler.EvaluateSchedules(now)
	if adv != 0 {
		t.Fatalf("expected 0 advanced stages on write failure, got %d", adv)
	}

	// Stage index must NOT have moved on write failure
	postEvalSched, _ := scheduler.GetSchedule(store.DefaultProjectID, flagKey)
	if postEvalSched.CurrentStageIdx != 0 {
		t.Fatalf("BUG: CurrentStageIdx advanced to %d despite store write failure!", postEvalSched.CurrentStageIdx)
	}
	if postEvalSched.Status != domain.CanaryStatusNeedsAttention {
		t.Fatalf("expected status NEEDS_ATTENTION on advancement failure, got: %s", postEvalSched.Status)
	}

	// 4. Restore store health: advancement should now succeed
	faulty.failRollout = false
	scheduler.mu.Lock()
	scheduler.schedules[scheduleKey(store.DefaultProjectID, flagKey)].Status = domain.CanaryStatusActive
	scheduler.mu.Unlock()

	advSuccess := scheduler.EvaluateSchedules(now)
	if advSuccess != 1 {
		t.Fatalf("expected 1 advanced stage after store recovery, got %d", advSuccess)
	}

	recoveredSched, _ := scheduler.GetSchedule(store.DefaultProjectID, flagKey)
	if recoveredSched.CurrentStageIdx != 1 {
		t.Fatalf("expected CurrentStageIdx to be 1, got %d", recoveredSched.CurrentStageIdx)
	}
}
