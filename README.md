# SC Metrics Agent

A comprehensive, production-ready agent for collecting VM-level system metrics and transmitting them to a central ingestor service. Built with Go and designed for reliability, performance, and complete observability.

## Overview

The SC Metrics Agent implements a **Collect → Decorate → Aggregate → Write** pipeline to gather comprehensive system metrics from VMs using Prometheus collectors and gopsutil, then transmit them to a configurable HTTP endpoint with Snappy compression.

### Key Features

- **Comprehensive metrics collection** including processes, CPU, memory, network, storage, load averages, and system information
- **Automatic VM identification** using `dmidecode -s system-uuid` with intelligent fallbacks
- **High-performance logging** with structured JSON output using Zap logger
- **Advanced compression** using klauspost/compress/snappy for optimal efficiency
- **Payload transparency** with pre-compression logging for debugging and monitoring
- **Configurable collectors** with granular enable/disable control
- **Resilient HTTP client** with retry logic and rate limiting
- **Diagnostic reporting** for agent health monitoring
- **Graceful shutdown** with final diagnostics transmission
- **Container-ready** with Docker support and environment variable configuration

## Architecture

The agent follows a modular pipeline architecture:

```
┌─────────┐    ┌───────────┐    ┌───────────┐    ┌───────┐
│ Collect │ -> │ Decorate  │ -> │ Aggregate │ -> │ Write │
└─────────┘    └───────────┘    └───────────┘    └───────┘
```

### Components

1. **Collector**: Gathers comprehensive system metrics using gopsutil and procfs
2. **Decorator**: Adds VM ID and custom labels to all metrics
3. **Aggregator**: Converts Prometheus metrics to internal format for transmission
4. **Writer**: Sends compressed metric batches via HTTP POST with retry logic

### Data Flow

1. **Collection**: System metrics collected from multiple sources (procfs, sysfs, gopsutil)
2. **Decoration**: VM identifier and custom labels are added to each metric
3. **Aggregation**: Metrics are converted to `MetricWithValue` structs and sorted
4. **Transmission**: Data is JSON-serialized, Snappy-compressed, and sent via HTTP

## Comprehensive Metrics Collected

### Process Metrics
- **node_processes_pids**: Total number of process IDs
- **node_processes_state**: Process count by state (running, sleeping, etc.)
- **node_processes_threads**: Total thread count across all processes
- **node_processes_threads_state**: Thread count by state
- **node_processes_max_processes**: System PID limit from kernel
- **node_processes_max_threads**: System thread limit from kernel

### CPU Metrics
- **node_cpu_seconds_total**: CPU time spent in each mode (user, system, idle, iowait, etc.)
- **node_cpu_frequency_hertz**: Current CPU frequency for each core
- **node_context_switches_total**: Total context switches
- **node_intr_total**: Total interrupts serviced
- **node_softirqs_total**: Total software interrupts

### Memory Metrics
- **node_memory_MemTotal_bytes**: Total system memory
- **node_memory_MemFree_bytes**: Free memory available
- **node_memory_MemAvailable_bytes**: Available memory for new processes
- **node_memory_MemUsed_bytes**: Used memory
- **node_memory_Buffers_bytes**: Buffer cache memory
- **node_memory_Cached_bytes**: Page cache memory
- **node_memory_SwapTotal_bytes**: Total swap space
- **node_memory_SwapFree_bytes**: Free swap space
- **node_memory_SwapUsed_bytes**: Used swap space

### Virtual Memory Statistics
- **node_vmstat_pgpgin**: Pages read from disk
- **node_vmstat_pgpgout**: Pages written to disk  
- **node_vmstat_pswpin**: Swap pages read
- **node_vmstat_pswpout**: Swap pages written

### Storage Metrics
- **node_disk_reads_completed_total**: Completed disk reads
- **node_disk_writes_completed_total**: Completed disk writes
- **node_disk_read_bytes_total**: Bytes read from disk
- **node_disk_written_bytes_total**: Bytes written to disk
- **node_disk_read_time_seconds_total**: Time spent reading
- **node_disk_write_time_seconds_total**: Time spent writing

### Filesystem Metrics
- **node_filesystem_size_bytes**: Total filesystem size
- **node_filesystem_free_bytes**: Free space available
- **node_filesystem_used_bytes**: Space currently used
- **node_filesystem_files**: Total inodes available
- **node_filesystem_files_free**: Free inodes available

### Network Metrics
- **node_network_receive_bytes_total**: Bytes received per interface
- **node_network_transmit_bytes_total**: Bytes transmitted per interface
- **node_network_receive_packets_total**: Packets received per interface
- **node_network_transmit_packets_total**: Packets transmitted per interface
- **node_network_receive_errs_total**: Receive errors per interface
- **node_network_transmit_errs_total**: Transmit errors per interface
- **node_network_receive_drop_total**: Dropped received packets
- **node_network_transmit_drop_total**: Dropped transmitted packets

