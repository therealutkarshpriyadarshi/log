package reliability

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"
)

var (
	ErrCircuitOpen     = errors.New("circuit breaker is open")
	ErrTooManyRequests = errors.New("too many requests")
)

// State represents the circuit breaker state
type State int

const (
	StateClosed State = iota
	StateOpen
	StateHalfOpen
)

func (s State) String() string {
	switch s {
	case StateClosed:
		return "closed"
	case StateOpen:
		return "open"
	case StateHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

// CircuitBreakerConfig holds configuration for the circuit breaker
type CircuitBreakerConfig struct {
	MaxRequests       uint32
	Interval          time.Duration
	Timeout           time.Duration
	ReadyToTrip       func(counts Counts) bool
	OnStateChange     func(from State, to State)
	IsSuccessful      func(err error) bool
}

// Counts holds the circuit breaker statistics
type Counts struct {
	Requests             uint32
	TotalSuccesses       uint32
	TotalFailures        uint32
	ConsecutiveSuccesses uint32
	ConsecutiveFailures  uint32
}

// CircuitBreaker implements the circuit breaker pattern
type CircuitBreaker struct {
	config CircuitBreakerConfig

	mu          sync.RWMutex
	state       State
	generation  uint64
	counts      Counts
	expiry      time.Time
}

// NewCircuitBreaker creates a new circuit breaker
func NewCircuitBreaker(config CircuitBreakerConfig) *CircuitBreaker {
	if config.MaxRequests == 0 {
		config.MaxRequests = 1
	}

	if config.Interval == 0 {
		config.Interval = 60 * time.Second
	}

	if config.Timeout == 0 {
		config.Timeout = 60 * time.Second
	}

	if config.ReadyToTrip == nil {
		config.ReadyToTrip = defaultReadyToTrip
	}

	if config.IsSuccessful == nil {
		config.IsSuccessful = defaultIsSuccessful
	}

	cb := &CircuitBreaker{
		config: config,
		state:  StateClosed,
		expiry: time.Now().Add(config.Interval),
	}

	return cb
}

// Execute runs the given function if the circuit breaker allows it
func (cb *CircuitBreaker) Execute(ctx context.Context, fn func() error) error {
	generation, err := cb.beforeRequest()
	if err != nil {
		return err
	}

	defer func() {
		if r := recover(); r != nil {
			cb.afterRequest(generation, false)
			panic(r)
		}
	}()

	err = fn()
	cb.afterRequest(generation, cb.config.IsSuccessful(err))

	return err
}

// Call is an alias for Execute for backward compatibility
func (cb *CircuitBreaker) Call(fn func() error) error {
	return cb.Execute(context.Background(), fn)
}

// State returns the current state of the circuit breaker
func (cb *CircuitBreaker) State() State {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	now := time.Now()
	state, _ := cb.currentState(now)
	return state
}

// Counts returns the current counts
func (cb *CircuitBreaker) Counts() Counts {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	return cb.counts
}

// beforeRequest checks if request is allowed
func (cb *CircuitBreaker) beforeRequest() (uint64, error) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	now := time.Now()
	state, generation := cb.currentState(now)

	if state == StateOpen {
		return generation, ErrCircuitOpen
	} else if state == StateHalfOpen && cb.counts.Requests >= cb.config.MaxRequests {
		return generation, ErrTooManyRequests
	}

	cb.counts.Requests++
	return generation, nil
}

// afterRequest records the result of a request
func (cb *CircuitBreaker) afterRequest(before uint64, success bool) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	now := time.Now()
	state, generation := cb.currentState(now)

	if generation != before {
		return
	}

	if success {
		cb.onSuccess(state, now)
	} else {
		cb.onFailure(state, now)
	}
}

// onSuccess handles successful requests
func (cb *CircuitBreaker) onSuccess(state State, now time.Time) {
	switch state {
	case StateClosed:
		cb.counts.TotalSuccesses++
		cb.counts.ConsecutiveSuccesses++
		cb.counts.ConsecutiveFailures = 0
	case StateHalfOpen:
		cb.counts.TotalSuccesses++
		cb.counts.ConsecutiveSuccesses++
		cb.counts.ConsecutiveFailures = 0

		if cb.counts.ConsecutiveSuccesses >= cb.config.MaxRequests {
			cb.setState(StateClosed, now)
		}
	}
}

