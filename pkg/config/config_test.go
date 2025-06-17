package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	
	assert.NotNil(t, cfg)
	assert.Equal(t, 30*time.Second, cfg.CollectionInterval)
	assert.Equal(t, "https://api.cloud.strettch.dev/resource-manager/api/v1/metrics/ingest", cfg.IngestorEndpoint)
	assert.Equal(t, 30*time.Second, cfg.HTTPTimeout)
	// VM ID might be empty on test systems without dmidecode
	if cfg.VMID == "" {
		cfg.VMID = "test-vm-id"
	}
	assert.NotEmpty(t, cfg.VMID)
	assert.NotNil(t, cfg.Labels)
	assert.True(t, cfg.Collectors.Processes)
	assert.Equal(t, "info", cfg.LogLevel)
	assert.Equal(t, 3, cfg.MaxRetries)
	assert.Equal(t, 5*time.Second, cfg.RetryInterval)
}

func TestLoad_DefaultsOnly(t *testing.T) {
	// Clear environment variables
	clearEnvVars()
	
	// Set test VM ID to avoid validation failure on test systems
	os.Setenv("SC_VM_ID", "test-vm-default")
	defer os.Unsetenv("SC_VM_ID")
	
	cfg, err := Load()
	require.NoError(t, err)
	
	// Should match defaults (except VM ID which we set)
	expected := DefaultConfig()
	assert.Equal(t, expected.CollectionInterval, cfg.CollectionInterval)
	assert.Equal(t, expected.IngestorEndpoint, cfg.IngestorEndpoint)
	assert.Equal(t, expected.HTTPTimeout, cfg.HTTPTimeout)
	assert.Equal(t, expected.LogLevel, cfg.LogLevel)
	assert.Equal(t, expected.MaxRetries, cfg.MaxRetries)
	assert.Equal(t, expected.RetryInterval, cfg.RetryInterval)
	assert.Equal(t, expected.Collectors.Processes, cfg.Collectors.Processes)
	assert.Equal(t, "test-vm-default", cfg.VMID)
}

func TestLoad_FromEnvironment(t *testing.T) {
	// Clear environment first
	clearEnvVars()
	
	// Set test environment variables
	testEnvVars := map[string]string{
		"SC_COLLECTION_INTERVAL": "60s",
		"SC_INGESTOR_ENDPOINT":   "https://test.example.com/ingest",
		"SC_HTTP_TIMEOUT":        "45s",
		"SC_VM_ID":               "test-vm-123",
		"SC_LOG_LEVEL":           "debug",
		"SC_MAX_RETRIES":         "5",
		"SC_RETRY_INTERVAL":      "10s",
		"SC_LABELS":              "env=test,region=us-west-2",
		"SC_COLLECTOR_PROCESSES": "false",
	}
	
	for key, value := range testEnvVars {
		os.Setenv(key, value)
	}
	defer clearEnvVars()
	
	cfg, err := Load()
	require.NoError(t, err)
	
	assert.Equal(t, 60*time.Second, cfg.CollectionInterval)
	assert.Equal(t, "https://test.example.com/ingest", cfg.IngestorEndpoint)
	assert.Equal(t, 45*time.Second, cfg.HTTPTimeout)
	assert.Equal(t, "test-vm-123", cfg.VMID)
	assert.Equal(t, "debug", cfg.LogLevel)
	assert.Equal(t, 5, cfg.MaxRetries)
	assert.Equal(t, 10*time.Second, cfg.RetryInterval)
	assert.Equal(t, "test", cfg.Labels["env"])
	assert.Equal(t, "us-west-2", cfg.Labels["region"])
	assert.False(t, cfg.Collectors.Processes)
}

func TestLoad_FromFile(t *testing.T) {
	// Create temporary config file
	configContent := `
collection_interval: 45s
ingestor_endpoint: "https://config.example.com/ingest"
http_timeout: 20s
vm_id: "config-vm-456"
labels:
  environment: "staging"
  team: "devops"
collectors:
  processes: true
log_level: "warn"
max_retries: 2
retry_interval: 3s
`
	
	tempFile := createTempConfigFile(t, configContent)
	defer os.Remove(tempFile)
	
	// Clear environment and set config file path
	clearEnvVars()
	os.Setenv("SC_AGENT_CONFIG", tempFile)
	defer os.Unsetenv("SC_AGENT_CONFIG")
	
	cfg, err := Load()
	require.NoError(t, err)
	
	assert.Equal(t, 45*time.Second, cfg.CollectionInterval)
	assert.Equal(t, "https://config.example.com/ingest", cfg.IngestorEndpoint)
	assert.Equal(t, 20*time.Second, cfg.HTTPTimeout)
	assert.Equal(t, "config-vm-456", cfg.VMID)
	assert.Equal(t, "warn", cfg.LogLevel)
	assert.Equal(t, 2, cfg.MaxRetries)
	assert.Equal(t, 3*time.Second, cfg.RetryInterval)
	assert.Equal(t, "staging", cfg.Labels["environment"])
	assert.Equal(t, "devops", cfg.Labels["team"])
	assert.True(t, cfg.Collectors.Processes)
}

