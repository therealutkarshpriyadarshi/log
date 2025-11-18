#!/bin/bash

# Run chaos tests

set -e

echo "Starting chaos testing environment..."

# Build and start all services
docker-compose -f docker-compose.test.yml build
docker-compose -f docker-compose.test.yml up -d

echo "Waiting for services to be ready..."
sleep 60

# Verify log aggregator is healthy
echo -n "Verifying log aggregator health"
until curl -s http://localhost:8081/health | grep -q "healthy"; do
    echo -n "."
    sleep 2
done
echo " ✓"

echo ""
echo "Starting chaos tests..."
echo ""
echo "⚠️  WARNING: These tests will deliberately break things!"
echo ""

# Run chaos tests
go test -v -tags=chaos -timeout=30m ./test/chaos/

TEST_EXIT_CODE=$?

# Cleanup
echo ""
echo "Cleaning up..."
docker-compose -f docker-compose.test.yml down -v

if [ $TEST_EXIT_CODE -eq 0 ]; then
    echo "✓ Chaos tests passed - system is resilient!"
else
    echo "✗ Chaos tests failed - system needs improvement"
fi

exit $TEST_EXIT_CODE
