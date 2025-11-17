package reliability

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestCircuitBreaker_ClosedState(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{
		MaxRequests: 3,
		Interval:    time.Second,
		Timeout:     time.Second,
	})

	// Should start in closed state
	if cb.State() != StateClosed {
		t.Errorf("initial state = %v, want %v", cb.State(), StateClosed)
	}

	// Execute successful requests
	for i := 0; i < 5; i++ {
		err := cb.Execute(context.Background(), func() error {
			return nil
		})
		if err != nil {
			t.Errorf("Execute() error = %v", err)
		}
	}

	// Should still be closed
	if cb.State() != StateClosed {
		t.Errorf("state = %v, want %v", cb.State(), StateClosed)
	}
}

func TestCircuitBreaker_OpenState(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{
		MaxRequests: 3,
		Interval:    time.Second,
		Timeout:     100 * time.Millisecond,
		ReadyToTrip: func(counts Counts) bool {
			return counts.ConsecutiveFailures >= 3
		},
	})

	// Trigger failures to open the circuit
	for i := 0; i < 3; i++ {
		_ = cb.Execute(context.Background(), func() error {
			return errors.New("error")
		})
	}

	// Should be open now
	if cb.State() != StateOpen {
		t.Errorf("state = %v, want %v", cb.State(), StateOpen)
	}

	// Request should fail immediately
	err := cb.Execute(context.Background(), func() error {
		t.Error("should not execute when circuit is open")
		return nil
	})

	if !errors.Is(err, ErrCircuitOpen) {
		t.Errorf("expected ErrCircuitOpen, got %v", err)
	}
}

func TestCircuitBreaker_HalfOpenState(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{
		MaxRequests: 2,
		Interval:    time.Second,
		Timeout:     100 * time.Millisecond,
		ReadyToTrip: func(counts Counts) bool {
			return counts.ConsecutiveFailures >= 2
		},
	})

	// Open the circuit
	for i := 0; i < 2; i++ {
		_ = cb.Execute(context.Background(), func() error {
			return errors.New("error")
		})
	}

	if cb.State() != StateOpen {
		t.Fatalf("state = %v, want %v", cb.State(), StateOpen)
	}

	// Wait for timeout
	time.Sleep(150 * time.Millisecond)

	// Should transition to half-open
	// Execute to trigger state check
	_ = cb.Execute(context.Background(), func() error {
		return nil
	})

	if cb.State() != StateHalfOpen && cb.State() != StateClosed {
		t.Errorf("state = %v, want half-open or closed", cb.State())
	}
}

func TestCircuitBreaker_HalfOpenToClosed(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{
		MaxRequests: 2,
		Interval:    time.Second,
		Timeout:     50 * time.Millisecond,
		ReadyToTrip: func(counts Counts) bool {
			return counts.ConsecutiveFailures >= 2
		},
	})

	// Open the circuit
	for i := 0; i < 2; i++ {
		_ = cb.Execute(context.Background(), func() error {
			return errors.New("error")
		})
	}

	// Wait for timeout to enter half-open
	time.Sleep(100 * time.Millisecond)

	// Succeed enough times to close
	for i := 0; i < 2; i++ {
		err := cb.Execute(context.Background(), func() error {
			return nil
		})
		if err != nil {
			t.Errorf("Execute() error = %v", err)
		}
	}

	// Should be closed now
	if cb.State() != StateClosed {
		t.Errorf("state = %v, want %v", cb.State(), StateClosed)
	}
}

func TestCircuitBreaker_HalfOpenToOpen(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{
		MaxRequests: 3,
		Interval:    time.Second,
		Timeout:     50 * time.Millisecond,
		ReadyToTrip: func(counts Counts) bool {
			return counts.ConsecutiveFailures >= 2
		},
	})

	// Open the circuit
	for i := 0; i < 2; i++ {
		_ = cb.Execute(context.Background(), func() error {
			return errors.New("error")
		})
	}

	// Wait for timeout
	time.Sleep(100 * time.Millisecond)

	// Fail again to reopen
	_ = cb.Execute(context.Background(), func() error {
		return errors.New("error")
	})

	// Should be open again
	if cb.State() != StateOpen {
		t.Errorf("state = %v, want %v", cb.State(), StateOpen)
	}
}

func TestCircuitBreaker_Metrics(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{
		MaxRequests: 3,
		Interval:    time.Second,
		Timeout:     time.Second,
	})

	// Execute some requests
	for i := 0; i < 5; i++ {
		_ = cb.Execute(context.Background(), func() error {
			if i%2 == 0 {
				return nil
			}
			return errors.New("error")
		})
	}

	metrics := cb.Metrics()

	if metrics.Requests != 5 {
		t.Errorf("Requests = %d, want 5", metrics.Requests)
	}

	if metrics.TotalSuccesses == 0 {
		t.Errorf("TotalSuccesses should be > 0")
	}

	if metrics.TotalFailures == 0 {
		t.Errorf("TotalFailures should be > 0")
	}
}

