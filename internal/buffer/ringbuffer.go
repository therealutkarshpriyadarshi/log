package buffer

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/therealutkarshpriyadarshi/log/pkg/types"
)

var (
	ErrBufferFull   = errors.New("buffer is full")
	ErrBufferClosed = errors.New("buffer is closed")
)

// BackpressureStrategy defines how to handle backpressure
type BackpressureStrategy string

const (
	// Block blocks the producer when buffer is full
	BackpressureBlock BackpressureStrategy = "block"
	// Drop drops the oldest event when buffer is full
	BackpressureDrop BackpressureStrategy = "drop"
	// Sample samples events when buffer is full (keep every Nth event)
	BackpressureSample BackpressureStrategy = "sample"
)

// RingBufferConfig holds configuration for the ring buffer
type RingBufferConfig struct {
	Size                 int
	BackpressureStrategy BackpressureStrategy
	SampleRate           int // For sample strategy: keep 1 out of N events
	BlockTimeout         time.Duration
}

// RingBuffer is a lock-free circular buffer for log events
type RingBuffer struct {
	buffer   []*types.LogEvent
	size     uint64
	mask     uint64
	writePos uint64
	readPos  uint64

	config RingBufferConfig

	// Metrics
	enqueued uint64
	dequeued uint64
	dropped  uint64
	sampled  uint64

	// Control
	closed    uint32
	notEmpty  chan struct{}
	notFull   chan struct{}
	mu        sync.RWMutex
}

// NewRingBuffer creates a new ring buffer with the given configuration
func NewRingBuffer(config RingBufferConfig) (*RingBuffer, error) {
	if config.Size <= 0 {
		config.Size = 1024 // Default size
	}

	// Ensure size is power of 2 for efficient masking
	size := nextPowerOfTwo(uint64(config.Size))

	if config.BackpressureStrategy == "" {
		config.BackpressureStrategy = BackpressureBlock
	}

	if config.SampleRate <= 0 {
		config.SampleRate = 10
	}

	if config.BlockTimeout == 0 {
		config.BlockTimeout = 5 * time.Second
	}

	rb := &RingBuffer{
		buffer:   make([]*types.LogEvent, size),
		size:     size,
		mask:     size - 1,
		config:   config,
		notEmpty: make(chan struct{}, 1),
		notFull:  make(chan struct{}, 1),
	}

	return rb, nil
}

// Enqueue adds an event to the buffer
func (rb *RingBuffer) Enqueue(ctx context.Context, event *types.LogEvent) error {
	if atomic.LoadUint32(&rb.closed) == 1 {
		return ErrBufferClosed
	}

	switch rb.config.BackpressureStrategy {
	case BackpressureBlock:
		return rb.enqueueBlocking(ctx, event)
	case BackpressureDrop:
		return rb.enqueueDrop(event)
	case BackpressureSample:
		return rb.enqueueSample(event)
	default:
		return rb.enqueueBlocking(ctx, event)
	}
}

// enqueueBlocking blocks when buffer is full
func (rb *RingBuffer) enqueueBlocking(ctx context.Context, event *types.LogEvent) error {
	for {
		writePos := atomic.LoadUint64(&rb.writePos)
		readPos := atomic.LoadUint64(&rb.readPos)

		// Check if buffer is full
		if writePos-readPos >= rb.size {
			// Buffer is full, wait
			select {
			case <-rb.notFull:
				continue
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(rb.config.BlockTimeout):
				return ErrBufferFull
			}
		}

		// Try to claim a slot
		if atomic.CompareAndSwapUint64(&rb.writePos, writePos, writePos+1) {
			rb.buffer[writePos&rb.mask] = event
			atomic.AddUint64(&rb.enqueued, 1)

			// Signal that buffer is not empty
			select {
			case rb.notEmpty <- struct{}{}:
			default:
			}

			return nil
		}
	}
}

// enqueueDrop drops oldest event when buffer is full
func (rb *RingBuffer) enqueueDrop(event *types.LogEvent) error {
	for {
		writePos := atomic.LoadUint64(&rb.writePos)
		readPos := atomic.LoadUint64(&rb.readPos)

		// Check if buffer is full
		if writePos-readPos >= rb.size {
			// Drop the oldest event by advancing read position
			atomic.CompareAndSwapUint64(&rb.readPos, readPos, readPos+1)
			atomic.AddUint64(&rb.dropped, 1)
		}

		// Try to claim a slot
		if atomic.CompareAndSwapUint64(&rb.writePos, writePos, writePos+1) {
			rb.buffer[writePos&rb.mask] = event
			atomic.AddUint64(&rb.enqueued, 1)

			// Signal that buffer is not empty
			select {
			case rb.notEmpty <- struct{}{}:
			default:
			}

			return nil
		}
	}
}

