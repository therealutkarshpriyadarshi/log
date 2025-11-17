package metrics

import (
	"runtime"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Namespace for all metrics
const namespace = "logaggregator"

// Collector provides a central place for all application metrics
type Collector struct {
	// Input metrics
	InputEventsReceived   *prometheus.CounterVec
	InputBytesReceived    *prometheus.CounterVec
	InputEventsDropped    *prometheus.CounterVec
	InputConnectionsTotal *prometheus.GaugeVec
	InputRateLimited      *prometheus.CounterVec

	// Parser metrics
	ParserEventsProcessed *prometheus.CounterVec
	ParserEventsFailed    *prometheus.CounterVec
	ParserDuration        *prometheus.HistogramVec

	// Buffer metrics
	BufferSize        *prometheus.GaugeVec
	BufferUtilization *prometheus.GaugeVec
	BufferDropped     *prometheus.CounterVec
	BufferBlocked     *prometheus.CounterVec

	// WAL metrics
	WALWriteBytes      *prometheus.CounterVec
	WALSegments        *prometheus.GaugeVec
	WALWriteDuration   *prometheus.HistogramVec
	WALCompactionCount *prometheus.CounterVec

	// Output metrics
	OutputEventsSent   *prometheus.CounterVec
	OutputEventsFailed *prometheus.CounterVec
	OutputBytesSent    *prometheus.CounterVec
	OutputDuration     *prometheus.HistogramVec
	OutputBatchSize    *prometheus.HistogramVec

	// Worker pool metrics
	WorkerPoolSize    *prometheus.GaugeVec
	WorkerPoolJobs    *prometheus.CounterVec
	WorkerPoolRetries *prometheus.CounterVec
	WorkerJobDuration *prometheus.HistogramVec

	// System metrics
	SystemGoroutines *prometheus.Gauge
	SystemMemAlloc   *prometheus.Gauge
	SystemMemSys     *prometheus.Gauge
	SystemGCPauses   *prometheus.Histogram

	// Dead letter queue metrics
	DLQEventsWritten *prometheus.Counter
	DLQSize          *prometheus.Gauge

	// Circuit breaker metrics
	CircuitBreakerState       *prometheus.GaugeVec
	CircuitBreakerConsecutive *prometheus.GaugeVec

	// Health metrics
	HealthStatus *prometheus.GaugeVec

	registry *prometheus.Registry
	mu       sync.RWMutex
	started  bool
}

// NewCollector creates a new metrics collector
func NewCollector() *Collector {
	registry := prometheus.NewRegistry()

	c := &Collector{
		registry: registry,
	}

	c.initInputMetrics()
	c.initParserMetrics()
	c.initBufferMetrics()
	c.initWALMetrics()
	c.initOutputMetrics()
	c.initWorkerPoolMetrics()
	c.initSystemMetrics()
	c.initDLQMetrics()
	c.initCircuitBreakerMetrics()
	c.initHealthMetrics()

	return c
}

func (c *Collector) initInputMetrics() {
	c.InputEventsReceived = promauto.With(c.registry).NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: "input",
			Name:      "events_received_total",
			Help:      "Total number of events received by input source",
		},
		[]string{"input_name", "input_type"},
	)

	c.InputBytesReceived = promauto.With(c.registry).NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: "input",
			Name:      "bytes_received_total",
			Help:      "Total bytes received by input source",
		},
		[]string{"input_name", "input_type"},
	)

	c.InputEventsDropped = promauto.With(c.registry).NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: "input",
			Name:      "events_dropped_total",
			Help:      "Total number of events dropped by input source",
		},
		[]string{"input_name", "input_type", "reason"},
	)

	c.InputConnectionsTotal = promauto.With(c.registry).NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: "input",
			Name:      "connections_total",
			Help:      "Current number of active connections",
		},
		[]string{"input_name", "input_type"},
	)

	c.InputRateLimited = promauto.With(c.registry).NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: "input",
			Name:      "rate_limited_total",
			Help:      "Total number of rate-limited requests",
		},
		[]string{"input_name", "input_type"},
	)
}

