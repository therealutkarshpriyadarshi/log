package buffer

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/therealutkarshpriyadarshi/log/pkg/types"
)

func TestNewRingBuffer(t *testing.T) {
	tests := []struct {
		name     string
		config   RingBufferConfig
		wantSize uint64
	}{
		{
			name:     "default size",
			config:   RingBufferConfig{},
			wantSize: 1024,
		},
		{
			name:     "custom size rounded up to power of 2",
			config:   RingBufferConfig{Size: 1000},
			wantSize: 1024,
		},
		{
			name:     "power of 2 size",
			config:   RingBufferConfig{Size: 2048},
			wantSize: 2048,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rb, err := NewRingBuffer(tt.config)
			if err != nil {
				t.Fatalf("NewRingBuffer() error = %v", err)
			}
			if rb.size != tt.wantSize {
				t.Errorf("size = %d, want %d", rb.size, tt.wantSize)
			}
		})
	}
}

func TestRingBuffer_EnqueueDequeue(t *testing.T) {
	rb, err := NewRingBuffer(RingBufferConfig{Size: 10})
	if err != nil {
		t.Fatalf("NewRingBuffer() error = %v", err)
	}
	defer rb.Close()

	ctx := context.Background()

	// Enqueue an event
	event := &types.LogEvent{
		Message: "test message",
		Source:  "test",
	}

	if err := rb.Enqueue(ctx, event); err != nil {
		t.Fatalf("Enqueue() error = %v", err)
	}

	// Dequeue the event
	dequeued, err := rb.Dequeue(ctx)
	if err != nil {
		t.Fatalf("Dequeue() error = %v", err)
	}

	if dequeued.Message != event.Message {
		t.Errorf("Dequeued message = %s, want %s", dequeued.Message, event.Message)
	}

	// Buffer should be empty now
	if !rb.Empty() {
		t.Errorf("Buffer should be empty")
	}
}

func TestRingBuffer_BlockingBackpressure(t *testing.T) {
	rb, err := NewRingBuffer(RingBufferConfig{
		Size:                 4,
		BackpressureStrategy: BackpressureBlock,
		BlockTimeout:         100 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewRingBuffer() error = %v", err)
	}
	defer rb.Close()

	ctx := context.Background()

	// Fill the buffer
	for i := 0; i < 4; i++ {
		event := &types.LogEvent{Message: "test"}
		if err := rb.Enqueue(ctx, event); err != nil {
			t.Fatalf("Enqueue() error = %v", err)
		}
	}

	// Buffer should be full
	if !rb.Full() {
		t.Errorf("Buffer should be full")
	}

	// Try to enqueue one more - should timeout
	event := &types.LogEvent{Message: "test"}
	err = rb.Enqueue(ctx, event)
	if err != ErrBufferFull {
		t.Errorf("Expected ErrBufferFull, got %v", err)
	}
}

func TestRingBuffer_DropBackpressure(t *testing.T) {
	rb, err := NewRingBuffer(RingBufferConfig{
		Size:                 4,
		BackpressureStrategy: BackpressureDrop,
	})
	if err != nil {
		t.Fatalf("NewRingBuffer() error = %v", err)
	}
	defer rb.Close()

	ctx := context.Background()

	// Fill the buffer
	for i := 0; i < 4; i++ {
		event := &types.LogEvent{Message: "old"}
		if err := rb.Enqueue(ctx, event); err != nil {
			t.Fatalf("Enqueue() error = %v", err)
		}
	}

	// Enqueue a new event - should drop oldest
	newEvent := &types.LogEvent{Message: "new"}
	if err := rb.Enqueue(ctx, newEvent); err != nil {
		t.Fatalf("Enqueue() error = %v", err)
	}

	// Check that an event was dropped
	metrics := rb.Metrics()
	if metrics.Dropped == 0 {
		t.Errorf("Expected dropped events, got 0")
	}
}

func TestRingBuffer_SampleBackpressure(t *testing.T) {
	rb, err := NewRingBuffer(RingBufferConfig{
		Size:                 4,
		BackpressureStrategy: BackpressureSample,
		SampleRate:           2,
	})
	if err != nil {
		t.Fatalf("NewRingBuffer() error = %v", err)
	}
	defer rb.Close()

	ctx := context.Background()

	// Fill the buffer
	for i := 0; i < 4; i++ {
		event := &types.LogEvent{Message: "test"}
		if err := rb.Enqueue(ctx, event); err != nil {
			t.Fatalf("Enqueue() error = %v", err)
		}
	}

	// Try to enqueue more - should sample
	for i := 0; i < 10; i++ {
		event := &types.LogEvent{Message: "sampled"}
		if err := rb.Enqueue(ctx, event); err != nil {
			t.Fatalf("Enqueue() error = %v", err)
		}
	}

	// Check that some events were dropped
	metrics := rb.Metrics()
	if metrics.Dropped == 0 {
		t.Errorf("Expected dropped events, got 0")
	}
}

func TestRingBuffer_ConcurrentAccess(t *testing.T) {
	rb, err := NewRingBuffer(RingBufferConfig{Size: 1000})
	if err != nil {
		t.Fatalf("NewRingBuffer() error = %v", err)
	}
	defer rb.Close()

	ctx := context.Background()
	numProducers := 10
	numConsumers := 10
	eventsPerProducer := 100

	var wg sync.WaitGroup

	// Start producers
	for i := 0; i < numProducers; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < eventsPerProducer; j++ {
				event := &types.LogEvent{Message: "test"}
				if err := rb.Enqueue(ctx, event); err != nil {
					t.Errorf("Producer %d: Enqueue() error = %v", id, err)
					return
				}
			}
		}(i)
	}

	// Start consumers
	consumed := make(chan int, numConsumers)
	for i := 0; i < numConsumers; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			count := 0
			for {
				event, ok := rb.TryDequeue()
				if ok && event != nil {
					count++
				} else {
					// Give producers time to finish
					if rb.Empty() {
						time.Sleep(10 * time.Millisecond)
						if rb.Empty() {
							break
						}
					}
				}
			}
			consumed <- count
		}(i)
	}

	wg.Wait()
	close(consumed)

	// Count total consumed
	totalConsumed := 0
	for count := range consumed {
		totalConsumed += count
	}

	expectedTotal := numProducers * eventsPerProducer
	if totalConsumed != expectedTotal {
		t.Errorf("Consumed %d events, expected %d", totalConsumed, expectedTotal)
	}
}

