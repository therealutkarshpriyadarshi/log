package benchmark

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/therealutkarshpriyadarshi/log/internal/buffer"
	"github.com/therealutkarshpriyadarshi/log/internal/parser"
	"github.com/therealutkarshpriyadarshi/log/internal/pool"
	"github.com/therealutkarshpriyadarshi/log/internal/worker"
	"github.com/therealutkarshpriyadarshi/log/pkg/types"
)

// BenchmarkParserJSON benchmarks JSON parser
func BenchmarkParserJSON(b *testing.B) {
	cfg := &parser.ParserConfig{
		Type:         parser.ParserTypeJSON,
		TimeField:    "timestamp",
		LevelField:   "level",
		MessageField: "message",
	}

	p, err := parser.New(cfg)
	if err != nil {
		b.Fatal(err)
	}

	logLine := `{"timestamp":"2024-01-01T10:00:00Z","level":"info","message":"Test log message","user_id":123,"request_id":"abc-123"}`

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := p.Parse(logLine, "test.log")
		if err != nil {
			b.Fatal(err)
		}
	}

	b.ReportMetric(float64(b.N)/b.Elapsed().Seconds(), "events/sec")
}

// BenchmarkParserJSONWithPool benchmarks JSON parser with object pooling
func BenchmarkParserJSONWithPool(b *testing.B) {
	cfg := &parser.ParserConfig{
		Type:         parser.ParserTypeJSON,
		TimeField:    "timestamp",
		LevelField:   "level",
		MessageField: "message",
	}

	p, err := parser.New(cfg)
	if err != nil {
		b.Fatal(err)
	}

	logLine := `{"timestamp":"2024-01-01T10:00:00Z","level":"info","message":"Test log message","user_id":123,"request_id":"abc-123"}`

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		event, err := p.Parse(logLine, "test.log")
		if err != nil {
			b.Fatal(err)
		}
		pool.PutEvent(event)
	}

	b.ReportMetric(float64(b.N)/b.Elapsed().Seconds(), "events/sec")
}

// BenchmarkParserRegex benchmarks regex parser
func BenchmarkParserRegex(b *testing.B) {
	cfg := &parser.ParserConfig{
		Type:       parser.ParserTypeRegex,
		Pattern:    `^(?P<timestamp>\S+)\s+\[(?P<level>\w+)\]\s+(?P<message>.*)$`,
		TimeField:  "timestamp",
		LevelField: "level",
	}

	p, err := parser.New(cfg)
	if err != nil {
		b.Fatal(err)
	}

	logLine := "2024-01-01T10:00:00Z [INFO] This is a test log message"

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := p.Parse(logLine, "test.log")
		if err != nil {
			b.Fatal(err)
		}
	}

	b.ReportMetric(float64(b.N)/b.Elapsed().Seconds(), "events/sec")
}

// BenchmarkRingBufferEnqueue benchmarks ring buffer enqueue
func BenchmarkRingBufferEnqueue(b *testing.B) {
	cfg := buffer.RingBufferConfig{
		Size:                 1024 * 1024,
		BackpressureStrategy: buffer.BackpressureDrop,
	}

	rb, err := buffer.NewRingBuffer(cfg)
	if err != nil {
		b.Fatal(err)
	}

	event := &types.LogEvent{
		Timestamp: time.Now().Format(time.RFC3339),
		Level:     "info",
		Message:   "Test message",
		Source:    "test.log",
		Fields:    make(map[string]interface{}),
	}

	ctx := context.Background()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = rb.Enqueue(ctx, event)
	}

	b.ReportMetric(float64(b.N)/b.Elapsed().Seconds(), "events/sec")
}

// BenchmarkRingBufferEnqueueDequeue benchmarks concurrent enqueue/dequeue
func BenchmarkRingBufferEnqueueDequeue(b *testing.B) {
	cfg := buffer.RingBufferConfig{
		Size:                 1024 * 1024,
		BackpressureStrategy: buffer.BackpressureBlock,
	}

	rb, err := buffer.NewRingBuffer(cfg)
	if err != nil {
		b.Fatal(err)
	}

	ctx := context.Background()

	b.ResetTimer()
	b.ReportAllocs()

	// Start dequeue goroutine
	go func() {
		for i := 0; i < b.N; i++ {
			_, _ = rb.Dequeue(ctx)
		}
	}()

	// Enqueue
	for i := 0; i < b.N; i++ {
		event := pool.GetEvent()
		event.Timestamp = time.Now().Format(time.RFC3339)
		event.Level = "info"
		event.Message = fmt.Sprintf("Test message %d", i)
		event.Source = "test.log"

		_ = rb.Enqueue(ctx, event)
	}

	b.ReportMetric(float64(b.N)/b.Elapsed().Seconds(), "events/sec")
}

