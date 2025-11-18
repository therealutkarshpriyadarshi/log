#!/bin/bash

# Run unit tests with coverage

set -e

echo "Running unit tests with coverage..."

# Run tests
go test -v -race -coverprofile=coverage.out -covermode=atomic ./...

# Generate coverage report
echo ""
echo "Generating coverage report..."
go tool cover -func=coverage.out

# Generate HTML coverage report
echo ""
echo "Generating HTML coverage report..."
go tool cover -html=coverage.out -o coverage.html

echo ""
echo "Coverage report generated: coverage.html"
echo "Total coverage:"
go tool cover -func=coverage.out | grep total | awk '{print $3}'
