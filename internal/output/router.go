package output

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/therealutkarshpriyadarshi/log/pkg/types"
)

// RouterConfig contains configuration for the multi-output router
type RouterConfig struct {
	// Outputs is the list of output configurations
	Outputs []OutputConfig `yaml:"outputs"`

	// FailureStrategy defines how to handle output failures (continue, stop)
	FailureStrategy string `yaml:"failure_strategy,omitempty"`

	// Parallel enables parallel sending to all outputs
	Parallel bool `yaml:"parallel,omitempty"`
}

// OutputConfig wraps an output with its specific configuration
type OutputConfig struct {
	Type   string                 `yaml:"type"`
	Name   string                 `yaml:"name,omitempty"`
	Config map[string]interface{} `yaml:"config"`
}

// DefaultRouterConfig returns default router configuration
func DefaultRouterConfig() RouterConfig {
	return RouterConfig{
		Outputs:         []OutputConfig{},
		FailureStrategy: "continue", // continue on failure
		Parallel:        true,
	}
}

// Router routes events to multiple outputs
type Router struct {
	config  RouterConfig
	outputs []Output
	metrics *RouterMetrics
	mu      sync.RWMutex
	closed  atomic.Bool
}

// RouterMetrics tracks aggregate metrics across all outputs
type RouterMetrics struct {
	TotalEventsSent   int64           `json:"total_events_sent"`
	TotalEventsFailed int64           `json:"total_events_failed"`
	TotalBytesSent    int64           `json:"total_bytes_sent"`
	OutputMetrics     []*OutputMetrics `json:"output_metrics"`
}

// NewRouter creates a new multi-output router
func NewRouter(config RouterConfig) (*Router, error) {
	if len(config.Outputs) == 0 {
		return nil, fmt.Errorf("no outputs configured")
	}

	router := &Router{
		config:  config,
		outputs: make([]Output, 0, len(config.Outputs)),
		metrics: &RouterMetrics{
			OutputMetrics: make([]*OutputMetrics, 0, len(config.Outputs)),
		},
	}

	return router, nil
}

// AddOutput adds an output to the router
func (r *Router) AddOutput(output Output) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.outputs = append(r.outputs, output)
	r.metrics.OutputMetrics = append(r.metrics.OutputMetrics, output.Metrics())
}

// Send sends an event to all configured outputs
func (r *Router) Send(ctx context.Context, event *types.LogEvent) error {
	if r.closed.Load() {
		return fmt.Errorf("router is closed")
	}

	if len(r.outputs) == 0 {
		return fmt.Errorf("no outputs available")
	}

	if r.config.Parallel {
		return r.sendParallel(ctx, event)
	}

	return r.sendSequential(ctx, event)
}

// SendBatch sends a batch of events to all configured outputs
func (r *Router) SendBatch(ctx context.Context, events []*types.LogEvent) error {
	if r.closed.Load() {
		return fmt.Errorf("router is closed")
	}

	if len(r.outputs) == 0 {
		return fmt.Errorf("no outputs available")
	}

	if r.config.Parallel {
		return r.sendBatchParallel(ctx, events)
	}

	return r.sendBatchSequential(ctx, events)
}

// sendParallel sends an event to all outputs in parallel
func (r *Router) sendParallel(ctx context.Context, event *types.LogEvent) error {
	r.mu.RLock()
	outputs := r.outputs
	r.mu.RUnlock()

	var wg sync.WaitGroup
	errors := make(chan error, len(outputs))

	for _, output := range outputs {
		wg.Add(1)
		go func(out Output) {
			defer wg.Done()
			if err := out.Send(ctx, event); err != nil {
				errors <- fmt.Errorf("%s: %w", out.Name(), err)
			}
		}(output)
	}

	wg.Wait()
	close(errors)

	// Collect errors
	var errs []error
	for err := range errors {
		errs = append(errs, err)
		atomic.AddInt64(&r.metrics.TotalEventsFailed, 1)
	}

	// Update success metrics
	successCount := int64(len(outputs)) - int64(len(errs))
	atomic.AddInt64(&r.metrics.TotalEventsSent, successCount)
	atomic.AddInt64(&r.metrics.TotalBytesSent, int64(len(event.Raw))*successCount)

	if len(errs) > 0 {
		if r.config.FailureStrategy == "stop" {
			return fmt.Errorf("failed to send to %d outputs: %v", len(errs), errs)
		}
		// Continue strategy - log errors but don't fail
	}

	return nil
}

// sendSequential sends an event to all outputs sequentially
func (r *Router) sendSequential(ctx context.Context, event *types.LogEvent) error {
	r.mu.RLock()
	outputs := r.outputs
	r.mu.RUnlock()

	var errs []error

	for _, output := range outputs {
		if err := output.Send(ctx, event); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", output.Name(), err))
			atomic.AddInt64(&r.metrics.TotalEventsFailed, 1)

			if r.config.FailureStrategy == "stop" {
				return fmt.Errorf("failed to send to output %s: %w", output.Name(), err)
			}
		} else {
			atomic.AddInt64(&r.metrics.TotalEventsSent, 1)
			atomic.AddInt64(&r.metrics.TotalBytesSent, int64(len(event.Raw)))
		}
	}

	if len(errs) > 0 && r.config.FailureStrategy == "continue" {
		// Log errors but don't fail
		return nil
	}

	return nil
}

