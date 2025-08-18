package pipeline

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"
	"github.com/strettch/sc-metrics-agent/pkg/aggregate"
	"github.com/strettch/sc-metrics-agent/pkg/clients/metadata"
	"github.com/strettch/sc-metrics-agent/pkg/clients/tsclient"
	"github.com/strettch/sc-metrics-agent/pkg/collector"
	"github.com/strettch/sc-metrics-agent/pkg/decorator"
)

// Processor implements the Collect -> Decorate -> Aggregate -> Write pipeline
type Processor struct {
	collector        collector.Collector
	decorator        decorator.MetricDecorator
	aggregator       aggregate.Aggregator
	writer           tsclient.MetricWriter
	authMgr          *metadata.AuthManager // External auth handling
	logger           *zap.Logger
	lastProcessTime  time.Time
	lastMetricCount  int
	lastError        string
}

// ProcessingStats holds statistics about pipeline processing
type ProcessingStats struct {
	CollectedFamilies int           `json:"collected_families"`
	DecoratedFamilies int           `json:"decorated_families"`
	AggregatedMetrics int           `json:"aggregated_metrics"`
	WrittenMetrics    int           `json:"written_metrics"`
	ProcessingTime    time.Duration `json:"processing_time"`
	Timestamp         int64         `json:"timestamp"`
}

// NewProcessor creates a new pipeline processor
func NewProcessor(
	collector collector.Collector,
	decorator decorator.MetricDecorator,
	aggregator aggregate.Aggregator,
	writer tsclient.MetricWriter,
	authMgr *metadata.AuthManager,
	logger *zap.Logger,
) *Processor {
	p := &Processor{
		collector:  collector,
		decorator:  decorator,
		aggregator: aggregator,
		writer:     writer,
		authMgr:    authMgr,
		logger:     logger,
	}
	
	// Initialize auth and start refresh loop
	ctx := context.Background()
	if err := authMgr.EnsureValidToken(ctx); err != nil {
		logger.Error("Failed to get initial auth token", zap.Error(err))
	}
	authMgr.StartRefresh(ctx)
	logger.Info("Auth refresh loop started")
	
	return p
}


// Process executes the complete pipeline: Collect -> Decorate -> Aggregate -> Write
func (p *Processor) Process(ctx context.Context) error {
	// Get the current auth token
	authToken := p.authMgr.GetCurrentToken()
	if authToken == "" {
		p.lastError = "auth token is empty"
		return fmt.Errorf("auth token is empty - refresh may have failed")
	}

	startTime := time.Now()
	p.logger.Debug("Starting metrics processing pipeline")

	// Check context before starting
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	// Step 1: Collect metrics
	p.logger.Debug("Step 1: Collecting metrics")
	metricFamilies, err := p.collector.Collect(ctx)
	if err != nil {
		p.lastError = fmt.Sprintf("collection failed: %v", err)
		return fmt.Errorf("failed to collect metrics: %w", err)
	}

	if len(metricFamilies) == 0 {
		p.logger.Info("No metrics collected, skipping pipeline")
		return nil
	}

	p.logger.Debug("Metrics collected successfully", zap.Int("metric_families", len(metricFamilies)))

	// Check context after collection
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	// Step 2: Decorate metrics
	p.logger.Debug("Step 2: Decorating metrics")
	decoratedFamilies, err := p.decorator.Decorate(metricFamilies)
	if err != nil {
		p.lastError = fmt.Sprintf("decoration failed: %v", err)
		return fmt.Errorf("failed to decorate metrics: %w", err)
	}

	p.logger.Debug("Metrics decorated successfully", zap.Int("decorated_families", len(decoratedFamilies)))

	// Check context after decoration
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	// Step 3: Aggregate metrics
	p.logger.Debug("Step 3: Aggregating metrics")
	aggregatedMetrics, err := p.aggregator.Aggregate(decoratedFamilies)
	if err != nil {
		p.lastError = fmt.Sprintf("aggregation failed: %v", err)
		return fmt.Errorf("failed to aggregate metrics: %w", err)
	}

	if len(aggregatedMetrics) == 0 {
		p.logger.Warn("No metrics after aggregation")
		return nil
	}

	p.logger.Debug("Metrics aggregated successfully", zap.Int("aggregated_metrics", len(aggregatedMetrics)))

	// Sort metrics for consistent ordering
	aggregate.SortMetrics(aggregatedMetrics)

	// Check context after aggregation
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	// Step 4: Write metrics
	p.logger.Debug("Step 4: Writing metrics")
	if err := p.writer.WriteMetrics(ctx, aggregatedMetrics, authToken); err != nil {
		p.lastError = fmt.Sprintf("write failed: %v", err)
		return fmt.Errorf("failed to write metrics: %w", err)
	}

	// Update processing statistics
	processingTime := time.Since(startTime)
	p.lastProcessTime = startTime
	p.lastMetricCount = len(aggregatedMetrics)
	p.lastError = "" // Clear error on successful processing

	p.logger.Info("Pipeline processing completed successfully",
		zap.Int("collected_families", len(metricFamilies)),
		zap.Int("decorated_families", len(decoratedFamilies)),
		zap.Int("aggregated_metrics", len(aggregatedMetrics)),
		zap.Duration("processing_time", processingTime))

	return nil
}

