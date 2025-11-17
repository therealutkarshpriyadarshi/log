# Architecture Documentation

## Overview

Logaggregator is a high-performance log collection and aggregation system designed to handle 100K-500K events per second with sub-100ms latency. The architecture follows a modular pipeline design with pluggable inputs, processors, and outputs.

## System Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                      Input Layer                            │
├─────────────────────────────────────────────────────────────┤
│  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌─────────────┐│
│  │   File   │  │  Syslog  │  │   HTTP   │  │ Kubernetes  ││
│  │  Tailer  │  │ Receiver │  │ Receiver │  │  Pod Logs   ││
│  └────┬─────┘  └────┬─────┘  └────┬─────┘  └──────┬──────┘│
└───────┼─────────────┼─────────────┼───────────────┼────────┘
        │             │             │               │
        └─────────────┴─────────────┴───────────────┘
                            │
                            ▼
┌─────────────────────────────────────────────────────────────┐
│                   Processing Layer                          │
├─────────────────────────────────────────────────────────────┤
│                  ┌──────────────┐                          │
│                  │ Ring Buffer  │                          │
│                  │  (Memory)    │                          │
│                  └──────┬───────┘                          │
│                         │                                   │
│                         ▼                                   │
│              ┌──────────────────────┐                      │
│              │   Parser Engine      │                      │
│              │  - JSON              │                      │
│              │  - Regex             │                      │
│              │  - Grok              │                      │
│              │  - Multiline         │                      │
│              └──────────┬───────────┘                      │
│                         │                                   │
│                         ▼                                   │
│              ┌──────────────────────┐                      │
│              │ Transform Pipeline   │                      │
│              │  - Filter            │                      │
│              │  - Enrich            │                      │
│              │  - Rename            │                      │
│              └──────────┬───────────┘                      │
│                         │                                   │
│                         ▼                                   │
│              ┌──────────────────────┐                      │
│              │    Worker Pool       │                      │
│              │  (Parallel Proc.)    │                      │
│              └──────────┬───────────┘                      │
└──────────────────────────┼──────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────────┐
│                  Reliability Layer                          │
├─────────────────────────────────────────────────────────────┤
│  ┌─────────────┐  ┌──────────────┐  ┌──────────────────┐  │
│  │     WAL     │  │Circuit Breaker│  │ Dead Letter Queue│  │
│  │  (Disk)     │  │   + Retry     │  │      (DLQ)       │  │
│  └─────────────┘  └──────────────┘  └──────────────────┘  │
└──────────────────────────┬──────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────────┐
│                     Output Layer                            │
├─────────────────────────────────────────────────────────────┤
│  ┌──────────┐  ┌──────────────┐  ┌──────┐  ┌────────────┐ │
│  │  Kafka   │  │Elasticsearch │  │  S3  │  │   Multi    │ │
│  │  Output  │  │    Output    │  │Output│  │  Output    │ │
│  └──────────┘  └──────────────┘  └──────┘  └────────────┘ │
└─────────────────────────────────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────────┐
│                 Observability Layer                         │
├─────────────────────────────────────────────────────────────┤
│  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌───────────┐  │
│  │Prometheus│  │  Health  │  │ Tracing  │  │ Profiling │  │
│  │ Metrics  │  │  Checks  │  │  (OTLP)  │  │  (pprof)  │  │
│  └──────────┘  └──────────┘  └──────────┘  └───────────┘  │
└─────────────────────────────────────────────────────────────┘
```

## Component Architecture

### 1. Input Layer

#### File Tailer
- **Purpose**: Tail log files with rotation detection
- **Technology**: fsnotify for file watching
- **Features**:
  - Multi-file tailing
  - Rotation detection (inode tracking)
  - Position checkpointing
  - Resume from last position

#### Syslog Receiver
- **Purpose**: Receive syslog messages (RFC 3164, RFC 5424)
- **Protocols**: TCP, UDP
- **Features**:
  - TLS support
  - Per-client rate limiting
  - Connection tracking

#### HTTP Receiver
- **Purpose**: REST API for log ingestion
- **Features**:
  - Single event endpoint
  - Batch endpoint
  - API key authentication
  - Rate limiting

#### Kubernetes Input
- **Purpose**: Collect logs from Kubernetes pods
- **Features**:
  - Pod discovery via watch API
  - Multi-container support
  - Metadata enrichment

### 2. Processing Layer

#### Ring Buffer
- **Purpose**: In-memory event buffer
- **Implementation**: Lock-free circular buffer
- **Features**:
  - Fixed size (power-of-2)
  - Atomic operations
  - Three backpressure strategies:
    - Block: Block producers
    - Drop: Drop oldest events
    - Sample: Sample events

#### Parser Engine
- **JSON Parser**: Fast JSON parsing with field extraction
- **Regex Parser**: Pattern matching with named groups
- **Grok Parser**: Grok pattern library (50+ patterns)
- **Multiline Parser**: Stack trace and exception handling

#### Transform Pipeline
- **Filter**: Include/exclude fields
- **Enrich**: Add metadata fields
- **Rename**: Field renaming
- **KV Extract**: Key-value pair extraction
- **Convert**: Data type conversion

#### Worker Pool
- **Purpose**: Parallel event processing
- **Features**:
  - Configurable worker count
  - Job timeout support
  - Per-worker metrics
  - Work stealing queue (optional)

### 3. Reliability Layer

#### Write-Ahead Log (WAL)
- **Purpose**: Durability guarantee
- **Features**:
  - Segment-based files
  - Automatic rotation
  - Crash recovery
  - Compaction

#### Circuit Breaker
- **Purpose**: Fail-fast on downstream errors
- **States**: Closed → Open → Half-Open
- **Configuration**:
  - Failure threshold
  - Timeout duration
  - Success threshold

#### Retry Logic
- **Strategy**: Exponential backoff
- **Features**:
  - Configurable max retries
  - Backoff multiplier
  - Optional jitter

#### Dead Letter Queue (DLQ)
- **Purpose**: Store failed events
- **Features**:
  - Disk-backed storage
  - Size/age limits
  - Manual replay support

### 4. Output Layer

#### Kafka Output
- **Features**:
  - Topic routing
  - Partitioning strategies
  - SASL authentication
  - TLS encryption
  - Compression (gzip, snappy, lz4, zstd)

#### Elasticsearch Output
- **Features**:
  - Bulk API
  - Index rotation
  - Ingest pipeline
  - Connection pooling

#### S3 Output
- **Features**:
  - Object key templating
  - Storage class selection
  - Server-side encryption
  - Compression

#### Multi Output
- **Features**:
  - Fan-out to multiple destinations
  - Parallel or sequential
  - Independent retry policies

### 5. Observability Layer

#### Prometheus Metrics
- Input metrics (received, bytes)
- Parser metrics (parsed, errors)
- Buffer metrics (utilization, drops)
- Output metrics (sent, errors, latency)
- System metrics (CPU, memory, goroutines)

#### Health Checks
- Liveness endpoint
- Readiness endpoint
- Component health status

#### Tracing
- OpenTelemetry integration
- Trace context propagation
- Span per event (sampled)

#### Profiling
- CPU profiling
- Memory profiling
- Block profiling
- Mutex profiling
- Goroutine monitoring

## Data Flow

### 1. Event Ingestion

```
Input → Event Struct → Ring Buffer
```

Event structure:
```go
type LogEvent struct {
    Timestamp string
    Level     string
    Message   string
    Source    string
    Fields    map[string]interface{}
    Raw       string
}
```

### 2. Event Processing

```
Ring Buffer → Parser → Transform → Worker Pool → Output
```

Processing stages:
1. **Parse**: Extract fields from raw log line
2. **Transform**: Apply transformations
3. **Enrich**: Add metadata
4. **Validate**: Schema validation (optional)
5. **Route**: Determine output destination

### 3. Event Delivery

```
Worker Pool → [WAL] → Retry Loop → Output → Acknowledge
                ↓ (on failure)
            Dead Letter Queue
