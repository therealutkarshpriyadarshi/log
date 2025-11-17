package output

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/therealutkarshpriyadarshi/log/pkg/types"
)

func TestBatcherBasic(t *testing.T) {
	var flushedCount int64
	var flushedEvents []*types.LogEvent
	var mu sync.Mutex

	flushFn := func(ctx context.Context, events []*types.LogEvent) error {
		mu.Lock()
		defer mu.Unlock()
		atomic.AddInt64(&flushedCount, int64(len(events)))
		flushedEvents = append(flushedEvents, events...)
		return nil
	}

	config := BatcherConfig{
		MaxBatchSize:  5,
		MaxBatchBytes: 10000,
		FlushInterval: 100 * time.Millisecond,
	}

	batcher := NewBatcher(config, flushFn)
	defer batcher.Stop()

	// Add events
	for i := 0; i < 12; i++ {
		event := &types.LogEvent{
			Message: "test event",
			Raw:     "test event",
		}
		err := batcher.Add(context.Background(), event)
		if err != nil {
			t.Fatalf("failed to add event: %v", err)
		}
	}

	// Wait for flush
	time.Sleep(200 * time.Millisecond)

	// Should have flushed 12 events (2 batches of 5 + 1 batch of 2)
	count := atomic.LoadInt64(&flushedCount)
	if count != 12 {
		t.Errorf("expected 12 events flushed, got %d", count)
	}
}

func TestBatcherFlushOnSize(t *testing.T) {
	var flushedBatches int64
	var mu sync.Mutex

	flushFn := func(ctx context.Context, events []*types.LogEvent) error {
		mu.Lock()
		defer mu.Unlock()
		atomic.AddInt64(&flushedBatches, 1)
		if len(events) != 5 {
			t.Errorf("expected batch size 5, got %d", len(events))
		}
		return nil
	}

	config := BatcherConfig{
		MaxBatchSize:  5,
		MaxBatchBytes: 10000,
		FlushInterval: 10 * time.Second, // Long interval
	}

	batcher := NewBatcher(config, flushFn)
	defer batcher.Stop()

	// Add exactly 5 events (should trigger immediate flush)
	for i := 0; i < 5; i++ {
		event := &types.LogEvent{
			Message: "test event",
			Raw:     "test",
		}
		err := batcher.Add(context.Background(), event)
		if err != nil {
			t.Fatalf("failed to add event: %v", err)
		}
	}

	// Give it a moment to flush
	time.Sleep(50 * time.Millisecond)

	batches := atomic.LoadInt64(&flushedBatches)
	if batches != 1 {
		t.Errorf("expected 1 batch flushed, got %d", batches)
	}
}

func TestBatcherFlushOnInterval(t *testing.T) {
	var flushedCount int64

	flushFn := func(ctx context.Context, events []*types.LogEvent) error {
		atomic.AddInt64(&flushedCount, int64(len(events)))
		return nil
	}

	config := BatcherConfig{
		MaxBatchSize:  100,
		MaxBatchBytes: 10000,
		FlushInterval: 100 * time.Millisecond,
	}

	batcher := NewBatcher(config, flushFn)
	defer batcher.Stop()

	// Add 3 events (less than max batch size)
	for i := 0; i < 3; i++ {
		event := &types.LogEvent{
			Message: "test event",
			Raw:     "test",
		}
		err := batcher.Add(context.Background(), event)
		if err != nil {
			t.Fatalf("failed to add event: %v", err)
		}
	}

	// Wait for interval flush
	time.Sleep(200 * time.Millisecond)

	count := atomic.LoadInt64(&flushedCount)
	if count != 3 {
		t.Errorf("expected 3 events flushed, got %d", count)
	}
}

func TestBatcherManualFlush(t *testing.T) {
	var flushedCount int64

	flushFn := func(ctx context.Context, events []*types.LogEvent) error {
		atomic.AddInt64(&flushedCount, int64(len(events)))
		return nil
	}

	config := BatcherConfig{
		MaxBatchSize:  100,
		MaxBatchBytes: 10000,
		FlushInterval: 10 * time.Second, // Long interval
	}

	batcher := NewBatcher(config, flushFn)
	defer batcher.Stop()

	// Add events
	for i := 0; i < 5; i++ {
		event := &types.LogEvent{
			Message: "test event",
			Raw:     "test",
		}
		err := batcher.Add(context.Background(), event)
		if err != nil {
			t.Fatalf("failed to add event: %v", err)
		}
	}

	// Manual flush
	err := batcher.Flush(context.Background())
	if err != nil {
		t.Fatalf("failed to flush: %v", err)
	}

	count := atomic.LoadInt64(&flushedCount)
	if count != 5 {
		t.Errorf("expected 5 events flushed, got %d", count)
	}
}

func TestBatcherSize(t *testing.T) {
	flushFn := func(ctx context.Context, events []*types.LogEvent) error {
		return nil
	}

	config := BatcherConfig{
		MaxBatchSize:  100,
		MaxBatchBytes: 10000,
		FlushInterval: 10 * time.Second,
	}

	batcher := NewBatcher(config, flushFn)
	defer batcher.Stop()

	// Add events
	for i := 0; i < 7; i++ {
		event := &types.LogEvent{
			Message: "test event",
			Raw:     "test",
		}
		err := batcher.Add(context.Background(), event)
		if err != nil {
			t.Fatalf("failed to add event: %v", err)
		}
	}

	size := batcher.Size()
	if size != 7 {
		t.Errorf("expected size 7, got %d", size)
	}
}
