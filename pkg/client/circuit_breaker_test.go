package client

import (
	"context"
	"testing"
	"time"
)

func TestCircuitBreakerStateTransitions(t *testing.T) {
	cb := NewCircuitBreaker(3, 50*time.Millisecond)

	steps := []struct {
		name          string
		action        func()
		expectedState CircuitState
		expectedAllow bool
	}{
		{
			name:          "Initial state CLOSED",
			action:        func() {},
			expectedState: StateClosed,
			expectedAllow: true,
		},
		{
			name: "1st Failure (Below threshold)",
			action: func() {
				cb.OnFailure()
			},
			expectedState: StateClosed,
			expectedAllow: true,
		},
		{
			name: "2nd Failure (Below threshold)",
			action: func() {
				cb.OnFailure()
			},
			expectedState: StateClosed,
			expectedAllow: true,
		},
		{
			name: "3rd Failure (Trips to OPEN)",
			action: func() {
				cb.OnFailure()
			},
			expectedState: StateOpen,
			expectedAllow: false,
		},
		{
			name: "Cooldown elapsed (Transitions to HALF_OPEN on probe)",
			action: func() {
				time.Sleep(60 * time.Millisecond)
			},
			expectedState: StateHalfOpen,
			expectedAllow: true,
		},
		{
			name: "Success probes reset to CLOSED",
			action: func() {
				cb.OnSuccess()
				cb.OnSuccess()
			},
			expectedState: StateClosed,
			expectedAllow: true,
		},
	}

	for _, step := range steps {
		t.Run(step.name, func(t *testing.T) {
			step.action()
			allow := cb.Allow()
			if allow != step.expectedAllow {
				t.Errorf("cb.Allow() = %v, expected %v", allow, step.expectedAllow)
			}
			if cb.State() != step.expectedState {
				t.Errorf("cb.State() = %s, expected %s", cb.State(), step.expectedState)
			}
		})
	}
}

func TestClientCircuitBreakerFastFailing(t *testing.T) {
	c := New("http://127.0.0.1:59999",
		WithCircuitBreaker(2, 50*time.Millisecond),
	)
	defer c.Close()

	ctx := context.Background()
	evalCtx := Context{UserID: "usr_fail_test"}

	tests := []struct {
		name           string
		attempt        int
		expectedReason string
		expectFastFail bool
	}{
		{name: "First failure (Network error)", attempt: 1, expectedReason: "", expectFastFail: false},
		{name: "Second failure (Trips circuit)", attempt: 2, expectedReason: "", expectFastFail: false},
		{name: "Third attempt (Fast fails with circuit_breaker_open)", attempt: 3, expectedReason: "circuit_breaker_open", expectFastFail: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			start := time.Now()
			res, _ := c.Evaluate(ctx, "test-flag", evalCtx)
			elapsed := time.Since(start)

			if tt.expectFastFail {
				if res.Reason != tt.expectedReason {
					t.Errorf("expected reason %q, got %q", tt.expectedReason, res.Reason)
				}
				if elapsed > 20*time.Millisecond {
					t.Errorf("fast-fail elapsed %v, expected < 20ms", elapsed)
				}
			}
		})
	}
}