// BenchmarkWorkerPool benchmarks worker pool processing
func BenchmarkWorkerPool(b *testing.B) {
	cfg := worker.PoolConfig{
		WorkerCount: 4,
		JobTimeout:  5 * time.Second,
	}

	pool := worker.NewPool(cfg)
	pool.Start()
	defer pool.Stop()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		job := worker.Job{
			ID: fmt.Sprintf("job-%d", i),
			Execute: func(ctx context.Context) error {
				// Simulate some work
				return nil
			},
		}

		_ = pool.Submit(job)
	}

	b.ReportMetric(float64(b.N)/b.Elapsed().Seconds(), "jobs/sec")
}

// BenchmarkObjectPooling benchmarks object pooling
func BenchmarkObjectPooling(b *testing.B) {
	b.Run("WithoutPool", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			event := &types.LogEvent{
				Timestamp: time.Now().Format(time.RFC3339),
				Level:     "info",
				Message:   "Test message",
				Source:    "test.log",
				Fields:    make(map[string]interface{}),
			}
			_ = event
		}
	})

	b.Run("WithPool", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			event := pool.GetEvent()
			event.Timestamp = time.Now().Format(time.RFC3339)
			event.Level = "info"
			event.Message = "Test message"
			event.Source = "test.log"
			pool.PutEvent(event)
		}
	})
}

// BenchmarkByteBufferPooling benchmarks byte buffer pooling
func BenchmarkByteBufferPooling(b *testing.B) {
	data := []byte("This is some test data that will be written to a buffer")

	b.Run("WithoutPool", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			var buf []byte
			buf = append(buf, data...)
			_ = buf
		}
	})

	b.Run("WithPool", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			buf := pool.GetByteBuffer()
			buf.Write(data)
			pool.PutByteBuffer(buf)
		}
	})
}

// BenchmarkParallelParsing benchmarks parallel parsing with different worker counts
func BenchmarkParallelParsing(b *testing.B) {
	cfg := &parser.ParserConfig{
		Type:         parser.ParserTypeJSON,
		TimeField:    "timestamp",
		LevelField:   "level",
		MessageField: "message",
	}

	p, err := parser.New(cfg)
	if err != nil {
		b.Fatal(err)
	}

	logLine := `{"timestamp":"2024-01-01T10:00:00Z","level":"info","message":"Test log message","user_id":123}`

	for _, workers := range []int{1, 2, 4, 8, 16} {
		b.Run(fmt.Sprintf("Workers-%d", workers), func(b *testing.B) {
			b.ReportAllocs()
			b.SetParallelism(workers)
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					_, _ = p.Parse(logLine, "test.log")
				}
			})
			b.ReportMetric(float64(b.N)/b.Elapsed().Seconds(), "events/sec")
		})
	}
}

// BenchmarkEndToEnd benchmarks end-to-end processing
func BenchmarkEndToEnd(b *testing.B) {
	// Setup parser
	parserCfg := &parser.ParserConfig{
		Type:         parser.ParserTypeJSON,
		TimeField:    "timestamp",
		LevelField:   "level",
		MessageField: "message",
	}
	p, _ := parser.New(parserCfg)

	// Setup buffer
	bufferCfg := buffer.RingBufferConfig{
		Size:                 1024 * 1024,
		BackpressureStrategy: buffer.BackpressureDrop,
	}
	rb, _ := buffer.NewRingBuffer(bufferCfg)

	logLine := `{"timestamp":"2024-01-01T10:00:00Z","level":"info","message":"Test log message"}`
	ctx := context.Background()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		// Parse
		event, err := p.Parse(logLine, "test.log")
		if err != nil {
			b.Fatal(err)
		}

		// Buffer
		_ = rb.Enqueue(ctx, event)
	}

	b.ReportMetric(float64(b.N)/b.Elapsed().Seconds(), "events/sec")
}

// BenchmarkHighThroughput simulates high throughput scenario
func BenchmarkHighThroughput(b *testing.B) {
	parserCfg := &parser.ParserConfig{
		Type:         parser.ParserTypeJSON,
		TimeField:    "timestamp",
		LevelField:   "level",
		MessageField: "message",
	}
	p, _ := parser.New(parserCfg)

	logLines := []string{
		`{"timestamp":"2024-01-01T10:00:00Z","level":"info","message":"User login","user_id":123}`,
		`{"timestamp":"2024-01-01T10:00:01Z","level":"warn","message":"High latency detected","latency_ms":350}`,
		`{"timestamp":"2024-01-01T10:00:02Z","level":"error","message":"Database connection failed","error":"timeout"}`,
		`{"timestamp":"2024-01-01T10:00:03Z","level":"info","message":"Request processed","duration_ms":45}`,
	}

	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			logLine := logLines[i%len(logLines)]
			event, _ := p.Parse(logLine, "test.log")
			pool.PutEvent(event)
			i++
		}
	})

	b.ReportMetric(float64(b.N)/b.Elapsed().Seconds(), "events/sec")
}
