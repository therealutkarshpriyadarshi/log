package worker

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/therealutkarshpriyadarshi/log/pkg/types"
)

func TestNewWorkerPool(t *testing.T) {
	jobFunc := func(ctx context.Context, event *types.LogEvent) error {
		return nil
	}

	tests := []struct {
		name    string
		config  PoolConfig
		wantErr bool
	}{
		{
			name: "default config",
			config: PoolConfig{},
			wantErr: false,
		},
		{
			name: "custom config",
			config: PoolConfig{
				NumWorkers: 8,
				QueueSize:  500,
				JobTimeout: 10 * time.Second,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pool, err := NewWorkerPool(tt.config, jobFunc)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewWorkerPool() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err == nil {
				defer pool.Stop()

				if len(pool.workers) == 0 {
					t.Errorf("pool should have workers")
				}
			}
		})
	}
}

func TestWorkerPool_Submit(t *testing.T) {
	var processed uint64
	jobFunc := func(ctx context.Context, event *types.LogEvent) error {
		atomic.AddUint64(&processed, 1)
		return nil
	}

	config := PoolConfig{
		NumWorkers: 2,
		QueueSize:  10,
		JobTimeout: 1 * time.Second,
	}

	pool, err := NewWorkerPool(config, jobFunc)
	if err != nil {
		t.Fatalf("NewWorkerPool() error = %v", err)
	}
	defer pool.Stop()

	pool.Start()

	// Submit a job
	event := &types.LogEvent{
		Message: "test",
		Source:  "test",
	}

	ctx := context.Background()
	if err := pool.Submit(ctx, event); err != nil {
		t.Fatalf("Submit() error = %v", err)
	}

	// Wait a bit for processing
	time.Sleep(100 * time.Millisecond)

	if atomic.LoadUint64(&processed) != 1 {
		t.Errorf("expected 1 job processed, got %d", atomic.LoadUint64(&processed))
	}
}

func TestWorkerPool_SubmitAsync(t *testing.T) {
	var processed uint64
	jobFunc := func(ctx context.Context, event *types.LogEvent) error {
		atomic.AddUint64(&processed, 1)
		time.Sleep(50 * time.Millisecond)
		return nil
	}

	config := PoolConfig{
		NumWorkers: 2,
		QueueSize:  100,
	}

	pool, err := NewWorkerPool(config, jobFunc)
	if err != nil {
		t.Fatalf("NewWorkerPool() error = %v", err)
	}
	defer pool.Stop()

	pool.Start()

	// Submit multiple jobs asynchronously
	numJobs := 10
	for i := 0; i < numJobs; i++ {
		event := &types.LogEvent{
			Message: "test",
			Source:  "test",
		}
		if err := pool.SubmitAsync(event); err != nil {
			t.Fatalf("SubmitAsync() error = %v", err)
		}
	}

	// Wait for all jobs to be processed
	time.Sleep(1 * time.Second)

	if atomic.LoadUint64(&processed) != uint64(numJobs) {
		t.Errorf("expected %d jobs processed, got %d", numJobs, atomic.LoadUint64(&processed))
	}
}

func TestWorkerPool_JobError(t *testing.T) {
	expectedErr := errors.New("job error")
	jobFunc := func(ctx context.Context, event *types.LogEvent) error {
		return expectedErr
	}

	config := PoolConfig{
		NumWorkers: 1,
		QueueSize:  10,
	}

	pool, err := NewWorkerPool(config, jobFunc)
	if err != nil {
		t.Fatalf("NewWorkerPool() error = %v", err)
	}
	defer pool.Stop()

	pool.Start()

	event := &types.LogEvent{Message: "test"}
	ctx := context.Background()

	err = pool.Submit(ctx, event)
	if err == nil {
		t.Errorf("expected error, got nil")
	}

	if err != expectedErr {
		t.Errorf("expected %v, got %v", expectedErr, err)
	}
}

