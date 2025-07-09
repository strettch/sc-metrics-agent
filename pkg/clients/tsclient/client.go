package tsclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/klauspost/compress/snappy"
	"go.uber.org/zap"
	"github.com/strettch/sc-metrics-agent/pkg/aggregate"
)

const (
	// ContentType for timeseries binary data
	ContentTypeTimeseriesBinary = "application/timeseries-binary-0"
	
	// Headers
	HeaderContentType     = "Content-Type"
	HeaderContentEncoding = "Content-Encoding"
	HeaderUserAgent       = "User-Agent"
	HeaderRetryAfter      = "Retry-After"
	
	// Values
	ContentEncodingSnappy = "snappy"
	UserAgentValue        = "sc-metrics-agent/1.0"
	
	// Defaults
	DefaultTimeout    = 30 * time.Second
	DefaultMaxRetries = 3
	DefaultRetryDelay = 5 * time.Second
)

// Client handles HTTP communication with the timeseries ingestor
type Client struct {
	endpoint   string
	httpClient *http.Client
	logger     *zap.Logger
	maxRetries int
	retryDelay time.Duration
}

// ClientConfig holds client configuration
type ClientConfig struct {
	Endpoint   string
	Timeout    time.Duration
	MaxRetries int
	RetryDelay time.Duration
}

// Response represents the server response
type Response struct {
	StatusCode   int
	Body         []byte
	Headers      http.Header
	RetryAfter   time.Duration
	Error        error
}

// DiagnosticPayload represents agent health information
type DiagnosticPayload struct {
	AgentID         string                 `json:"agent_id"`
	Timestamp       int64                  `json:"timestamp"`
	Status          string                 `json:"status"`
	LastError       string                 `json:"last_error,omitempty"`
	MetricsCount    int                    `json:"metrics_count"`
	CollectorStatus map[string]bool        `json:"collector_status"`
	Metadata        map[string]interface{} `json:"metadata,omitempty"`
}

// NewClient creates a new HTTP client for timeseries data
func NewClient(endpoint string, timeout time.Duration, logger *zap.Logger) *Client {
	return &Client{
		endpoint: endpoint,
		httpClient: &http.Client{
			Timeout: timeout,
			Transport: &http.Transport{
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 10,
				IdleConnTimeout:     90 * time.Second,
			},
		},
		logger:     logger,
		maxRetries: DefaultMaxRetries,
		retryDelay: DefaultRetryDelay,
	}
}

// NewClientWithConfig creates a new client with custom configuration
func NewClientWithConfig(config ClientConfig, logger *zap.Logger) *Client {
	timeout := config.Timeout
	if timeout == 0 {
		timeout = DefaultTimeout
	}
	
	maxRetries := config.MaxRetries
	if maxRetries <= 0 {
		maxRetries = DefaultMaxRetries
	}
	
	retryDelay := config.RetryDelay
	if retryDelay == 0 {
		retryDelay = DefaultRetryDelay
	}

	return &Client{
		endpoint: config.Endpoint,
		httpClient: &http.Client{
			Timeout: timeout,
			Transport: &http.Transport{
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 10,
				IdleConnTimeout:     90 * time.Second,
			},
		},
		logger:     logger,
		maxRetries: maxRetries,
		retryDelay: retryDelay,
	}
}

// SendMetrics sends a batch of metrics to the ingestor
func (c *Client) SendMetrics(ctx context.Context, metrics []aggregate.MetricWithValue) (*Response, error) {
	if len(metrics) == 0 {
		return nil, fmt.Errorf("no metrics to send")
	}

	c.logger.Debug("Preparing to send metrics", zap.Int("metrics_count", len(metrics)))

	// Serialize metrics to JSON
	payload, err := json.Marshal(metrics)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal metrics: %w", err)
	}

	// Log the payload before compression (without sensitive data)
	c.logger.Debug("Sending metrics payload (before compression)",
		zap.Int("metrics_count", len(metrics)),
		zap.Int("payload_size_bytes", len(payload)),
	)

	// Compress with Snappy
	compressed := snappy.Encode(nil, payload)
	
	c.logger.Debug("Compressed payload",
		zap.Int("original_size", len(payload)),
		zap.Int("compressed_size", len(compressed)),
		zap.Float64("compression_ratio", float64(len(compressed))/float64(len(payload))),
	)

	return c.sendWithRetry(ctx, compressed, ContentTypeTimeseriesBinary)
}

