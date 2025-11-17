package reliability

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"
)

var (
	ErrMaxRetriesExceeded = errors.New("maximum retries exceeded")
	ErrRetryAborted       = errors.New("retry aborted")
)

// RetryConfig holds configuration for retry logic
type RetryConfig struct {
	MaxRetries     int
	InitialBackoff time.Duration
	MaxBackoff     time.Duration
	Multiplier     float64
	Jitter         bool
}

// RetryFunc is a function that can be retried
type RetryFunc func(ctx context.Context) error

// Retry executes a function with exponential backoff retry logic
func Retry(ctx context.Context, config RetryConfig, fn RetryFunc) error {
	if config.MaxRetries <= 0 {
		config.MaxRetries = 3 // Default
	}

	if config.InitialBackoff == 0 {
		config.InitialBackoff = 100 * time.Millisecond
	}

	if config.MaxBackoff == 0 {
		config.MaxBackoff = 30 * time.Second
	}

	if config.Multiplier == 0 {
		config.Multiplier = 2.0
	}

	var lastErr error
	backoff := config.InitialBackoff

	for attempt := 0; attempt <= config.MaxRetries; attempt++ {
		// Try the function
		err := fn(ctx)
		if err == nil {
			return nil // Success
		}

		lastErr = err

		// Check if we should retry
		if !isRetryable(err) {
			return err
		}

		// Check if we've exhausted retries
		if attempt == config.MaxRetries {
			return fmt.Errorf("%w: %v", ErrMaxRetriesExceeded, lastErr)
		}

		// Calculate backoff
		if attempt > 0 {
			backoff = time.Duration(float64(backoff) * config.Multiplier)
			if backoff > config.MaxBackoff {
				backoff = config.MaxBackoff
			}

			if config.Jitter {
				backoff = addJitter(backoff)
			}
		}

		// Wait before retry
		select {
		case <-ctx.Done():
			return fmt.Errorf("%w: %v", ErrRetryAborted, ctx.Err())
		case <-time.After(backoff):
		}
	}

	return fmt.Errorf("%w: %v", ErrMaxRetriesExceeded, lastErr)
}

// RetryWithBackoff executes a function with custom backoff strategy
func RetryWithBackoff(ctx context.Context, maxRetries int, backoffFunc func(attempt int) time.Duration, fn RetryFunc) error {
	var lastErr error

	for attempt := 0; attempt <= maxRetries; attempt++ {
		err := fn(ctx)
		if err == nil {
			return nil
		}

		lastErr = err

		if !isRetryable(err) {
			return err
		}

		if attempt == maxRetries {
			return fmt.Errorf("%w: %v", ErrMaxRetriesExceeded, lastErr)
		}

		backoff := backoffFunc(attempt)

		select {
		case <-ctx.Done():
			return fmt.Errorf("%w: %v", ErrRetryAborted, ctx.Err())
		case <-time.After(backoff):
		}
	}

	return fmt.Errorf("%w: %v", ErrMaxRetriesExceeded, lastErr)
}

// isRetryable determines if an error should trigger a retry
func isRetryable(err error) bool {
	// Add logic to determine if error is retryable
	// For now, retry all errors except context errors
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	return true
}

// addJitter adds randomness to backoff duration
func addJitter(d time.Duration) time.Duration {
	// Add ±20% jitter
	jitter := float64(d) * 0.2
	offset := (float64(time.Now().UnixNano()%1000) / 1000.0) * jitter
	return time.Duration(float64(d) + offset - jitter/2)
}

// ExponentialBackoff calculates exponential backoff duration
func ExponentialBackoff(attempt int, initial time.Duration, multiplier float64, max time.Duration) time.Duration {
	backoff := time.Duration(float64(initial) * math.Pow(multiplier, float64(attempt)))
	if backoff > max {
		backoff = max
	}
	return backoff
}

// LinearBackoff calculates linear backoff duration
func LinearBackoff(attempt int, increment time.Duration, max time.Duration) time.Duration {
	backoff := time.Duration(attempt) * increment
	if backoff > max {
		backoff = max
	}
	return backoff
}

// ConstantBackoff returns a constant backoff duration
func ConstantBackoff(duration time.Duration) func(int) time.Duration {
	return func(attempt int) time.Duration {
		return duration
	}
}
