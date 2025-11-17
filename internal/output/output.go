package output

import (
	"context"
	"time"

	"github.com/therealutkarshpriyadarshi/log/pkg/types"
)

// Output defines the interface for all output plugins
type Output interface {
	// Send sends a single event to the output destination
	Send(ctx context.Context, event *types.LogEvent) error

	// SendBatch sends a batch of events to the output destination
	SendBatch(ctx context.Context, events []*types.LogEvent) error

	// Close closes the output and releases resources
	Close() error

	// Name returns the name of the output plugin
	Name() string

	// Metrics returns the current metrics for this output
	Metrics() *OutputMetrics
}

// OutputMetrics tracks performance and health metrics for an output
type OutputMetrics struct {
	EventsSent      int64         `json:"events_sent"`
	EventsFailed    int64         `json:"events_failed"`
	BytesSent       int64         `json:"bytes_sent"`
	BatchesSent     int64         `json:"batches_sent"`
	RetryCount      int64         `json:"retry_count"`
	LastSendTime    time.Time     `json:"last_send_time"`
	LastError       string        `json:"last_error,omitempty"`
	LastErrorTime   time.Time     `json:"last_error_time,omitempty"`
	AvgBatchSize    float64       `json:"avg_batch_size"`
	AvgLatency      time.Duration `json:"avg_latency"`
}

// CompressionType defines the compression algorithm to use
type CompressionType string

const (
	CompressionNone   CompressionType = "none"
	CompressionGzip   CompressionType = "gzip"
	CompressionSnappy CompressionType = "snappy"
	CompressionLZ4    CompressionType = "lz4"
)

// BaseConfig contains common configuration for all outputs
type BaseConfig struct {
	// Type is the output type (kafka, elasticsearch, s3, etc.)
	Type string `yaml:"type"`

	// Name is a unique identifier for this output instance
	Name string `yaml:"name,omitempty"`

	// BatchSize is the number of events to batch before sending
	BatchSize int `yaml:"batch_size,omitempty"`

	// BatchTimeout is the maximum time to wait before sending a partial batch
	BatchTimeout time.Duration `yaml:"batch_timeout,omitempty"`

	// Compression specifies the compression algorithm
	Compression CompressionType `yaml:"compression,omitempty"`

	// FlushInterval is how often to flush buffered events
	FlushInterval time.Duration `yaml:"flush_interval,omitempty"`

	// MaxRetries is the maximum number of retries for failed sends
	MaxRetries int `yaml:"max_retries,omitempty"`

	// RetryBackoff is the initial backoff duration for retries
	RetryBackoff time.Duration `yaml:"retry_backoff,omitempty"`

	// Timeout is the timeout for send operations
	Timeout time.Duration `yaml:"timeout,omitempty"`
}

// DefaultBaseConfig returns a base config with sensible defaults
func DefaultBaseConfig() BaseConfig {
	return BaseConfig{
		BatchSize:     100,
		BatchTimeout:  5 * time.Second,
		Compression:   CompressionNone,
		FlushInterval: 1 * time.Second,
		MaxRetries:    3,
		RetryBackoff:  100 * time.Millisecond,
		Timeout:       30 * time.Second,
	}
}
