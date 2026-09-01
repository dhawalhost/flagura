package flagura

import (
	"errors"
	"sync"
	"time"
)

// CircuitBreakerState represents the status of the circuit breaker.
type CircuitBreakerState string

const (
	StateClosed   CircuitBreakerState = "CLOSED"
	StateHalfOpen CircuitBreakerState = "HALF_OPEN"
	StateOpen     CircuitBreakerState = "OPEN"
)

// ErrCircuitBreakerOpen is returned when requests are fast-failed because the remote control plane is unreachable.
var ErrCircuitBreakerOpen = errors.New("circuit breaker is OPEN: Flagura control plane is currently unreachable, falling back to local/cached value")

// CircuitBreaker prevents cascading network timeouts by fast-failing remote evaluations.
type CircuitBreaker struct {
	mu           sync.Mutex
	state        CircuitBreakerState
	failureCount int
	threshold    int
	cooldown     time.Duration
	lastFailure  time.Time
}

// NewCircuitBreaker initializes a circuit breaker with threshold and cooldown.
func NewCircuitBreaker(threshold int, cooldown time.Duration) *CircuitBreaker {
	if threshold <= 0 {
		threshold = 5
	}
	if cooldown <= 0 {
		cooldown = 10 * time.Second
	}
	return &CircuitBreaker{
		state:     StateClosed,
		threshold: threshold,
		cooldown:  cooldown,
	}
}

// Allow checks if a request should proceed or be fast-failed.
func (cb *CircuitBreaker) Allow() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	now := time.Now()

	switch cb.state {
	case StateClosed:
		return true
	case StateOpen:
		if now.Sub(cb.lastFailure) >= cb.cooldown {
			cb.state = StateHalfOpen
			return true
		}
		return false
	case StateHalfOpen:
		return true
	default:
		return true
	}
}

// RecordSuccess records a successful network operation and resets the circuit breaker.
func (cb *CircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.failureCount = 0
	cb.state = StateClosed
}

// RecordFailure records a failed network operation and trips the circuit breaker if threshold is reached.
func (cb *CircuitBreaker) RecordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.failureCount++
	cb.lastFailure = time.Now()

	if cb.failureCount >= cb.threshold {
		cb.state = StateOpen
	}
}

// State returns the current circuit breaker state.
func (cb *CircuitBreaker) State() CircuitBreakerState {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	if cb.state == StateOpen && time.Since(cb.lastFailure) >= cb.cooldown {
		cb.state = StateHalfOpen
	}
	return cb.state
}
