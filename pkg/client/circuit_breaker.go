package client

import (
	"sync"
	"time"
)

// CircuitState represents the state of the circuit breaker.
type CircuitState string

const (
	// StateClosed allows requests to pass through normally.
	StateClosed CircuitState = "CLOSED"
	// StateOpen fails requests immediately without hitting the network.
	StateOpen CircuitState = "OPEN"
	// StateHalfOpen allows a probe request to check if the upstream server has recovered.
	StateHalfOpen CircuitState = "HALF_OPEN"
)

// CircuitBreaker protects the application by failing fast when the remote Flagura server fails.
type CircuitBreaker struct {
	mu               sync.RWMutex
	state            CircuitState
	failureCount     int
	consecutiveSucc  int
	failureThreshold int
	cooldown         time.Duration
	lastStateChange  time.Time
}

// NewCircuitBreaker creates a new 3-state CircuitBreaker.
func NewCircuitBreaker(threshold int, cooldown time.Duration) *CircuitBreaker {
	if threshold <= 0 {
		threshold = 5
	}
	if cooldown <= 0 {
		cooldown = 10 * time.Second
	}
	return &CircuitBreaker{
		state:            StateClosed,
		failureThreshold: threshold,
		cooldown:         cooldown,
		lastStateChange:  time.Now(),
	}
}

// State returns the current circuit state.
func (cb *CircuitBreaker) State() CircuitState {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	if cb.state == StateOpen && time.Since(cb.lastStateChange) >= cb.cooldown {
		cb.state = StateHalfOpen
		cb.lastStateChange = time.Now()
	}

	return cb.state
}

// Allow reports whether a request is allowed to proceed to the network.
func (cb *CircuitBreaker) Allow() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	if cb.state == StateClosed {
		return true
	}

	if cb.state == StateOpen {
		if time.Since(cb.lastStateChange) >= cb.cooldown {
			cb.state = StateHalfOpen
			cb.lastStateChange = time.Now()
			return true
		}
		return false
	}

	// StateHalfOpen: allow probe request
	return true
}

// OnSuccess records a successful network response.
func (cb *CircuitBreaker) OnSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	if cb.state == StateHalfOpen {
		cb.consecutiveSucc++
		if cb.consecutiveSucc >= 2 {
			cb.state = StateClosed
			cb.failureCount = 0
			cb.consecutiveSucc = 0
			cb.lastStateChange = time.Now()
		}
	} else if cb.state == StateClosed {
		cb.failureCount = 0
	}
}

// OnFailure records a network failure or 5xx server error.
func (cb *CircuitBreaker) OnFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.failureCount++
	cb.consecutiveSucc = 0

	if cb.state == StateHalfOpen || cb.failureCount >= cb.failureThreshold {
		cb.state = StateOpen
		cb.lastStateChange = time.Now()
	}
}
