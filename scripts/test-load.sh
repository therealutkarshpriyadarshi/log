#!/bin/bash

# Run load tests

set -e

# Configuration
RATE=${RATE:-1000}
DURATION=${DURATION:-300s}
WORKERS=${WORKERS:-10}

echo "Starting load testing environment..."
echo "Configuration:"
echo "  Rate: $RATE events/sec"
echo "  Duration: $DURATION"
echo "  Workers: $WORKERS"
echo ""

# Build and start services
docker-compose -f docker-compose.test.yml build
docker-compose -f docker-compose.test.yml up -d

echo "Waiting for services to be ready..."
sleep 60

# Verify health
until curl -s http://localhost:8081/health | grep -q "healthy"; do
    echo "Waiting for log aggregator..."
    sleep 5
done

echo "Log aggregator is ready!"
echo ""

# Start load test
echo "Starting load test..."
docker-compose -f docker-compose.test.yml --profile load-test up load-generator

# Show results
echo ""
echo "Load test completed!"
echo ""
echo "Results saved to ./test/results/"

# Show metrics
echo ""
echo "Fetching final metrics..."
curl -s http://localhost:9091/metrics | grep -E "(log_events|log_processing)" || true

# Cleanup
echo ""
echo "Cleaning up..."
docker-compose -f docker-compose.test.yml down -v

echo ""
echo "Load test complete!"
