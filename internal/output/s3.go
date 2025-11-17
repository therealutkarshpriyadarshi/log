package output

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	logtypes "github.com/therealutkarshpriyadarshi/log/pkg/types"
)

// S3Config contains S3-specific configuration
type S3Config struct {
	BaseConfig `yaml:",inline"`

	// Bucket is the S3 bucket name
	Bucket string `yaml:"bucket"`

	// Region is the AWS region
	Region string `yaml:"region"`

	// Prefix is the key prefix for objects
	Prefix string `yaml:"prefix,omitempty"`

	// KeyTemplate is the template for object keys (supports time patterns)
	KeyTemplate string `yaml:"key_template,omitempty"`

	// StorageClass is the S3 storage class (STANDARD, GLACIER, etc.)
	StorageClass string `yaml:"storage_class,omitempty"`

	// ServerSideEncryption specifies encryption (AES256, aws:kms)
	ServerSideEncryption string `yaml:"server_side_encryption,omitempty"`

	// ACL is the canned ACL (private, public-read, etc.)
	ACL string `yaml:"acl,omitempty"`

	// UploadConcurrency is the number of concurrent uploads
	UploadConcurrency int `yaml:"upload_concurrency,omitempty"`

	// AccessKeyID for authentication (optional, uses default credentials if not set)
	AccessKeyID string `yaml:"access_key_id,omitempty"`

	// SecretAccessKey for authentication
	SecretAccessKey string `yaml:"secret_access_key,omitempty"`

	// SessionToken for temporary credentials
	SessionToken string `yaml:"session_token,omitempty"`

	// Endpoint for S3-compatible services (e.g., MinIO)
	Endpoint string `yaml:"endpoint,omitempty"`

	// UsePathStyle forces path-style addressing
	UsePathStyle bool `yaml:"use_path_style,omitempty"`

	// ContentType for uploaded objects
	ContentType string `yaml:"content_type,omitempty"`
}

// DefaultS3Config returns default S3 configuration
func DefaultS3Config() S3Config {
	return S3Config{
		BaseConfig:        DefaultBaseConfig(),
		Region:            "us-east-1",
		Prefix:            "logs/",
		KeyTemplate:       "{{.Year}}/{{.Month}}/{{.Day}}/{{.Hour}}/{{.Timestamp}}.json",
		StorageClass:      "STANDARD",
		ACL:               "private",
		UploadConcurrency: 5,
		ContentType:       "application/json",
	}
}

// S3Output sends events to S3
type S3Output struct {
	config     S3Config
	client     *s3.Client
	batcher    *Batcher
	metrics    *OutputMetrics
	compressor Compressor
	mu         sync.RWMutex
	closed     atomic.Bool
}

// NewS3Output creates a new S3 output
func NewS3Output(s3Config S3Config) (*S3Output, error) {
	if s3Config.Bucket == "" {
		return nil, fmt.Errorf("no bucket specified")
	}

	if s3Config.Region == "" {
		return nil, fmt.Errorf("no region specified")
	}

	// Load AWS config
	ctx := context.Background()
	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion(s3Config.Region),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	// Create S3 client
	var opts []func(*s3.Options)

	if s3Config.Endpoint != "" {
		opts = append(opts, func(o *s3.Options) {
			o.BaseEndpoint = aws.String(s3Config.Endpoint)
			o.UsePathStyle = s3Config.UsePathStyle
		})
	}

	client := s3.NewFromConfig(cfg, opts...)

	// Get compressor
	compressor, err := GetCompressor(s3Config.Compression)
	if err != nil {
		return nil, err
	}

	output := &S3Output{
		config:     s3Config,
		client:     client,
		metrics:    &OutputMetrics{},
		compressor: compressor,
	}

	// Create batcher
	if s3Config.BatchSize > 1 {
		output.batcher = NewBatcher(BatcherConfig{
			MaxBatchSize:  s3Config.BatchSize,
			MaxBatchBytes: 100 * 1024 * 1024, // 100MB
			FlushInterval: s3Config.FlushInterval,
		}, output.sendBatchInternal)
	}

	return output, nil
}