// sendBatchParallel sends a batch to all outputs in parallel
func (r *Router) sendBatchParallel(ctx context.Context, events []*types.LogEvent) error {
	r.mu.RLock()
	outputs := r.outputs
	r.mu.RUnlock()

	var wg sync.WaitGroup
	errors := make(chan error, len(outputs))

	for _, output := range outputs {
		wg.Add(1)
		go func(out Output) {
			defer wg.Done()
			if err := out.SendBatch(ctx, events); err != nil {
				errors <- fmt.Errorf("%s: %w", out.Name(), err)
			}
		}(output)
	}

	wg.Wait()
	close(errors)

	// Collect errors
	var errs []error
	for err := range errors {
		errs = append(errs, err)
		atomic.AddInt64(&r.metrics.TotalEventsFailed, int64(len(events)))
	}

	// Update success metrics
	successCount := int64(len(outputs)) - int64(len(errs))
	var totalBytes int64
	for _, event := range events {
		totalBytes += int64(len(event.Raw))
	}
	atomic.AddInt64(&r.metrics.TotalEventsSent, int64(len(events))*successCount)
	atomic.AddInt64(&r.metrics.TotalBytesSent, totalBytes*successCount)

	if len(errs) > 0 {
		if r.config.FailureStrategy == "stop" {
			return fmt.Errorf("failed to send to %d outputs: %v", len(errs), errs)
		}
	}

	return nil
}

// sendBatchSequential sends a batch to all outputs sequentially
func (r *Router) sendBatchSequential(ctx context.Context, events []*types.LogEvent) error {
	r.mu.RLock()
	outputs := r.outputs
	r.mu.RUnlock()

	var errs []error
	var totalBytes int64
	for _, event := range events {
		totalBytes += int64(len(event.Raw))
	}

	for _, output := range outputs {
		if err := output.SendBatch(ctx, events); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", output.Name(), err))
			atomic.AddInt64(&r.metrics.TotalEventsFailed, int64(len(events)))

			if r.config.FailureStrategy == "stop" {
				return fmt.Errorf("failed to send to output %s: %w", output.Name(), err)
			}
		} else {
			atomic.AddInt64(&r.metrics.TotalEventsSent, int64(len(events)))
			atomic.AddInt64(&r.metrics.TotalBytesSent, totalBytes)
		}
	}

	if len(errs) > 0 && r.config.FailureStrategy == "continue" {
		return nil
	}

	return nil
}

// Close closes all outputs
func (r *Router) Close() error {
	if !r.closed.CompareAndSwap(false, true) {
		return nil // Already closed
	}

	r.mu.RLock()
	outputs := r.outputs
	r.mu.RUnlock()

	var errs []error
	for _, output := range outputs {
		if err := output.Close(); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", output.Name(), err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("failed to close %d outputs: %v", len(errs), errs)
	}

	return nil
}

// Name returns the router name
func (r *Router) Name() string {
	return "router"
}

// Metrics returns the aggregate metrics
func (r *Router) Metrics() *OutputMetrics {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Aggregate metrics from all outputs
	var totalSent, totalFailed, totalBytes, totalBatches int64
	var totalLatency time.Duration
	var totalBatchSize float64
	var lastSendTime, lastErrorTime time.Time
	var lastError string

	for _, output := range r.outputs {
		metrics := output.Metrics()
		totalSent += metrics.EventsSent
		totalFailed += metrics.EventsFailed
		totalBytes += metrics.BytesSent
		totalBatches += metrics.BatchesSent
		totalLatency += metrics.AvgLatency
		totalBatchSize += metrics.AvgBatchSize

		if metrics.LastSendTime.After(lastSendTime) {
			lastSendTime = metrics.LastSendTime
		}
		if metrics.LastErrorTime.After(lastErrorTime) {
			lastErrorTime = metrics.LastErrorTime
			lastError = metrics.LastError
		}
	}

	avgLatency := time.Duration(0)
	if len(r.outputs) > 0 {
		avgLatency = totalLatency / time.Duration(len(r.outputs))
	}

	avgBatchSize := 0.0
	if len(r.outputs) > 0 {
		avgBatchSize = totalBatchSize / float64(len(r.outputs))
	}

	return &OutputMetrics{
		EventsSent:    totalSent,
		EventsFailed:  totalFailed,
		BytesSent:     totalBytes,
		BatchesSent:   totalBatches,
		LastSendTime:  lastSendTime,
		LastError:     lastError,
		LastErrorTime: lastErrorTime,
		AvgBatchSize:  avgBatchSize,
		AvgLatency:    avgLatency,
	}
}

// GetOutputs returns all configured outputs
func (r *Router) GetOutputs() []Output {
	r.mu.RLock()
	defer r.mu.RUnlock()

	outputs := make([]Output, len(r.outputs))
	copy(outputs, r.outputs)
	return outputs
}