func (c *Collector) initParserMetrics() {
	c.ParserEventsProcessed = promauto.With(c.registry).NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: "parser",
			Name:      "events_processed_total",
			Help:      "Total number of events successfully parsed",
		},
		[]string{"parser_type"},
	)

	c.ParserEventsFailed = promauto.With(c.registry).NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: "parser",
			Name:      "events_failed_total",
			Help:      "Total number of events that failed parsing",
		},
		[]string{"parser_type", "reason"},
	)

	c.ParserDuration = promauto.With(c.registry).NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: "parser",
			Name:      "duration_seconds",
			Help:      "Time taken to parse an event",
			Buckets:   prometheus.ExponentialBuckets(0.00001, 2, 15), // 10µs to ~300ms
		},
		[]string{"parser_type"},
	)
}

func (c *Collector) initBufferMetrics() {
	c.BufferSize = promauto.With(c.registry).NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: "buffer",
			Name:      "size_bytes",
			Help:      "Current buffer size in bytes",
		},
		[]string{"buffer_type"},
	)

	c.BufferUtilization = promauto.With(c.registry).NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: "buffer",
			Name:      "utilization_ratio",
			Help:      "Buffer utilization ratio (0.0-1.0)",
		},
		[]string{"buffer_type"},
	)

	c.BufferDropped = promauto.With(c.registry).NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: "buffer",
			Name:      "events_dropped_total",
			Help:      "Total number of events dropped due to buffer full",
		},
		[]string{"buffer_type", "backpressure_strategy"},
	)

	c.BufferBlocked = promauto.With(c.registry).NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: "buffer",
			Name:      "blocked_total",
			Help:      "Total number of times buffer was blocked",
		},
		[]string{"buffer_type"},
	)
}

func (c *Collector) initWALMetrics() {
	c.WALWriteBytes = promauto.With(c.registry).NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: "wal",
			Name:      "write_bytes_total",
			Help:      "Total bytes written to WAL",
		},
		[]string{"wal_dir"},
	)

	c.WALSegments = promauto.With(c.registry).NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: "wal",
			Name:      "segments_total",
			Help:      "Current number of WAL segments",
		},
		[]string{"wal_dir"},
	)

	c.WALWriteDuration = promauto.With(c.registry).NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: "wal",
			Name:      "write_duration_seconds",
			Help:      "Time taken to write to WAL",
			Buckets:   prometheus.ExponentialBuckets(0.0001, 2, 12), // 100µs to ~400ms
		},
		[]string{"wal_dir"},
	)

	c.WALCompactionCount = promauto.With(c.registry).NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: "wal",
			Name:      "compaction_total",
			Help:      "Total number of WAL compactions",
		},
		[]string{"wal_dir"},
	)
}

func (c *Collector) initOutputMetrics() {
	c.OutputEventsSent = promauto.With(c.registry).NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: "output",
			Name:      "events_sent_total",
			Help:      "Total number of events successfully sent to output",
		},
		[]string{"output_name", "output_type"},
	)

	c.OutputEventsFailed = promauto.With(c.registry).NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: "output",
			Name:      "events_failed_total",
			Help:      "Total number of events that failed to send",
		},
		[]string{"output_name", "output_type", "reason"},
	)

	c.OutputBytesSent = promauto.With(c.registry).NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: "output",
			Name:      "bytes_sent_total",
			Help:      "Total bytes sent to output",
		},
		[]string{"output_name", "output_type"},
	)

	c.OutputDuration = promauto.With(c.registry).NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: "output",
			Name:      "duration_seconds",
			Help:      "Time taken to send events to output",
			Buckets:   prometheus.ExponentialBuckets(0.001, 2, 12), // 1ms to ~4s
		},
		[]string{"output_name", "output_type"},
	)

	c.OutputBatchSize = promauto.With(c.registry).NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: "output",
			Name:      "batch_size",
			Help:      "Number of events in each batch sent to output",
			Buckets:   prometheus.ExponentialBuckets(1, 2, 12), // 1 to 4096
		},
		[]string{"output_name", "output_type"},
	)
}