// WriteDiagnostics sends diagnostic information to the ingestor
// This is called when the main pipeline fails to provide agent health status
func (p *Processor) WriteDiagnostics(ctx context.Context) error {
	p.logger.Debug("Writing diagnostics to ingestor")
	
	// Get the current auth token
	authToken := p.authMgr.GetCurrentToken()

	// Determine agent status
	status := "healthy"
	if p.lastError != "" {
		status = "error"
	}

	// Get collector status if available
	collectorStatus := make(map[string]bool)
	if systemCollector, ok := p.collector.(*collector.SystemCollector); ok {
		collectorStatus = systemCollector.GetEnabledCollectors()
	}

	// Send diagnostics
	if err := p.writer.WriteDiagnostics(ctx, p.getAgentID(), status, p.lastError, collectorStatus, authToken); err != nil {
		p.logger.Error("Failed to write diagnostics", zap.Error(err))
		return fmt.Errorf("failed to write diagnostics: %w", err)
	}

	p.logger.Info("Diagnostics sent successfully", zap.String("status", status))
	return nil
}

// GetProcessingStats returns current processing statistics
func (p *Processor) GetProcessingStats() ProcessingStats {
	return ProcessingStats{
		WrittenMetrics: p.lastMetricCount,
		ProcessingTime: time.Since(p.lastProcessTime),
		Timestamp:      p.lastProcessTime.UnixMilli(),
	}
}

// GetLastError returns the last error encountered during processing
func (p *Processor) GetLastError() string {
	return p.lastError
}

// GetLastProcessTime returns the timestamp of the last successful processing
func (p *Processor) GetLastProcessTime() time.Time {
	return p.lastProcessTime
}

// GetLastMetricCount returns the number of metrics processed in the last run
func (p *Processor) GetLastMetricCount() int {
	return p.lastMetricCount
}

// Close performs cleanup for the processor
func (p *Processor) Close() error {
	p.logger.Debug("Closing pipeline processor")

	// Close auth manager
	p.authMgr.Close()

	// Close collector
	if closer, ok := p.collector.(interface{ Close() error }); ok {
		if err := closer.Close(); err != nil {
			p.logger.Warn("Failed to close collector", zap.Error(err))
		}
	}
	
	// Close writer
	if err := p.writer.Close(); err != nil {
		p.logger.Warn("Failed to close writer", zap.Error(err))
	}
	
	p.logger.Debug("Pipeline processor closed")
	return nil
}

// getAgentID generates a unique identifier for this agent instance
func (p *Processor) getAgentID() string {
	// In a real implementation, this might be configured or derived from system info
	// For now, we'll use a simple approach
	if systemCollector, ok := p.collector.(*collector.SystemCollector); ok {
		enabled := systemCollector.GetEnabledCollectors()
		if len(enabled) > 0 {
			return fmt.Sprintf("sc-agent-%d", time.Now().Unix())
		}
	}
	return fmt.Sprintf("sc-agent-%d", time.Now().Unix())
}

// ValidateConfiguration checks if the processor is properly configured
func (p *Processor) ValidateConfiguration() error {
	if p.collector == nil {
		return fmt.Errorf("collector is nil")
	}
	
	if p.decorator == nil {
		return fmt.Errorf("decorator is nil")
	}
	
	if p.aggregator == nil {
		return fmt.Errorf("aggregator is nil")
	}
	
	if p.writer == nil {
		return fmt.Errorf("writer is nil")
	}
	
	if p.logger == nil {
		return fmt.Errorf("logger is nil")
	}

	if p.authMgr == nil {
		return fmt.Errorf("auth manager is nil")
	}

	return nil
}

// ProcessWithTimeout executes the pipeline with a timeout
func (p *Processor) ProcessWithTimeout(ctx context.Context, timeout time.Duration) error {
	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	
	return p.Process(timeoutCtx)
}