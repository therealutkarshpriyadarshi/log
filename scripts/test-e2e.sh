#!/bin/bash

# Run E2E tests with full Docker environment

set -e

echo "Starting E2E test environment..."

# Build the application image
echo "Building application image..."
docker-compose -f docker-compose.test.yml build log-aggregator

# Start all services
docker-compose -f docker-compose.test.yml up -d

echo "Waiting for all services to be ready..."

# Wait for log aggregator health check
echo -n "Waiting for log aggregator"
RETRY_COUNT=0
MAX_RETRIES=60
until curl -s http://localhost:8081/health | grep -q "healthy" || [ $RETRY_COUNT -eq $MAX_RETRIES ]; do
    echo -n "."
    sleep 2
    RETRY_COUNT=$((RETRY_COUNT + 1))
done

if [ $RETRY_COUNT -eq $MAX_RETRIES ]; then
    echo " ✗"
    echo "Log aggregator failed to become healthy"
    echo "Logs:"
    docker logs test-log-aggregator
    docker-compose -f docker-compose.test.yml down -v
    exit 1
fi

echo " ✓"

echo ""
echo "All services ready!"
echo ""

# Show service status
echo "Service Status:"
docker-compose -f docker-compose.test.yml ps

echo ""

# Set environment variables for tests
export LOG_AGGREGATOR_URL="http://localhost:8080"
export HEALTH_URL="http://localhost:8081/health"
export METRICS_URL="http://localhost:9091/metrics"
export KAFKA_BROKERS="localhost:29092"
export ELASTICSEARCH_URL="http://localhost:9200"
export S3_ENDPOINT="http://localhost:9000"

# Run E2E tests
echo "Running E2E tests..."
go test -v -tags=e2e -timeout=15m ./test/e2e/

TEST_EXIT_CODE=$?

# Show logs on failure
if [ $TEST_EXIT_CODE -ne 0 ]; then
    echo ""
    echo "E2E tests failed. Showing service logs:"
    echo "========================================"
    docker-compose -f docker-compose.test.yml logs log-aggregator
fi

# Cleanup
echo ""
echo "Cleaning up..."
docker-compose -f docker-compose.test.yml down -v

exit $TEST_EXIT_CODE
