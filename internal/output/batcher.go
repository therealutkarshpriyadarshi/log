package output

import (
	"context"
	"sync"
	"time"

	"github.com/therealutkarshpriyadarshi/log/pkg/types"
)

// BatcherConfig configures the batching behavior
type BatcherConfig struct {
	MaxBatchSize  int
	MaxBatchBytes int
	FlushInterval time.Duration
}

// Batcher accumulates events and flushes them in batches
type Batcher struct {
	config   BatcherConfig
	events   []*types.LogEvent
	size     int
	mu       sync.Mutex
	flushFn  func(ctx context.Context, events []*types.LogEvent) error
	stopCh   chan struct{}
	flushCh  chan struct{}
	doneCh   chan struct{}
}

// NewBatcher creates a new batcher
func NewBatcher(config BatcherConfig, flushFn func(ctx context.Context, events []*types.LogEvent) error) *Batcher {
	b := &Batcher{
		config:  config,
		events:  make([]*types.LogEvent, 0, config.MaxBatchSize),
		flushFn: flushFn,
		stopCh:  make(chan struct{}),
		flushCh: make(chan struct{}, 1),
		doneCh:  make(chan struct{}),
	}

	// Start the flush ticker
	go b.flushLoop()

	return b
}

// Add adds an event to the batch
func (b *Batcher) Add(ctx context.Context, event *types.LogEvent) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.events = append(b.events, event)
	b.size += len(event.Raw)

	// Flush if batch is full
	if len(b.events) >= b.config.MaxBatchSize || b.size >= b.config.MaxBatchBytes {
		return b.flushLocked(ctx)
	}

	return nil
}

// Flush forces a flush of the current batch
func (b *Batcher) Flush(ctx context.Context) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.flushLocked(ctx)
}

// flushLocked flushes the current batch (must be called with lock held)
func (b *Batcher) flushLocked(ctx context.Context) error {
	if len(b.events) == 0 {
		return nil
	}

	// Copy events to flush
	toFlush := make([]*types.LogEvent, len(b.events))
	copy(toFlush, b.events)

	// Reset batch
	b.events = b.events[:0]
	b.size = 0

	// Flush without holding lock
	b.mu.Unlock()
	err := b.flushFn(ctx, toFlush)
	b.mu.Lock()

	return err
}

// flushLoop periodically flushes the batch
func (b *Batcher) flushLoop() {
	ticker := time.NewTicker(b.config.FlushInterval)
	defer ticker.Stop()
	defer close(b.doneCh)

	for {
		select {
		case <-ticker.C:
			b.Flush(context.Background())
		case <-b.flushCh:
			b.Flush(context.Background())
		case <-b.stopCh:
			// Final flush on shutdown
			b.Flush(context.Background())
			return
		}
	}
}

// Stop stops the batcher and flushes remaining events
func (b *Batcher) Stop() error {
	close(b.stopCh)
	<-b.doneCh
	return nil
}

// Size returns the current number of events in the batch
func (b *Batcher) Size() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.events)
}
