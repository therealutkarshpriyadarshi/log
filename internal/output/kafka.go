package output

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/IBM/sarama"
	"github.com/therealutkarshpriyadarshi/log/pkg/types"
)

// KafkaConfig contains Kafka-specific configuration
type KafkaConfig struct {
	BaseConfig `yaml:",inline"`

	// Brokers is the list of Kafka broker addresses
	Brokers []string `yaml:"brokers"`

	// Topic is the default Kafka topic to send messages to
	Topic string `yaml:"topic"`

	// TopicField optionally specifies a field to use for dynamic topic routing
	TopicField string `yaml:"topic_field,omitempty"`

	// PartitionKey specifies the field to use for partitioning
	PartitionKey string `yaml:"partition_key,omitempty"`

	// PartitionStrategy defines how to partition messages (hash, random, round-robin, manual)
	PartitionStrategy string `yaml:"partition_strategy,omitempty"`

	// RequiredAcks specifies the number of acknowledgments required (0, 1, -1)
	RequiredAcks int16 `yaml:"required_acks,omitempty"`

	// CompressionCodec specifies the compression codec (none, gzip, snappy, lz4, zstd)
	CompressionCodec string `yaml:"compression_codec,omitempty"`

	// MaxMessageBytes is the maximum size of a single message
	MaxMessageBytes int `yaml:"max_message_bytes,omitempty"`

	// IdempotentWrites enables idempotent producer for exactly-once semantics
	IdempotentWrites bool `yaml:"idempotent_writes,omitempty"`

	// EnableTLS enables TLS for connections
	EnableTLS bool `yaml:"enable_tls,omitempty"`

	// SASL configuration
	SASLEnabled   bool   `yaml:"sasl_enabled,omitempty"`
	SASLMechanism string `yaml:"sasl_mechanism,omitempty"` // PLAIN, SCRAM-SHA-256, SCRAM-SHA-512
	SASLUsername  string `yaml:"sasl_username,omitempty"`
	SASLPassword  string `yaml:"sasl_password,omitempty"`

	// ClientID is the client identifier
	ClientID string `yaml:"client_id,omitempty"`

	// Version is the Kafka protocol version
	Version string `yaml:"version,omitempty"`
}

// DefaultKafkaConfig returns default Kafka configuration
func DefaultKafkaConfig() KafkaConfig {
	return KafkaConfig{
		BaseConfig:        DefaultBaseConfig(),
		Brokers:           []string{"localhost:9092"},
		Topic:             "logs",
		PartitionStrategy: "hash",
		RequiredAcks:      1,
		CompressionCodec:  "none",
		MaxMessageBytes:   1000000, // 1MB
		IdempotentWrites:  false,
		ClientID:          "logaggregator",
		Version:           "3.0.0",
	}
}

// KafkaOutput sends events to Kafka
type KafkaOutput struct {
	config   KafkaConfig
	producer sarama.SyncProducer
	batcher  *Batcher
	metrics  *OutputMetrics
	mu       sync.RWMutex
	closed   atomic.Bool
}

