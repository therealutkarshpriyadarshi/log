package pool

import (
	"testing"
)

func TestEventPool(t *testing.T) {
	// Get event from pool
	event := GetEvent()
	if event == nil {
		t.Fatal("Expected non-nil event")
	}

	// Verify event is clean
	if event.Timestamp != "" {
		t.Errorf("Expected empty timestamp, got %s", event.Timestamp)
	}

	if event.Level != "" {
		t.Errorf("Expected empty level, got %s", event.Level)
	}

	if event.Message != "" {
		t.Errorf("Expected empty message, got %s", event.Message)
	}

	if len(event.Fields) != 0 {
		t.Errorf("Expected empty fields, got %d", len(event.Fields))
	}

	// Set some values
	event.Timestamp = "2024-01-01T10:00:00Z"
	event.Level = "info"
	event.Message = "Test message"
	event.Fields["key"] = "value"

	// Return to pool
	PutEvent(event)

	// Get another event
	event2 := GetEvent()
	if event2 == nil {
		t.Fatal("Expected non-nil event")
	}

	// Verify it's clean (could be the same object)
	if event2.Timestamp != "" {
		t.Errorf("Expected empty timestamp, got %s", event2.Timestamp)
	}

	if len(event2.Fields) != 0 {
		t.Errorf("Expected empty fields, got %d", len(event2.Fields))
	}
}

func TestByteBufferPool(t *testing.T) {
	// Get buffer from pool
	buf := GetByteBuffer()
	if buf == nil {
		t.Fatal("Expected non-nil buffer")
	}

	// Verify buffer is empty
	if buf.Len() != 0 {
		t.Errorf("Expected empty buffer, got %d bytes", buf.Len())
	}

	// Write some data
	data := []byte("test data")
	buf.Write(data)

	if buf.Len() != len(data) {
		t.Errorf("Expected %d bytes, got %d", len(data), buf.Len())
	}

	// Return to pool
	PutByteBuffer(buf)

	// Get another buffer
	buf2 := GetByteBuffer()
	if buf2 == nil {
		t.Fatal("Expected non-nil buffer")
	}

	// Verify it's clean
	if buf2.Len() != 0 {
		t.Errorf("Expected empty buffer, got %d bytes", buf2.Len())
	}
}

func TestStringBuilderPool(t *testing.T) {
	pool := NewStringBuilderPool()
	if pool == nil {
		t.Fatal("Expected non-nil pool")
	}

	// Get builder from pool
	buf := pool.Get()
	if buf == nil {
		t.Fatal("Expected non-nil buffer")
	}

	// Write some data
	buf.WriteString("test")
	if buf.String() != "test" {
		t.Errorf("Expected 'test', got '%s'", buf.String())
	}

	// Return to pool
	pool.Put(buf)

	// Get another builder
	buf2 := pool.Get()
	if buf2 == nil {
		t.Fatal("Expected non-nil buffer")
	}

	// Verify it's clean
	if buf2.Len() != 0 {
		t.Errorf("Expected empty buffer, got %d bytes", buf2.Len())
	}
}

func TestSlicePool(t *testing.T) {
	sizes := []int{512, 4096, 65536}
	pool := NewSlicePool(sizes)

	// Test getting slices
	for _, size := range sizes {
		slice := pool.Get(size)
		if len(slice) != size {
			t.Errorf("Expected slice of length %d, got %d", size, len(slice))
		}

		// Return to pool
		pool.Put(slice)
	}

	// Test getting a size not in the pool
	slice := pool.Get(100)
	if len(slice) != 100 {
		t.Errorf("Expected slice of length 100, got %d", len(slice))
	}
}

func TestMapPool(t *testing.T) {
	pool := NewMapPool(8)
	if pool == nil {
		t.Fatal("Expected non-nil pool")
	}

	// Get map from pool
	m := pool.Get()
	if m == nil {
		t.Fatal("Expected non-nil map")
	}

	// Verify map is empty
	if len(m) != 0 {
		t.Errorf("Expected empty map, got %d entries", len(m))
	}

	// Add some entries
	m["key1"] = "value1"
	m["key2"] = 123

	if len(m) != 2 {
		t.Errorf("Expected 2 entries, got %d", len(m))
	}

	// Return to pool
	pool.Put(m)

	// Get another map
	m2 := pool.Get()
	if m2 == nil {
		t.Fatal("Expected non-nil map")
	}

	// Verify it's clean
	if len(m2) != 0 {
		t.Errorf("Expected empty map, got %d entries", len(m2))
	}
}

func TestDefaultPools(t *testing.T) {
	// Test DefaultSlicePool
	slice := DefaultSlicePool.Get(512)
	if len(slice) != 512 {
		t.Errorf("Expected slice of length 512, got %d", len(slice))
	}
	DefaultSlicePool.Put(slice)

	// Test DefaultMapPool
	m := DefaultMapPool.Get()
	if m == nil {
		t.Fatal("Expected non-nil map")
	}
	m["test"] = "value"
	DefaultMapPool.Put(m)
}

// Benchmarks

func BenchmarkEventPoolAllocation(b *testing.B) {
	b.Run("WithoutPool", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			event := &struct {
				Timestamp string
				Level     string
				Message   string
				Fields    map[string]interface{}
			}{
				Fields: make(map[string]interface{}, 8),
			}
			_ = event
		}
	})

	b.Run("WithPool", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			event := GetEvent()
			PutEvent(event)
		}
	})
}

func BenchmarkByteBufferAllocation(b *testing.B) {
	data := []byte("test data")

	b.Run("WithoutPool", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			var buf []byte
			buf = append(buf, data...)
			_ = buf
		}
	})

	b.Run("WithPool", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			buf := GetByteBuffer()
			buf.Write(data)
			PutByteBuffer(buf)
		}
	})
}

func BenchmarkMapAllocation(b *testing.B) {
	b.Run("WithoutPool", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			m := make(map[string]interface{}, 8)
			m["key"] = "value"
			_ = m
		}
	})

	b.Run("WithPool", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			m := DefaultMapPool.Get()
			m["key"] = "value"
			DefaultMapPool.Put(m)
		}
	})
}
