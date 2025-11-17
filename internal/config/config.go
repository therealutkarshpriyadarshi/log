package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Config represents the main configuration
type Config struct {
	Inputs       InputsConfig       `yaml:"inputs"`
	Logging      LoggingConfig      `yaml:"logging"`
	Output       OutputConfig       `yaml:"output"`
	Parser       *ParserConfig      `yaml:"parser,omitempty"`
	Transforms   []TransformConfig  `yaml:"transforms,omitempty"`
	Buffer       *BufferConfig      `yaml:"buffer,omitempty"`
	WAL          *WALConfig         `yaml:"wal,omitempty"`
	WorkerPool   *WorkerPoolConfig  `yaml:"worker_pool,omitempty"`
	Reliability  *ReliabilityConfig `yaml:"reliability,omitempty"`
	DeadLetter   *DeadLetterConfig  `yaml:"dead_letter,omitempty"`
	Metrics      *MetricsConfig     `yaml:"metrics,omitempty"`
	Health       *HealthConfig      `yaml:"health,omitempty"`
	Tracing      *TracingConfig     `yaml:"tracing,omitempty"`
	Profiling    *ProfilingConfig   `yaml:"profiling,omitempty"`
	Performance  *PerformanceConfig `yaml:"performance,omitempty"`
}

// InputsConfig defines input sources
type InputsConfig struct {
	Files      []FileInputConfig       `yaml:"files,omitempty"`
	Syslog     []SyslogInputConfig     `yaml:"syslog,omitempty"`
	HTTP       []HTTPInputConfig       `yaml:"http,omitempty"`
	Kubernetes []KubernetesInputConfig `yaml:"kubernetes,omitempty"`
}

// FileInputConfig defines file input configuration
type FileInputConfig struct {
	Paths              []string          `yaml:"paths"`
	CheckpointPath     string            `yaml:"checkpoint_path"`
	CheckpointInterval time.Duration     `yaml:"checkpoint_interval"`
	Parser             *ParserConfig     `yaml:"parser,omitempty"`
	Transforms         []TransformConfig `yaml:"transforms,omitempty"`
}

// ParserConfig holds parser configuration
type ParserConfig struct {
	Type         string            `yaml:"type"`
	Pattern      string            `yaml:"pattern,omitempty"`
	GrokPattern  string            `yaml:"grok_pattern,omitempty"`
	TimeFormat   string            `yaml:"time_format,omitempty"`
	TimeField    string            `yaml:"time_field,omitempty"`
	LevelField   string            `yaml:"level_field,omitempty"`
	MessageField string            `yaml:"message_field,omitempty"`
	Multiline    *MultilineConfig  `yaml:"multiline,omitempty"`
	CustomFields map[string]string `yaml:"custom_fields,omitempty"`
}

// MultilineConfig holds configuration for multi-line log handling
type MultilineConfig struct {
	Pattern  string `yaml:"pattern"`
	Negate   bool   `yaml:"negate"`
	Match    string `yaml:"match"`
	MaxLines int    `yaml:"max_lines"`
	Timeout  string `yaml:"timeout"`
}

// TransformConfig holds transformation configuration
type TransformConfig struct {
	Type          string            `yaml:"type"`
	Fields        []string          `yaml:"fields,omitempty"`
	IncludeFields []string          `yaml:"include_fields,omitempty"`
	ExcludeFields []string          `yaml:"exclude_fields,omitempty"`
	Rename        map[string]string `yaml:"rename,omitempty"`
	Add           map[string]string `yaml:"add,omitempty"`
	Patterns      []string          `yaml:"patterns,omitempty"`
	FieldSplit    string            `yaml:"field_split,omitempty"`
	ValueSplit    string            `yaml:"value_split,omitempty"`
	Prefix        string            `yaml:"prefix,omitempty"`
}

// LoggingConfig defines logging configuration
type LoggingConfig struct {
	Level  string `yaml:"level"`
	Format string `yaml:"format"` // json or console
}