// NewKafkaOutput creates a new Kafka output
func NewKafkaOutput(config KafkaConfig) (*KafkaOutput, error) {
	if len(config.Brokers) == 0 {
		return nil, fmt.Errorf("no brokers specified")
	}

	if config.Topic == "" {
		return nil, fmt.Errorf("no topic specified")
	}

	// Create Sarama config
	saramaConfig := sarama.NewConfig()
	saramaConfig.Producer.Return.Successes = true
	saramaConfig.Producer.Return.Errors = true
	saramaConfig.Producer.RequiredAcks = sarama.RequiredAcks(config.RequiredAcks)
	saramaConfig.Producer.Idempotent = config.IdempotentWrites
	saramaConfig.ClientID = config.ClientID

	// Set compression
	switch config.CompressionCodec {
	case "gzip":
		saramaConfig.Producer.Compression = sarama.CompressionGZIP
	case "snappy":
		saramaConfig.Producer.Compression = sarama.CompressionSnappy
	case "lz4":
		saramaConfig.Producer.Compression = sarama.CompressionLZ4
	case "zstd":
		saramaConfig.Producer.Compression = sarama.CompressionZSTD
	default:
		saramaConfig.Producer.Compression = sarama.CompressionNone
	}

	// Set partitioner
	switch config.PartitionStrategy {
	case "random":
		saramaConfig.Producer.Partitioner = sarama.NewRandomPartitioner
	case "round-robin":
		saramaConfig.Producer.Partitioner = sarama.NewRoundRobinPartitioner
	case "manual":
		saramaConfig.Producer.Partitioner = sarama.NewManualPartitioner
	default: // hash
		saramaConfig.Producer.Partitioner = sarama.NewHashPartitioner
	}

	// Set max message bytes
	if config.MaxMessageBytes > 0 {
		saramaConfig.Producer.MaxMessageBytes = config.MaxMessageBytes
	}

	// Set Kafka version
	if config.Version != "" {
		version, err := sarama.ParseKafkaVersion(config.Version)
		if err != nil {
			return nil, fmt.Errorf("invalid Kafka version: %w", err)
		}
		saramaConfig.Version = version
	}

	// Enable SASL if configured
	if config.SASLEnabled {
		saramaConfig.Net.SASL.Enable = true
		saramaConfig.Net.SASL.User = config.SASLUsername
		saramaConfig.Net.SASL.Password = config.SASLPassword

		switch config.SASLMechanism {
		case "SCRAM-SHA-256":
			saramaConfig.Net.SASL.Mechanism = sarama.SASLTypeSCRAMSHA256
		case "SCRAM-SHA-512":
			saramaConfig.Net.SASL.Mechanism = sarama.SASLTypeSCRAMSHA512
		default:
			saramaConfig.Net.SASL.Mechanism = sarama.SASLTypePlaintext
		}
	}

	// Enable TLS if configured
	if config.EnableTLS {
		saramaConfig.Net.TLS.Enable = true
	}

	// Create producer
	producer, err := sarama.NewSyncProducer(config.Brokers, saramaConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create Kafka producer: %w", err)
	}

	output := &KafkaOutput{
		config:   config,
		producer: producer,
		metrics:  &OutputMetrics{},
	}

	// Create batcher if batch size > 1
	if config.BatchSize > 1 {
		output.batcher = NewBatcher(BatcherConfig{
			MaxBatchSize:  config.BatchSize,
			MaxBatchBytes: config.MaxMessageBytes * config.BatchSize,
			FlushInterval: config.FlushInterval,
		}, output.sendBatchInternal)
	}

	return output, nil
}

// Send sends a single event to Kafka
func (k *KafkaOutput) Send(ctx context.Context, event *types.LogEvent) error {
	if k.closed.Load() {
		return fmt.Errorf("kafka output is closed")
	}

	// Use batcher if configured
	if k.batcher != nil {
		return k.batcher.Add(ctx, event)
	}

	return k.sendSingle(ctx, event)
}

// SendBatch sends a batch of events to Kafka
func (k *KafkaOutput) SendBatch(ctx context.Context, events []*types.LogEvent) error {
	if k.closed.Load() {
		return fmt.Errorf("kafka output is closed")
	}

	return k.sendBatchInternal(ctx, events)
}

// sendSingle sends a single event without batching
func (k *KafkaOutput) sendSingle(ctx context.Context, event *types.LogEvent) error {
	msg, err := k.buildMessage(event)
	if err != nil {
		atomic.AddInt64(&k.metrics.EventsFailed, 1)
		k.metrics.LastError = err.Error()
		k.metrics.LastErrorTime = time.Now()
		return err
	}

	startTime := time.Now()
	partition, offset, err := k.producer.SendMessage(msg)
	latency := time.Since(startTime)

	if err != nil {
		atomic.AddInt64(&k.metrics.EventsFailed, 1)
		k.metrics.LastError = err.Error()
		k.metrics.LastErrorTime = time.Now()
		return fmt.Errorf("failed to send message to Kafka: %w", err)
	}

	// Update metrics
	atomic.AddInt64(&k.metrics.EventsSent, 1)
	atomic.AddInt64(&k.metrics.BytesSent, int64(len(event.Raw)))
	k.metrics.LastSendTime = time.Now()

	// Update average latency (simple moving average)
	k.mu.Lock()
	k.metrics.AvgLatency = (k.metrics.AvgLatency + latency) / 2
	k.mu.Unlock()

	_ = partition // Can be used for logging
	_ = offset    // Can be used for logging

	return nil
}