// onFailure handles failed requests
func (cb *CircuitBreaker) onFailure(state State, now time.Time) {
	switch state {
	case StateClosed:
		cb.counts.TotalFailures++
		cb.counts.ConsecutiveFailures++
		cb.counts.ConsecutiveSuccesses = 0

		if cb.config.ReadyToTrip(cb.counts) {
			cb.setState(StateOpen, now)
		}
	case StateHalfOpen:
		cb.setState(StateOpen, now)
	}
}

// currentState returns the current state
func (cb *CircuitBreaker) currentState(now time.Time) (State, uint64) {
	switch cb.state {
	case StateClosed:
		if !cb.expiry.IsZero() && cb.expiry.Before(now) {
			cb.toNewGeneration(now)
		}
	case StateOpen:
		if cb.expiry.Before(now) {
			cb.setState(StateHalfOpen, now)
		}
	}
	return cb.state, cb.generation
}

// setState changes the state of the circuit breaker
func (cb *CircuitBreaker) setState(state State, now time.Time) {
	if cb.state == state {
		return
	}

	prev := cb.state
	cb.state = state

	cb.toNewGeneration(now)

	if cb.config.OnStateChange != nil {
		cb.config.OnStateChange(prev, state)
	}
}

// toNewGeneration starts a new generation
func (cb *CircuitBreaker) toNewGeneration(now time.Time) {
	cb.generation++
	cb.counts = Counts{}

	var zero time.Time
	switch cb.state {
	case StateClosed:
		if cb.config.Interval == 0 {
			cb.expiry = zero
		} else {
			cb.expiry = now.Add(cb.config.Interval)
		}
	case StateOpen:
		cb.expiry = now.Add(cb.config.Timeout)
	default: // StateHalfOpen
		cb.expiry = zero
	}
}

// Reset resets the circuit breaker to closed state
func (cb *CircuitBreaker) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.toNewGeneration(time.Now())
	cb.state = StateClosed
}

// defaultReadyToTrip returns true when consecutive failures >= 5
func defaultReadyToTrip(counts Counts) bool {
	return counts.ConsecutiveFailures >= 5
}

// defaultIsSuccessful returns true if error is nil
func defaultIsSuccessful(err error) bool {
	return err == nil
}

// Metrics returns circuit breaker statistics
type Metrics struct {
	State                State
	Requests             uint32
	TotalSuccesses       uint32
	TotalFailures        uint32
	ConsecutiveSuccesses uint32
	ConsecutiveFailures  uint32
	ErrorRate            float64
}

// Metrics returns current metrics
func (cb *CircuitBreaker) Metrics() Metrics {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	counts := cb.counts
	var errorRate float64
	if counts.Requests > 0 {
		errorRate = float64(counts.TotalFailures) / float64(counts.Requests) * 100.0
	}

	return Metrics{
		State:                cb.state,
		Requests:             counts.Requests,
		TotalSuccesses:       counts.TotalSuccesses,
		TotalFailures:        counts.TotalFailures,
		ConsecutiveSuccesses: counts.ConsecutiveSuccesses,
		ConsecutiveFailures:  counts.ConsecutiveFailures,
		ErrorRate:            errorRate,
	}
}

// TwoStepCircuitBreaker allows checking and marking separately
type TwoStepCircuitBreaker struct {
	cb *CircuitBreaker
}

// NewTwoStepCircuitBreaker creates a two-step circuit breaker
func NewTwoStepCircuitBreaker(config CircuitBreakerConfig) *TwoStepCircuitBreaker {
	return &TwoStepCircuitBreaker{
		cb: NewCircuitBreaker(config),
	}
}

// Allow checks if a request is allowed
func (tscb *TwoStepCircuitBreaker) Allow() (done func(success bool), err error) {
	generation, err := tscb.cb.beforeRequest()
	if err != nil {
		return nil, err
	}

	return func(success bool) {
		tscb.cb.afterRequest(generation, success)
	}, nil
}

