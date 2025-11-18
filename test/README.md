# Test Suite Documentation

This directory contains the comprehensive test suite for the log aggregator application.

## Overview

The test suite consists of multiple layers:

1. **Unit Tests** - Fast, isolated tests for individual components
2. **Integration Tests** - Tests with real external services (Kafka, Elasticsearch, S3)
3. **E2E Tests** - End-to-end tests with the full application stack
4. **Chaos Tests** - Resilience tests simulating failures and recovery
5. **Load Tests** - Performance and stress testing

## Prerequisites

### For Unit Tests
- Go 1.21+
- No external dependencies required

### For Integration/E2E/Chaos Tests
- Docker 20.10+
- Docker Compose 2.0+
- 8GB+ RAM recommended
- 20GB+ free disk space

## Quick Start

### Run All Tests
```bash
./scripts/test-all.sh
```

### Run Specific Test Suites

#### Unit Tests Only
```bash
./scripts/test-unit.sh
# or
make test
```

#### Integration Tests
```bash
./scripts/test-integration.sh
```

#### E2E Tests
```bash
./scripts/test-e2e.sh
```

#### Chaos Tests
```bash
./scripts/test-chaos.sh
```

#### Load Tests
```bash
./scripts/test-load.sh
# or with custom parameters
RATE=5000 DURATION=600s WORKERS=20 ./scripts/test-load.sh
```

## Test Structure

```
test/
├── integration/        # Integration tests with real services
│   └── real_services_test.go
├── e2e/               # End-to-end tests
│   ├── e2e_test.go
│   └── README.md
├── chaos/             # Chaos engineering tests
│   └── chaos_test.go
├── config/            # Test configurations
│   ├── config.test.yaml
│   └── prometheus.yml
├── data/              # Test data files
├── results/           # Test results and reports
└── README.md          # This file
```

## Test Categories

### 1. Unit Tests

Location: `internal/*/\*_test.go`

Run with:
```bash
go test ./...
# or with coverage
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

Features:
- Fast execution (< 1 minute)
- No external dependencies
- Race condition detection with `-race` flag
- Code coverage reporting

### 2. Integration Tests

Location: `test/integration/real_services_test.go`

Run with:
```bash
go test -tags=integration ./test/integration/
```

Tests included:
- **Kafka Integration**: Producer/consumer operations, batch processing
- **Elasticsearch Integration**: Indexing, searching, bulk operations
- **S3 Integration**: Object storage operations with MinIO
- **Full Pipeline**: End-to-end log processing

Services used:
- Kafka (Confluent) on port 29092
- Elasticsearch 8.11 on port 9200
- MinIO (S3-compatible) on port 9000
- Redis on port 6379

### 3. E2E Tests

Location: `test/e2e/e2e_test.go`

Run with:
```bash
go test -tags=e2e ./test/e2e/
```

Tests included:
- Health check endpoint validation
- Metrics endpoint validation
- HTTP log ingestion
- Kafka output verification
- Elasticsearch output verification
- S3 output verification
- High throughput testing (1000+ logs/sec)
- System resilience

### 4. Chaos Tests

Location: `test/chaos/chaos_test.go`

Run with:
```bash
go test -tags=chaos ./test/chaos/
```

Tests included:
- **Kill Log Aggregator**: Verify recovery after crash
- **Kafka Failure**: Test buffering and recovery
- **Elasticsearch Failure**: Circuit breaker validation
- **Network Latency**: Timeout handling under high latency
- **Multiple Failures**: Cascading failure scenarios
- **Rapid Restarts**: Stability under frequent restarts

Chaos operations:
- Container killing
- Container pausing/unpausing
- Network partition simulation
- Latency injection
- Resource exhaustion

### 5. Load Tests

Location: `cmd/loadtest/`

Run with:
```bash
docker-compose -f docker-compose.test.yml --profile load-test up
# or
./scripts/test-load.sh
```

Configuration:
```bash
export RATE=1000          # Events per second
export DURATION=300s      # Test duration
export WORKERS=10         # Concurrent workers
```

Metrics collected:
- Throughput (events/sec)
- Latency (p50, p95, p99)
- Error rate
- Resource usage (CPU, memory)

## Docker Compose Services

The test infrastructure uses `docker-compose.test.yml`:

| Service | Port(s) | Purpose |
|---------|---------|---------|
| zookeeper | 2181 | Kafka coordination |
| kafka | 9092, 29092 | Message queue |
| elasticsearch | 9200, 9300 | Log indexing and search |
| minio | 9000, 9001 | S3-compatible storage |
| redis | 6379 | Caching/state |
| prometheus | 9090 | Metrics collection |
| jaeger | 16686, 4317, 4318 | Distributed tracing |
| log-aggregator | 8080, 514, 9091, 8081 | Application under test |
| chaos-controller | - | Chaos testing |
| load-generator | - | Load testing |

## Environment Variables

### Integration Tests
```bash
export KAFKA_BROKERS="localhost:29092"
export ELASTICSEARCH_URL="http://localhost:9200"
export S3_ENDPOINT="http://localhost:9000"
export S3_ACCESS_KEY="minioadmin"
export S3_SECRET_KEY="minioadmin"
export S3_BUCKET="test-logs"
```

### E2E Tests
```bash
export LOG_AGGREGATOR_URL="http://localhost:8080"
export HEALTH_URL="http://localhost:8081/health"
export METRICS_URL="http://localhost:9091/metrics"
```

### Load Tests
```bash
export TARGET_URL="http://log-aggregator:8080"
export RATE=1000
export DURATION=300s
export WORKERS=10
```

## CI/CD Integration

### GitHub Actions

The test suite integrates with GitHub Actions (`.github/workflows/ci.yml`):

```yaml
- Unit tests run on every push
- Integration tests run on pull requests
- E2E tests run before merges
- Chaos tests run nightly
- Load tests run weekly
```

### Running in CI

```bash
# Unit tests (fast)
make test

