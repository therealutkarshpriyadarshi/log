package metrics

import (
	"runtime"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

func TestNewCollector(t *testing.T) {
	c := NewCollector()
	if c == nil {
		t.Fatal("NewCollector returned nil")
	}

	if c.registry == nil {
		t.Error("registry is nil")
	}

	if c.InputEventsReceived == nil {
		t.Error("InputEventsReceived is nil")
	}

	if c.ParserDuration == nil {
		t.Error("ParserDuration is nil")
	}

	if c.OutputEventsSent == nil {
		t.Error("OutputEventsSent is nil")
	}
}

func TestInputMetrics(t *testing.T) {
	c := NewCollector()

	// Test counter
	c.InputEventsReceived.WithLabelValues("test-input", "file").Add(100)

	// Verify metric value
	metric := &dto.Metric{}
	if err := c.InputEventsReceived.WithLabelValues("test-input", "file").(prometheus.Counter).Write(metric); err != nil {
		t.Fatalf("Failed to write metric: %v", err)
	}

	if metric.Counter.GetValue() != 100 {
		t.Errorf("Expected 100, got %f", metric.Counter.GetValue())
	}
}

func TestParserMetrics(t *testing.T) {
	c := NewCollector()

	// Test counter
	c.ParserEventsProcessed.WithLabelValues("json").Add(50)

	// Test histogram
	c.ParserDuration.WithLabelValues("json").Observe(0.001) // 1ms

	// Verify counter
	metric := &dto.Metric{}
	if err := c.ParserEventsProcessed.WithLabelValues("json").(prometheus.Counter).Write(metric); err != nil {
		t.Fatalf("Failed to write metric: %v", err)
	}

	if metric.Counter.GetValue() != 50 {
		t.Errorf("Expected 50, got %f", metric.Counter.GetValue())
	}
}

func TestBufferMetrics(t *testing.T) {
	c := NewCollector()

	// Test gauge
	c.BufferSize.WithLabelValues("memory").Set(1024)
	c.BufferUtilization.WithLabelValues("memory").Set(0.75)

	// Test counter
	c.BufferDropped.WithLabelValues("memory", "drop").Add(10)

	// Verify gauge
	metric := &dto.Metric{}
	if err := c.BufferSize.WithLabelValues("memory").(prometheus.Gauge).Write(metric); err != nil {
		t.Fatalf("Failed to write metric: %v", err)
	}

	if metric.Gauge.GetValue() != 1024 {
		t.Errorf("Expected 1024, got %f", metric.Gauge.GetValue())
	}
}

func TestOutputMetrics(t *testing.T) {
	c := NewCollector()

	// Test metrics
	c.OutputEventsSent.WithLabelValues("kafka-out", "kafka").Add(1000)
	c.OutputBytesSent.WithLabelValues("kafka-out", "kafka").Add(50000)
	c.OutputDuration.WithLabelValues("kafka-out", "kafka").Observe(0.050) // 50ms
	c.OutputBatchSize.WithLabelValues("kafka-out", "kafka").Observe(100)

	// Verify counter
	metric := &dto.Metric{}
	if err := c.OutputEventsSent.WithLabelValues("kafka-out", "kafka").(prometheus.Counter).Write(metric); err != nil {
		t.Fatalf("Failed to write metric: %v", err)
	}

	if metric.Counter.GetValue() != 1000 {
		t.Errorf("Expected 1000, got %f", metric.Counter.GetValue())
	}
}

func TestSystemMetrics(t *testing.T) {
	c := NewCollector()

	// Collect system metrics
	c.collectSystemMetrics()

	// Verify metrics are set
	metric := &dto.Metric{}

	if err := c.SystemGoroutines.Write(metric); err != nil {
		t.Fatalf("Failed to write metric: %v", err)
	}

	goroutines := runtime.NumGoroutine()
	if metric.Gauge.GetValue() <= 0 {
		t.Errorf("Expected positive goroutine count, got %f", metric.Gauge.GetValue())
	}

	if int(metric.Gauge.GetValue()) != goroutines {
		t.Logf("Goroutines metric: %d, actual: %d (may differ due to timing)", int(metric.Gauge.GetValue()), goroutines)
	}

	if err := c.SystemMemAlloc.Write(metric); err != nil {
		t.Fatalf("Failed to write metric: %v", err)
	}

	if metric.Gauge.GetValue() <= 0 {
		t.Errorf("Expected positive memory allocation, got %f", metric.Gauge.GetValue())
	}
}

func TestStartStop(t *testing.T) {
	c := NewCollector()

	if c.started {
		t.Error("Collector should not be started initially")
	}

	c.Start()

	if !c.started {
		t.Error("Collector should be started after Start()")
	}

	// Wait a bit to let the background goroutine collect metrics
	time.Sleep(100 * time.Millisecond)

	c.Stop()

	if c.started {
		t.Error("Collector should not be started after Stop()")
	}
}

func TestGetGlobalCollector(t *testing.T) {
	c1 := GetGlobalCollector()
	if c1 == nil {
		t.Fatal("GetGlobalCollector returned nil")
	}

	c2 := GetGlobalCollector()
	if c1 != c2 {
		t.Error("GetGlobalCollector should return the same instance")
	}

	if !c1.started {
		t.Error("Global collector should be started")
	}
}

func TestWALMetrics(t *testing.T) {
	c := NewCollector()

	c.WALWriteBytes.WithLabelValues("/tmp/wal").Add(4096)
	c.WALSegments.WithLabelValues("/tmp/wal").Set(5)
	c.WALWriteDuration.WithLabelValues("/tmp/wal").Observe(0.001)
	c.WALCompactionCount.WithLabelValues("/tmp/wal").Add(1)

	// Verify metrics
	metric := &dto.Metric{}
	if err := c.WALWriteBytes.WithLabelValues("/tmp/wal").(prometheus.Counter).Write(metric); err != nil {
		t.Fatalf("Failed to write metric: %v", err)
	}

	if metric.Counter.GetValue() != 4096 {
		t.Errorf("Expected 4096, got %f", metric.Counter.GetValue())
	}
}

func TestWorkerPoolMetrics(t *testing.T) {
	c := NewCollector()

	c.WorkerPoolSize.WithLabelValues("default").Set(10)
	c.WorkerPoolJobs.WithLabelValues("default", "completed").Add(100)
	c.WorkerPoolRetries.WithLabelValues("default").Add(5)
	c.WorkerJobDuration.WithLabelValues("default").Observe(0.050)

	// Verify metrics
	metric := &dto.Metric{}
	if err := c.WorkerPoolSize.WithLabelValues("default").(prometheus.Gauge).Write(metric); err != nil {
		t.Fatalf("Failed to write metric: %v", err)
	}

	if metric.Gauge.GetValue() != 10 {
		t.Errorf("Expected 10, got %f", metric.Gauge.GetValue())
	}
}

func TestCircuitBreakerMetrics(t *testing.T) {
	c := NewCollector()

	c.CircuitBreakerState.WithLabelValues("kafka").Set(0) // Closed
	c.CircuitBreakerConsecutive.WithLabelValues("kafka").Set(0)

	// Verify metrics
	metric := &dto.Metric{}
	if err := c.CircuitBreakerState.WithLabelValues("kafka").(prometheus.Gauge).Write(metric); err != nil {
		t.Fatalf("Failed to write metric: %v", err)
	}

	if metric.Gauge.GetValue() != 0 {
		t.Errorf("Expected 0, got %f", metric.Gauge.GetValue())
	}
}

func TestHealthMetrics(t *testing.T) {
	c := NewCollector()

	c.HealthStatus.WithLabelValues("input").Set(1) // Healthy

	// Verify metrics
	metric := &dto.Metric{}
	if err := c.HealthStatus.WithLabelValues("input").(prometheus.Gauge).Write(metric); err != nil {
		t.Fatalf("Failed to write metric: %v", err)
	}

	if metric.Gauge.GetValue() != 1 {
		t.Errorf("Expected 1, got %f", metric.Gauge.GetValue())
	}
}
