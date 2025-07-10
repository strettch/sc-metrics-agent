package helpers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	"github.com/klauspost/compress/snappy"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// TimeseriesMetric represents the expected format from the agent
type TimeseriesMetric struct {
	Name      string            `json:"name"`
	Labels    map[string]string `json:"labels"`
	Value     float64           `json:"value"`
	Timestamp int64             `json:"timestamp"`
	Type      string            `json:"type"`
}

// TimeseriesMetrics represents an array of timeseries metrics
type TimeseriesMetrics []TimeseriesMetric

// MetricsProcessingResponse matches resource-manager response format
type MetricsProcessingResponse struct {
	Status    string   `json:"status"`
	Processed int      `json:"processed"`
	Failed    int      `json:"failed"`
	Errors    []string `json:"errors,omitempty"`
}

// ApiResponse matches resource-manager API response structure
type ApiResponse struct {
	Message string                    `json:"message"`
	Data    MetricsProcessingResponse `json:"data"`
	Errors  interface{}               `json:"errors"`
}

// MockIngestServer simulates the resource-manager /metrics/ingest endpoint
// with exact validation logic matching the real implementation
type MockIngestServer struct {
	server   *httptest.Server
	logger   *zap.Logger
	requests []IngestRequest
}

// IngestRequest captures details of received requests for testing
type IngestRequest struct {
	ContentType     string
	ContentEncoding string
	UserAgent       string
	Body            []byte
	DecompressedBody []byte
	ParsedMetrics   TimeseriesMetrics
	Timestamp       time.Time
}

// ResourceManagerSupportedMetrics is the exact whitelist from resource-manager
var ResourceManagerSupportedMetrics = map[string]bool{
	// CPU Metrics
	"node_cpu_seconds_total": true,

	// Memory Metrics
	"node_memory_MemTotal_bytes":     true,
	"node_memory_MemFree_bytes":      true,
	"node_memory_MemAvailable_bytes": true,
	"node_memory_Buffers_bytes":      true,
	"node_memory_Cached_bytes":       true,
	"node_memory_SwapTotal_bytes":    true,
	"node_memory_SwapFree_bytes":     true,

	// Load Average Metrics
	"node_load1":  true,
	"node_load5":  true,
	"node_load15": true,

	// Disk Stats Metrics
	"node_disk_reads_completed_total":  true,
	"node_disk_writes_completed_total": true,
	"node_disk_read_bytes_total":       true,
	"node_disk_written_bytes_total":    true,

	// Network Metrics
	"node_network_receive_bytes_total":    true,
	"node_network_transmit_bytes_total":   true,
	"node_network_receive_packets_total":  true,
	"node_network_transmit_packets_total": true,

	// Filesystem Metrics
	"node_filesystem_size_bytes": true,
}

// NewMockIngestServer creates a new mock server that behaves exactly like resource-manager
func NewMockIngestServer(logger *zap.Logger) *MockIngestServer {
	mock := &MockIngestServer{
		logger:   logger,
		requests: make([]IngestRequest, 0),
	}

	mock.server = httptest.NewServer(http.HandlerFunc(mock.handleIngest))
	return mock
}

// URL returns the mock server URL
func (m *MockIngestServer) URL() string {
	return m.server.URL + "/metrics/ingest"
}

// Close shuts down the mock server
func (m *MockIngestServer) Close() {
	m.server.Close()
}

// GetRequests returns all received requests for testing
func (m *MockIngestServer) GetRequests() []IngestRequest {
	return m.requests
}

// ClearRequests clears the request history
func (m *MockIngestServer) ClearRequests() {
	m.requests = make([]IngestRequest, 0)
}

// handleIngest implements the exact logic from resource-manager metrics handler
func (m *MockIngestServer) handleIngest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if !strings.HasSuffix(r.URL.Path, "/metrics/ingest") {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	contentType := r.Header.Get("Content-Type")
	contentEncoding := r.Header.Get("Content-Encoding")
	userAgent := r.Header.Get("User-Agent")

	m.logger.Info("Received metric ingestion request",
		zap.String("content-type", contentType),
		zap.String("content-encoding", contentEncoding),
		zap.String("user-agent", userAgent),
	)

	// Read request body
	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		m.logger.Error("Failed to read request body", zap.Error(err))
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}
	defer func() {
		if err := r.Body.Close(); err != nil {
			m.logger.Warn("Failed to close request body", zap.Error(err))
		}
	}()

	request := IngestRequest{
		ContentType:     contentType,
		ContentEncoding: contentEncoding,
		UserAgent:       userAgent,
		Body:            bodyBytes,
		Timestamp:       time.Now(),
	}

	// Handle Snappy compression (matching resource-manager logic)
	decompressedBody := bodyBytes
	if contentEncoding == "snappy" {
		decoded, err := snappy.Decode(nil, bodyBytes)
		if err != nil {
			m.logger.Error("Failed to decompress snappy request body", zap.Error(err))
			http.Error(w, "Failed to decompress request body", http.StatusBadRequest)
			return
		}
		decompressedBody = decoded
		m.logger.Debug("Successfully decompressed Snappy payload", 
			zap.Int("original_size", len(bodyBytes)),
			zap.Int("decompressed_size", len(decompressedBody)))
	}

	request.DecompressedBody = decompressedBody

	// Handle different content types (matching resource-manager switch statement)
	switch {
	case strings.Contains(contentType, "application/timeseries-binary-0"):
		response := m.handleTimeseriesMetrics(decompressedBody, &request)
		m.sendResponse(w, response)
	default:
		m.logger.Error("Unsupported content type", zap.String("content_type", contentType))
		http.Error(w, fmt.Sprintf("Unsupported content type: %s", contentType), http.StatusBadRequest)
		return
	}

	// Store request for testing
	m.requests = append(m.requests, request)
}

