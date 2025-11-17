# End-to-End Tests

This directory contains end-to-end tests for the logaggregator system.

## Overview

E2E tests validate the complete system behavior in a realistic environment, including:

- Full pipeline from input to output
- Multiple inputs running simultaneously
- Output to real destinations (Kafka, Elasticsearch, S3)
- System behavior under load
- Failure recovery scenarios
- Kubernetes deployment validation

## Prerequisites

- Docker and Docker Compose for running test dependencies
- Kubernetes cluster (Minikube, kind, or k3d) for K8s tests
- Go 1.21+

## Running E2E Tests

### Local E2E Tests

```bash
# Start test dependencies (Kafka, Elasticsearch, etc.)
docker-compose -f test/e2e/docker-compose.yaml up -d

# Run E2E tests
go test -v -tags=e2e ./test/e2e/...

# Cleanup
docker-compose -f test/e2e/docker-compose.yaml down
```

### Kubernetes E2E Tests

```bash
# Create test cluster
kind create cluster --name logaggregator-test

# Run K8s E2E tests
go test -v -tags=e2e,k8s ./test/e2e/k8s/...

# Cleanup
kind delete cluster --name logaggregator-test
```

## Test Categories

### 1. Basic Pipeline Tests
- File tailing → stdout
- HTTP input → stdout
- Syslog input → stdout

### 2. Output Tests
- Events → Kafka
- Events → Elasticsearch
- Events → S3
- Events → Multiple outputs

### 3. Load Tests
- High throughput (100K+ events/sec)
- Sustained load (1 hour+)
- Burst traffic

### 4. Failure Tests
- Input failure recovery
- Output failure recovery
- Network partition handling
- Disk full scenarios
- OOM scenarios

### 5. Kubernetes Tests
- Deployment validation
- DaemonSet validation
- Pod log collection
- Service discovery
- Rolling updates

## Writing E2E Tests

Example E2E test:

```go
// +build e2e

package e2e

import (
    "testing"
    "time"
)

func TestFullPipeline(t *testing.T) {
    // 1. Start logaggregator
    // 2. Send test events
    // 3. Verify events in output
    // 4. Cleanup
}
```

## CI/CD Integration

E2E tests are run in CI/CD pipeline:

1. On pull requests (basic E2E tests)
2. On main branch (full E2E test suite)
3. On releases (K8s E2E tests included)

## Debugging E2E Tests

```bash
# Run with verbose logging
go test -v -tags=e2e ./test/e2e/... -args -log-level=debug

# Run specific test
go test -v -tags=e2e ./test/e2e/... -run TestFullPipeline

# Keep containers running on failure
go test -v -tags=e2e ./test/e2e/... -args -keep-containers
```
