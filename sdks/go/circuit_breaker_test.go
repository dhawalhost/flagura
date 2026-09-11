package flagura

import (
	"sync"
	"testing"
	"time"
)

func TestCircuitBreaker_HalfOpenTrialLimitAndTrip(t *testing.T) {
	threshold := 2
	cooldown := 50 * time.Millisecond
	cb := NewCircuitBreaker(threshold, cooldown)

	if !cb.Allow() {
		t.Fatalf("expected initial state CLOSED to allow requests")
	}

	// 1. Trip breaker to OPEN
	cb.RecordFailure()
	cb.RecordFailure()

	if cb.State() != StateOpen {
		t.Fatalf("expected state OPEN after %d failures, got %s", threshold, cb.State())
	}
	if cb.Allow() {
		t.Fatalf("expected OPEN state to reject requests")
	}

	// 2. Wait for cooldown to expire
	time.Sleep(cooldown + 10*time.Millisecond)

	// 3. First request in HalfOpen should be allowed as trial
	if !cb.Allow() {
		t.Fatalf("expected first request after cooldown to be allowed as trial")
	}
	if cb.State() != StateHalfOpen {
		t.Fatalf("expected state HALF_OPEN, got %s", cb.State())
	}

	// 4. Concurrent calls while trial is in flight should fail fast (not thundering herd)
	var wg sync.WaitGroup
	allowedCount := 0
	var mu sync.Mutex

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if cb.Allow() {
				mu.Lock()
				allowedCount++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	if allowedCount != 0 {
		t.Fatalf("expected 0 concurrent requests allowed while trial in flight, got %d", allowedCount)
	}

	// 5. If trial request fails, immediately trip back to OPEN
	cb.RecordFailure()
	if cb.State() != StateOpen {
		t.Fatalf("expected state to immediately trip to OPEN after trial failure, got %s", cb.State())
	}

	// 6. Test trial success transitions to CLOSED
	time.Sleep(cooldown + 10*time.Millisecond)
	if !cb.Allow() {
		t.Fatalf("expected trial request to be allowed after second cooldown")
	}
	cb.RecordSuccess()
	if cb.State() != StateClosed {
		t.Fatalf("expected state CLOSED after trial success, got %s", cb.State())
	}
	if !cb.Allow() {
		t.Fatalf("expected CLOSED state to allow requests")
	}
}
