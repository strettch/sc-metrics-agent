package tsclient

import (
	"context"
	"fmt"
	"time"

	"github.com/strettch/sc-metrics-agent/pkg/aggregate"
	"go.uber.org/zap"
)

// MetricWriter defines the interface for writing metrics to an ingestor
type MetricWriter interface {
	WriteMetrics(ctx context.Context, metrics []aggregate.MetricWithValue, authToken string) error
	WriteDiagnostics(ctx context.Context, agentID string, status string, lastError string, collectorStatus map[string]bool, authToken string) error
	SendHeartbeat(ctx context.Context, authToken string, version string) error
	Close() error
}

// metricWriter implements the MetricWriter interface
type metricWriter struct {
	client *Client
	logger *zap.Logger
}

// NewMetricWriter creates a new metric writer
func NewMetricWriter(client *Client, logger *zap.Logger) MetricWriter {
	return &metricWriter{
		client: client,
		logger: logger,
	}
}

// WriteMetrics sends metrics to the ingestor
func (mw *metricWriter) WriteMetrics(ctx context.Context, metrics []aggregate.MetricWithValue, authToken string) error {
	if len(metrics) == 0 {
		mw.logger.Debug("No metrics to write")
		return nil
	}

	mw.logger.Debug("Writing metrics to ingestor", zap.Int("metric_count", len(metrics)))

	response, err := mw.client.SendMetrics(ctx, metrics, authToken)
	if err != nil {
		mw.logger.Error("Failed to send metrics", zap.Error(err))
		return fmt.Errorf("failed to send metrics: %w", err)
	}

	// Check response status
	if response.StatusCode >= 200 && response.StatusCode < 300 {
		mw.logger.Info("Successfully sent metrics",
			zap.Int("status_code", response.StatusCode),
			zap.Int("metric_count", len(metrics)))
		return nil
	}

	// Handle non-success status codes
	errorMsg := fmt.Sprintf("ingestor returned status %d", response.StatusCode)
	if len(response.Body) > 0 {
		errorMsg += fmt.Sprintf(": %s", string(response.Body))
	}

	mw.logger.Error("Ingestor returned error status",
		zap.Int("status_code", response.StatusCode),
		zap.String("response_body", string(response.Body)))

	return fmt.Errorf("failed to write metrics: %s", errorMsg)
}

// WriteDiagnostics sends diagnostic information to the ingestor
func (mw *metricWriter) WriteDiagnostics(ctx context.Context, agentID string, status string, lastError string, collectorStatus map[string]bool, authToken string) error {
	mw.logger.Debug("Writing diagnostics to ingestor", zap.String("agent_id", agentID))

	diagnostics := DiagnosticPayload{
		AgentID:         agentID,
		Timestamp:       time.Now().UnixMilli(),
		Status:          status,
		LastError:       lastError,
		MetricsCount:    0, // Will be set by caller if needed
		CollectorStatus: collectorStatus,
		Metadata: map[string]interface{}{
			"version": "1.0",
			"go_version": "1.24.3",
		},
	}

	response, err := mw.client.SendDiagnostics(ctx, diagnostics, authToken)
	if err != nil {
		mw.logger.Error("Failed to send diagnostics", zap.Error(err))
		return fmt.Errorf("failed to send diagnostics: %w", err)
	}

	// Check response status
	if response.StatusCode >= 200 && response.StatusCode < 300 {
		mw.logger.Info("Successfully sent diagnostics",
			zap.Int("status_code", response.StatusCode),
			zap.String("agent_id", agentID))
		return nil
	}

	// Handle non-success status codes
	errorMsg := fmt.Sprintf("ingestor returned status %d", response.StatusCode)
	if len(response.Body) > 0 {
		errorMsg += fmt.Sprintf(": %s", string(response.Body))
	}

	mw.logger.Warn("Ingestor returned error status for diagnostics",
		zap.Int("status_code", response.StatusCode),
		zap.String("response_body", string(response.Body)))

	return fmt.Errorf("failed to write diagnostics: %s", errorMsg)
}

// SendHeartbeat sends a heartbeat to the resource manager
func (mw *metricWriter) SendHeartbeat(ctx context.Context, authToken string, version string) error {
	mw.logger.Debug("Sending heartbeat")

	response, err := mw.client.SendHeartbeat(ctx, authToken, version)
	if err != nil {
		mw.logger.Error("Failed to send heartbeat", zap.Error(err))
		return fmt.Errorf("failed to send heartbeat: %w", err)
	}

	if response.StatusCode >= 200 && response.StatusCode < 300 {
		mw.logger.Info("Successfully sent heartbeat", zap.Int("status_code", response.StatusCode))
		return nil
	}

	errorMsg := fmt.Sprintf("heartbeat returned status %d", response.StatusCode)
	if len(response.Body) > 0 {
		errorMsg += fmt.Sprintf(": %s", string(response.Body))
	}
	mw.logger.Error("Heartbeat failed", zap.String("error", errorMsg))
	return fmt.Errorf(errorMsg)
}

// Close closes the metric writer and its underlying client
func (mw *metricWriter) Close() error {
	mw.logger.Debug("Closing metric writer")
	return mw.client.Close()
}

// BatchedMetricWriter wraps a MetricWriter to provide batching functionality
type BatchedMetricWriter struct {
	writer    MetricWriter
	batchSize int
	logger    *zap.Logger
}

// NewBatchedMetricWriter creates a metric writer that batches metrics
func NewBatchedMetricWriter(writer MetricWriter, batchSize int, logger *zap.Logger) *BatchedMetricWriter {
	if batchSize <= 0 {
		batchSize = 1000 // Default batch size
	}
	
	return &BatchedMetricWriter{
		writer:    writer,
		batchSize: batchSize,
		logger:    logger,
	}
}

// WriteMetrics writes metrics in batches
func (bmw *BatchedMetricWriter) WriteMetrics(ctx context.Context, metrics []aggregate.MetricWithValue, authToken string) error {
	if len(metrics) == 0 {
		return nil
	}

	batches := aggregate.BatchMetrics(metrics, bmw.batchSize)
	bmw.logger.Debug("Writing metrics in batches",
		zap.Int("total_metrics", len(metrics)),
		zap.Int("batch_count", len(batches)),
		zap.Int("batch_size", bmw.batchSize))

	for i, batch := range batches {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		bmw.logger.Debug("Writing batch",
			zap.Int("batch", i+1),
			zap.Int("total_batches", len(batches)),
			zap.Int("batch_metrics", len(batch)))

		if err := bmw.writer.WriteMetrics(ctx, batch, authToken); err != nil {
			return fmt.Errorf("failed to write batch %d/%d: %w", i+1, len(batches), err)
		}
	}

	bmw.logger.Info("Successfully wrote all metric batches", zap.Int("total_metrics", len(metrics)))
	return nil
}

// WriteDiagnostics delegates to the underlying writer
func (bmw *BatchedMetricWriter) WriteDiagnostics(ctx context.Context, agentID string, status string, lastError string, collectorStatus map[string]bool, authToken string) error {
	return bmw.writer.WriteDiagnostics(ctx, agentID, status, lastError, collectorStatus, authToken)
}

// SendHeartbeat delegates to the underlying writer
func (bmw *BatchedMetricWriter) SendHeartbeat(ctx context.Context, authToken string, version string) error {
	return bmw.writer.SendHeartbeat(ctx, authToken, version)
}

// Close closes the underlying writer
func (bmw *BatchedMetricWriter) Close() error {
	return bmw.writer.Close()
}