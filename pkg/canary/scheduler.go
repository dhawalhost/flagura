package canary

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/dhawalhost/flagura/pkg/domain"
	"github.com/dhawalhost/flagura/pkg/store"
)

// StreamBroadcaster interface allows the scheduler to broadcast SSE updates.
type StreamBroadcaster interface {
	Broadcast(event string, payload interface{})
}

// CanaryScheduler manages progressive canary schedules and automated health gates.
type CanaryScheduler struct {
	mu          sync.RWMutex
	schedules   map[string]*domain.CanarySchedule // key: flagKey
	store       store.Store
	broadcaster StreamBroadcaster
	stopCh      chan struct{}
	closeOnce   sync.Once
}

// NewCanaryScheduler creates a new thread-safe CanaryScheduler.
func NewCanaryScheduler(st store.Store, b StreamBroadcaster) *CanaryScheduler {
	return &CanaryScheduler{
		schedules:   make(map[string]*domain.CanarySchedule),
		store:       st,
		broadcaster: b,
		stopCh:      make(chan struct{}),
	}
}

// StartBackgroundLoop begins periodic evaluation of active canary schedules.
func (cs *CanaryScheduler) StartBackgroundLoop(interval time.Duration) {
	if interval <= 0 {
		interval = 15 * time.Second
	}
	ticker := time.NewTicker(interval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				cs.EvaluateSchedules(time.Now().UTC())
			case <-cs.stopCh:
				return
			}
		}
	}()
}

// SubmitSchedule registers or replaces an active canary schedule for a flag.
func (cs *CanaryScheduler) SubmitSchedule(ctx context.Context, sched domain.CanarySchedule) (*domain.CanarySchedule, error) {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	now := time.Now().UTC()
	if sched.ID == "" {
		sched.ID = fmt.Sprintf("canary_%s_%d", sched.FlagKey, now.Unix())
	}
	if sched.Environment == "" {
		sched.Environment = domain.EnvProduction
	}
	if len(sched.Stages) == 0 {
		return nil, fmt.Errorf("canary schedule must contain at least 1 stage")
	}

	sched.Status = domain.CanaryStatusActive
	sched.CurrentStageIdx = 0
	sched.CreatedAt = now
	sched.UpdatedAt = now
	sched.LastEvaluatedAt = now
	sched.Stages[0].StartedAt = now

	cs.schedules[sched.FlagKey] = &sched

	// Apply initial stage rollout immediately
	initialPct := sched.Stages[0].TargetPercentage
	_, _, err := cs.store.UpdateRollout(ctx, sched.FlagKey, sched.Environment, initialPct, "canary-scheduler-auto")
	if err != nil {
		return nil, fmt.Errorf("failed to apply initial canary rollout percentage: %w", err)
	}

	if cs.broadcaster != nil {
		cs.broadcaster.Broadcast("canary_update", sched)
	}

	return &sched, nil
}

// GetSchedule returns the active schedule for a flag.
func (cs *CanaryScheduler) GetSchedule(flagKey string) (*domain.CanarySchedule, bool) {
	cs.mu.RLock()
	defer cs.mu.RUnlock()

	sched, ok := cs.schedules[flagKey]
	if !ok {
		return nil, false
	}
	copySched := *sched
	return &copySched, true
}

// CancelSchedule halts and removes an active canary schedule.
func (cs *CanaryScheduler) CancelSchedule(flagKey string) bool {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	if sched, ok := cs.schedules[flagKey]; ok {
		sched.Status = domain.CanaryStatusPaused
		delete(cs.schedules, flagKey)
		return true
	}
	return false
}

// EvaluateSchedules checks all active schedules for stage advancement or health-gate rollbacks.
func (cs *CanaryScheduler) EvaluateSchedules(now time.Time) int {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	advancedCount := 0
	ctx := context.Background()

	for flagKey, sched := range cs.schedules {
		if sched.Status != domain.CanaryStatusActive {
			continue
		}

		sched.LastEvaluatedAt = now

		// 1. Advance stage if duration has elapsed
		advanced, nextStage := sched.NextStage(now)
		if advanced && nextStage != nil {
			_, _, err := cs.store.UpdateRollout(ctx, flagKey, sched.Environment, nextStage.TargetPercentage, "canary-scheduler-auto")
			if err == nil {
				advancedCount++
				if cs.broadcaster != nil {
					cs.broadcaster.Broadcast("canary_stage_advanced", map[string]interface{}{
						"flag_key":          flagKey,
						"stage_index":       sched.CurrentStageIdx,
						"target_percentage": nextStage.TargetPercentage,
					})
				}
			}
		}

		// 2. If all stages completed, mark completed
		if sched.Status == domain.CanaryStatusCompleted {
			if cs.broadcaster != nil {
				cs.broadcaster.Broadcast("canary_completed", map[string]interface{}{
					"flag_key": flagKey,
				})
			}
		}
	}

	return advancedCount
}

// TriggerHealthRollback automatically rolls back an active canary if external APM reports a breach.
func (cs *CanaryScheduler) TriggerHealthRollback(ctx context.Context, flagKey, reason string) error {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	sched, ok := cs.schedules[flagKey]
	if !ok {
		return fmt.Errorf("no active canary schedule for flag %s", flagKey)
	}

	sched.Status = domain.CanaryStatusRolledBack
	sched.RollbackReason = reason
	sched.UpdatedAt = time.Now().UTC()

	// Rollback percentage to 0%
	_, _, err := cs.store.UpdateRollout(ctx, flagKey, sched.Environment, 0.0, "canary-health-rollback")
	if err != nil {
		return fmt.Errorf("failed to revert rollout to 0%%: %w", err)
	}

	if cs.broadcaster != nil {
		cs.broadcaster.Broadcast("canary_rollback", map[string]interface{}{
			"flag_key": flagKey,
			"reason":   reason,
		})
	}

	return nil
}

// Close terminates background goroutines.
func (cs *CanaryScheduler) Close() {
	cs.closeOnce.Do(func() {
		close(cs.stopCh)
	})
}