// OutputConfig defines output configuration
type OutputConfig struct {
	Type string `yaml:"type"` // stdout, file, kafka, elasticsearch, s3, multi
	Path string `yaml:"path,omitempty"`

	// Kafka output configuration
	Kafka *KafkaOutputConfig `yaml:"kafka,omitempty"`

	// Elasticsearch output configuration
	Elasticsearch *ElasticsearchOutputConfig `yaml:"elasticsearch,omitempty"`

	// S3 output configuration
	S3 *S3OutputConfig `yaml:"s3,omitempty"`

	// Multi-output configuration
	Multi *MultiOutputConfig `yaml:"multi,omitempty"`
}

// KafkaOutputConfig holds Kafka-specific configuration
type KafkaOutputConfig struct {
	Brokers           []string      `yaml:"brokers"`
	Topic             string        `yaml:"topic"`
	TopicField        string        `yaml:"topic_field,omitempty"`
	PartitionKey      string        `yaml:"partition_key,omitempty"`
	PartitionStrategy string        `yaml:"partition_strategy,omitempty"`
	RequiredAcks      int16         `yaml:"required_acks,omitempty"`
	CompressionCodec  string        `yaml:"compression_codec,omitempty"`
	MaxMessageBytes   int           `yaml:"max_message_bytes,omitempty"`
	BatchSize         int           `yaml:"batch_size,omitempty"`
	BatchTimeout      time.Duration `yaml:"batch_timeout,omitempty"`
	FlushInterval     time.Duration `yaml:"flush_interval,omitempty"`
	SASLEnabled       bool          `yaml:"sasl_enabled,omitempty"`
	SASLMechanism     string        `yaml:"sasl_mechanism,omitempty"`
	SASLUsername      string        `yaml:"sasl_username,omitempty"`
	SASLPassword      string        `yaml:"sasl_password,omitempty"`
	EnableTLS         bool          `yaml:"enable_tls,omitempty"`
}

// ElasticsearchOutputConfig holds Elasticsearch-specific configuration
type ElasticsearchOutputConfig struct {
	Addresses           []string      `yaml:"addresses"`
	Index               string        `yaml:"index"`
	IndexRotation       string        `yaml:"index_rotation,omitempty"`
	IndexTimestampField string        `yaml:"index_timestamp_field,omitempty"`
	Pipeline            string        `yaml:"pipeline,omitempty"`
	Username            string        `yaml:"username,omitempty"`
	Password            string        `yaml:"password,omitempty"`
	CloudID             string        `yaml:"cloud_id,omitempty"`
	APIKey              string        `yaml:"api_key,omitempty"`
	BatchSize           int           `yaml:"batch_size,omitempty"`
	BatchTimeout        time.Duration `yaml:"batch_timeout,omitempty"`
	FlushInterval       time.Duration `yaml:"flush_interval,omitempty"`
	BulkWorkers         int           `yaml:"bulk_workers,omitempty"`
	MaxRetries          int           `yaml:"max_retries,omitempty"`
}

// S3OutputConfig holds S3-specific configuration
type S3OutputConfig struct {
	Bucket               string        `yaml:"bucket"`
	Region               string        `yaml:"region"`
	Prefix               string        `yaml:"prefix,omitempty"`
	KeyTemplate          string        `yaml:"key_template,omitempty"`
	StorageClass         string        `yaml:"storage_class,omitempty"`
	ServerSideEncryption string        `yaml:"server_side_encryption,omitempty"`
	ACL                  string        `yaml:"acl,omitempty"`
	Compression          string        `yaml:"compression,omitempty"`
	BatchSize            int           `yaml:"batch_size,omitempty"`
	BatchTimeout         time.Duration `yaml:"batch_timeout,omitempty"`
	FlushInterval        time.Duration `yaml:"flush_interval,omitempty"`
	Endpoint             string        `yaml:"endpoint,omitempty"`
	UsePathStyle         bool          `yaml:"use_path_style,omitempty"`
}

// MultiOutputConfig holds configuration for multiple outputs
type MultiOutputConfig struct {
	Outputs         []OutputDefinition `yaml:"outputs"`
	FailureStrategy string             `yaml:"failure_strategy,omitempty"`
	Parallel        bool               `yaml:"parallel,omitempty"`
}

// OutputDefinition defines a single output in multi-output mode
type OutputDefinition struct {
	Name          string                      `yaml:"name"`
	Type          string                      `yaml:"type"`
	Kafka         *KafkaOutputConfig         `yaml:"kafka,omitempty"`
	Elasticsearch *ElasticsearchOutputConfig `yaml:"elasticsearch,omitempty"`
	S3            *S3OutputConfig            `yaml:"s3,omitempty"`
}

