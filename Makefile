# SC Metrics Agent Makefile

.PHONY: all build test clean install lint fmt vet run deps help release docker

# Build variables
BINARY_NAME=sc-metrics-agent
BUILD_DIR=build
MAIN_PATH=./cmd/agent
VERSION := $(shell git describe --tags --always --dirty)
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_TIME ?= $(shell date -u '+%Y-%m-%d_%H:%M:%S')

# Go variables
GO_VERSION = 1.24.3
GOOS ?= $(shell go env GOOS)
GOARCH ?= $(shell go env GOARCH)

# Linker flags
LDFLAGS = -s -w \
	-X 'main.version=$(VERSION)' \
	-X 'main.commit=$(COMMIT)' \
	-X 'main.buildTime=$(BUILD_TIME)'

# Default target
all: clean deps lint test build

# Build the binary
build:
	@echo "Building $(BINARY_NAME) v$(VERSION) for $(GOOS)/$(GOARCH)..."
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) go build \
		-ldflags="$(LDFLAGS)" \
		-o $(BUILD_DIR)/$(BINARY_NAME) \
		$(MAIN_PATH)
	@echo "Build complete: $(BUILD_DIR)/$(BINARY_NAME)"

# Build for multiple platforms
build-all: clean deps
	@echo "Building for multiple platforms..."
	@mkdir -p $(BUILD_DIR)
	GOOS=linux GOARCH=amd64 $(MAKE) build
	@mv $(BUILD_DIR)/$(BINARY_NAME) $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64
	GOOS=linux GOARCH=arm64 $(MAKE) build
	@mv $(BUILD_DIR)/$(BINARY_NAME) $(BUILD_DIR)/$(BINARY_NAME)-linux-arm64
	GOOS=darwin GOARCH=amd64 $(MAKE) build
	@mv $(BUILD_DIR)/$(BINARY_NAME) $(BUILD_DIR)/$(BINARY_NAME)-darwin-amd64
	GOOS=darwin GOARCH=arm64 $(MAKE) build
	@mv $(BUILD_DIR)/$(BINARY_NAME) $(BUILD_DIR)/$(BINARY_NAME)-darwin-arm64
	GOOS=windows GOARCH=amd64 $(MAKE) build
	@mv $(BUILD_DIR)/$(BINARY_NAME) $(BUILD_DIR)/$(BINARY_NAME)-windows-amd64.exe
	@echo "Multi-platform build complete"

# Install dependencies
deps:
	@echo "Installing dependencies..."
	go mod download
	go mod tidy

# Run tests
test:
	@echo "Running tests..."
	go test -race -coverprofile=coverage.out ./...

# Run tests with verbose output
test-verbose:
	@echo "Running tests with verbose output..."
	go test -race -coverprofile=coverage.out -v ./...

# Run tests and show coverage
test-coverage: test
	@echo "Generating coverage report..."
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

# Run benchmarks
bench:
	@echo "Running benchmarks..."
	go test -bench=. -benchmem ./...

# Lint code
lint:
	@echo "Running linters..."
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run; \
	else \
		echo "golangci-lint not installed, running basic checks..."; \
		go vet ./...; \
		gofmt -d .; \
	fi

# Format code
fmt:
	@echo "Formatting code..."
	go fmt ./...
	@if command -v goimports >/dev/null 2>&1; then \
		goimports -w .; \
	fi

# Vet code
vet:
	@echo "Running go vet..."
	go vet ./...

# Run the agent with default config
run: build
	@echo "Running $(BINARY_NAME)..."
	./$(BUILD_DIR)/$(BINARY_NAME)

# Run the agent with debug logging
run-debug: build
	@echo "Running $(BINARY_NAME) with debug logging..."
	SC_LOG_LEVEL=debug ./$(BUILD_DIR)/$(BINARY_NAME)

# Run the agent with custom config
run-config: build
	@echo "Running $(BINARY_NAME) with config file..."
	SC_AGENT_CONFIG=config.yaml ./$(BUILD_DIR)/$(BINARY_NAME)

# Install the binary and scripts
install:
	@echo "Installing $(BINARY_NAME) and scripts to /usr/local/bin..."
	@sudo cp $(BUILD_DIR)/$(BINARY_NAME) /usr/local/bin/$(BINARY_NAME)
	@sudo chmod +x /usr/local/bin/$(BINARY_NAME)
	@echo "$(BINARY_NAME) and scripts installed successfully."

# Uninstall the binary and scripts
uninstall:
	@echo "Uninstalling $(BINARY_NAME) and scripts from /usr/local/bin..."
	@sudo rm -f /usr/local/bin/$(BINARY_NAME)
	@echo "$(BINARY_NAME) and scripts uninstalled successfully."

# Clean build artifacts
clean:
	@echo "Cleaning build artifacts..."
	rm -rf $(BUILD_DIR)
	rm -f coverage.out coverage.html
	go clean -testcache

# Generate mocks (if mockery is available)
mocks:
	@echo "Generating mocks..."
	@if command -v mockery >/dev/null 2>&1; then \
		mockery --all --output ./mocks; \
	else \
		echo "mockery not installed, skipping mock generation"; \
	fi

# Security scan
security:
	@echo "Running security scan..."
	@if command -v gosec >/dev/null 2>&1; then \
		gosec ./...; \
	else \
		echo "gosec not installed, skipping security scan"; \
	fi

# Check for vulnerabilities
vuln-check:
	@echo "Checking for vulnerabilities..."
	@if command -v govulncheck >/dev/null 2>&1; then \
		govulncheck ./...; \
	else \
		echo "govulncheck not installed, skipping vulnerability check"; \
	fi

