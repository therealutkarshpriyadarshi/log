#!/bin/bash

# Run integration tests with real services

set -e

echo "Starting integration test infrastructure..."

# Start required services
docker-compose -f docker-compose.test.yml up -d zookeeper kafka elasticsearch minio redis prometheus jaeger

echo "Waiting for services to be ready..."

# Wait for Kafka
echo -n "Waiting for Kafka"
until docker exec test-kafka kafka-broker-api-versions --bootstrap-server localhost:9092 &> /dev/null; do
    echo -n "."
    sleep 2
done
echo " ✓"

# Wait for Elasticsearch
echo -n "Waiting for Elasticsearch"
until curl -s http://localhost:9200/_cluster/health &> /dev/null; do
    echo -n "."
    sleep 2
done
echo " ✓"

# Wait for MinIO
echo -n "Waiting for MinIO"
until curl -s http://localhost:9000/minio/health/live &> /dev/null; do
    echo -n "."
    sleep 2
done
echo " ✓"

# Wait for Redis
echo -n "Waiting for Redis"
until docker exec test-redis redis-cli ping &> /dev/null; do
    echo -n "."
    sleep 2
done
echo " ✓"

echo ""
echo "All services ready!"
echo ""

# Set environment variables for tests
export KAFKA_BROKERS="localhost:29092"
export ELASTICSEARCH_URL="http://localhost:9200"
export S3_ENDPOINT="http://localhost:9000"
export S3_ACCESS_KEY="minioadmin"
export S3_SECRET_KEY="minioadmin"
export S3_BUCKET="test-logs"

# Run integration tests
echo "Running integration tests..."
go test -v -tags=integration -timeout=10m ./test/integration/

TEST_EXIT_CODE=$?

# Cleanup
echo ""
echo "Cleaning up..."
docker-compose -f docker-compose.test.yml down -v

exit $TEST_EXIT_CODE
