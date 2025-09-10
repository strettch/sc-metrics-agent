# Multi-stage build for SC Metrics Agent
FROM golang:1.24.3-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git ca-certificates tzdata

# Set working directory
WORKDIR /app

# Copy go mod files first for better caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags='-s -w -extldflags "-static"' \
    -a -installsuffix cgo \
    -o sc-agent \
    ./cmd/agent

# Final stage - minimal runtime image
FROM alpine:3.19

# Install runtime dependencies
RUN apk add --no-cache \
    ca-certificates \
    tzdata \
    && rm -rf /var/cache/apk/*

# Create non-root user (though process metrics may require root)
RUN addgroup -g 1000 -S sc-agent && \
    adduser -u 1000 -S sc-agent -G sc-agent

# Set working directory
WORKDIR /app

# Copy binary from builder stage
COPY --from=builder /app/sc-agent .

# Copy configuration files
COPY config.example.yaml ./config.example.yaml

# Create directories for logs and data
RUN mkdir -p /var/log/sc-agent /var/lib/sc-agent && \
    chown -R sc-agent:sc-agent /app /var/log/sc-agent /var/lib/sc-agent

# Set permissions
RUN chmod +x sc-agent

# Environment variables with defaults
ENV SC_LOG_LEVEL=info
ENV SC_COLLECTION_INTERVAL=30s
ENV SC_HTTP_TIMEOUT=30s
ENV SC_COLLECTOR_PROCESSES=true

# Expose health check port (if implemented)
EXPOSE 8081

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD pgrep -f sc-agent > /dev/null || exit 1

# Use root user for process metrics collection
# In production, consider using specific capabilities instead
USER root

# Default command
CMD ["./sc-agent"]

# Labels for metadata
LABEL maintainer="SC Metrics Team" \
      version="1.0.0" \
      description="Lightweight VM process metrics collection agent" \
      org.opencontainers.image.source="https://github.com/strettch/sc-metrics-agent" \
      org.opencontainers.image.title="SC Metrics Agent" \
      org.opencontainers.image.description="Collects process-level metrics from VMs and sends to ingestor service"