# Create a release tarball
release: clean build-all
	@echo "Creating release archives..."
	@mkdir -p $(BUILD_DIR)/release
	@for binary in $(BUILD_DIR)/$(BINARY_NAME)-*; do \
		if [ -f "$$binary" ]; then \
			basename=$$(basename $$binary); \
			dirname=$${basename#$(BINARY_NAME)-}; \
			mkdir -p $(BUILD_DIR)/release/$$dirname; \
			cp $$binary $(BUILD_DIR)/release/$$dirname/$(BINARY_NAME); \
			cp README.md $(BUILD_DIR)/release/$$dirname/; \
			cp config.example.yaml $(BUILD_DIR)/release/$$dirname/; \
			cd $(BUILD_DIR)/release && tar -czf $$dirname.tar.gz $$dirname/; \
			cd ../..; \
		fi \
	done
	@echo "Release archives created in $(BUILD_DIR)/release/"

# Docker build
docker:
	@echo "Building Docker image..."
	docker build -t sc-metrics-agent:$(VERSION) .
	docker tag sc-metrics-agent:$(VERSION) sc-metrics-agent:latest

# Docker run
docker-run: docker
	@echo "Running Docker container..."
	docker run --rm -it \
		--pid=host \
		--privileged \
		-e SC_LOG_LEVEL=debug \
		-e SC_INGESTOR_ENDPOINT=http://host.docker.internal:8080/ingest \
		sc-metrics-agent:$(VERSION)

# Release tagging scripts
release-patch:
	@echo "Creating patch release..."
	@./packaging/scripts/release.sh patch

release-minor:
	@echo "Creating minor release..."
	@./packaging/scripts/release.sh minor

release-major:
	@echo "Creating major release..."
	@./packaging/scripts/release.sh major

# Development setup
dev-setup:
	@echo "Setting up development environment..."
	go install golang.org/x/tools/cmd/goimports@latest
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	go install github.com/securecodewarrior/gosec/v2/cmd/gosec@latest
	go install golang.org/x/vuln/cmd/govulncheck@latest
	go install github.com/vektra/mockery/v2@latest
	@echo "Development tools installed"

# Validate configuration file
validate-config:
	@echo "Validating configuration..."
	@if [ -f config.yaml ]; then \
		echo "Found config.yaml, validating..."; \
		SC_AGENT_CONFIG=config.yaml go run $(MAIN_PATH) -validate-config 2>/dev/null || echo "Config validation not implemented yet"; \
	else \
		echo "No config.yaml found, using defaults"; \
	fi

# Show build info
info:
	@echo "Build Information:"
	@echo "  Version: $(VERSION)"
	@echo "  Commit:  $(COMMIT)"
	@echo "  Time:    $(BUILD_TIME)"
	@echo "  GOOS:    $(GOOS)"
	@echo "  GOARCH:  $(GOARCH)"
	@echo "  Go:      $(shell go version)"

# Watch for changes and rebuild (requires entr)
watch:
	@echo "Watching for changes (requires 'entr' command)..."
	@if command -v entr >/dev/null 2>&1; then \
		find . -name '*.go' | entr -r make run; \
	else \
		echo "entr not installed. Install with: brew install entr (macOS) or apt-get install entr (Ubuntu)"; \
	fi

# Profile the application
profile: build
	@echo "Running with CPU profiling enabled..."
	@mkdir -p $(BUILD_DIR)/profiles
	SC_ENABLE_PPROF=true ./$(BUILD_DIR)/$(BINARY_NAME) &
	@echo "Agent started with profiling. Access profiles at http://localhost:6060/debug/pprof/"
	@echo "Press Ctrl+C to stop"

# Show help
help:
	@echo "SC Metrics Agent Build Commands:"
	@echo ""
	@echo "Building:"
	@echo "  make build      - Build binary for current platform"
	@echo "  make build-all  - Build binaries for all platforms"
	@echo "  make install    - Install binary to /usr/local/bin"
	@echo "  make uninstall  - Remove binary from /usr/local/bin"
	@echo ""
	@echo "Testing:"
	@echo "  make test           - Run tests"
	@echo "  make test-verbose   - Run tests with verbose output"
	@echo "  make test-coverage  - Run tests and generate coverage report"
	@echo "  make bench          - Run benchmarks"
	@echo ""
	@echo "Code Quality:"
	@echo "  make lint        - Run linters"
	@echo "  make fmt         - Format code"
	@echo "  make vet         - Run go vet"
	@echo "  make security    - Run security scan"
	@echo "  make vuln-check  - Check for vulnerabilities"
	@echo ""
	@echo "Running:"
	@echo "  make run         - Run agent with default config"
	@echo "  make run-debug   - Run agent with debug logging"
	@echo "  make run-config  - Run agent with config.yaml"
	@echo ""
	@echo "Docker:"
	@echo "  make docker      - Build Docker image"
	@echo "  make docker-run  - Build and run Docker container"
	@echo ""
	@echo "Development:"
	@echo "  make dev-setup   - Install development tools"
	@echo "  make watch       - Watch for changes and rebuild"
	@echo "  make profile     - Run with profiling enabled"
	@echo "  make mocks       - Generate mocks"
	@echo ""
	@echo "Utilities:"
	@echo "  make deps        - Install dependencies"
	@echo "  make clean       - Clean build artifacts"
	@echo "  make release     - Create release archives"
	@echo "  make info        - Show build information"
	@echo "  make validate-config - Validate configuration file"
	@echo "  make help        - Show this help message"