// BufferConfig holds buffer configuration
type BufferConfig struct {
	Type                 string        `yaml:"type"` // memory, disk
	Size                 int           `yaml:"size"`
	BackpressureStrategy string        `yaml:"backpressure_strategy"` // block, drop, sample
	SampleRate           int           `yaml:"sample_rate,omitempty"`
	BlockTimeout         time.Duration `yaml:"block_timeout,omitempty"`
}

// WALConfig holds Write-Ahead Log configuration
type WALConfig struct {
	Enabled          bool          `yaml:"enabled"`
	Dir              string        `yaml:"dir"`
	SegmentSize      int64         `yaml:"segment_size,omitempty"`
	MaxSegments      int           `yaml:"max_segments,omitempty"`
	SyncInterval     time.Duration `yaml:"sync_interval,omitempty"`
	CompactionPolicy string        `yaml:"compaction_policy,omitempty"`
}

// WorkerPoolConfig holds worker pool configuration
type WorkerPoolConfig struct {
	NumWorkers     int           `yaml:"num_workers"`
	QueueSize      int           `yaml:"queue_size,omitempty"`
	JobTimeout     time.Duration `yaml:"job_timeout,omitempty"`
	EnableStealing bool          `yaml:"enable_stealing,omitempty"`
}

// ReliabilityConfig holds retry and circuit breaker configuration
type ReliabilityConfig struct {
	Retry         *RetryConfig         `yaml:"retry,omitempty"`
	CircuitBreaker *CircuitBreakerConfig `yaml:"circuit_breaker,omitempty"`
}

// RetryConfig holds retry configuration
type RetryConfig struct {
	MaxRetries     int           `yaml:"max_retries"`
	InitialBackoff time.Duration `yaml:"initial_backoff,omitempty"`
	MaxBackoff     time.Duration `yaml:"max_backoff,omitempty"`
	Multiplier     float64       `yaml:"multiplier,omitempty"`
	Jitter         bool          `yaml:"jitter,omitempty"`
}

// CircuitBreakerConfig holds circuit breaker configuration
type CircuitBreakerConfig struct {
	MaxRequests        uint32        `yaml:"max_requests,omitempty"`
	Interval           time.Duration `yaml:"interval,omitempty"`
	Timeout            time.Duration `yaml:"timeout,omitempty"`
	FailureThreshold   uint32        `yaml:"failure_threshold,omitempty"`
}

// DeadLetterConfig holds dead letter queue configuration
type DeadLetterConfig struct {
	Enabled       bool          `yaml:"enabled"`
	Dir           string        `yaml:"dir"`
	MaxSize       int64         `yaml:"max_size,omitempty"`
	MaxAge        time.Duration `yaml:"max_age,omitempty"`
	FlushInterval time.Duration `yaml:"flush_interval,omitempty"`
}

// MetricsConfig holds metrics configuration
type MetricsConfig struct {
	Enabled    bool                      `yaml:"enabled"`
	Address    string                    `yaml:"address"`
	Path       string                    `yaml:"path,omitempty"`
	Extraction *MetricsExtractionConfig  `yaml:"extraction,omitempty"`
}

// MetricsExtractionConfig holds configuration for extracting metrics from logs
type MetricsExtractionConfig struct {
	Enabled bool                  `yaml:"enabled"`
	Rules   []MetricExtractionRule `yaml:"rules,omitempty"`
}

// MetricExtractionRule defines a single metric extraction rule
type MetricExtractionRule struct {
	Name        string            `yaml:"name"`
	Type        string            `yaml:"type"` // counter, gauge, histogram
	Field       string            `yaml:"field"`
	Pattern     string            `yaml:"pattern,omitempty"`
	Labels      map[string]string `yaml:"labels,omitempty"`
	LabelFields map[string]string `yaml:"label_fields,omitempty"`
	Help        string            `yaml:"help"`
	Buckets     []float64         `yaml:"buckets,omitempty"`
}