### Network Connection Metrics
- **node_netstat_connections**: Active connections by protocol and state
- **node_sockstat_sockets_used**: Sockets in use by protocol

### System Information Metrics
- **node_load1**: 1-minute load average
- **node_load5**: 5-minute load average
- **node_load15**: 15-minute load average
- **node_boot_time_seconds**: System boot time
- **node_time_seconds**: Current system time
- **node_uptime_seconds**: System uptime
- **node_entropy_available_bits**: Available entropy

### Advanced Metrics
- **node_thermal_zone_temp**: Temperature readings from thermal zones
- **node_pressure_cpu_waiting_seconds_total**: CPU pressure stall information
- **node_pressure_memory_waiting_seconds_total**: Memory pressure stall information
- **node_pressure_io_waiting_seconds_total**: I/O pressure stall information

## Installation

### Prerequisites

- Go 1.24.3 or later
- Linux system (recommended for full metric collection)
- Network connectivity to ingestor endpoint
- Root privileges (recommended for complete system access)

### Build from Source

```bash
git clone https://github.com/strettch/sc-metrics-agent.git
cd sc-metrics-agent
go mod download
make build
```

### Quick Start

```bash
# Build and run with default configuration
make build
sudo ./build/sc-agent

# Run with custom configuration
sudo SC_AGENT_CONFIG=/etc/sc-agent/config.yaml ./build/sc-agent

# Run with environment variables
sudo SC_INGESTOR_ENDPOINT=https://metrics.company.com/ingest \
     SC_VM_ID=web-server-prod-01 \
     SC_LOG_LEVEL=debug \
     ./build/sc-agent
```

## Configuration

### Automatic VM ID Detection

The agent automatically detects VM ID using:
1. `dmidecode -s system-uuid` (primary method)
2. `/etc/machine-id` (fallback)
3. `/proc/sys/kernel/random/boot_id` (fallback)
4. Hostname (final fallback)

### Configuration File

Create `config.yaml` from the example:

```bash
cp config.example.yaml config.yaml
```

Example configuration:

```yaml
collection_interval: 30s
ingestor_endpoint: "https://metrics.company.com/ingest"
vm_id: ""  # Leave empty for auto-detection

labels:
  environment: "production"
  region: "us-east-1"
  team: "platform"

collectors:
  # Process metrics (required)
  processes: true
  
  # CPU and load metrics
  cpu: true
  cpu_freq: true
  loadavg: true
  
  # Memory metrics
  memory: true
  vmstat: true
  
  # Storage metrics
  disk: true
  diskstats: true
  filesystem: true
  
  # Network metrics
  network: true
  netdev: true
  netstat: true
  sockstat: true
  
  # System metrics
  uname: true
  time: true
  uptime: true
  entropy: true
  interrupts: true
  
  # Advanced metrics
  thermal: true
  pressure: true
  schedstat: true

log_level: "info"
max_retries: 3
retry_interval: 5s
```

### Environment Variables

All configuration options can be set via environment variables:

```bash
# Basic configuration
export SC_COLLECTION_INTERVAL=60s
export SC_INGESTOR_ENDPOINT=https://metrics.example.com/ingest
export SC_VM_ID=my-unique-vm-id
export SC_LOG_LEVEL=debug
export SC_LABELS=env=prod,region=us-west-2,team=devops

# Collector toggles
export SC_COLLECTOR_PROCESSES=true
export SC_COLLECTOR_CPU=true
export SC_COLLECTOR_MEMORY=true
export SC_COLLECTOR_DISK=true
export SC_COLLECTOR_NETWORK=true
export SC_COLLECTOR_FILESYSTEM=true
# ... and many more (see config.example.yaml)
```

## HTTP Protocol

### Metrics Transmission

**Request:**
```
POST /ingest
Content-Type: application/timeseries-binary-0
Content-Encoding: snappy
User-Agent: sc-metrics-agent/1.0

[Snappy-compressed JSON payload]
```

**Payload Format:**
```json
[
  {
    "name": "node_processes_pids",
    "labels": {
      "vm_id": "web-server-01",
      "environment": "production"
    },
    "value": 42.0,
    "timestamp": 1677123456789,
    "type": "gauge"
  }
]
```

### Diagnostics Transmission

**Request:**
```
POST /ingest
Content-Type: application/diagnostics-binary-0
Content-Encoding: snappy
```

**Diagnostic Payload:**
```json
{
  "agent_id": "sc-agent-1677123456",
  "timestamp": 1677123456789,
  "status": "healthy",
  "last_error": "",
  "metrics_count": 156,
  "collector_status": {
    "processes": true,
    "cpu": true,
    "memory": true
  },
  "metadata": {
    "version": "1.0",
    "go_version": "1.24.3"
  }
}
```

