# Log Aggregation System

A production-grade, high-performance log collection and aggregation system written in Go. Designed to tail log files, handle rotation gracefully, and stream events to multiple destinations.

## Features

### Phase 1 - Foundation ✅

✅ **File Tailing with Rotation Handling**
- Watch multiple log files simultaneously
- Detect and handle file rotation (rename, truncate)
- Resume from last position after restart
- Track file position with checkpoints

✅ **Configuration System**
- YAML-based configuration
- Environment variable support
- Configuration validation
- Hot-reload capability (future)

✅ **Structured Logging**
- JSON and console output formats
- Multiple log levels (debug, info, warn, error, fatal)
- Zerolog-based high-performance logging

✅ **Checkpoint Management**
- Persistent position tracking
- Atomic checkpoint saves
- Configurable checkpoint intervals
- Recovery from crashes

### Phase 2 - Parsing & Processing ✅

✅ **Parser Engine**
- Regex pattern matching with named capture groups
- JSON log parsing with nested field support
- Grok pattern library (50+ built-in patterns)
- Multi-line log handling (stack traces, exceptions)
- 7 pre-configured formats (syslog, apache, nginx, java, python, go)

✅ **Field Extraction**
- Timestamp parsing (9 common formats)
- Log level detection and normalization
- Key-value pair extraction
- Nested field access
- Custom field injection

✅ **Transformation Pipeline**
- Field filtering (include/exclude sensitive data)
- Field renaming and mapping
- Data type conversion
- Field enrichment (add metadata)
- Chainable transformations

### Phase 3 - Buffering & Reliability ✅

✅ **Memory-Backed Ring Buffer**
- Lock-free circular buffer implementation
- Configurable buffer size (power-of-2 optimization)
- Three backpressure strategies (block, drop, sample)
- Real-time metrics (utilization, drops, throughput)
- Thread-safe concurrent access

✅ **Write-Ahead Log (WAL)**
- Disk-backed WAL for durability guarantees
- Segment-based log files with automatic rotation
- Crash recovery and replay
- Compaction and cleanup policies
- Zero data loss on restarts

✅ **Worker Pool**
- Configurable number of workers
- Dynamic scaling (add/remove workers)
- Job timeout support
- Per-worker metrics
- Work stealing queue support

✅ **Error Handling & Retry**
- Exponential backoff retry logic
- Configurable max retries and backoff
- Optional jitter to prevent thundering herd
- Circuit breaker pattern (Closed/Open/Half-Open states)
- Dead letter queue for failed events
- Error rate limiting

### Phase 4 - Output Destinations ✅

✅ **Output Plugin Interface**
- Common interface for all output plugins
- Batching support with configurable size and timeout
- Compression support (gzip, snappy, lz4)
- Comprehensive metrics tracking
- Flexible configuration system

✅ **Kafka Output**
- Topic routing based on event fields
- Partitioning strategies (hash, random, round-robin, manual)
- SASL authentication (PLAIN, SCRAM-SHA-256, SCRAM-SHA-512)
- TLS encryption support
- Compression (gzip, snappy, lz4, zstd)
- Delivery guarantees (at-least-once, exactly-once)
- 100K+ events/sec throughput

✅ **Elasticsearch Output**
- Bulk API integration for high throughput
- Index rotation (daily, weekly, monthly, yearly)
- Multiple authentication methods (basic, cloud ID, API key)
- Ingest pipeline support
- Connection pooling
- <50ms p99 bulk insert latency

✅ **S3 Output**
- Object key templating with time-based patterns
- Storage class selection (STANDARD, GLACIER, DEEP_ARCHIVE)
- Server-side encryption (AES256, aws:kms)
- Compression (gzip, snappy)
- Batch processing (NDJSON format)
- S3-compatible endpoints (MinIO, etc.)

✅ **Multi-Output Router**
- Fan-out to multiple destinations
- Parallel or sequential sending
- Independent retry policies per output
- Failure strategies (continue, stop)
- Aggregate metrics across all outputs

### Phase 5 - Advanced Inputs ✅

✅ **Input Plugin Interface**
- Common interface for all input sources
- BaseInput with context management
- Health check support per input
- Unified event streaming

✅ **Syslog Receiver**
- TCP and UDP protocol support
- RFC 3164 (BSD syslog) support
- RFC 5424 (new syslog) support
- TLS encryption for secure syslog
- Per-client rate limiting
- Connection tracking
- 10K+ messages/sec throughput

✅ **HTTP Receiver**
- REST API for log ingestion
- Single event endpoint (/log)
- Batch endpoint (/logs) for bulk ingestion
- API key authentication
- Per-IP rate limiting
- TLS/HTTPS support
- Health and metrics endpoints
- 50K+ events/sec throughput