func TestWorkerPool_Timeout(t *testing.T) {
	jobFunc := func(ctx context.Context, event *types.LogEvent) error {
		// Simulate long-running job
		time.Sleep(2 * time.Second)
		return nil
	}

	config := PoolConfig{
		NumWorkers: 1,
		QueueSize:  10,
		JobTimeout: 100 * time.Millisecond,
	}

	pool, err := NewWorkerPool(config, jobFunc)
	if err != nil {
		t.Fatalf("NewWorkerPool() error = %v", err)
	}
	defer pool.Stop()

	pool.Start()

	event := &types.LogEvent{Message: "test"}
	ctx := context.Background()

	err = pool.Submit(ctx, event)
	if err != ErrJobTimeout {
		t.Errorf("expected ErrJobTimeout, got %v", err)
	}

	metrics := pool.Metrics()
	if metrics.JobsTimeout == 0 {
		t.Errorf("expected timeout count > 0")
	}
}

func TestWorkerPool_ConcurrentSubmit(t *testing.T) {
	var processed uint64
	var mu sync.Mutex
	jobFunc := func(ctx context.Context, event *types.LogEvent) error {
		mu.Lock()
		defer mu.Unlock()
		atomic.AddUint64(&processed, 1)
		time.Sleep(10 * time.Millisecond)
		return nil
	}

	config := PoolConfig{
		NumWorkers: 10,
		QueueSize:  1000,
	}

	pool, err := NewWorkerPool(config, jobFunc)
	if err != nil {
		t.Fatalf("NewWorkerPool() error = %v", err)
	}
	defer pool.Stop()

	pool.Start()

	// Submit jobs from multiple goroutines
	numGoroutines := 10
	jobsPerGoroutine := 10
	var wg sync.WaitGroup

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < jobsPerGoroutine; j++ {
				event := &types.LogEvent{Message: "test"}
				_ = pool.SubmitAsync(event)
			}
		}()
	}

	wg.Wait()

	// Wait for processing
	time.Sleep(2 * time.Second)

	expectedJobs := uint64(numGoroutines * jobsPerGoroutine)
	if atomic.LoadUint64(&processed) != expectedJobs {
		t.Errorf("expected %d jobs processed, got %d", expectedJobs, atomic.LoadUint64(&processed))
	}
}

func TestWorkerPool_Scale(t *testing.T) {
	jobFunc := func(ctx context.Context, event *types.LogEvent) error {
		return nil
	}

	config := PoolConfig{
		NumWorkers: 2,
		QueueSize:  10,
	}

	pool, err := NewWorkerPool(config, jobFunc)
	if err != nil {
		t.Fatalf("NewWorkerPool() error = %v", err)
	}
	defer pool.Stop()

	pool.Start()

	initialWorkers := len(pool.workers)
	if initialWorkers != 2 {
		t.Errorf("expected 2 initial workers, got %d", initialWorkers)
	}

	// Scale up
	if err := pool.Scale(5); err != nil {
		t.Fatalf("Scale() error = %v", err)
	}

	if len(pool.workers) != 5 {
		t.Errorf("expected 5 workers after scaling up, got %d", len(pool.workers))
	}

	// Scale down
	if err := pool.Scale(3); err != nil {
		t.Fatalf("Scale() error = %v", err)
	}

	if len(pool.workers) != 3 {
		t.Errorf("expected 3 workers after scaling down, got %d", len(pool.workers))
	}
}

func TestWorkerPool_Stop(t *testing.T) {
	jobFunc := func(ctx context.Context, event *types.LogEvent) error {
		return nil
	}

	config := PoolConfig{
		NumWorkers: 2,
		QueueSize:  10,
	}

	pool, err := NewWorkerPool(config, jobFunc)
	if err != nil {
		t.Fatalf("NewWorkerPool() error = %v", err)
	}

	pool.Start()

	// Stop the pool
	if err := pool.Stop(); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}

	// Try to submit after stop
	event := &types.LogEvent{Message: "test"}
	err = pool.SubmitAsync(event)
	if err != ErrPoolClosed {
		t.Errorf("expected ErrPoolClosed, got %v", err)
	}
}