func (c *Collector) initWorkerPoolMetrics() {
	c.WorkerPoolSize = promauto.With(c.registry).NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: "worker_pool",
			Name:      "workers_total",
			Help:      "Current number of workers in the pool",
		},
		[]string{"pool_name"},
	)

	c.WorkerPoolJobs = promauto.With(c.registry).NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: "worker_pool",
			Name:      "jobs_total",
			Help:      "Total number of jobs processed",
		},
		[]string{"pool_name", "status"},
	)

	c.WorkerPoolRetries = promauto.With(c.registry).NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: "worker_pool",
			Name:      "retries_total",
			Help:      "Total number of job retries",
		},
		[]string{"pool_name"},
	)

	c.WorkerJobDuration = promauto.With(c.registry).NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: "worker_pool",
			Name:      "job_duration_seconds",
			Help:      "Time taken to process a job",
			Buckets:   prometheus.ExponentialBuckets(0.001, 2, 12), // 1ms to ~4s
		},
		[]string{"pool_name"},
	)
}

func (c *Collector) initSystemMetrics() {
	c.SystemGoroutines = promauto.With(c.registry).NewGauge(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: "system",
			Name:      "goroutines_total",
			Help:      "Current number of goroutines",
		},
	)

	c.SystemMemAlloc = promauto.With(c.registry).NewGauge(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: "system",
			Name:      "memory_allocated_bytes",
			Help:      "Bytes of allocated heap objects",
		},
	)

	c.SystemMemSys = promauto.With(c.registry).NewGauge(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: "system",
			Name:      "memory_system_bytes",
			Help:      "Total bytes of memory obtained from the OS",
		},
	)

	c.SystemGCPauses = promauto.With(c.registry).NewHistogram(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: "system",
			Name:      "gc_pause_seconds",
			Help:      "GC pause duration",
			Buckets:   prometheus.ExponentialBuckets(0.00001, 2, 15), // 10µs to ~300ms
		},
	)
}

func (c *Collector) initDLQMetrics() {
	c.DLQEventsWritten = promauto.With(c.registry).NewCounter(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: "dlq",
			Name:      "events_written_total",
			Help:      "Total number of events written to dead letter queue",
		},
	)

	c.DLQSize = promauto.With(c.registry).NewGauge(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: "dlq",
			Name:      "size_bytes",
			Help:      "Current size of dead letter queue in bytes",
		},
	)
}

func (c *Collector) initCircuitBreakerMetrics() {
	c.CircuitBreakerState = promauto.With(c.registry).NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: "circuit_breaker",
			Name:      "state",
			Help:      "Circuit breaker state (0=closed, 1=open, 2=half-open)",
		},
		[]string{"name"},
	)

	c.CircuitBreakerConsecutive = promauto.With(c.registry).NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: "circuit_breaker",
			Name:      "consecutive_failures",
			Help:      "Current number of consecutive failures",
		},
		[]string{"name"},
	)
}

func (c *Collector) initHealthMetrics() {
	c.HealthStatus = promauto.With(c.registry).NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: "health",
			Name:      "status",
			Help:      "Health status of components (1=healthy, 0=unhealthy)",
		},
		[]string{"component"},
	)
}

// Start begins collecting system metrics periodically
func (c *Collector) Start() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.started {
		return
	}

	c.started = true

	// Collect system metrics every 15 seconds
	go func() {
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()

		for range ticker.C {
			c.collectSystemMetrics()
		}
	}()
}

// Stop stops the metrics collector
func (c *Collector) Stop() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.started = false
}

// collectSystemMetrics gathers runtime metrics
func (c *Collector) collectSystemMetrics() {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	c.SystemGoroutines.Set(float64(runtime.NumGoroutine()))
	c.SystemMemAlloc.Set(float64(m.Alloc))
	c.SystemMemSys.Set(float64(m.Sys))

	// Record GC pause time
	if len(m.PauseNs) > 0 {
		lastPause := m.PauseNs[(m.NumGC+255)%256]
		c.SystemGCPauses.Observe(float64(lastPause) / 1e9)
	}
}

// Registry returns the Prometheus registry
func (c *Collector) Registry() *prometheus.Registry {
	return c.registry
}

// Global metrics collector
var (
	globalCollector *Collector
	once            sync.Once
)

// GetGlobalCollector returns the global metrics collector
func GetGlobalCollector() *Collector {
	once.Do(func() {
		globalCollector = NewCollector()
		globalCollector.Start()
	})
	return globalCollector
}