## Advanced Features

### Payload Logging

The agent logs complete metric payloads before compression for debugging:

```json
{
  "level": "debug",
  "msg": "Sending metrics payload (before compression)",
  "metrics_count": 156,
  "payload_size_bytes": 8432,
  "payload_preview": "[{\"name\":\"node_cpu_seconds_total\",...}]",
  "time": "2024-01-15T10:30:45Z"
}
```

### Compression Efficiency

Snappy compression typically achieves 50-80% size reduction:

```json
{
  "level": "debug", 
  "msg": "Compressed payload",
  "original_size": 8432,
  "compressed_size": 4216,
  "compression_ratio": 0.5,
  "time": "2024-01-15T10:30:45Z"
}
```

### Error Handling & Resilience

- **Automatic retry** on HTTP status codes: 429, 500, 502, 503, 504
- **Rate limiting** compliance via `Retry-After` headers
- **Partial collection** continues even if some collectors fail
- **Diagnostic fallback** sends agent health when metrics fail
- **Graceful degradation** with detailed error logging

## Deployment

### Systemd Service

Create `/etc/systemd/system/sc-agent.service`:

```ini
[Unit]
Description=SC Metrics Agent
After=network.target
Wants=network.target

[Service]
Type=simple
User=root
Group=root
ExecStart=/usr/local/bin/sc-agent
Environment=SC_AGENT_CONFIG=/etc/sc-agent/config.yaml
Restart=always
RestartSec=5
KillMode=mixed
KillSignal=SIGTERM

[Install]
WantedBy=multi-user.target
```

```bash
sudo systemctl daemon-reload
sudo systemctl enable sc-agent
sudo systemctl start sc-agent
```

### Docker Deployment

```dockerfile
FROM golang:1.24.3-alpine AS builder
WORKDIR /app
COPY . .
RUN go mod download && go build -o sc-agent ./cmd/agent

FROM alpine:latest
RUN apk --no-cache add ca-certificates dmidecode
WORKDIR /root/
COPY --from=builder /app/sc-agent .
CMD ["./sc-agent"]
```

```bash
docker build -t sc-agent .
docker run -d \
  --name sc-agent \
  --pid=host \
  --privileged \
  -v /proc:/host/proc:ro \
  -v /sys:/host/sys:ro \
  -e SC_INGESTOR_ENDPOINT=https://metrics.example.com/ingest \
  -e SC_VM_ID=docker-host-01 \
  sc-agent
```

### Kubernetes Deployment

```yaml
apiVersion: apps/v1
kind: DaemonSet
metadata:
  name: sc-metrics-agent
spec:
  selector:
    matchLabels:
      app: sc-metrics-agent
  template:
    metadata:
      labels:
        app: sc-metrics-agent
    spec:
      hostPID: true
      hostNetwork: true
      containers:
      - name: sc-agent
        image: sc-agent:latest
        securityContext:
          privileged: true
        env:
        - name: SC_INGESTOR_ENDPOINT
          value: "https://metrics.company.com/ingest"
        - name: SC_VM_ID
          valueFrom:
            fieldRef:
              fieldPath: spec.nodeName
        - name: SC_LABELS
          value: "cluster=prod,datacenter=us-east-1"
        volumeMounts:
        - name: proc
          mountPath: /host/proc
          readOnly: true
        - name: sys
          mountPath: /host/sys
          readOnly: true
      volumes:
      - name: proc
        hostPath:
          path: /proc
      - name: sys
        hostPath:
          path: /sys
```

## Development

### Build Commands

```bash
# Development build and test
make build
make test
make test-coverage

# Production builds
make build-all          # Multi-platform binaries
make release            # Release archives

# Code quality
make lint               # Run linters
make fmt                # Format code
make security           # Security scan

# Development tools
make dev-setup          # Install dev dependencies
make watch              # Auto-rebuild on changes
```

### Project Structure

```
sc-metrics-agent/
├── cmd/agent/           # Main application entry point
├── pkg/
│   ├── aggregate/       # Metric aggregation and processing
│   ├── clients/         # HTTP client with Snappy compression
│   ├── collector/       # Comprehensive system metric collectors
│   ├── config/          # Configuration management with dmidecode
│   ├── decorator/       # Metric labeling and enhancement
│   └── pipeline/        # Processing pipeline orchestration
├── config.example.yaml  # Comprehensive configuration example
├── Dockerfile          # Container deployment
├── Makefile           # Build and development automation
└── README.md          # This documentation
```

### Testing

