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

### Environment Variables

Use environment variables in your config:

```yaml
inputs:
  files:
    - paths:
        - ${LOG_PATH}
      checkpoint_path: ${CHECKPOINT_DIR}

logging:
  level: ${LOG_LEVEL}
```

Then run:
```bash
export LOG_PATH=/var/log/app.log
export CHECKPOINT_DIR=/tmp/checkpoints
export LOG_LEVEL=debug
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

**Current Status**: **Phase 2 Complete** ✅

See [ROADMAP.md](ROADMAP.md) for the complete development plan.

### Completed Phases

- ✅ **Phase 1**: Foundation - File tailing, checkpoints, configuration
- ✅ **Phase 2**: Parsing & Processing - Regex, JSON, Grok, multi-line, transformations

### Upcoming Phases

- **Phase 3**: Buffering & reliability (WAL, backpressure)
- **Phase 4**: Output destinations (Kafka, Elasticsearch, S3)
- **Phase 5**: Advanced inputs (Kubernetes, syslog, HTTP)
- **Phase 6**: Metrics & observability (Prometheus, Grafana)
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