// HealthConfig holds health check configuration
type HealthConfig struct {
	Enabled      bool          `yaml:"enabled"`
	Address      string        `yaml:"address"`
	LivenessPath string        `yaml:"liveness_path,omitempty"`
	ReadinessPath string       `yaml:"readiness_path,omitempty"`
	Timeout      time.Duration `yaml:"timeout,omitempty"`
}

// TracingConfig holds tracing configuration
type TracingConfig struct {
	Enabled      bool    `yaml:"enabled"`
	Endpoint     string  `yaml:"endpoint,omitempty"`
	SampleRate   float64 `yaml:"sample_rate,omitempty"`
	EnableStdout bool    `yaml:"enable_stdout,omitempty"`
}

// ProfilingConfig holds profiling configuration
type ProfilingConfig struct {
	Enabled            bool   `yaml:"enabled"`
	Address            string `yaml:"address"`
	CPUProfilePath     string `yaml:"cpu_profile,omitempty"`
	MemProfilePath     string `yaml:"mem_profile,omitempty"`
	BlockProfile       bool   `yaml:"block_profile"`
	MutexProfile       bool   `yaml:"mutex_profile"`
	GoroutineThreshold int    `yaml:"goroutine_threshold"`
}

// PerformanceConfig holds performance tuning configuration
type PerformanceConfig struct {
	EnablePooling      bool `yaml:"enable_pooling"`
	GOMAXPROCS         int  `yaml:"gomaxprocs"`
	GCPercent          int  `yaml:"gc_percent"`
	ChannelBufferSize  int  `yaml:"channel_buffer_size"`
	MaxConcurrentReads int  `yaml:"max_concurrent_reads"`
}

// Default values
const (
	DefaultCheckpointPath     = "/var/lib/logaggregator/checkpoints"
	DefaultCheckpointInterval = 5 * time.Second
	DefaultLogLevel           = "info"
	DefaultLogFormat          = "json"
)

// Load loads configuration from a YAML file with environment variable overrides
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	// Expand environment variables in the YAML content
	expandedData := []byte(os.ExpandEnv(string(data)))

	var cfg Config
	if err := yaml.Unmarshal(expandedData, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	// Apply defaults
	cfg.applyDefaults()

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return &cfg, nil
}

// applyDefaults sets default values for unspecified configuration
func (c *Config) applyDefaults() {
	if c.Logging.Level == "" {
		c.Logging.Level = DefaultLogLevel
	}
	if c.Logging.Format == "" {
		c.Logging.Format = DefaultLogFormat
	}
	if c.Output.Type == "" {
		c.Output.Type = "stdout"
	}

	for i := range c.Inputs.Files {
		if c.Inputs.Files[i].CheckpointPath == "" {
			c.Inputs.Files[i].CheckpointPath = DefaultCheckpointPath
		}
		if c.Inputs.Files[i].CheckpointInterval == 0 {
			c.Inputs.Files[i].CheckpointInterval = DefaultCheckpointInterval
		}
	}
}

// Validate validates the configuration
func (c *Config) Validate() error {
	// Check that at least one input is configured
	totalInputs := len(c.Inputs.Files) + len(c.Inputs.Syslog) + len(c.Inputs.HTTP) + len(c.Inputs.Kubernetes)
	if totalInputs == 0 {
		return fmt.Errorf("at least one input must be configured")
	}

	// Validate file inputs
	for i, fileInput := range c.Inputs.Files {
		if len(fileInput.Paths) == 0 {
			return fmt.Errorf("file input %d has no paths configured", i)
		}
	}

	// Validate syslog inputs
	for i, syslogInput := range c.Inputs.Syslog {
		if syslogInput.Name == "" {
			return fmt.Errorf("syslog input %d has no name configured", i)
		}
		if syslogInput.Address == "" {
			return fmt.Errorf("syslog input %d has no address configured", i)
		}
	}

	// Validate HTTP inputs
	for i, httpInput := range c.Inputs.HTTP {
		if httpInput.Name == "" {
			return fmt.Errorf("HTTP input %d has no name configured", i)
		}
		if httpInput.Address == "" {
			return fmt.Errorf("HTTP input %d has no address configured", i)
		}
	}

	// Validate Kubernetes inputs
	for i, k8sInput := range c.Inputs.Kubernetes {
		if k8sInput.Name == "" {
			return fmt.Errorf("Kubernetes input %d has no name configured", i)
		}
	}

	validLogLevels := map[string]bool{
		"debug": true, "info": true, "warn": true, "error": true, "fatal": true,
	}
	if !validLogLevels[c.Logging.Level] {
		return fmt.Errorf("invalid log level: %s", c.Logging.Level)
	}

	validLogFormats := map[string]bool{
		"json": true, "console": true,
	}
	if !validLogFormats[c.Logging.Format] {
		return fmt.Errorf("invalid log format: %s", c.Logging.Format)
	}

	return nil
}