```bash
# Run all tests
go test ./...

# Test with race detection
go test -race ./...

# Benchmark tests
go test -bench=. ./...

# Integration test with mock server
go run test_payload_logging.go
```

## Monitoring

### Structured Logging

All logs use structured JSON format with Zap:

```json
{
  "level": "info",
  "time": "2024-01-15T10:30:45Z",
  "caller": "pipeline/processor.go:147",
  "msg": "Pipeline processing completed successfully",
  "collected_families": 12,
  "decorated_families": 12,
  "aggregated_metrics": 156,
  "processing_time": "45.2ms"
}
```

### Health Monitoring

Monitor agent health through:

1. **Log Analysis**: Check for error patterns and processing times
2. **Diagnostic Payloads**: Monitor agent status and collector health
3. **Process Monitoring**: Ensure agent process remains running
4. **Metric Flow**: Verify metrics are reaching the ingestor
5. **Resource Usage**: Monitor agent CPU and memory consumption

### Performance Metrics

Key performance indicators:

- **Collection Time**: Time to gather all enabled metrics
- **Processing Time**: Complete pipeline execution time
- **Compression Ratio**: Payload size reduction efficiency
- **HTTP Response Time**: Network transmission latency
- **Error Rate**: Failed collection or transmission percentage

## Troubleshooting

### Common Issues

**Agent fails to start:**
```bash
# Check configuration syntax
go run ./cmd/agent -validate-config

# Verify permissions for system access
sudo ./build/sc-agent

# Test network connectivity
curl -X POST https://your-ingestor-endpoint/ingest
```

**Missing metrics:**
```bash
# Check collector status in logs
grep "collector" /var/log/sc-agent.log

# Verify system permissions
ls -la /proc /sys

# Test individual collectors
SC_COLLECTOR_PROCESSES=true SC_LOG_LEVEL=debug ./build/sc-agent
```

**High resource usage:**
```bash
# Monitor with reduced collectors
SC_COLLECTOR_THERMAL=false SC_COLLECTOR_PRESSURE=false ./build/sc-agent

# Increase collection interval
SC_COLLECTION_INTERVAL=60s ./build/sc-agent

# Check system load
top -p $(pgrep sc-agent)
```

### Debug Mode

Enable comprehensive debugging:

```bash
SC_LOG_LEVEL=debug ./build/sc-agent
```

Debug logs include:
- Metric collection details for each collector
- Complete payload contents before compression
- HTTP request/response debugging
- Compression statistics and ratios
- Processing pipeline timing information

## Performance Tuning

### Collection Optimization

**High-frequency monitoring:**
```yaml
collection_interval: 10s
collectors:
  processes: true
  cpu: true
  memory: true
  loadavg: true
  # Disable expensive collectors
  pressure: false
  thermal: false
```

**Resource-constrained environments:**
```yaml
collection_interval: 120s
collectors:
  # Enable only essential metrics
  processes: true
  cpu: true
  memory: true
  loadavg: true
  # Disable detailed metrics
  disk: false
  network: false
  filesystem: false
```

### Network Optimization

- **Compression**: Snappy achieves 50-80% size reduction
- **Batching**: Default 1000 metrics per HTTP request
- **Keep-alive**: HTTP connections reused for efficiency
- **Retry logic**: Exponential backoff prevents network storms

## Security Considerations

### Permissions

- **Root access**: Required for complete system metric visibility
- **Proc access**: Needs read access to `/proc` and `/sys` filesystems
- **Network access**: Requires outbound HTTPS connectivity

### Data Privacy

- **No sensitive data**: Metrics contain only system performance data
- **Configurable labels**: Control what metadata is transmitted
- **Network encryption**: HTTPS recommended for transmission
- **Local storage**: No metrics cached locally

### Container Security

- **Minimal image**: Alpine-based with only required dependencies
- **Non-root option**: Can run unprivileged with reduced metrics
- **Read-only mounts**: System directories mounted read-only
- **Capability restrictions**: Only required Linux capabilities

## License

This project is licensed under the MIT License - see the LICENSE file for details.

## Contributing

1. Fork the repository
2. Create your feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add comprehensive disk metrics'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

## Support

For issues and questions:

1. Check the troubleshooting section above
2. Review existing GitHub issues
3. Create a new issue with:
   - Agent version and configuration
   - Complete error messages and debug logs
   - Steps to reproduce the problem
   - System information (OS, kernel version, hardware)
   - Expected vs actual behavior

## Changelog

### v1.0.0
- Comprehensive system metrics collection (150+ metrics)
- Automatic VM ID detection via dmidecode
- Zap structured logging with payload transparency
- klauspost/compress/snappy for optimal compression
- Production-ready deployment options
- Complete Docker and Kubernetes support
- Extensive configuration options and environment variables
- Advanced error handling and diagnostic reporting