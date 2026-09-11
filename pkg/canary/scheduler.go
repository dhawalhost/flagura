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

func scheduleKey(projectID, flagKey string) string {
	if projectID == "" {
		projectID = store.DefaultProjectID
	}
	return projectID + ":" + flagKey
}

// SubmitSchedule registers or replaces an active canary schedule for a flag.
func (cs *CanaryScheduler) SubmitSchedule(ctx context.Context, sched domain.CanarySchedule) (*domain.CanarySchedule, error) {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	now := time.Now().UTC()
	if sched.ID == "" {
		sched.ID = fmt.Sprintf("canary_%s_%d", sched.FlagKey, now.Unix())
	}
	if sched.ProjectID == "" {
		sched.ProjectID = store.DefaultProjectID
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

	key := scheduleKey(sched.ProjectID, sched.FlagKey)
	cs.schedules[key] = &sched

	// Apply initial stage rollout immediately
	initialPct := sched.Stages[0].TargetPercentage
	_, _, err := cs.store.UpdateRolloutByProject(ctx, sched.ProjectID, sched.FlagKey, sched.Environment, initialPct, "canary-scheduler-auto")
	if err != nil {
		return nil, fmt.Errorf("failed to apply initial canary rollout percentage: %w", err)
	}

	if cs.broadcaster != nil {
		cs.broadcaster.Broadcast("canary_update", sched)
	}

	return &sched, nil
}

// GetSchedule returns the active schedule for a flag in a project.
func (cs *CanaryScheduler) GetSchedule(projectID, flagKey string) (*domain.CanarySchedule, bool) {
	cs.mu.RLock()
	defer cs.mu.RUnlock()

	sched, ok := cs.schedules[scheduleKey(projectID, flagKey)]
	if !ok {
		return nil, false
	}
	copySched := *sched
	return &copySched, true
}

// CancelSchedule halts and removes an active canary schedule.
func (cs *CanaryScheduler) CancelSchedule(projectID, flagKey string) bool {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	key := scheduleKey(projectID, flagKey)
	if sched, ok := cs.schedules[key]; ok {
		sched.Status = domain.CanaryStatusPaused
		delete(cs.schedules, key)
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

	for _, sched := range cs.schedules {
		if sched.Status != domain.CanaryStatusActive {
			continue
		}

		sched.LastEvaluatedAt = now

		// 1. Check if stage can advance without mutating state prematurely
		canAdvance, nextStage, isCompletion := sched.CanAdvanceStage(now)
		if canAdvance {
			if isCompletion {
				sched.CommitStageAdvancement(now)
				if cs.broadcaster != nil {
					cs.broadcaster.Broadcast("canary_completed", map[string]interface{}{
						"project_id": sched.ProjectID,
						"flag_key":   sched.FlagKey,
					})
				}
			} else if nextStage != nil {
				// Persist rollout update FIRST
				_, _, err := cs.store.UpdateRolloutByProject(ctx, sched.ProjectID, sched.FlagKey, sched.Environment, nextStage.TargetPercentage, "canary-scheduler-auto")
				if err != nil {
					// Persistence failed: do not advance stage, mark NEEDS_ATTENTION
					sched.Status = domain.CanaryStatusNeedsAttention
					sched.RollbackReason = fmt.Sprintf("stage advancement failed to persist: %v", err)
					sched.UpdatedAt = now
				} else {
					// Persistence confirmed: advance in-memory stage
					sched.CommitStageAdvancement(now)
					advancedCount++
					if cs.broadcaster != nil {
						cs.broadcaster.Broadcast("canary_stage_advanced", map[string]interface{}{
							"project_id":        sched.ProjectID,
							"flag_key":          sched.FlagKey,
							"stage_index":       sched.CurrentStageIdx,
							"target_percentage": nextStage.TargetPercentage,
						})
					}
				}
			}
		}
	}

	return advancedCount
}

// TriggerHealthRollback automatically rolls back an active canary if external APM reports a breach.
func (cs *CanaryScheduler) TriggerHealthRollback(ctx context.Context, projectID, flagKey, reason string) error {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	key := scheduleKey(projectID, flagKey)
	sched, ok := cs.schedules[key]
	if !ok {
		return fmt.Errorf("no active canary schedule for flag %s in project %s", flagKey, projectID)
	}

	now := time.Now().UTC()

	// 1. Rollback percentage to 0% in persistence layer FIRST
	_, _, err := cs.store.UpdateRolloutByProject(ctx, sched.ProjectID, sched.FlagKey, sched.Environment, 0.0, "canary-health-rollback")
	if err != nil {
		sched.Status = domain.CanaryStatusNeedsAttention
		sched.RollbackReason = fmt.Sprintf("rollback failed to persist: %v", err)
		sched.UpdatedAt = now
		return fmt.Errorf("failed to revert rollout to 0%%: %w", err)
	}

	// 2. Only mutate in-memory schedule AFTER underlying store write succeeds
	sched.Status = domain.CanaryStatusRolledBack
	sched.RollbackReason = reason
	sched.UpdatedAt = now

	if cs.broadcaster != nil {
		cs.broadcaster.Broadcast("canary_rollback", map[string]interface{}{
			"project_id": sched.ProjectID,
			"flag_key":   sched.FlagKey,
			"reason":     reason,
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