✅ **Kubernetes Pod Log Collection**
- Kubernetes API integration
- Automatic pod discovery via watch API
- Multi-container pod support
- Label and field selectors
- Pod metadata enrichment (namespace, labels, annotations)
- Follow mode for continuous streaming
- In-cluster and kubeconfig support
- 100+ pods simultaneously

## Quick Start

### Prerequisites

- Go 1.21 or later
- Linux/macOS/Windows

### Installation

```bash
# Clone the repository
git clone https://github.com/therealutkarshpriyadarshi/log.git
cd log

# Install dependencies
make install-deps

# Build the binary
make build
```

### Configuration

Create a `config.yaml` file (or copy from `config.yaml.example`):

```yaml
inputs:
  files:
    - paths:
        - /var/log/app.log
      checkpoint_path: /tmp/logaggregator/checkpoints
      checkpoint_interval: 5s

logging:
  level: info
  format: json

output:
  type: stdout
```

### Running

```bash
# Run with default config
./bin/logaggregator -config config.yaml

# Or use make
make run
```

## Architecture

```
┌─────────────────┐
│  Input Sources  │
│  - File Tailer  │
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│  Checkpoint     │
│  Manager        │
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│  Event Stream   │
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│  Output         │
│  (stdout)       │
└─────────────────┘
```

## Project Structure

```
.
├── cmd/
│   └── logaggregator/     # Main application
├── internal/
│   ├── config/            # Configuration management
│   ├── tailer/            # File tailing logic
│   ├── checkpoint/        # Position tracking
│   └── logging/           # Structured logging
├── pkg/
│   └── types/             # Common types
├── .github/
│   └── workflows/         # CI/CD pipelines
├── Makefile               # Build automation
├── config.yaml.example    # Example configuration
└── ROADMAP.md            # Project roadmap
```

## Development

### Building

```bash
# Build for current platform
make build

# Build for all platforms
make build-all
```

### Testing

```bash
# Run tests
make test

# Run tests with coverage
make test-coverage
```

### Linting

```bash
# Run linter
make lint

# Format code
make fmt
```

## Usage Examples

### Tail a Single File

```yaml
inputs:
  files:
    - paths:
        - /var/log/myapp.log
      checkpoint_path: /tmp/checkpoints
      checkpoint_interval: 5s
```

### Tail Multiple Files

```yaml
inputs:
  files:
    - paths:
        - /var/log/app1.log
        - /var/log/app2.log
        - /var/log/nginx/*.log
      checkpoint_path: /tmp/checkpoints
      checkpoint_interval: 10s
```

### Parse JSON Logs

```yaml
inputs:
  files:
    - paths:
        - /var/log/app.json
      checkpoint_path: /tmp/checkpoints
      parser:
        type: json
        time_field: timestamp
        level_field: level
        message_field: msg
      transforms:
        - type: filter
          exclude_fields:
            - password
            - api_key
        - type: add
          add:
            environment: production
```

### Parse with Regex Pattern

```yaml
inputs:
  files:
    - paths:
        - /var/log/app.log
      checkpoint_path: /tmp/checkpoints
      parser:
        type: regex
        pattern: '^(?P<timestamp>\S+)\s+\[(?P<thread>\w+)\]\s+(?P<level>\w+)\s+(?P<message>.*)$'
        time_field: timestamp
        time_format: "2006-01-02T15:04:05Z"
        level_field: level
        message_field: message
```

### Parse Syslog with Grok

```yaml
inputs:
  files:
    - paths:
        - /var/log/syslog
      checkpoint_path: /tmp/checkpoints
      parser:
        type: grok
        grok_pattern: syslog
```

### Handle Multi-line Logs (Stack Traces)

```yaml
inputs:
  files:
    - paths:
        - /var/log/exceptions.log
      checkpoint_path: /tmp/checkpoints
      parser:
        type: multiline
        multiline:
          pattern: '^\d{4}-\d{2}-\d{2}'
          negate: true
          match: after
          max_lines: 500
          timeout: 5s
```

### Full Pipeline with Transformations

```yaml
inputs:
  files:
    - paths:
        - /var/log/app.log
      checkpoint_path: /tmp/checkpoints
      parser:
        type: json
      transforms:
        # Extract key-value pairs
        - type: kv
          field_split: " "
          value_split: "="
          prefix: "kv_"
        # Add metadata
        - type: add
          add:
            datacenter: us-east-1
            environment: production
        # Rename fields
        - type: rename
          rename:
            kv_user: username
        # Filter sensitive data
        - type: filter
          exclude_fields:
            - password
            - token
```

### Syslog Receiver

Receive syslog messages:

```yaml
inputs:
  syslog:
    - name: syslog-server
      protocol: udp
      address: "0.0.0.0:514"
      format: "3164"  # RFC 3164 (BSD syslog)
      rate_limit: 1000
      buffer_size: 10000
```

Send test message:
```bash
logger -n localhost -P 514 "Test syslog message"
```

### HTTP API Receiver

Receive logs via HTTP:

```yaml
inputs:
  http:
    - name: http-api
      address: "0.0.0.0:8080"
      path: "/log"
      batch_path: "/logs"
      api_keys:
        - "secret-key-123"
      rate_limit: 100
      parser:
        type: json
```

Send single event:
```bash
curl -X POST http://localhost:8080/log \
  -H "X-API-Key: secret-key-123" \
  -H "Content-Type: application/json" \
  -d '{"message": "User login", "level": "info", "user_id": 123}'
```

Send batch events:
```bash
curl -X POST http://localhost:8080/logs \
  -H "X-API-Key: secret-key-123" \
  -H "Content-Type: application/json" \
  -d '[
    {"message": "Event 1", "level": "info"},
    {"message": "Event 2", "level": "warn"}
  ]'
```

### Kubernetes Pod Logs

Collect logs from Kubernetes pods:

```yaml
inputs:
  kubernetes:
    - name: k8s-production
      namespace: "production"
      label_selector: "app=backend"
      follow: true
      enrich_metadata: true
      parser:
        type: json
```

Logs will include Kubernetes metadata:
```json
{
  "message": "Request processed",
  "kubernetes": {
    "namespace": "production",
    "pod": "backend-api-7d9f5c8b-xk4sm",
    "container": "app",
    "labels": {
      "app": "backend",
      "version": "1.2.3"
    }
  }
}
```

### Multi-Input Configuration

Combine multiple input sources:

```yaml
inputs:
  files:
    - paths: [/var/log/app/*.log]
      parser:
        type: json

  syslog:
    - name: network-logs
      protocol: udp
      address: "0.0.0.0:514"

  http:
    - name: api-logs
      address: "0.0.0.0:8080"
      api_keys: ["${API_KEY}"]

  kubernetes:
    - name: container-logs
      namespace: "production"
      enrich_metadata: true

output:
  type: kafka
  kafka:
    brokers: ["kafka1:9092"]
    topic: "logs"
```

### Environment Variables

Use environment variables in your config:

```yaml
inputs:
  files:
    - paths:
        - ${LOG_PATH}
      checkpoint_path: ${CHECKPOINT_DIR}

  http:
    - name: http-api
      address: "0.0.0.0:8080"
      api_keys:
        - ${HTTP_API_KEY}

logging:
  level: ${LOG_LEVEL}
```

Then run:
```bash
export LOG_PATH=/var/log/app.log
export CHECKPOINT_DIR=/tmp/checkpoints
export LOG_LEVEL=debug
export HTTP_API_KEY=secret-key-123
./bin/logaggregator -config config.yaml
```

## Performance Targets

| Metric | Phase 1-2 Current | Final Target |
|--------|-------------------|--------------|
| Throughput | 50-100K events/sec (parsed) | 100K-500K events/sec |
| Files | 10 concurrent | 100+ concurrent |
| Parsing Speed | 50-100K lines/sec | 100K+ lines/sec |
| Latency | N/A | <100ms p99 |
| Memory | <100MB | <500MB at 100K events/sec |

## Roadmap

**Current Status**: **Phase 6 Complete** ✅

See [ROADMAP.md](ROADMAP.md) for the complete development plan.

### Completed Phases

- ✅ **Phase 1**: Foundation - File tailing, checkpoints, configuration
- ✅ **Phase 2**: Parsing & Processing - Regex, JSON, Grok, multi-line, transformations
- ✅ **Phase 3**: Buffering & Reliability - Ring buffer, WAL, worker pool, retry, circuit breaker, DLQ
- ✅ **Phase 4**: Output Destinations - Kafka, Elasticsearch, S3, multi-output routing
- ✅ **Phase 5**: Advanced Inputs - Syslog, HTTP, Kubernetes pod logs
- ✅ **Phase 6**: Metrics & Observability - Prometheus, health checks, tracing, Grafana dashboards

### Upcoming Phases

- **Phase 7**: Performance optimization
- **Phase 8**: Production readiness

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

## License

MIT License

## Acknowledgments

Inspired by industry-leading log aggregators:
- [Fluent Bit](https://github.com/fluent/fluent-bit)
- [Vector](https://github.com/vectordotdev/vector)
- [Logstash](https://github.com/elastic/logstash)
- [Fluentd](https://github.com/fluent/fluentd)