// LoadOrDefault loads configuration from file or returns a default configuration
func LoadOrDefault(path string) *Config {
	cfg, err := Load(path)
	if err != nil {
		return DefaultConfig()
	}
	return cfg
}

// SyslogInputConfig defines syslog input configuration
type SyslogInputConfig struct {
	Name       string        `yaml:"name"`
	Protocol   string        `yaml:"protocol"` // tcp, udp, both
	Address    string        `yaml:"address"`
	Format     string        `yaml:"format"` // 3164, 5424
	TLSEnabled bool          `yaml:"tls_enabled,omitempty"`
	TLSCert    string        `yaml:"tls_cert,omitempty"`
	TLSKey     string        `yaml:"tls_key,omitempty"`
	RateLimit  int           `yaml:"rate_limit,omitempty"`
	BufferSize int           `yaml:"buffer_size,omitempty"`
	Parser     *ParserConfig `yaml:"parser,omitempty"`
	Transforms []TransformConfig `yaml:"transforms,omitempty"`
}

// HTTPInputConfig defines HTTP input configuration
type HTTPInputConfig struct {
	Name         string            `yaml:"name"`
	Address      string            `yaml:"address"`
	Path         string            `yaml:"path,omitempty"`
	BatchPath    string            `yaml:"batch_path,omitempty"`
	APIKeys      []string          `yaml:"api_keys,omitempty"`
	RateLimit    int               `yaml:"rate_limit,omitempty"`
	MaxBodySize  int64             `yaml:"max_body_size,omitempty"`
	TLSEnabled   bool              `yaml:"tls_enabled,omitempty"`
	TLSCert      string            `yaml:"tls_cert,omitempty"`
	TLSKey       string            `yaml:"tls_key,omitempty"`
	BufferSize   int               `yaml:"buffer_size,omitempty"`
	ReadTimeout  time.Duration     `yaml:"read_timeout,omitempty"`
	WriteTimeout time.Duration     `yaml:"write_timeout,omitempty"`
	Parser       *ParserConfig     `yaml:"parser,omitempty"`
	Transforms   []TransformConfig `yaml:"transforms,omitempty"`
}

// KubernetesInputConfig defines Kubernetes input configuration
type KubernetesInputConfig struct {
	Name             string            `yaml:"name"`
	Kubeconfig       string            `yaml:"kubeconfig,omitempty"`
	Namespace        string            `yaml:"namespace,omitempty"`
	LabelSelector    string            `yaml:"label_selector,omitempty"`
	FieldSelector    string            `yaml:"field_selector,omitempty"`
	ContainerPattern string            `yaml:"container_pattern,omitempty"`
	Follow           bool              `yaml:"follow"`
	IncludePrevious  bool              `yaml:"include_previous,omitempty"`
	TailLines        int64             `yaml:"tail_lines,omitempty"`
	EnrichMetadata   bool              `yaml:"enrich_metadata"`
	BufferSize       int               `yaml:"buffer_size,omitempty"`
	Parser           *ParserConfig     `yaml:"parser,omitempty"`
	Transforms       []TransformConfig `yaml:"transforms,omitempty"`
}

// DefaultConfig returns a default configuration
func DefaultConfig() *Config {
	cfg := &Config{
		Inputs: InputsConfig{
			Files: []FileInputConfig{
				{
					Paths:              []string{"/var/log/app.log"},
					CheckpointPath:     DefaultCheckpointPath,
					CheckpointInterval: DefaultCheckpointInterval,
				},
			},
		},
		Logging: LoggingConfig{
			Level:  DefaultLogLevel,
			Format: DefaultLogFormat,
		},
		Output: OutputConfig{
			Type: "stdout",
		},
	}
	return cfg
}
