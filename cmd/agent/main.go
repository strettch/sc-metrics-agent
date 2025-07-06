package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/strettch/sc-metrics-agent/pkg/aggregate"
	"github.com/strettch/sc-metrics-agent/pkg/clients/tsclient"
	"github.com/strettch/sc-metrics-agent/pkg/collector"
	"github.com/strettch/sc-metrics-agent/pkg/config"
	"github.com/strettch/sc-metrics-agent/pkg/decorator"
	"github.com/strettch/sc-metrics-agent/pkg/pipeline"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// These variables are set via linker flags during the build process (see Makefile)
var (
	version   = "dev" // Default value if not set by LDFLAGS
	commit    = "unknown"
	buildTime = "unknown"
)

func main() {
	versionFlag := flag.Bool("v", false, "Print version and exit")
	flag.Parse()

	if *versionFlag {
		fmt.Printf("Version: %s\nCommit: %s\nBuildTime: %s\n", version, commit, buildTime)
		os.Exit(0)
	}
	// Initialize logger
	logger := initLogger("info")
	defer logger.Sync()

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		if strings.Contains(err.Error(), "vm_id cannot be determined") {
			logger.Fatal("Failed to load configuration - VM ID detection failed",
				zap.Error(err),
				zap.String("help", "Ensure dmidecode is installed and accessible, or set vm_id manually in config.yaml or SC_VM_ID environment variable"),
				zap.String("dmidecode_check", "Run 'dmidecode -s system-uuid' to test VM ID detection"),
			)
		} else {
			logger.Fatal("Failed to load configuration", zap.Error(err))
		}
	}

	// Update log level from config
	if cfg.LogLevel != "info" {
		logger = initLogger(cfg.LogLevel)
		defer logger.Sync()
	}

	logger.Info("Starting SC metrics agent",
		zap.Duration("collection_interval", cfg.CollectionInterval),
		zap.String("ingestor_endpoint", cfg.IngestorEndpoint),
		zap.String("vm_id", cfg.VMID),
		zap.Any("collectors", cfg.Collectors),
	)

	// Create context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Set up signal handling
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Initialize components
	systemCollector, err := collector.NewSystemCollector(cfg.Collectors, logger)
	if err != nil {
		logger.Fatal("Failed to create system collector", zap.Error(err))
	}

	metricDecorator := decorator.NewMetricDecorator(cfg.VMID, cfg.Labels, logger)
	aggregator := aggregate.NewAggregator(logger)
	
	// Create HTTP client for metric writing
	clientConfig := tsclient.ClientConfig{
		Endpoint:   cfg.IngestorEndpoint,
		Timeout:    cfg.HTTPTimeout,
		MaxRetries: cfg.MaxRetries,
		RetryDelay: cfg.RetryInterval,
	}
	httpClient := tsclient.NewClientWithConfig(clientConfig, logger)
	metricWriter := tsclient.NewMetricWriter(httpClient, logger)

	// Create processing pipeline
	pipelineProcessor := pipeline.NewProcessor(
		systemCollector,
		metricDecorator,
		aggregator,
		metricWriter,
		logger,
	)

	// Start collection loop
	ticker := time.NewTicker(cfg.CollectionInterval)
	defer ticker.Stop()

	logger.Info("Agent started successfully")

	// Simple main execution loop
	for {
		select {
		case <-ticker.C:
			if err := pipelineProcessor.Process(ctx); err != nil {
				logger.Error("Failed to process metrics pipeline", zap.Error(err))
			}

		case sig := <-sigChan:
			logger.Info("Received shutdown signal, cleaning up", zap.String("signal", sig.String()))
			cancel()
			
			// Clean up resources
			if err := pipelineProcessor.Close(); err != nil {
				logger.Error("Error during cleanup", zap.Error(err))
			}
			
			logger.Info("Agent shutdown complete")
			return

		case <-ctx.Done():
			logger.Info("Context cancelled, shutting down")
			
			// Clean up resources
			if err := pipelineProcessor.Close(); err != nil {
				logger.Error("Error during cleanup", zap.Error(err))
			}
			return
		}
	}
}

func initLogger(logLevel string) *zap.Logger {
	// Parse log level
	level := zapcore.InfoLevel
	switch logLevel {
	case "debug":
		level = zapcore.DebugLevel
	case "info":
		level = zapcore.InfoLevel
	case "warn":
		level = zapcore.WarnLevel
	case "error":
		level = zapcore.ErrorLevel
	case "fatal":
		level = zapcore.FatalLevel
	case "panic":
		level = zapcore.PanicLevel
	}

	// Create encoder config for JSON output
	encoderConfig := zapcore.EncoderConfig{
		TimeKey:        "time",
		LevelKey:       "level",
		NameKey:        "logger",
		CallerKey:      "caller",
		FunctionKey:    zapcore.OmitKey,
		MessageKey:     "msg",
		StacktraceKey:  "stacktrace",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    zapcore.LowercaseLevelEncoder,
		EncodeTime:     zapcore.ISO8601TimeEncoder,
		EncodeDuration: zapcore.SecondsDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
	}

	// Create core
	core := zapcore.NewCore(
		zapcore.NewJSONEncoder(encoderConfig),
		zapcore.AddSync(os.Stdout),
		level,
	)

	// Create logger
	logger := zap.New(core, zap.AddCaller())
	
	return logger
}

func init() {
	// Ensure we can capture process metrics
	if os.Getuid() != 0 {
		fmt.Fprintf(os.Stderr, "Warning: Running as non-root user may limit system metric collection\n")
	}
}