func TestCircuitBreaker_Reset(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{
		MaxRequests: 3,
		Interval:    time.Second,
		Timeout:     time.Second,
		ReadyToTrip: func(counts Counts) bool {
			return counts.ConsecutiveFailures >= 2
		},
	})

	// Open the circuit
	for i := 0; i < 2; i++ {
		_ = cb.Execute(context.Background(), func() error {
			return errors.New("error")
		})
	}

	if cb.State() != StateOpen {
		t.Fatalf("state = %v, want %v", cb.State(), StateOpen)
	}

	// Reset
	cb.Reset()

	// Should be closed now
	if cb.State() != StateClosed {
		t.Errorf("state after reset = %v, want %v", cb.State(), StateClosed)
	}

	// Counts should be reset
	counts := cb.Counts()
	if counts.Requests != 0 {
		t.Errorf("Requests after reset = %d, want 0", counts.Requests)
	}
}

func TestCircuitBreaker_OnStateChange(t *testing.T) {
	var stateChanges []State

	cb := NewCircuitBreaker(CircuitBreakerConfig{
		MaxRequests: 3,
		Interval:    time.Second,
		Timeout:     50 * time.Millisecond,
		ReadyToTrip: func(counts Counts) bool {
			return counts.ConsecutiveFailures >= 2
		},
		OnStateChange: func(from State, to State) {
			stateChanges = append(stateChanges, to)
		},
	})

	// Open the circuit
	for i := 0; i < 2; i++ {
		_ = cb.Execute(context.Background(), func() error {
			return errors.New("error")
		})
	}

	if len(stateChanges) == 0 {
		t.Errorf("expected state changes")
	}

	lastState := stateChanges[len(stateChanges)-1]
	if lastState != StateOpen {
		t.Errorf("last state change = %v, want %v", lastState, StateOpen)
	}
}

func TestTwoStepCircuitBreaker_Allow(t *testing.T) {
	tscb := NewTwoStepCircuitBreaker(CircuitBreakerConfig{
		MaxRequests: 3,
		Interval:    time.Second,
		Timeout:     time.Second,
	})

	done, err := tscb.Allow()
	if err != nil {
		t.Fatalf("Allow() error = %v", err)
	}

	// Mark as successful
	done(true)

	// Should still be closed
	if tscb.State() != StateClosed {
		t.Errorf("state = %v, want %v", tscb.State(), StateClosed)
	}
}

func TestTwoStepCircuitBreaker_AllowWhenOpen(t *testing.T) {
	tscb := NewTwoStepCircuitBreaker(CircuitBreakerConfig{
		MaxRequests: 3,
		Interval:    time.Second,
		Timeout:     time.Second,
		ReadyToTrip: func(counts Counts) bool {
			return counts.ConsecutiveFailures >= 2
		},
	})

	// Open the circuit
	for i := 0; i < 2; i++ {
		done, _ := tscb.Allow()
		done(false)
	}

	// Should not allow requests
	_, err := tscb.Allow()
	if !errors.Is(err, ErrCircuitOpen) {
		t.Errorf("expected ErrCircuitOpen, got %v", err)
	}
}

func TestMultiCircuitBreaker(t *testing.T) {
	mcb := NewMultiCircuitBreaker()

	config := CircuitBreakerConfig{
		MaxRequests: 3,
		Interval:    time.Second,
		Timeout:     time.Second,
	}

	// Execute for different keys
	err1 := mcb.Execute(context.Background(), "key1", config, func() error {
		return nil
	})

	err2 := mcb.Execute(context.Background(), "key2", config, func() error {
		return errors.New("error")
	})

	if err1 != nil {
		t.Errorf("Execute(key1) error = %v", err1)
	}

	if err2 == nil {
		t.Errorf("Execute(key2) should return error")
	}

	// Check states
	states := mcb.States()
	if len(states) != 2 {
		t.Errorf("expected 2 circuit breakers, got %d", len(states))
	}
}

func TestMultiCircuitBreaker_AllMetrics(t *testing.T) {
	mcb := NewMultiCircuitBreaker()

	config := CircuitBreakerConfig{
		MaxRequests: 3,
		Interval:    time.Second,
		Timeout:     time.Second,
	}

	// Execute for different keys
	for i := 0; i < 5; i++ {
		_ = mcb.Execute(context.Background(), "key1", config, func() error {
			return nil
		})
	}

	metrics := mcb.AllMetrics()
	if len(metrics) != 1 {
		t.Errorf("expected 1 metric entry, got %d", len(metrics))
	}

	key1Metrics := metrics["key1"]
	if key1Metrics.Requests != 5 {
		t.Errorf("Requests = %d, want 5", key1Metrics.Requests)
	}
}

func BenchmarkCircuitBreaker_Execute(b *testing.B) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{
		MaxRequests: 100,
		Interval:    time.Second,
		Timeout:     time.Second,
	})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = cb.Execute(context.Background(), func() error {
			return nil
		})
	}
}
