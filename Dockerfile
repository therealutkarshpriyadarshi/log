# Multi-stage build for minimal production image

# Stage 1: Build
FROM golang:1.21-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git make ca-certificates tzdata

# Set working directory
WORKDIR /build

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies (cached if go.mod/go.sum unchanged)
RUN go mod download

# Copy source code
COPY . .

# Build the application
# CGO_ENABLED=0 for static binary
# -ldflags for smaller binary size
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-w -s" \
    -o /build/logaggregator \
    ./cmd/logaggregator

# Stage 2: Runtime
FROM alpine:3.19

# Install runtime dependencies
RUN apk add --no-cache ca-certificates tzdata

# Create non-root user
RUN addgroup -g 1000 logaggregator && \
    adduser -D -u 1000 -G logaggregator logaggregator

# Create necessary directories
RUN mkdir -p /var/lib/logaggregator/checkpoints \
             /var/lib/logaggregator/wal \
             /var/lib/logaggregator/dlq \
             /etc/logaggregator && \
    chown -R logaggregator:logaggregator /var/lib/logaggregator /etc/logaggregator

# Copy binary from builder
COPY --from=builder /build/logaggregator /usr/local/bin/logaggregator

# Copy example config (optional)
COPY --from=builder /build/config.yaml.example /etc/logaggregator/config.yaml.example

# Set ownership
RUN chown logaggregator:logaggregator /usr/local/bin/logaggregator

# Switch to non-root user
USER logaggregator

# Set working directory
WORKDIR /var/lib/logaggregator

# Expose ports
# 8080: HTTP input
# 514: Syslog (UDP/TCP)
# 9090: Metrics
# 8081: Health checks
# 6060: Profiling (disable in production)
EXPOSE 8080 514 9090 8081

# Health check
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8081/health || exit 1

# Default command
ENTRYPOINT ["/usr/local/bin/logaggregator"]
CMD ["-config", "/etc/logaggregator/config.yaml"]