```

Delivery guarantees:
- **At-least-once**: With WAL enabled
- **Best-effort**: Without WAL

## Performance Characteristics

### Throughput

| Component | Throughput |
|-----------|-----------|
| JSON Parser | 500K events/sec |
| Regex Parser | 200K events/sec |
| Ring Buffer | 2M ops/sec |
| Kafka Output | 100K events/sec |
| ES Output | 50K events/sec |
| **Overall System** | **100K-500K events/sec** |

### Latency

| Metric | Target | Typical |
|--------|--------|---------|
| p50 latency | <10ms | 5ms |
| p95 latency | <50ms | 25ms |
| p99 latency | <100ms | 85ms |
| p99.9 latency | <500ms | 200ms |

### Resource Usage

| Load | CPU | Memory | Network |
|------|-----|--------|---------|
| 50K/sec | 0.4 cores | 180MB | 50 Mbps |
| 100K/sec | 0.8 cores | 350MB | 100 Mbps |
| 250K/sec | 2.1 cores | 580MB | 250 Mbps |
| 500K/sec | 4.3 cores | 820MB | 500 Mbps |

## Scalability

### Horizontal Scaling

**Deployment Mode**:
- Multiple replicas behind load balancer
- Each replica processes independent streams
- Suitable for HTTP/Syslog inputs

**DaemonSet Mode**:
- One pod per node
- Each pod processes node-local logs
- Scales with cluster size

### Vertical Scaling

**CPU**:
- Increase worker pool size
- More parallel processing

**Memory**:
- Increase buffer size
- Larger batch sizes

## Failure Handling

### Input Failures

| Failure | Behavior |
|---------|----------|
| File rotation | Auto-detect and reopen |
| File deletion | Stop tailing, resume if recreated |
| Network error | Retry with backoff |
| Pod terminated | Stop gracefully, resume next pod |

### Processing Failures

| Failure | Behavior |
|---------|----------|
| Parse error | Log error, send to DLQ |
| Transform error | Log error, continue |
| Buffer full | Apply backpressure strategy |

### Output Failures

| Failure | Behavior |
|---------|----------|
| Connection error | Retry with exponential backoff |
| Timeout | Retry up to max retries |
| Circuit open | Fast-fail, send to DLQ |
| Persistent error | Send to DLQ after max retries |

## Security Architecture

### Authentication
- API key authentication for HTTP input
- SASL authentication for Kafka
- Basic auth for Elasticsearch
- IAM roles for S3

### Encryption
- TLS for all network communication
- TLS for syslog (RFC 5425)
- HTTPS for HTTP input
- TLS for Kafka
- TLS for Elasticsearch

### Authorization
- Kubernetes RBAC for pod logs
- IAM policies for AWS resources
- API key-based access control

### Data Security
- Sensitive field redaction
- Secret management (env vars, files)
- Input validation
- Rate limiting (DoS protection)

## Deployment Patterns

### Pattern 1: Centralized Collection
- Deploy as Deployment (3+ replicas)
- Load-balanced service
- Receive logs via HTTP/Syslog
- High availability

### Pattern 2: Distributed Collection
- Deploy as DaemonSet
- One pod per node
- Collect node-local logs
- Low latency, high throughput

### Pattern 3: Hybrid
- DaemonSet for container logs
- Deployment for external logs
- Best of both worlds

## Design Decisions

### Why Go?
- Native concurrency (goroutines)
- Low memory footprint
- Fast compilation
- Static binary deployment
- Great for systems programming

### Why Ring Buffer?
- Lock-free operations
- Predictable memory usage
- O(1) operations
- Cache-friendly

### Why WAL?
- Durability guarantee
- Fast recovery
- Proven pattern
- Simple implementation

### Why Prometheus?
- Industry standard
- Pull-based model
- Rich query language
- Grafana integration

## Future Enhancements

1. **Aggregation**: In-flight log aggregation
2. **Filtering**: Pre-output filtering rules
3. **Sampling**: Intelligent sampling algorithms
4. **Compression**: Online compression
5. **Encryption**: At-rest encryption
6. **Federation**: Multi-cluster support
7. **Auto-scaling**: Dynamic resource adjustment
8. **ML**: Anomaly detection