// State returns the current state
func (tscb *TwoStepCircuitBreaker) State() State {
	return tscb.cb.State()
}

// Metrics returns current metrics
func (tscb *TwoStepCircuitBreaker) Metrics() Metrics {
	return tscb.cb.Metrics()
}

// Reset resets the circuit breaker
func (tscb *TwoStepCircuitBreaker) Reset() {
	tscb.cb.Reset()
}

// MultiCircuitBreaker manages multiple circuit breakers
type MultiCircuitBreaker struct {
	breakers map[string]*CircuitBreaker
	mu       sync.RWMutex
}

// NewMultiCircuitBreaker creates a multi-circuit breaker
func NewMultiCircuitBreaker() *MultiCircuitBreaker {
	return &MultiCircuitBreaker{
		breakers: make(map[string]*CircuitBreaker),
	}
}

// GetOrCreate gets or creates a circuit breaker for the given key
func (mcb *MultiCircuitBreaker) GetOrCreate(key string, config CircuitBreakerConfig) *CircuitBreaker {
	mcb.mu.RLock()
	cb, exists := mcb.breakers[key]
	mcb.mu.RUnlock()

	if exists {
		return cb
	}

	mcb.mu.Lock()
	defer mcb.mu.Unlock()

	// Double-check
	cb, exists = mcb.breakers[key]
	if exists {
		return cb
	}

	cb = NewCircuitBreaker(config)
	mcb.breakers[key] = cb
	return cb
}

// Execute executes a function with the circuit breaker for the given key
func (mcb *MultiCircuitBreaker) Execute(ctx context.Context, key string, config CircuitBreakerConfig, fn func() error) error {
	cb := mcb.GetOrCreate(key, config)
	return cb.Execute(ctx, fn)
}

// States returns the states of all circuit breakers
func (mcb *MultiCircuitBreaker) States() map[string]State {
	mcb.mu.RLock()
	defer mcb.mu.RUnlock()

	states := make(map[string]State, len(mcb.breakers))
	for key, cb := range mcb.breakers {
		states[key] = cb.State()
	}
	return states
}

// AllMetrics returns metrics for all circuit breakers
func (mcb *MultiCircuitBreaker) AllMetrics() map[string]Metrics {
	mcb.mu.RLock()
	defer mcb.mu.RUnlock()

	metrics := make(map[string]Metrics, len(mcb.breakers))
	for key, cb := range mcb.breakers {
		metrics[key] = cb.Metrics()
	}
	return metrics
}

// Reset resets all circuit breakers
func (mcb *MultiCircuitBreaker) Reset() {
	mcb.mu.Lock()
	defer mcb.mu.Unlock()

	for _, cb := range mcb.breakers {
		cb.Reset()
	}
}

// rateLimitedCircuitBreaker combines circuit breaker with rate limiting
type rateLimitedCircuitBreaker struct {
	cb         *CircuitBreaker
	maxRate    uint32
	interval   time.Duration
	count      uint32
	lastReset  time.Time
	mu         sync.Mutex
}

// NewRateLimitedCircuitBreaker creates a circuit breaker with rate limiting
func NewRateLimitedCircuitBreaker(config CircuitBreakerConfig, maxRate uint32, interval time.Duration) *rateLimitedCircuitBreaker {
	return &rateLimitedCircuitBreaker{
		cb:        NewCircuitBreaker(config),
		maxRate:   maxRate,
		interval:  interval,
		lastReset: time.Now(),
	}
}

// Execute executes with rate limiting and circuit breaker
func (rlcb *rateLimitedCircuitBreaker) Execute(ctx context.Context, fn func() error) error {
	rlcb.mu.Lock()
	now := time.Now()
	if now.Sub(rlcb.lastReset) >= rlcb.interval {
		rlcb.count = 0
		rlcb.lastReset = now
	}

	if atomic.LoadUint32(&rlcb.count) >= rlcb.maxRate {
		rlcb.mu.Unlock()
		return errors.New("rate limit exceeded")
	}

	atomic.AddUint32(&rlcb.count, 1)
	rlcb.mu.Unlock()

	return rlcb.cb.Execute(ctx, fn)
}