# Integration tests (requires Docker)
./scripts/test-integration.sh

# Full suite (requires time and resources)
./scripts/test-all.sh
```

## Troubleshooting

### Services Not Starting

```bash
# Check Docker is running
docker ps

# Check Docker Compose
docker-compose -f docker-compose.test.yml ps

# View logs
docker-compose -f docker-compose.test.yml logs

# Restart services
docker-compose -f docker-compose.test.yml restart
```

### Tests Timing Out

```bash
# Increase timeout
go test -timeout=30m -tags=e2e ./test/e2e/

# Wait longer for services
sleep 120  # before running tests
```

### Port Conflicts

```bash
# Check ports in use
lsof -i :9092  # Kafka
lsof -i :9200  # Elasticsearch
lsof -i :9000  # MinIO

# Stop conflicting services
docker-compose -f docker-compose.test.yml down -v
```

### Cleanup

```bash
# Stop all containers
docker-compose -f docker-compose.test.yml down

# Remove volumes
docker-compose -f docker-compose.test.yml down -v

# Remove images
docker-compose -f docker-compose.test.yml down --rmi all -v

# Clean Docker system
docker system prune -af --volumes
```

## Test Data

### Sample Log Formats

#### JSON
```json
{
  "timestamp": "2024-01-01T00:00:00Z",
  "level": "info",
  "message": "User logged in",
  "user_id": "123",
  "ip": "192.168.1.1"
}
```

#### Syslog
```
Jan 1 00:00:00 hostname app[1234]: User logged in
```

#### Apache Combined
```
192.168.1.1 - - [01/Jan/2024:00:00:00 +0000] "GET /api/users HTTP/1.1" 200 1234
```

## Performance Benchmarks

Expected performance on standard hardware (4 CPU, 8GB RAM):

| Metric | Target | Actual |
|--------|--------|--------|
| Throughput | 10,000 events/sec | TBD |
| Latency (p95) | < 100ms | TBD |
| Memory Usage | < 2GB | TBD |
| CPU Usage | < 80% | TBD |

## Contributing

### Adding New Tests

1. **Unit Test**: Add `*_test.go` file in the same package
2. **Integration Test**: Add to `test/integration/real_services_test.go`
3. **E2E Test**: Add to `test/e2e/e2e_test.go`
4. **Chaos Test**: Add to `test/chaos/chaos_test.go`

### Test Naming Convention

```go
// Unit tests
func TestFunctionName(t *testing.T) { ... }
func TestFunctionName_EdgeCase(t *testing.T) { ... }

// Integration tests
func TestKafkaIntegration(t *testing.T) { ... }

// E2E tests
func TestE2E_HTTPLogIngestion(t *testing.T) { ... }

// Chaos tests
func TestChaos_KafkaFailure(t *testing.T) { ... }
```

### Build Tags

```go
// +build integration
// +build e2e
// +build chaos
```

## Resources

- [Go Testing Documentation](https://golang.org/pkg/testing/)
- [Docker Compose Reference](https://docs.docker.com/compose/)
- [Chaos Engineering Principles](https://principlesofchaos.org/)
- [Kafka Testing Guide](https://kafka.apache.org/documentation/#testing)
- [Elasticsearch Testing](https://www.elastic.co/guide/en/elasticsearch/reference/current/testing.html)

## Support

For issues or questions:
1. Check this README
2. Review test logs: `docker-compose -f docker-compose.test.yml logs`
3. Open an issue on GitHub
4. Contact the development team
