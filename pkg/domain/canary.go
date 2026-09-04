package domain

import (
	"time"
)

// CanaryStatus represents the active lifecycle state of a progressive rollout.
type CanaryStatus string

const (
	CanaryStatusActive     CanaryStatus = "ACTIVE"
	CanaryStatusPaused     CanaryStatus = "PAUSED"
	CanaryStatusCompleted  CanaryStatus = "COMPLETED"
	CanaryStatusRolledBack CanaryStatus = "ROLLED_BACK"
)

// CanaryStage represents a single step in a multi-stage progressive rollout.
type CanaryStage struct {
	Index            int       `json:"index"`
	TargetPercentage float64   `json:"target_percentage"` // e.g. 5, 25, 50, 100
	DurationSec      int64     `json:"duration_sec"`      // Duration to remain at this stage in seconds
	StartedAt        time.Time `json:"started_at"`
	CompletedAt      time.Time `json:"completed_at,omitempty"`
}

// CanaryGuardrails defines automated rollback thresholds.
type CanaryGuardrails struct {
	MaxErrorRatePct float64 `json:"max_error_rate_pct"` // e.g. 1.0 = 1.0% error rate
	MaxP99LatencyMs float64 `json:"max_p99_latency_ms"` // e.g. 200 = 200ms
	AutoRollback    bool    `json:"auto_rollback"`      // If true, automatically reverts to 0%
	AlertWebhookURL string  `json:"alert_webhook_url"`  // Optional APM webhook notification
}

// CanarySchedule holds the full multi-stage schedule and health gate configuration.
type CanarySchedule struct {
	ID              string           `json:"id"`
	FlagKey         string           `json:"flag_key"`
	Environment     Environment      `json:"environment"`
	Status          CanaryStatus     `json:"status"`
	CurrentStageIdx int              `json:"current_stage_idx"`
	Stages          []CanaryStage    `json:"stages"`
	Guardrails      CanaryGuardrails `json:"guardrails"`
	RollbackReason  string           `json:"rollback_reason,omitempty"`
	CreatedAt       time.Time        `json:"created_at"`
	UpdatedAt       time.Time        `json:"updated_at"`
	LastEvaluatedAt time.Time        `json:"last_evaluated_at"`
}

// NextStage advances the schedule to the subsequent stage if time has elapsed.
func (cs *CanarySchedule) NextStage(now time.Time) (bool, *CanaryStage) {
	if cs.Status != CanaryStatusActive || cs.CurrentStageIdx >= len(cs.Stages) {
		return false, nil
	}

	currentStage := &cs.Stages[cs.CurrentStageIdx]
	elapsedSec := now.Sub(currentStage.StartedAt).Seconds()

	if elapsedSec >= float64(currentStage.DurationSec) {
		currentStage.CompletedAt = now
		cs.CurrentStageIdx++
		if cs.CurrentStageIdx >= len(cs.Stages) {
			cs.Status = CanaryStatusCompleted
			cs.UpdatedAt = now
			return false, nil
		}
		next := &cs.Stages[cs.CurrentStageIdx]
		next.StartedAt = now
		cs.UpdatedAt = now
		return true, next
	}

	return false, nil
}