// sendBatchInternal sends a batch of events
func (k *KafkaOutput) sendBatchInternal(ctx context.Context, events []*types.LogEvent) error {
	if len(events) == 0 {
		return nil
	}

	startTime := time.Now()
	var totalBytes int64

	// Build messages
	messages := make([]*sarama.ProducerMessage, len(events))
	for i, event := range events {
		msg, err := k.buildMessage(event)
		if err != nil {
			atomic.AddInt64(&k.metrics.EventsFailed, 1)
			continue
		}
		messages[i] = msg
		totalBytes += int64(len(event.Raw))
	}

	// Send messages
	// Note: SyncProducer doesn't have a native batch API, so we send individually
	// In production, you might want to use AsyncProducer for better batching
	var failedCount int64
	for _, msg := range messages {
		if msg == nil {
			continue
		}
		_, _, err := k.producer.SendMessage(msg)
		if err != nil {
			failedCount++
			k.metrics.LastError = err.Error()
			k.metrics.LastErrorTime = time.Now()
		}
	}

	latency := time.Since(startTime)
	successCount := int64(len(events)) - failedCount

	// Update metrics
	atomic.AddInt64(&k.metrics.EventsSent, successCount)
	atomic.AddInt64(&k.metrics.EventsFailed, failedCount)
	atomic.AddInt64(&k.metrics.BytesSent, totalBytes)
	atomic.AddInt64(&k.metrics.BatchesSent, 1)
	k.metrics.LastSendTime = time.Now()

	// Update average batch size and latency
	k.mu.Lock()
	if k.metrics.BatchesSent > 0 {
		k.metrics.AvgBatchSize = float64(k.metrics.EventsSent) / float64(k.metrics.BatchesSent)
	}
	k.metrics.AvgLatency = (k.metrics.AvgLatency + latency) / 2
	k.mu.Unlock()

	if failedCount > 0 {
		return fmt.Errorf("%d out of %d events failed to send", failedCount, len(events))
	}

	return nil
}

// buildMessage creates a Kafka producer message from a log event
func (k *KafkaOutput) buildMessage(event *types.LogEvent) (*sarama.ProducerMessage, error) {
	// Determine topic
	topic := k.config.Topic
	if k.config.TopicField != "" {
		if topicValue, ok := event.Fields[k.config.TopicField]; ok && topicValue != "" {
			topic = topicValue
		}
	}

	// Serialize event to JSON
	value, err := json.Marshal(event)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal event: %w", err)
	}

	msg := &sarama.ProducerMessage{
		Topic: topic,
		Value: sarama.ByteEncoder(value),
	}

	// Set partition key if configured
	if k.config.PartitionKey != "" {
		if keyValue, ok := event.Fields[k.config.PartitionKey]; ok && keyValue != "" {
			msg.Key = sarama.StringEncoder(keyValue)
		}
	}

	return msg, nil
}

// Close closes the Kafka output
func (k *KafkaOutput) Close() error {
	if !k.closed.CompareAndSwap(false, true) {
		return nil // Already closed
	}

	// Stop batcher first
	if k.batcher != nil {
		if err := k.batcher.Stop(); err != nil {
			return err
		}
	}

	// Close producer
	if k.producer != nil {
		return k.producer.Close()
	}

	return nil
}

// Name returns the output name
func (k *KafkaOutput) Name() string {
	if k.config.Name != "" {
		return k.config.Name
	}
	return "kafka"
}

// Metrics returns the current metrics
func (k *KafkaOutput) Metrics() *OutputMetrics {
	k.mu.RLock()
	defer k.mu.RUnlock()

	// Return a copy
	metricsCopy := *k.metrics
	return &metricsCopy
}
