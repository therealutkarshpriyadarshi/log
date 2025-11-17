.PHONY: build test lint clean run install-deps benchmark loadtest profile

# Binary name
BINARY_NAME=logaggregator
BUILD_DIR=bin

# Build the application
build:
	@echo "Building $(BINARY_NAME)..."
	@mkdir -p $(BUILD_DIR)
	go build -v -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/logaggregator

# Build load test tool
build-loadtest:
	@echo "Building loadtest tool..."
	@mkdir -p $(BUILD_DIR)
	go build -v -o $(BUILD_DIR)/loadtest ./cmd/loadtest

# Run tests
test:
	@echo "Running tests..."
	go test -v -race -coverprofile=coverage.out ./...

# Run tests with coverage report
test-coverage: test
	@echo "Generating coverage report..."
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

# Run linter
lint:
	@echo "Running linter..."
	golangci-lint run --timeout=5m

# Clean build artifacts
clean:
	@echo "Cleaning..."
	rm -rf $(BUILD_DIR)
	rm -f coverage.out coverage.html

# Run the application
run: build
	@echo "Running $(BINARY_NAME)..."
	./$(BUILD_DIR)/$(BINARY_NAME) -config config.yaml

# Install dependencies
install-deps:
	@echo "Installing dependencies..."
	go mod download
	go mod tidy

# Format code
fmt:
	@echo "Formatting code..."
	go fmt ./...
	goimports -w -local github.com/therealutkarshpriyadarshi/log .

# Build for multiple platforms
build-all:
	@echo "Building for multiple platforms..."
	@mkdir -p $(BUILD_DIR)
	GOOS=linux GOARCH=amd64 go build -o $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64 ./cmd/logaggregator
	GOOS=linux GOARCH=arm64 go build -o $(BUILD_DIR)/$(BINARY_NAME)-linux-arm64 ./cmd/logaggregator
	GOOS=darwin GOARCH=amd64 go build -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-amd64 ./cmd/logaggregator
	GOOS=darwin GOARCH=arm64 go build -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-arm64 ./cmd/logaggregator
	GOOS=windows GOARCH=amd64 go build -o $(BUILD_DIR)/$(BINARY_NAME)-windows-amd64.exe ./cmd/logaggregator

# Install the binary to GOPATH/bin
install: build
	@echo "Installing $(BINARY_NAME) to $(GOPATH)/bin..."
	cp $(BUILD_DIR)/$(BINARY_NAME) $(GOPATH)/bin/

# Run benchmarks
benchmark:
	@echo "Running benchmarks..."
	go test -bench=. -benchmem -benchtime=5s ./internal/benchmark/

# Run benchmarks with profiling
benchmark-profile:
	@echo "Running benchmarks with CPU profiling..."
	@mkdir -p profiles
	go test -bench=. -benchmem -cpuprofile=profiles/cpu.prof -memprofile=profiles/mem.prof ./internal/benchmark/
	@echo "Profiles saved to profiles/"

# Run load test
loadtest: build-loadtest
	@echo "Running load test..."
	./$(BUILD_DIR)/loadtest -rate 100000 -duration 60 -workers 4

# Run load test with custom parameters
loadtest-custom: build-loadtest
	@echo "Running custom load test..."
	@echo "Usage: make loadtest-custom RATE=100000 DURATION=60 WORKERS=4"
	./$(BUILD_DIR)/loadtest -rate $(or $(RATE),100000) -duration $(or $(DURATION),60) -workers $(or $(WORKERS),4)

# Profile the application
profile:
	@echo "Starting profiling server on :6060"
	@echo "Access profiles at http://localhost:6060/debug/pprof/"
	@echo "CPU profile: go tool pprof http://localhost:6060/debug/pprof/profile?seconds=30"
	@echo "Heap profile: go tool pprof http://localhost:6060/debug/pprof/heap"
	@echo "Press Ctrl+C to stop"

help:
	@echo "Available targets:"
	@echo "  build              - Build the application"
	@echo "  build-loadtest     - Build load test tool"
	@echo "  test               - Run tests"
	@echo "  test-coverage      - Run tests with coverage report"
	@echo "  benchmark          - Run performance benchmarks"
	@echo "  benchmark-profile  - Run benchmarks with profiling"
	@echo "  loadtest           - Run load test (100K events/sec for 60s)"
	@echo "  loadtest-custom    - Run custom load test (set RATE, DURATION, WORKERS)"
	@echo "  profile            - Show profiling instructions"
	@echo "  lint               - Run linter"
	@echo "  clean              - Clean build artifacts"
	@echo "  run                - Build and run the application"
	@echo "  install-deps       - Install Go dependencies"
	@echo "  fmt                - Format code"
	@echo "  build-all          - Build for multiple platforms"
	@echo "  install            - Install binary to GOPATH/bin"
	@echo "  help               - Show this help message"