// enqueueSample samples events when buffer is full
func (rb *RingBuffer) enqueueSample(event *types.LogEvent) error {
	for {
		writePos := atomic.LoadUint64(&rb.writePos)
		readPos := atomic.LoadUint64(&rb.readPos)

		// Check if buffer is full
		if writePos-readPos >= rb.size {
			// Sample: only keep 1 out of N events
			sampled := atomic.AddUint64(&rb.sampled, 1)
			if sampled%uint64(rb.config.SampleRate) != 0 {
				atomic.AddUint64(&rb.dropped, 1)
				return nil // Drop this event
			}
		}

		// Try to claim a slot
		if atomic.CompareAndSwapUint64(&rb.writePos, writePos, writePos+1) {
			rb.buffer[writePos&rb.mask] = event
			atomic.AddUint64(&rb.enqueued, 1)

			// Signal that buffer is not empty
			select {
			case rb.notEmpty <- struct{}{}:
			default:
			}

			return nil
		}
	}
}

// Dequeue removes and returns an event from the buffer
func (rb *RingBuffer) Dequeue(ctx context.Context) (*types.LogEvent, error) {
	for {
		if atomic.LoadUint32(&rb.closed) == 1 && rb.Empty() {
			return nil, ErrBufferClosed
		}

		readPos := atomic.LoadUint64(&rb.readPos)
		writePos := atomic.LoadUint64(&rb.writePos)

		// Check if buffer is empty
		if readPos >= writePos {
			// Buffer is empty, wait
			select {
			case <-rb.notEmpty:
				continue
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}

		// Try to claim an event
		if atomic.CompareAndSwapUint64(&rb.readPos, readPos, readPos+1) {
			event := rb.buffer[readPos&rb.mask]
			rb.buffer[readPos&rb.mask] = nil // Clear reference for GC
			atomic.AddUint64(&rb.dequeued, 1)

			// Signal that buffer is not full
			select {
			case rb.notFull <- struct{}{}:
			default:
			}

			return event, nil
		}
	}
}

// TryDequeue attempts to dequeue without blocking
func (rb *RingBuffer) TryDequeue() (*types.LogEvent, bool) {
	if atomic.LoadUint32(&rb.closed) == 1 && rb.Empty() {
		return nil, false
	}

	readPos := atomic.LoadUint64(&rb.readPos)
	writePos := atomic.LoadUint64(&rb.writePos)

	// Check if buffer is empty
	if readPos >= writePos {
		return nil, false
	}

	// Try to claim an event
	if atomic.CompareAndSwapUint64(&rb.readPos, readPos, readPos+1) {
		event := rb.buffer[readPos&rb.mask]
		rb.buffer[readPos&rb.mask] = nil
		atomic.AddUint64(&rb.dequeued, 1)

		// Signal that buffer is not full
		select {
		case rb.notFull <- struct{}{}:
		default:
		}

		return event, true
	}

	return nil, false
}

// Empty checks if buffer is empty
func (rb *RingBuffer) Empty() bool {
	readPos := atomic.LoadUint64(&rb.readPos)
	writePos := atomic.LoadUint64(&rb.writePos)
	return readPos >= writePos
}

// Full checks if buffer is full
func (rb *RingBuffer) Full() bool {
	readPos := atomic.LoadUint64(&rb.readPos)
	writePos := atomic.LoadUint64(&rb.writePos)
	return writePos-readPos >= rb.size
}

// Size returns the current number of events in the buffer
func (rb *RingBuffer) Size() int {
	readPos := atomic.LoadUint64(&rb.readPos)
	writePos := atomic.LoadUint64(&rb.writePos)
	return int(writePos - readPos)
}

// Capacity returns the maximum capacity of the buffer
func (rb *RingBuffer) Capacity() int {
	return int(rb.size)
}

// Utilization returns the buffer utilization percentage (0-100)
func (rb *RingBuffer) Utilization() float64 {
	size := float64(rb.Size())
	capacity := float64(rb.Capacity())
	if capacity == 0 {
		return 0
	}
	return (size / capacity) * 100.0
}

// Metrics returns buffer metrics
func (rb *RingBuffer) Metrics() BufferMetrics {
	return BufferMetrics{
		Enqueued:    atomic.LoadUint64(&rb.enqueued),
		Dequeued:    atomic.LoadUint64(&rb.dequeued),
		Dropped:     atomic.LoadUint64(&rb.dropped),
		CurrentSize: rb.Size(),
		Capacity:    rb.Capacity(),
		Utilization: rb.Utilization(),
	}
}

// Close closes the buffer
func (rb *RingBuffer) Close() error {
	if !atomic.CompareAndSwapUint32(&rb.closed, 0, 1) {
		return ErrBufferClosed
	}

	// Signal waiting goroutines
	close(rb.notEmpty)
	close(rb.notFull)

	return nil
}

// BufferMetrics holds buffer statistics
type BufferMetrics struct {
	Enqueued    uint64
	Dequeued    uint64
	Dropped     uint64
	CurrentSize int
	Capacity    int
	Utilization float64
}

// nextPowerOfTwo returns the next power of 2 greater than or equal to n
func nextPowerOfTwo(n uint64) uint64 {
	if n == 0 {
		return 1
	}
	n--
	n |= n >> 1
	n |= n >> 2
	n |= n >> 4
	n |= n >> 8
	n |= n >> 16
	n |= n >> 32
	n++
	return n
}
