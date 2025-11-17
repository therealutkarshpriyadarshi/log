package reliability

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestRetry_Success(t *testing.T) {
	attempts := 0
	fn := func(ctx context.Context) error {
		attempts++
		if attempts < 3 {
			return errors.New("temporary error")
		}
		return nil
	}

	config := RetryConfig{
		MaxRetries:     5,
		InitialBackoff: 10 * time.Millisecond,
		Multiplier:     2.0,
	}

	err := Retry(context.Background(), config, fn)
	if err != nil {
		t.Errorf("Retry() error = %v, want nil", err)
	}

	if attempts != 3 {
		t.Errorf("attempts = %d, want 3", attempts)
	}
}

func TestRetry_MaxRetriesExceeded(t *testing.T) {
	attempts := 0
	fn := func(ctx context.Context) error {
		attempts++
		return errors.New("persistent error")
	}

	config := RetryConfig{
		MaxRetries:     3,
		InitialBackoff: 1 * time.Millisecond,
	}

	err := Retry(context.Background(), config, fn)
	if !errors.Is(err, ErrMaxRetriesExceeded) {
		t.Errorf("expected ErrMaxRetriesExceeded, got %v", err)
	}

	if attempts != 4 { // Initial attempt + 3 retries
		t.Errorf("attempts = %d, want 4", attempts)
	}
}

func TestRetry_ContextCanceled(t *testing.T) {
	fn := func(ctx context.Context) error {
		return errors.New("error")
	}

	config := RetryConfig{
		MaxRetries:     5,
		InitialBackoff: 100 * time.Millisecond,
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	err := Retry(ctx, config, fn)
	if !errors.Is(err, ErrRetryAborted) {
		t.Errorf("expected ErrRetryAborted, got %v", err)
	}
}

func TestRetry_ExponentialBackoff(t *testing.T) {
	attempts := []time.Time{}
	fn := func(ctx context.Context) error {
		attempts = append(attempts, time.Now())
		if len(attempts) < 3 {
			return errors.New("error")
		}
		return nil
	}

	config := RetryConfig{
		MaxRetries:     5,
		InitialBackoff: 50 * time.Millisecond,
		Multiplier:     2.0,
		MaxBackoff:     500 * time.Millisecond,
	}

	err := Retry(context.Background(), config, fn)
	if err != nil {
		t.Errorf("Retry() error = %v", err)
	}

	if len(attempts) != 3 {
		t.Fatalf("expected 3 attempts, got %d", len(attempts))
	}

	// Check backoff intervals (approximately)
	interval1 := attempts[1].Sub(attempts[0])
	interval2 := attempts[2].Sub(attempts[1])

	// Second interval should be roughly double the first
	if interval2 < interval1 {
		t.Errorf("backoff did not increase: interval1=%v, interval2=%v", interval1, interval2)
	}
}

func TestRetryWithBackoff_CustomBackoff(t *testing.T) {
	attempts := 0
	fn := func(ctx context.Context) error {
		attempts++
		if attempts < 3 {
			return errors.New("error")
		}
		return nil
	}

	backoffFunc := func(attempt int) time.Duration {
		return time.Duration(attempt*10) * time.Millisecond
	}

	err := RetryWithBackoff(context.Background(), 5, backoffFunc, fn)
	if err != nil {
		t.Errorf("RetryWithBackoff() error = %v", err)
	}

	if attempts != 3 {
		t.Errorf("attempts = %d, want 3", attempts)
	}
}

func TestExponentialBackoff(t *testing.T) {
	tests := []struct {
		attempt    int
		initial    time.Duration
		multiplier float64
		max        time.Duration
		wantMin    time.Duration
		wantMax    time.Duration
	}{
		{0, 100 * time.Millisecond, 2.0, 10 * time.Second, 100 * time.Millisecond, 100 * time.Millisecond},
		{1, 100 * time.Millisecond, 2.0, 10 * time.Second, 200 * time.Millisecond, 200 * time.Millisecond},
		{2, 100 * time.Millisecond, 2.0, 10 * time.Second, 400 * time.Millisecond, 400 * time.Millisecond},
		{10, 100 * time.Millisecond, 2.0, 5 * time.Second, 5 * time.Second, 5 * time.Second}, // Capped at max
	}

	for _, tt := range tests {
		got := ExponentialBackoff(tt.attempt, tt.initial, tt.multiplier, tt.max)
		if got < tt.wantMin || got > tt.wantMax {
			t.Errorf("ExponentialBackoff(%d) = %v, want between %v and %v", tt.attempt, got, tt.wantMin, tt.wantMax)
		}
	}
}

func TestLinearBackoff(t *testing.T) {
	tests := []struct {
		attempt   int
		increment time.Duration
		max       time.Duration
		want      time.Duration
	}{
		{0, 100 * time.Millisecond, 10 * time.Second, 0},
		{1, 100 * time.Millisecond, 10 * time.Second, 100 * time.Millisecond},
		{5, 100 * time.Millisecond, 10 * time.Second, 500 * time.Millisecond},
		{100, 100 * time.Millisecond, 5 * time.Second, 5 * time.Second}, // Capped at max
	}

	for _, tt := range tests {
		got := LinearBackoff(tt.attempt, tt.increment, tt.max)
		if got != tt.want {
			t.Errorf("LinearBackoff(%d) = %v, want %v", tt.attempt, got, tt.want)
		}
	}
}

func TestConstantBackoff(t *testing.T) {
	backoffFunc := ConstantBackoff(500 * time.Millisecond)

	for i := 0; i < 10; i++ {
		got := backoffFunc(i)
		if got != 500*time.Millisecond {
			t.Errorf("ConstantBackoff(%d) = %v, want 500ms", i, got)
		}
	}
}

func BenchmarkRetry(b *testing.B) {
	fn := func(ctx context.Context) error {
		return nil
	}

	config := RetryConfig{
		MaxRetries:     3,
		InitialBackoff: 1 * time.Millisecond,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Retry(context.Background(), config, fn)
	}
}