func TestWorkerPool_Metrics(t *testing.T) {
	var processed uint64
	jobFunc := func(ctx context.Context, event *types.LogEvent) error {
		atomic.AddUint64(&processed, 1)
		return nil
	}

	config := PoolConfig{
		NumWorkers: 4,
		QueueSize:  100,
	}

	pool, err := NewWorkerPool(config, jobFunc)
	if err != nil {
		t.Fatalf("NewWorkerPool() error = %v", err)
	}
	defer pool.Stop()

	pool.Start()

	// Submit some jobs
	for i := 0; i < 10; i++ {
		event := &types.LogEvent{Message: "test"}
		_ = pool.SubmitAsync(event)
	}

	time.Sleep(500 * time.Millisecond)

	metrics := pool.Metrics()

	if metrics.NumWorkers != 4 {
		t.Errorf("NumWorkers = %d, want 4", metrics.NumWorkers)
	}

	if metrics.JobsProcessed != 10 {
		t.Errorf("JobsProcessed = %d, want 10", metrics.JobsProcessed)
	}

	if len(metrics.WorkerMetrics) != 4 {
		t.Errorf("expected 4 worker metrics, got %d", len(metrics.WorkerMetrics))
	}

	successRate := metrics.SuccessRate()
	if successRate != 100.0 {
		t.Errorf("SuccessRate = %f, want 100.0", successRate)
	}
}

func TestWorkerPool_PartialFailures(t *testing.T) {
	var count uint64
	jobFunc := func(ctx context.Context, event *types.LogEvent) error {
		n := atomic.AddUint64(&count, 1)
		if n%2 == 0 {
			return errors.New("simulated error")
		}
		return nil
	}

	config := PoolConfig{
		NumWorkers: 2,
		QueueSize:  100,
	}

	pool, err := NewWorkerPool(config, jobFunc)
	if err != nil {
		t.Fatalf("NewWorkerPool() error = %v", err)
	}
	defer pool.Stop()

	pool.Start()

	// Submit jobs
	for i := 0; i < 10; i++ {
		event := &types.LogEvent{Message: "test"}
		_ = pool.SubmitAsync(event)
	}

	time.Sleep(500 * time.Millisecond)

	metrics := pool.Metrics()

	if metrics.JobsProcessed != 10 {
		t.Errorf("JobsProcessed = %d, want 10", metrics.JobsProcessed)
	}

	if metrics.JobsFailed == 0 {
		t.Errorf("expected some failed jobs")
	}

	successRate := metrics.SuccessRate()
	if successRate >= 100.0 {
		t.Errorf("SuccessRate should be less than 100 with failures")
	}
}

func BenchmarkWorkerPool_Submit(b *testing.B) {
	jobFunc := func(ctx context.Context, event *types.LogEvent) error {
		return nil
	}

	config := PoolConfig{
		NumWorkers: 10,
		QueueSize:  10000,
	}

	pool, _ := NewWorkerPool(config, jobFunc)
	defer pool.Stop()

	pool.Start()

	event := &types.LogEvent{Message: "benchmark"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = pool.SubmitAsync(event)
	}
}

func BenchmarkWorkerPool_Processing(b *testing.B) {
	var processed uint64
	jobFunc := func(ctx context.Context, event *types.LogEvent) error {
		atomic.AddUint64(&processed, 1)
		return nil
	}

	config := PoolConfig{
		NumWorkers: 10,
		QueueSize:  10000,
	}

	pool, _ := NewWorkerPool(config, jobFunc)
	defer pool.Stop()

	pool.Start()

	event := &types.LogEvent{Message: "benchmark"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = pool.SubmitAsync(event)
	}

	// Wait for all jobs to complete
	for atomic.LoadUint64(&processed) < uint64(b.N) {
		time.Sleep(10 * time.Millisecond)
	}
}