func TestLoad_EnvironmentOverridesFile(t *testing.T) {
	// Create config file
	configContent := `
collection_interval: 45s
ingestor_endpoint: "https://config.example.com/ingest"
vm_id: "config-vm"
log_level: "warn"
`
	
	tempFile := createTempConfigFile(t, configContent)
	defer os.Remove(tempFile)
	
	// Clear environment and set both file and env vars
	clearEnvVars()
	os.Setenv("SC_AGENT_CONFIG", tempFile)
	os.Setenv("SC_COLLECTION_INTERVAL", "120s")
	os.Setenv("SC_VM_ID", "env-vm")
	defer func() {
		os.Unsetenv("SC_AGENT_CONFIG")
		os.Unsetenv("SC_COLLECTION_INTERVAL")
		os.Unsetenv("SC_VM_ID")
	}()
	
	cfg, err := Load()
	require.NoError(t, err)
	
	// Environment should override file
	assert.Equal(t, 120*time.Second, cfg.CollectionInterval) // From env
	assert.Equal(t, "env-vm", cfg.VMID)                      // From env
	assert.Equal(t, "https://config.example.com/ingest", cfg.IngestorEndpoint) // From file
	assert.Equal(t, "warn", cfg.LogLevel)                    // From file
}

func TestLoad_InvalidConfigFile(t *testing.T) {
	// Create invalid YAML file
	invalidContent := `
collection_interval: 45s
ingestor_endpoint: "https://example.com"
invalid_yaml: [unclosed bracket
`
	
	tempFile := createTempConfigFile(t, invalidContent)
	defer os.Remove(tempFile)
	
	clearEnvVars()
	os.Setenv("SC_AGENT_CONFIG", tempFile)
	defer os.Unsetenv("SC_AGENT_CONFIG")
	
	_, err := Load()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to load config file")
}

func TestLoad_NonexistentConfigFile(t *testing.T) {
	clearEnvVars()
	os.Setenv("SC_AGENT_CONFIG", "/nonexistent/config.yaml")
	defer os.Unsetenv("SC_AGENT_CONFIG")
	
	_, err := Load()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to load config file")
}

func TestParseLabels(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected map[string]string
	}{
		{
			name:  "simple labels",
			input: "key1=value1,key2=value2",
			expected: map[string]string{
				"key1": "value1",
				"key2": "value2",
			},
		},
		{
			name:  "labels with spaces",
			input: " key1 = value1 , key2 = value2 ",
			expected: map[string]string{
				"key1": "value1",
				"key2": "value2",
			},
		},
		{
			name:  "single label",
			input: "env=production",
			expected: map[string]string{
				"env": "production",
			},
		},
		{
			name:     "empty string",
			input:    "",
			expected: map[string]string{},
		},
		{
			name:  "label with equals in value",
			input: "query=SELECT * FROM table WHERE id=123",
			expected: map[string]string{
				"query": "SELECT * FROM table WHERE id=123",
			},
		},
		{
			name:     "malformed labels",
			input:    "key1,key2=value2,key3",
			expected: map[string]string{"key2": "value2"},
		},
		{
			name:     "empty keys",
			input:    "=value1,key2=value2",
			expected: map[string]string{"key2": "value2"},
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseLabels(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestValidate(t *testing.T) {
	// Valid config should pass
	validConfig := DefaultConfig()
	validConfig.VMID = "test-vm-validation" // Set test VM ID
	err := validConfig.validate()
	assert.NoError(t, err)
	
	// Test invalid collection interval
	invalidConfig := *validConfig
	invalidConfig.CollectionInterval = 0
	err = invalidConfig.validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "collection_interval must be positive")
	
	// Test invalid HTTP timeout
	invalidConfig = *validConfig
	invalidConfig.HTTPTimeout = 0
	err = invalidConfig.validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "http_timeout must be positive")
	
	// Test empty ingestor endpoint
	invalidConfig = *validConfig
	invalidConfig.IngestorEndpoint = ""
	err = invalidConfig.validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "ingestor_endpoint cannot be empty")
	
	// Test empty VM ID
	invalidConfig = *validConfig
	invalidConfig.VMID = ""
	err = invalidConfig.validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "vm_id cannot be determined")
	
	// Test negative max retries
	invalidConfig = *validConfig
	invalidConfig.MaxRetries = -1
	err = invalidConfig.validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "max_retries cannot be negative")
	
	// Test invalid retry interval
	invalidConfig = *validConfig
	invalidConfig.RetryInterval = 0
	err = invalidConfig.validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "retry_interval must be positive")
	
	// Test invalid log level
	invalidConfig = *validConfig
	invalidConfig.LogLevel = "invalid"
	err = invalidConfig.validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid log_level")
	
	// Test valid log levels
	validLogLevels := []string{"debug", "info", "warn", "error", "fatal", "panic"}
	for _, level := range validLogLevels {
		validConfig.LogLevel = level
		err = validConfig.validate()
		assert.NoError(t, err, "Log level %s should be valid", level)
		
		validConfig.LogLevel = strings.ToUpper(level)
		err = validConfig.validate()
		assert.NoError(t, err, "Uppercase log level should be valid due to case-insensitive validation")
	}
}