// handleTimeseriesMetrics processes timeseries metrics with exact resource-manager validation
func (m *MockIngestServer) handleTimeseriesMetrics(bodyBytes []byte, request *IngestRequest) ApiResponse {
	var timeseriesMetrics TimeseriesMetrics
	if err := json.Unmarshal(bodyBytes, &timeseriesMetrics); err != nil {
		m.logger.Error("Failed to parse timeseries metrics", zap.Error(err))
		return ApiResponse{
			Message: "Invalid timeseries payload",
			Data: MetricsProcessingResponse{
				Status:    "error",
				Processed: 0,
				Failed:    1,
				Errors:    []string{fmt.Sprintf("JSON parse error: %s", err.Error())},
			},
		}
	}

	request.ParsedMetrics = timeseriesMetrics

	processed := 0
	failed := 0
	var errors []string

	// Process each metric with exact resource-manager logic
	for i, metric := range timeseriesMetrics {
		// Validate vm_id label (matching resource-manager logic)
		vmIDStr, exists := metric.Labels["vm_id"]
		if !exists {
			failed++
			errors = append(errors, fmt.Sprintf("Metric %d: vm_id label is required", i))
			continue
		}

		// Validate UUID format
		if _, err := uuid.Parse(vmIDStr); err != nil {
			failed++
			errors = append(errors, fmt.Sprintf("Metric %d: vm_id must be a valid UUID", i))
			continue
		}

		// Validate timestamp format (should be Unix milliseconds)
		if metric.Timestamp <= 0 {
			failed++
			errors = append(errors, fmt.Sprintf("Metric %d: invalid timestamp %d", i, metric.Timestamp))
			continue
		}

		// Validate metric name against whitelist (exact resource-manager logic)
		if !ResourceManagerSupportedMetrics[metric.Name] {
			failed++
			m.logger.Warn("Unsupported metric type received",
				zap.Int("index", i),
				zap.String("metric_name", metric.Name))
			errors = append(errors, fmt.Sprintf("Unsupported metric: %s", metric.Name))
			continue
		}

		// If we reach here, metric is valid
		processed++
	}

	responseData := MetricsProcessingResponse{
		Status:    "metrics processed",
		Processed: processed,
		Failed:    failed,
	}

	if len(errors) > 0 {
		// Limit error list to 10 items (matching resource-manager)
		if len(errors) > 10 {
			responseData.Errors = errors[:10]
		} else {
			responseData.Errors = errors
		}
	}

	return ApiResponse{
		Message: "Metrics queued for processing",
		Data:    responseData,
		Errors:  nil,
	}
}

// sendResponse sends API response in resource-manager format
func (m *MockIngestServer) sendResponse(w http.ResponseWriter, response ApiResponse) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted) // 202 status like resource-manager

	if err := json.NewEncoder(w).Encode(response); err != nil {
		m.logger.Error("Failed to encode response", zap.Error(err))
	}
}

// ValidateRequest validates that a request matches expected format
func (m *MockIngestServer) ValidateRequest(request IngestRequest) []string {
	var issues []string

	// Validate headers
	if request.ContentType != "application/timeseries-binary-0" {
		issues = append(issues, fmt.Sprintf("Expected content-type 'application/timeseries-binary-0', got '%s'", request.ContentType))
	}

	if request.ContentEncoding != "snappy" {
		issues = append(issues, fmt.Sprintf("Expected content-encoding 'snappy', got '%s'", request.ContentEncoding))
	}

	if !strings.Contains(request.UserAgent, "sc-metrics-agent") {
		issues = append(issues, fmt.Sprintf("Expected user-agent to contain 'sc-metrics-agent', got '%s'", request.UserAgent))
	}

	// Validate metrics format
	if len(request.ParsedMetrics) == 0 {
		issues = append(issues, "No metrics found in request")
	}

	for i, metric := range request.ParsedMetrics {
		// Check required fields
		if metric.Name == "" {
			issues = append(issues, fmt.Sprintf("Metric %d: missing name", i))
		}
		if metric.Labels == nil {
			issues = append(issues, fmt.Sprintf("Metric %d: missing labels", i))
		}
		if metric.Timestamp <= 0 {
			issues = append(issues, fmt.Sprintf("Metric %d: invalid timestamp", i))
		}

		// Check vm_id label
		if vmID, exists := metric.Labels["vm_id"]; !exists {
			issues = append(issues, fmt.Sprintf("Metric %d: missing vm_id label", i))
		} else if _, err := uuid.Parse(vmID); err != nil {
			issues = append(issues, fmt.Sprintf("Metric %d: invalid vm_id format", i))
		}

		// Check metric name against whitelist
		if !ResourceManagerSupportedMetrics[metric.Name] {
			issues = append(issues, fmt.Sprintf("Metric %d: unsupported metric name '%s'", i, metric.Name))
		}
	}

	return issues
}