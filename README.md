# SC Metrics Agent

[![Go Report Card](https://goreportcard.com/badge/github.com/strettch/sc-metrics-agent)](https://goreportcard.com/report/github.com/strettch/sc-metrics-agent)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

SC Metrics Agent is a comprehensive, production-ready agent for collecting VM-level system metrics and transmitting them to a central ingestor service. Built with Go, it's designed for reliability, performance, and complete observability.

## Overview

The SC Metrics Agent implements a **Collect → Decorate → Aggregate → Write** pipeline. It gathers a wide array of system metrics from VMs using Prometheus collectors and gopsutil, then transmits them to a configurable HTTP endpoint with Snappy compression for efficiency.

## Key Features

-   **Comprehensive Metrics Collection**: Gathers data on processes, CPU, memory, network, storage, load averages, and system information.
-   **Automatic VM Identification**: Uses `dmidecode -s system-uuid` with intelligent fallbacks to `/etc/machine-id`, `/proc/sys/kernel/random/boot_id`, and hostname.
-   **High-Performance Logging**: Structured JSON output using [Zap logger](https://github.com/uber-go/zap) for efficient and parseable logs.
-   **Advanced Compression**: Utilizes [klauspost/compress/snappy](https://github.com/klauspost/compress) for optimal data transmission efficiency.
-   **Payload Transparency**: Logs pre-compression payloads for easier debugging and monitoring.
-   **Configurable Collectors**: Granular control to enable or disable specific metric collectors.
-   **Resilient HTTP Client**: Includes retry logic and rate limiting for robust data delivery.
-   **Diagnostic Reporting**: Provides agent health monitoring capabilities.
-   **Graceful Shutdown**: Ensures final diagnostics are transmitted before exiting.
-   **Container-Ready**: Supports Docker and configuration via environment variables.

## Architecture

The agent follows a modular pipeline architecture:

```
┌─────────┐    ┌───────────┐    ┌───────────┐    ┌───────┐
│ Collect │ -> │ Decorate  │ -> │ Aggregate │ -> │ Write │
└─────────┘    └───────────┘    └───────────┘    └───────┘
```

### Components

1.  **Collector**: Gathers system metrics using gopsutil and procfs.
2.  **Decorator**: Enriches metrics with VM ID and custom labels.
3.  **Aggregator**: Converts Prometheus metrics into an internal format for transmission.
4.  **Writer**: Sends compressed metric batches via HTTP POST, complete with retry logic.

### Data Flow

1.  **Collection**: Metrics are gathered from various system sources (procfs, sysfs, gopsutil).
2.  **Decoration**: A unique VM identifier and any custom-defined labels are added to each metric.
3.  **Aggregation**: Metrics are transformed into `MetricWithValue` structs and sorted for consistency.
4.  **Transmission**: Data is serialized to JSON, compressed using Snappy, and then sent via HTTP POST to the configured ingestor.

## Installation

### Prerequisites

-   Go (version 1.20 or later recommended)
-   A Linux system (for full metric collection capabilities)
-   Network connectivity to the metrics ingestor endpoint
-   Root privileges (recommended for complete system access and `dmidecode`)

### Quick Install Script (Linux)

For a fast installation on Linux systems, you can use the following command:

```bash
curl -sSL https://repo.cloud.strettch.dev/install.sh | sudo bash
```

This script will download and install the latest version of the agent and set it up as a systemd service.

### Build from Source

```bash
git clone https://github.com/strettch/sc-metrics-agent.git
cd sc-metrics-agent
go mod tidy # or go mod download
make build
```

This will produce an executable binary in the `build/` directory (e.g., `build/sc-agent`).

### Systemd Service Setup (Recommended)

For production deployments, running the agent as a systemd service is recommended.

1.  **Copy the binary** to a standard location:
    ```bash
    sudo cp ./build/sc-agent /usr/local/bin/sc-metrics-agent
    ```
2.  **Create the configuration directory and file**:
    ```bash
    sudo mkdir -p /etc/sc-metrics-agent
    sudo cp config.example.yaml /etc/sc-metrics-agent/config.yaml
    # Edit /etc/sc-metrics-agent/config.yaml as needed
    ```
3.  **Copy the systemd service file**:
    ```bash
    sudo cp packaging/systemd/sc-metrics-agent.service /etc/systemd/system/
    ```
4.  **Reload systemd, enable, and start the service**:
    ```bash
    sudo systemctl daemon-reload
    sudo systemctl enable sc-metrics-agent.service
    sudo systemctl start sc-metrics-agent.service
    ```
5.  **Check the status**:
    ```bash
    sudo systemctl status sc-metrics-agent.service
    sudo journalctl -u sc-metrics-agent.service -f
    ```

## Configuration

The agent can be configured via a YAML file or environment variables. Environment variables override YAML settings.

### Configuration File

By default, the agent looks for `config.yaml` in the current directory or at `/etc/sc-metrics-agent/config.yaml` (when run as a service). An example configuration is provided in `config.example.yaml`.

Key configuration options:

-   `collection_interval`: How often to collect metrics (e.g., `30s`).
-   `ingestor_endpoint`: The URL of your metrics ingestor.
-   `vm_id`: (Optional) Manually set the VM ID. If empty, the agent attempts auto-detection.
-   `labels`: Custom key-value pairs to add to all metrics.
-   `collectors`: A map to enable/disable specific metric groups (e.g., `cpu: true`).
-   `log_level`: Logging verbosity (`debug`, `info`, `warn`, `error`, `fatal`).

### Environment Variables

-   `SC_AGENT_CONFIG`: Path to the configuration file.
-   `SC_COLLECTION_INTERVAL`: e.g., `60s`
-   `SC_INGESTOR_ENDPOINT`: e.g., `https://your-ingestor.com/api/metrics`
-   `SC_VM_ID`: Manually specify the VM ID.
-   `SC_LOG_LEVEL`: e.g., `debug`
-   `SC_LABEL_<KEY>`: For custom labels, e.g., `SC_LABEL_ENVIRONMENT=production`.

### VM ID Detection and Troubleshooting

The agent attempts to automatically determine a unique `vm_id` using the following methods in order:

1.  `dmidecode -s system-uuid` (requires `dmidecode` to be installed and accessible, often needs root)
2.  Contents of `/etc/machine-id`
3.  Contents of `/proc/sys/kernel/random/boot_id`
4.  The system's hostname

If the agent fails to start with an error like `"Failed to load configuration - VM ID detection failed"`, it means none of these methods succeeded. This is common in minimal environments or containers where `dmidecode` is not present or `/etc/machine-id` is not unique/available.

**Solution:**

The systemd service file (`packaging/systemd/sc-metrics-agent.service`) has been updated to attempt to set the `SC_VM_ID` environment variable using `dmidecode` before starting the agent:

```ini
[Service]
# ... other settings ...
ExecStartPre=/bin/sh -c "echo SC_VM_ID=$(/usr/sbin/dmidecode -s system-uuid 2>/dev/null || echo 'unknown-vm-id') > /run/sc-metrics-agent.env"
EnvironmentFile=/run/sc-metrics-agent.env
ExecStart=/usr/local/bin/sc-metrics-agent
Environment=SC_AGENT_CONFIG=/etc/sc-metrics-agent/config.yaml
# ... other settings ...
```

This `ExecStartPre` line tries to get the UUID via `dmidecode`. If it fails (e.g., `dmidecode` not found or no permission), it defaults to `unknown-vm-id`. The output is written to `/run/sc-metrics-agent.env`, which is then sourced by `EnvironmentFile`.

**To ensure this works:**

1.  **Install `dmidecode`**: On Debian/Ubuntu: `sudo apt update && sudo apt install dmidecode`. On RHEL/CentOS: `sudo yum install dmidecode`.
2.  Ensure the systemd service is reloaded and restarted after any changes: `sudo systemctl daemon-reload && sudo systemctl restart sc-metrics-agent.service`.

If `dmidecode` is not an option, you can manually set `SC_VM_ID`:

*   **In the systemd service file**: Add `Environment="SC_VM_ID=your-unique-id"`.
*   **In the `config.yaml`**: Set `vm_id: "your-unique-id"`.

## Usage

### Running Directly

```bash
# Build the agent
make build

# Run with default configuration (looks for config.yaml in current dir)
sudo ./build/sc-agent

# Specify a config file
sudo SC_AGENT_CONFIG=/path/to/your/config.yaml ./build/sc-agent

# Override settings with environment variables
sudo SC_INGESTOR_ENDPOINT=http://localhost:8080 SC_LOG_LEVEL=debug ./build/sc-agent
```

### Checking Version

```bash
./build/sc-agent -v
# or if installed via package manager
sc-metrics-agent -v
```

## Metrics Collected

The agent collects a wide range of metrics. Below is a summary. For a detailed list, please refer to the `collectors` section in `config.example.yaml` and the source code under `pkg/collector/`.

-   **Process Metrics**: PID count, process states, thread counts.
-   **CPU Metrics**: Usage per core (user, system, idle, iowait), CPU frequency, context switches, interrupts.
-   **Memory Metrics**: Total, free, available, used memory; buffer and cache sizes; swap space details.
-   **Virtual Memory Stats**: Page-ins/outs, swap-ins/outs (`vmstat`).
-   **Storage Metrics**: Disk reads/writes (completed operations, bytes, time spent).
-   **Filesystem Metrics**: Total, free, used space per filesystem; inode counts.
-   **Network Metrics**: Bytes/packets received/transmitted per interface, errors, drops.
-   **Network Connection Metrics**: Active connections by protocol/state (`netstat`), socket usage (`sockstat`).
-   **System Information**: Load averages (1, 5, 15 min), boot time, system time, uptime, entropy.
-   **Advanced Metrics**: Thermal zone temperatures, CPU/memory/IO pressure stall information.

## Development

### Building

```bash
make build
```

### Testing

```bash
make test
```

### Linting

```bash
make lint
```

### Releasing

The release process is managed by `make` targets and the `packaging/scripts/release.sh` script. It involves version bumping, git tagging, and potentially building release artifacts.

-   `make release-patch`
-   `make release-minor`
-   `make release-major`

## Contributing

Contributions are welcome! Please follow these steps:

1.  Fork the repository.
2.  Create a new branch (`git checkout -b feature/your-feature-name`).
3.  Make your changes.
4.  Ensure tests pass (`make test`).
5.  Ensure code is linted (`make lint`).
6.  Commit your changes (`git commit -am 'Add some feature'`).
7.  Push to the branch (`git push origin feature/your-feature-name`).
8.  Create a new Pull Request.

Please ensure your code follows Go best practices and includes tests for new functionality.

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details (assuming you will add one, if not, state the license directly).

---

*This README was generated with assistance from an AI coding partner.*