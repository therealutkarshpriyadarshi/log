package pool

import (
	"bytes"
	"sync"

	"github.com/therealutkarshpriyadarshi/log/pkg/types"
)

// EventPool is a pool of LogEvent objects to reduce allocations
var EventPool = sync.Pool{
	New: func() interface{} {
		return &types.LogEvent{
			Fields: make(map[string]interface{}, 8), // Pre-allocate some capacity
		}
	},
}

// GetEvent retrieves a LogEvent from the pool
func GetEvent() *types.LogEvent {
	event := EventPool.Get().(*types.LogEvent)
	// Reset the event
	event.Timestamp = ""
	event.Level = ""
	event.Message = ""
	event.Source = ""
	event.Raw = ""
	// Clear map but keep allocated memory
	for k := range event.Fields {
		delete(event.Fields, k)
	}
	return event
}

// PutEvent returns a LogEvent to the pool
func PutEvent(event *types.LogEvent) {
	if event != nil {
		EventPool.Put(event)
	}
}

// ByteBufferPool is a pool of byte buffers for parsing and I/O
var ByteBufferPool = sync.Pool{
	New: func() interface{} {
		return new(bytes.Buffer)
	},
}

// GetByteBuffer retrieves a byte buffer from the pool
func GetByteBuffer() *bytes.Buffer {
	buf := ByteBufferPool.Get().(*bytes.Buffer)
	buf.Reset()
	return buf
}

// PutByteBuffer returns a byte buffer to the pool
func PutByteBuffer(buf *bytes.Buffer) {
	if buf != nil {
		// Only pool buffers under 64KB to avoid holding too much memory
		if buf.Cap() < 64*1024 {
			buf.Reset()
			ByteBufferPool.Put(buf)
		}
	}
}

// StringBuilderPool is a pool of strings.Builder for efficient string concatenation
type StringBuilderPool struct {
	pool sync.Pool
}

// NewStringBuilderPool creates a new string builder pool
func NewStringBuilderPool() *StringBuilderPool {
	return &StringBuilderPool{
		pool: sync.Pool{
			New: func() interface{} {
				return new(bytes.Buffer)
			},
		},
	}
}

// Get retrieves a string builder from the pool
func (p *StringBuilderPool) Get() *bytes.Buffer {
	buf := p.pool.Get().(*bytes.Buffer)
	buf.Reset()
	return buf
}

// Put returns a string builder to the pool
func (p *StringBuilderPool) Put(buf *bytes.Buffer) {
	if buf != nil && buf.Cap() < 64*1024 {
		buf.Reset()
		p.pool.Put(buf)
	}
}

// SlicePool manages pools of byte slices with different sizes
type SlicePool struct {
	pools map[int]*sync.Pool
}

// NewSlicePool creates a new slice pool
func NewSlicePool(sizes []int) *SlicePool {
	sp := &SlicePool{
		pools: make(map[int]*sync.Pool),
	}

	for _, size := range sizes {
		s := size // Capture for closure
		sp.pools[size] = &sync.Pool{
			New: func() interface{} {
				b := make([]byte, s)
				return &b
			},
		}
	}

	return sp
}

// Get retrieves a byte slice of the specified size
func (sp *SlicePool) Get(size int) []byte {
	// Find the smallest pool that can fit this size
	for poolSize, pool := range sp.pools {
		if poolSize >= size {
			slicePtr := pool.Get().(*[]byte)
			return (*slicePtr)[:size]
		}
	}

	// If no pool fits, allocate directly
	return make([]byte, size)
}

// Put returns a byte slice to the appropriate pool
func (sp *SlicePool) Put(slice []byte) {
	cap := cap(slice)
	if pool, ok := sp.pools[cap]; ok {
		slicePtr := &slice
		pool.Put(slicePtr)
	}
}

// DefaultSlicePool is a pre-configured slice pool with common sizes
var DefaultSlicePool = NewSlicePool([]int{
	512,      // Small buffers
	4096,     // Medium buffers (page size)
	65536,    // Large buffers (64KB)
	1048576,  // Very large buffers (1MB)
})

// MapPool is a pool of string maps for parsed fields
type MapPool struct {
	pool sync.Pool
}

// NewMapPool creates a new map pool
func NewMapPool(initialCapacity int) *MapPool {
	return &MapPool{
		pool: sync.Pool{
			New: func() interface{} {
				return make(map[string]interface{}, initialCapacity)
			},
		},
	}
}

// Get retrieves a map from the pool
func (p *MapPool) Get() map[string]interface{} {
	m := p.pool.Get().(map[string]interface{})
	// Clear the map
	for k := range m {
		delete(m, k)
	}
	return m
}

// Put returns a map to the pool
func (p *MapPool) Put(m map[string]interface{}) {
	if m != nil && len(m) < 100 { // Don't pool very large maps
		p.pool.Put(m)
	}
}

// DefaultMapPool is a pre-configured map pool
var DefaultMapPool = NewMapPool(8)

// Stats returns pooling statistics
type Stats struct {
	EventPoolHits   uint64
	EventPoolMisses uint64
	BufferPoolHits  uint64
	BufferPoolMisses uint64
}

// Global stats (simplified for demonstration)
var globalStats Stats

// GetStats returns current pool statistics
func GetStats() Stats {
	return globalStats
}