// SendDiagnostics sends diagnostic information to the ingestor
func (c *Client) SendDiagnostics(ctx context.Context, diagnostics DiagnosticPayload) (*Response, error) {
	c.logger.Debug("Sending diagnostics", zap.String("agent_id", diagnostics.AgentID))

	// Serialize diagnostics to JSON
	payload, err := json.Marshal(diagnostics)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal diagnostics: %w", err)
	}

	// Log the diagnostics payload (without sensitive data)
	c.logger.Debug("Sending diagnostics payload",
		zap.String("agent_id", diagnostics.AgentID),
		zap.String("status", diagnostics.Status),
		zap.Int("payload_size_bytes", len(payload)),
	)

	// Compress with Snappy
	compressed := snappy.Encode(nil, payload)

	return c.sendWithRetry(ctx, compressed, "application/diagnostics-binary-0")
}

// sendWithRetry handles the HTTP request with retry logic
func (c *Client) sendWithRetry(ctx context.Context, data []byte, contentType string) (*Response, error) {
	var lastResponse *Response
	var lastErr error

	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		if attempt > 0 {
			// Wait before retry
			waitTime := c.retryDelay
			if lastResponse != nil && lastResponse.RetryAfter > 0 {
				waitTime = lastResponse.RetryAfter
			}

			c.logger.Info("Retrying request",
				zap.Int("attempt", attempt),
				zap.Duration("wait_time", waitTime),
			)

			select {
			case <-time.After(waitTime):
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}

		response, err := c.sendRequest(ctx, data, contentType)
		if err != nil {
			lastErr = err
			c.logger.Warn("Request failed", zap.Error(err), zap.Int("attempt", attempt))
			continue
		}

		lastResponse = response

		// Check if we should retry based on status code
		if c.shouldRetry(response.StatusCode) {
			c.logger.Warn("Request failed with retryable status",
				zap.Int("status_code", response.StatusCode),
				zap.Int("attempt", attempt),
			)
			continue
		}

		// Success or non-retryable error
		return response, nil
	}

	// All retries exhausted
	if lastResponse != nil {
		return lastResponse, fmt.Errorf("request failed after %d attempts, last status: %d", c.maxRetries+1, lastResponse.StatusCode)
	}
	return nil, fmt.Errorf("request failed after %d attempts: %w", c.maxRetries+1, lastErr)
}

// sendRequest sends a single HTTP request
func (c *Client) sendRequest(ctx context.Context, data []byte, contentType string) (*Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set(HeaderContentType, contentType)
	req.Header.Set(HeaderContentEncoding, ContentEncodingSnappy)
	req.Header.Set(HeaderUserAgent, UserAgentValue)

	c.logger.Debug("Sending HTTP request",
		zap.String("endpoint", c.endpoint),
		zap.String("content_type", contentType),
		zap.Int("payload_size", len(data)),
	)

	// Send request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// Parse retry-after header if present
	retryAfter := parseRetryAfter(resp.Header.Get(HeaderRetryAfter))

	response := &Response{
		StatusCode: resp.StatusCode,
		Body:       body,
		Headers:    resp.Header,
		RetryAfter: retryAfter,
	}

	c.logger.Debug("Received HTTP response",
		zap.Int("status_code", resp.StatusCode),
		zap.Int("response_size", len(body)),
		zap.Duration("retry_after", retryAfter),
	)

	return response, nil
}

// shouldRetry determines if a request should be retried based on status code
func (c *Client) shouldRetry(statusCode int) bool {
	switch statusCode {
	case http.StatusTooManyRequests, // 429
		http.StatusInternalServerError,     // 500
		http.StatusBadGateway,              // 502
		http.StatusServiceUnavailable,      // 503
		http.StatusGatewayTimeout:          // 504
		return true
	default:
		return false
	}
}

// parseRetryAfter parses the Retry-After header value
func parseRetryAfter(value string) time.Duration {
	if value == "" {
		return 0
	}

	// Try parsing as seconds (integer)
	if seconds, err := strconv.Atoi(value); err == nil && seconds > 0 {
		return time.Duration(seconds) * time.Second
	}

	// Try parsing as HTTP date (not commonly used for this header)
	if t, err := http.ParseTime(value); err == nil {
		duration := time.Until(t)
		if duration > 0 {
			return duration
		}
	}

	return 0
}

// Close closes the HTTP client
func (c *Client) Close() error {
	c.httpClient.CloseIdleConnections()
	return nil
}

// SetMaxRetries updates the maximum retry count
func (c *Client) SetMaxRetries(maxRetries int) {
	if maxRetries >= 0 {
		c.maxRetries = maxRetries
	}
}

// SetRetryDelay updates the retry delay
func (c *Client) SetRetryDelay(delay time.Duration) {
	if delay > 0 {
		c.retryDelay = delay
	}
}

// GetStats returns client statistics
func (c *Client) GetStats() map[string]interface{} {
	return map[string]interface{}{
		"endpoint":    c.endpoint,
		"max_retries": c.maxRetries,
		"retry_delay": c.retryDelay.String(),
		"timeout":     c.httpClient.Timeout.String(),
	}
}

// Helper function for min calculation
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}