func TestString(t *testing.T) {
	cfg := DefaultConfig()
	cfg.VMID = "test-vm"
	cfg.LogLevel = "debug"
	
	str := cfg.String()
	
	assert.Contains(t, str, "Config{")
	assert.Contains(t, str, "CollectionInterval:")
	assert.Contains(t, str, "IngestorEndpoint:")
	assert.Contains(t, str, "VMID:test-vm")
	assert.Contains(t, str, "LogLevel:debug")
	assert.Contains(t, str, "Collectors:")
}

func TestLoadFromEnv_InvalidValues(t *testing.T) {
	clearEnvVars()
	
	// Set invalid environment variables
	os.Setenv("SC_COLLECTION_INTERVAL", "invalid")
	os.Setenv("SC_HTTP_TIMEOUT", "invalid")
	os.Setenv("SC_MAX_RETRIES", "invalid")
	os.Setenv("SC_RETRY_INTERVAL", "invalid")
	os.Setenv("SC_COLLECTOR_PROCESSES", "invalid")
	os.Setenv("SC_VM_ID", "test-vm-invalid") // Set test VM ID
	defer clearEnvVars()
	
	cfg, err := Load()
	require.NoError(t, err)
	
	// Should use default values for invalid env vars
	defaults := DefaultConfig()
	assert.Equal(t, defaults.CollectionInterval, cfg.CollectionInterval)
	assert.Equal(t, defaults.HTTPTimeout, cfg.HTTPTimeout)
	assert.Equal(t, defaults.MaxRetries, cfg.MaxRetries)
	assert.Equal(t, defaults.RetryInterval, cfg.RetryInterval)
	assert.Equal(t, defaults.Collectors.Processes, cfg.Collectors.Processes)
	assert.Equal(t, "test-vm-invalid", cfg.VMID)
}

func TestCollectorConfig(t *testing.T) {
	cfg := DefaultConfig()
	cfg.VMID = "test-vm-collector" // Set test VM ID
	
	// Test default collector config
	assert.True(t, cfg.Collectors.Processes)
	
	// Test collector config from environment
	clearEnvVars()
	os.Setenv("SC_COLLECTOR_PROCESSES", "false")
	os.Setenv("SC_VM_ID", "test-vm-collector-env") // Set test VM ID
	defer clearEnvVars()
	
	cfg, err := Load()
	require.NoError(t, err)
	assert.False(t, cfg.Collectors.Processes)
	assert.Equal(t, "test-vm-collector-env", cfg.VMID)
}

// Helper functions

func clearEnvVars() {
	envVars := []string{
		"SC_AGENT_CONFIG",
		"SC_COLLECTION_INTERVAL",
		"SC_INGESTOR_ENDPOINT",
		"SC_HTTP_TIMEOUT",
		"SC_VM_ID",
		"SC_LOG_LEVEL",
		"SC_MAX_RETRIES",
		"SC_RETRY_INTERVAL",
		"SC_LABELS",
		"SC_COLLECTOR_PROCESSES",
	}
	
	for _, envVar := range envVars {
		os.Unsetenv(envVar)
	}
}

func createTempConfigFile(t *testing.T, content string) string {
	tempDir := t.TempDir()
	tempFile := filepath.Join(tempDir, "config.yaml")
	
	err := os.WriteFile(tempFile, []byte(content), 0644)
	require.NoError(t, err)
	
	return tempFile
}