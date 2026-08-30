package client_test

import (
	"context"
	"testing"
	"time"

	"github.com/dhawalhost/flagura/pkg/client"
)

func TestCircuitBreakerStateTransitions(t *testing.T) {
	// Threshold: 3 failures, Cooldown: 50ms
	cb := client.NewCircuitBreaker(3, 50*time.Millisecond)

	// 1. Initial State must be Closed
	if cb.State() != client.StateClosed {
		t.Fatalf("expected initial state CLOSED, got %s", cb.State())
	}
	if !cb.Allow() {
		t.Fatalf("expected Allow() to be true in CLOSED state")
	}

	// 2. Trigger failures below threshold
	cb.OnFailure()
	cb.OnFailure()
	if cb.State() != client.StateClosed {
		t.Fatalf("expected state CLOSED after 2 failures, got %s", cb.State())
	}

	// 3. 3rd failure trips circuit to OPEN
	cb.OnFailure()
	if cb.State() != client.StateOpen {
		t.Fatalf("expected state OPEN after 3 failures, got %s", cb.State())
	}
	if cb.Allow() {
		t.Fatalf("expected Allow() to be false immediately after tripping to OPEN")
	}

	// 4. Wait for cooldown period (50ms) -> transitions to HALF_OPEN
	time.Sleep(60 * time.Millisecond)
	if !cb.Allow() {
		t.Fatalf("expected Allow() to be true after cooldown in HALF_OPEN state")
	}
	if cb.State() != client.StateHalfOpen {
		t.Fatalf("expected state HALF_OPEN, got %s", cb.State())
	}

	// 5. Successful probe requests reset circuit to CLOSED
	cb.OnSuccess()
	cb.OnSuccess()
	if cb.State() != client.StateClosed {
		t.Fatalf("expected state CLOSED after successful probes, got %s", cb.State())
	}
}

func TestClientCircuitBreakerFastFailing(t *testing.T) {
	// Point to dead port with short timeout and low threshold
	c := client.New("http://127.0.0.1:59999",
		client.WithCircuitBreaker(2, 50*time.Millisecond),
	)
	defer c.Close()

	ctx := context.Background()
	evalCtx := client.Context{UserID: "usr_fail_test"}

	// Request 1 fails (network error)
	res1, _ := c.Evaluate(ctx, "test-flag", evalCtx)
	if res1.Reason == "circuit_breaker_open" {
		t.Errorf("request 1 should fail with network error, not circuit breaker")
	}

	// Request 2 fails (network error, trips circuit)
	res2, _ := c.Evaluate(ctx, "test-flag", evalCtx)
	if res2.Reason == "circuit_breaker_open" {
		t.Errorf("request 2 should trigger the trip, not yet open before request")
	}

	// Circuit should now be OPEN
	if c.CircuitBreakerState() != client.StateOpen {
		t.Fatalf("expected client circuit breaker state OPEN, got %s", c.CircuitBreakerState())
	}

	// Request 3 should fast-fail IMMEDIATELY without network delay
	start := time.Now()
	res3, err := c.Evaluate(ctx, "test-flag", evalCtx)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatalf("expected error on fast-failing circuit breaker request")
	}
	if res3.Reason != "circuit_breaker_open" {
		t.Fatalf("expected reason 'circuit_breaker_open', got '%s'", res3.Reason)
	}
	if elapsed > 20*time.Millisecond {
		t.Fatalf("fast-fail took too long: %v (expected < 20ms)", elapsed)
	}
}