// Send sends a single event to S3
func (s *S3Output) Send(ctx context.Context, event *types.LogEvent) error {
	if s.closed.Load() {
		return fmt.Errorf("s3 output is closed")
	}

	// Use batcher if configured
	if s.batcher != nil {
		return s.batcher.Add(ctx, event)
	}

	return s.sendSingle(ctx, event)
}

// SendBatch sends a batch of events to S3
func (s *S3Output) SendBatch(ctx context.Context, events []*types.LogEvent) error {
	if s.closed.Load() {
		return fmt.Errorf("s3 output is closed")
	}

	return s.sendBatchInternal(ctx, events)
}

// sendSingle sends a single event as a separate S3 object
func (s *S3Output) sendSingle(ctx context.Context, event *types.LogEvent) error {
	key := s.generateKey(event.Timestamp)

	// Serialize event
	data, err := json.Marshal(event)
	if err != nil {
		atomic.AddInt64(&s.metrics.EventsFailed, 1)
		s.metrics.LastError = err.Error()
		s.metrics.LastErrorTime = time.Now()
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	// Compress if needed
	data, err = s.compressor.Compress(data)
	if err != nil {
		atomic.AddInt64(&s.metrics.EventsFailed, 1)
		s.metrics.LastError = err.Error()
		s.metrics.LastErrorTime = time.Now()
		return fmt.Errorf("failed to compress data: %w", err)
	}

	// Upload to S3
	startTime := time.Now()
	err = s.uploadObject(ctx, key, data)
	latency := time.Since(startTime)

	if err != nil {
		atomic.AddInt64(&s.metrics.EventsFailed, 1)
		s.metrics.LastError = err.Error()
		s.metrics.LastErrorTime = time.Now()
		return err
	}

	// Update metrics
	atomic.AddInt64(&s.metrics.EventsSent, 1)
	atomic.AddInt64(&s.metrics.BytesSent, int64(len(data)))
	s.metrics.LastSendTime = time.Now()

	// Update average latency
	s.mu.Lock()
	s.metrics.AvgLatency = (s.metrics.AvgLatency + latency) / 2
	s.mu.Unlock()

	return nil
}

// sendBatchInternal sends a batch of events as a single S3 object
func (s *S3Output) sendBatchInternal(ctx context.Context, events []*types.LogEvent) error {
	if len(events) == 0 {
		return nil
	}

	startTime := time.Now()

	// Use first event's timestamp for key generation
	key := s.generateKey(events[0].Timestamp)

	// Serialize events as NDJSON (newline-delimited JSON)
	var buf bytes.Buffer
	for _, event := range events {
		data, err := json.Marshal(event)
		if err != nil {
			atomic.AddInt64(&s.metrics.EventsFailed, 1)
			continue
		}
		buf.Write(data)
		buf.WriteByte('\n')
	}

	data := buf.Bytes()

	// Compress if needed
	compressed, err := s.compressor.Compress(data)
	if err != nil {
		atomic.AddInt64(&s.metrics.EventsFailed, int64(len(events)))
		s.metrics.LastError = err.Error()
		s.metrics.LastErrorTime = time.Now()
		return fmt.Errorf("failed to compress data: %w", err)
	}

	// Upload to S3
	err = s.uploadObject(ctx, key, compressed)
	latency := time.Since(startTime)

	if err != nil {
		atomic.AddInt64(&s.metrics.EventsFailed, int64(len(events)))
		s.metrics.LastError = err.Error()
		s.metrics.LastErrorTime = time.Now()
		return err
	}

	// Update metrics
	atomic.AddInt64(&s.metrics.EventsSent, int64(len(events)))
	atomic.AddInt64(&s.metrics.BytesSent, int64(len(compressed)))
	atomic.AddInt64(&s.metrics.BatchesSent, 1)
	s.metrics.LastSendTime = time.Now()

	// Update average batch size and latency
	s.mu.Lock()
	if s.metrics.BatchesSent > 0 {
		s.metrics.AvgBatchSize = float64(s.metrics.EventsSent) / float64(s.metrics.BatchesSent)
	}
	s.metrics.AvgLatency = (s.metrics.AvgLatency + latency) / 2
	s.mu.Unlock()

	return nil
}

// uploadObject uploads data to S3
func (s *S3Output) uploadObject(ctx context.Context, key string, data []byte) error {
	input := &s3.PutObjectInput{
		Bucket:      aws.String(s.config.Bucket),
		Key:         aws.String(key),
		Body:        bytes.NewReader(data),
		ContentType: aws.String(s.config.ContentType),
	}

	// Set storage class
	if s.config.StorageClass != "" {
		input.StorageClass = s3types.StorageClass(s.config.StorageClass)
	}

	// Set ACL
	if s.config.ACL != "" {
		input.ACL = s3types.ObjectCannedACL(s.config.ACL)
	}

	// Set server-side encryption
	if s.config.ServerSideEncryption != "" {
		input.ServerSideEncryption = s3types.ServerSideEncryption(s.config.ServerSideEncryption)
	}

	// Add compression extension if compressed
	if s.config.Compression != CompressionNone {
		input.ContentEncoding = aws.String(string(s.config.Compression))
	}

	_, err := s.client.PutObject(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to upload to S3: %w", err)
	}

	return nil
}

// generateKey generates an S3 key from a template and timestamp
func (s *S3Output) generateKey(timestamp time.Time) string {
	if timestamp.IsZero() {
		timestamp = time.Now()
	}

	key := s.config.KeyTemplate
	if key == "" {
		key = "{{.Timestamp}}.json"
	}

	// Replace template variables
	replacements := map[string]string{
		"{{.Year}}":      fmt.Sprintf("%04d", timestamp.Year()),
		"{{.Month}}":     fmt.Sprintf("%02d", timestamp.Month()),
		"{{.Day}}":       fmt.Sprintf("%02d", timestamp.Day()),
		"{{.Hour}}":      fmt.Sprintf("%02d", timestamp.Hour()),
		"{{.Minute}}":    fmt.Sprintf("%02d", timestamp.Minute()),
		"{{.Second}}":    fmt.Sprintf("%02d", timestamp.Second()),
		"{{.Timestamp}}": fmt.Sprintf("%d", timestamp.Unix()),
		"{{.UnixNano}}":  fmt.Sprintf("%d", timestamp.UnixNano()),
	}

	for placeholder, value := range replacements {
		key = strings.ReplaceAll(key, placeholder, value)
	}

	// Add prefix
	if s.config.Prefix != "" {
		key = s.config.Prefix + key
	}

	// Add compression extension
	if s.config.Compression == CompressionGzip {
		key += ".gz"
	} else if s.config.Compression == CompressionSnappy {
		key += ".snappy"
	} else if s.config.Compression == CompressionLZ4 {
		key += ".lz4"
	}

	return key
}

// Close closes the S3 output
func (s *S3Output) Close() error {
	if !s.closed.CompareAndSwap(false, true) {
		return nil // Already closed
	}

	// Stop batcher first
	if s.batcher != nil {
		if err := s.batcher.Stop(); err != nil {
			return err
		}
	}

	return nil
}

// Name returns the output name
func (s *S3Output) Name() string {
	if s.config.Name != "" {
		return s.config.Name
	}
	return "s3"
}

// Metrics returns the current metrics
func (s *S3Output) Metrics() *OutputMetrics {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Return a copy
	metricsCopy := *s.metrics
	return &metricsCopy
}
