#!/bin/bash

# Full test suite runner
# Runs unit tests, integration tests, E2E tests, and chaos tests

set -e

echo "======================================"
echo "Running Full Test Suite"
echo "======================================"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Track test results
FAILED_TESTS=()
PASSED_TESTS=()

# Function to run a test and track results
run_test() {
    local test_name=$1
    local test_command=$2

    echo ""
    echo -e "${YELLOW}>>> Running: $test_name${NC}"
    echo "Command: $test_command"

    if eval "$test_command"; then
        echo -e "${GREEN}✓ PASSED: $test_name${NC}"
        PASSED_TESTS+=("$test_name")
        return 0
    else
        echo -e "${RED}✗ FAILED: $test_name${NC}"
        FAILED_TESTS+=("$test_name")
        return 1
    fi
}

# 1. Unit Tests
echo ""
echo "======================================"
echo "1. Running Unit Tests"
echo "======================================"
run_test "Unit Tests" "go test -v -race -coverprofile=coverage.out ./..." || true

# 2. Benchmark Tests
echo ""
echo "======================================"
echo "2. Running Benchmark Tests"
echo "======================================"
run_test "Benchmarks" "go test -bench=. -benchmem ./internal/benchmark/" || true

# 3. Linting
echo ""
echo "======================================"
echo "3. Running Linters"
echo "======================================"
run_test "Linting" "golangci-lint run --timeout=5m" || true

# 4. Integration Tests (requires Docker)
echo ""
echo "======================================"
echo "4. Running Integration Tests"
echo "======================================"

# Start test infrastructure
echo "Starting test infrastructure..."
docker-compose -f docker-compose.test.yml up -d zookeeper kafka elasticsearch minio redis

# Wait for services
echo "Waiting for services to be ready..."
sleep 30

# Run integration tests
run_test "Integration Tests" "go test -v -tags=integration ./test/integration/" || true

# 5. E2E Tests (requires full Docker setup)
echo ""
echo "======================================"
echo "5. Running E2E Tests"
echo "======================================"

# Start full test environment
echo "Starting full test environment..."
docker-compose -f docker-compose.test.yml up -d

# Wait for everything to be ready
echo "Waiting for all services to be ready..."
sleep 60

# Run E2E tests
run_test "E2E Tests" "go test -v -tags=e2e ./test/e2e/" || true

# 6. Chaos Tests (requires Docker)
echo ""
echo "======================================"
echo "6. Running Chaos Tests"
echo "======================================"

run_test "Chaos Tests" "go test -v -tags=chaos ./test/chaos/" || true

# 7. Load Tests (optional)
if [ "${RUN_LOAD_TESTS}" = "true" ]; then
    echo ""
    echo "======================================"
    echo "7. Running Load Tests"
    echo "======================================"

    docker-compose -f docker-compose.test.yml --profile load-test up -d load-generator

    echo "Load test running for 5 minutes..."
    sleep 300

    run_test "Load Tests" "docker logs test-load-generator" || true
fi

# Cleanup
echo ""
echo "======================================"
echo "Cleaning up test infrastructure"
echo "======================================"

docker-compose -f docker-compose.test.yml down -v

# Summary
echo ""
echo "======================================"
echo "Test Suite Summary"
echo "======================================"

echo ""
echo -e "${GREEN}Passed Tests: ${#PASSED_TESTS[@]}${NC}"
for test in "${PASSED_TESTS[@]}"; do
    echo -e "  ${GREEN}✓${NC} $test"
done

echo ""
if [ ${#FAILED_TESTS[@]} -gt 0 ]; then
    echo -e "${RED}Failed Tests: ${#FAILED_TESTS[@]}${NC}"
    for test in "${FAILED_TESTS[@]}"; do
        echo -e "  ${RED}✗${NC} $test"
    done
    echo ""
    echo -e "${RED}Some tests failed!${NC}"
    exit 1
else
    echo -e "${GREEN}All tests passed!${NC}"
    exit 0
fi