func TestRingBuffer_Metrics(t *testing.T) {
	rb, err := NewRingBuffer(RingBufferConfig{Size: 10})
	if err != nil {
		t.Fatalf("NewRingBuffer() error = %v", err)
	}
	defer rb.Close()

	ctx := context.Background()

	// Enqueue some events
	for i := 0; i < 5; i++ {
		event := &types.LogEvent{Message: "test"}
		if err := rb.Enqueue(ctx, event); err != nil {
			t.Fatalf("Enqueue() error = %v", err)
		}
	}

	metrics := rb.Metrics()

	if metrics.Enqueued != 5 {
		t.Errorf("Enqueued = %d, want 5", metrics.Enqueued)
	}

	if metrics.CurrentSize != 5 {
		t.Errorf("CurrentSize = %d, want 5", metrics.CurrentSize)
	}

	if metrics.Utilization != 31.25 { // 5/16 * 100 (size is rounded to 16)
		t.Errorf("Utilization = %f, want 31.25", metrics.Utilization)
	}

	// Dequeue 2 events
	for i := 0; i < 2; i++ {
		if _, err := rb.Dequeue(ctx); err != nil {
			t.Fatalf("Dequeue() error = %v", err)
		}
	}

	metrics = rb.Metrics()

	if metrics.Dequeued != 2 {
		t.Errorf("Dequeued = %d, want 2", metrics.Dequeued)
	}

	if metrics.CurrentSize != 3 {
		t.Errorf("CurrentSize = %d, want 3", metrics.CurrentSize)
	}
}

func TestRingBuffer_Close(t *testing.T) {
	rb, err := NewRingBuffer(RingBufferConfig{Size: 10})
	if err != nil {
		t.Fatalf("NewRingBuffer() error = %v", err)
	}

	// Close the buffer
	if err := rb.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	// Try to enqueue after close
	ctx := context.Background()
	event := &types.LogEvent{Message: "test"}
	err = rb.Enqueue(ctx, event)
	if err != ErrBufferClosed {
		t.Errorf("Expected ErrBufferClosed, got %v", err)
	}

	// Try to close again
	err = rb.Close()
	if err != ErrBufferClosed {
		t.Errorf("Expected ErrBufferClosed on second close, got %v", err)
	}
}

func TestNextPowerOfTwo(t *testing.T) {
	tests := []struct {
		input uint64
		want  uint64
	}{
		{0, 1},
		{1, 1},
		{2, 2},
		{3, 4},
		{4, 4},
		{5, 8},
		{1000, 1024},
		{1024, 1024},
		{1025, 2048},
	}

	for _, tt := range tests {
		got := nextPowerOfTwo(tt.input)
		if got != tt.want {
			t.Errorf("nextPowerOfTwo(%d) = %d, want %d", tt.input, got, tt.want)
		}
	}
}

func BenchmarkRingBuffer_Enqueue(b *testing.B) {
	rb, _ := NewRingBuffer(RingBufferConfig{Size: 10000})
	defer rb.Close()

	ctx := context.Background()
	event := &types.LogEvent{Message: "test"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = rb.Enqueue(ctx, event)
	}
}

func BenchmarkRingBuffer_Dequeue(b *testing.B) {
	rb, _ := NewRingBuffer(RingBufferConfig{Size: 10000})
	defer rb.Close()

	ctx := context.Background()

	// Pre-fill buffer
	for i := 0; i < b.N; i++ {
		event := &types.LogEvent{Message: "test"}
		_ = rb.Enqueue(ctx, event)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = rb.Dequeue(ctx)
	}
}
