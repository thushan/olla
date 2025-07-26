package profile

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/thushan/olla/internal/core/domain"
)

func TestConfigurableProfile_ContextPatterns(t *testing.T) {
	config := &domain.ProfileConfig{
		Name:    "test-profile",
		Version: "1.0",
	}
	config.Models.ContextPatterns = []domain.ContextPattern{
		{Pattern: "*-32k*", Context: 32768},
		{Pattern: "*-16k*", Context: 16384},
		{Pattern: "*-8k*", Context: 8192},
		{Pattern: "*:32k*", Context: 32768},
		{Pattern: "*:16k*", Context: 16384},
		{Pattern: "*:8k*", Context: 8192},
		{Pattern: "llama3*", Context: 8192},
		{Pattern: "gpt-4-turbo*", Context: 128000},
	}
	config.Request.ParsingRules.SupportsStreaming = true

	profile := NewConfigurableProfile(config)

	tests := []struct {
		name            string
		modelName       string
		expectedContext int64
	}{
		{"32k model", "mistral-32k", 32768},
		{"16k model", "claude-16k", 16384},
		{"8k model", "gemma-8k", 8192},
		{"llama3 model", "llama3-70b", 8192},
		{"gpt-4-turbo", "gpt-4-turbo-preview", 128000},
		{"no match defaults to 4096", "some-random-model", 4096},
		{"colon notation 32k", "qwen:32k", 32768},
		{"colon notation 16k", "phi:16k", 16384},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			caps := profile.GetModelCapabilities(tt.modelName, nil)
			assert.Equal(t, tt.expectedContext, caps.MaxContextLength)
		})
	}
}

func TestConfigurableProfile_ConcurrencyLimits(t *testing.T) {
	config := &domain.ProfileConfig{
		Name:    "test-profile",
		Version: "1.0",
	}
	config.Characteristics.MaxConcurrentRequests = 10
	config.Resources.ConcurrencyLimits = []domain.ConcurrencyLimitPattern{
		{MinMemoryGB: 30, MaxConcurrent: 1},
		{MinMemoryGB: 15, MaxConcurrent: 2},
		{MinMemoryGB: 8, MaxConcurrent: 4},
		{MinMemoryGB: 0, MaxConcurrent: 8},
	}
	config.Resources.ModelSizes = []domain.ModelSizePattern{
		{Patterns: []string{"70b"}, MinMemoryGB: 40},
		{Patterns: []string{"30b"}, MinMemoryGB: 20},
		{Patterns: []string{"13b"}, MinMemoryGB: 10},
		{Patterns: []string{"7b"}, MinMemoryGB: 6},
		{Patterns: []string{"3b"}, MinMemoryGB: 3},
	}
	config.Resources.Defaults = domain.ResourceRequirements{
		MinMemoryGB: 4,
	}

	profile := NewConfigurableProfile(config)

	tests := []struct {
		name               string
		modelName          string
		expectedConcurrent int
	}{
		{"70B model gets 1 concurrent", "llama3-70b", 1},
		{"30B model gets 2 concurrent", "falcon-30b", 2},
		{"13B model gets 4 concurrent", "vicuna-13b", 4},
		{"7B model gets 8 concurrent", "mistral-7b", 8},
		{"3B model gets 8 concurrent", "phi-3b", 8},
		{"unknown model uses default", "some-model", 8},
		{"no concurrency limits configured", "any-model", 10},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.name == "no concurrency limits configured" {
				// Test without concurrency limits
				configNoConcurrency := &domain.ProfileConfig{
					Name:    "test-profile",
					Version: "1.0",
				}
				configNoConcurrency.Characteristics.MaxConcurrentRequests = 10
				profileNoConcurrency := NewConfigurableProfile(configNoConcurrency)
				concurrent := profileNoConcurrency.GetOptimalConcurrency(tt.modelName)
				assert.Equal(t, tt.expectedConcurrent, concurrent)
			} else {
				concurrent := profile.GetOptimalConcurrency(tt.modelName)
				assert.Equal(t, tt.expectedConcurrent, concurrent)
			}
		})
	}
}

func TestConfigurableProfile_TimeoutScaling(t *testing.T) {
	config := &domain.ProfileConfig{
		Name:    "test-profile",
		Version: "1.0",
	}
	config.Characteristics.Timeout = 2 * time.Minute
	config.Resources.TimeoutScaling = domain.TimeoutScaling{
		BaseTimeoutSeconds: 30,
		LoadTimeBuffer:     true,
	}
	config.Resources.ModelSizes = []domain.ModelSizePattern{
		{Patterns: []string{"70b"}, EstimatedLoadTimeMS: 300000}, // 5 minutes
		{Patterns: []string{"13b"}, EstimatedLoadTimeMS: 60000},  // 1 minute
		{Patterns: []string{"7b"}, EstimatedLoadTimeMS: 30000},   // 30 seconds
	}
	config.Resources.Defaults = domain.ResourceRequirements{
		EstimatedLoadTimeMS: 5000, // 5 seconds
	}

	profile := NewConfigurableProfile(config)

	tests := []struct {
		name            string
		modelName       string
		expectedTimeout time.Duration
	}{
		{"70B model with load buffer", "llama3-70b", 30*time.Second + 5*time.Minute},
		{"13B model with load buffer", "vicuna-13b", 30*time.Second + 1*time.Minute},
		{"7B model with load buffer", "mistral-7b", 30*time.Second + 30*time.Second},
		{"unknown model uses default", "some-model", 30*time.Second + 5*time.Second},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			timeout := profile.GetRequestTimeout(tt.modelName)
			assert.Equal(t, tt.expectedTimeout, timeout)
		})
	}

	// Test without load time buffer
	config.Resources.TimeoutScaling.LoadTimeBuffer = false
	profileNoBuffer := NewConfigurableProfile(config)

	timeout := profileNoBuffer.GetRequestTimeout("llama3-70b")
	assert.Equal(t, 30*time.Second, timeout, "should not add load time when buffer is false")

	// Test without timeout scaling configured
	configNoScaling := &domain.ProfileConfig{
		Name:    "test-profile",
		Version: "1.0",
	}
	configNoScaling.Characteristics.Timeout = 3 * time.Minute
	profileNoScaling := NewConfigurableProfile(configNoScaling)

	timeout = profileNoScaling.GetRequestTimeout("any-model")
	assert.Equal(t, 3*time.Minute, timeout, "should use base timeout when scaling not configured")
}

func TestConfigurableProfile_IntegratedResourceManagement(t *testing.T) {
	// Test that all resource management features work together correctly
	config := &domain.ProfileConfig{
		Name:    "ollama",
		Version: "1.0",
	}

	// Model patterns
	config.Models.ContextPatterns = []domain.ContextPattern{
		{Pattern: "*:32k*", Context: 32768},
		{Pattern: "llama3*", Context: 8192},
	}

	// Resource patterns
	config.Resources.ModelSizes = []domain.ModelSizePattern{
		{
			Patterns:            []string{"70b"},
			MinMemoryGB:         40,
			RecommendedMemoryGB: 48,
			EstimatedLoadTimeMS: 300000,
		},
		{
			Patterns:            []string{"7b"},
			MinMemoryGB:         6,
			RecommendedMemoryGB: 8,
			EstimatedLoadTimeMS: 30000,
		},
	}

	// Concurrency limits
	config.Resources.ConcurrencyLimits = []domain.ConcurrencyLimitPattern{
		{MinMemoryGB: 30, MaxConcurrent: 1},
		{MinMemoryGB: 0, MaxConcurrent: 8},
	}

	// Timeout scaling
	config.Resources.TimeoutScaling = domain.TimeoutScaling{
		BaseTimeoutSeconds: 30,
		LoadTimeBuffer:     true,
	}

	config.Characteristics.MaxConcurrentRequests = 10

	profile := NewConfigurableProfile(config)

	t.Run("70B model with 32k context", func(t *testing.T) {
		modelName := "llama3-70b:32k"

		// Should get 32k context from pattern
		caps := profile.GetModelCapabilities(modelName, nil)
		assert.Equal(t, int64(32768), caps.MaxContextLength)

		// Should get 1 concurrent request limit
		concurrent := profile.GetOptimalConcurrency(modelName)
		assert.Equal(t, 1, concurrent)

		// Should get 30s base + 5min load time
		timeout := profile.GetRequestTimeout(modelName)
		assert.Equal(t, 30*time.Second+5*time.Minute, timeout)
	})

	t.Run("7B model defaults", func(t *testing.T) {
		modelName := "mistral-7b"

		// Should get default context
		caps := profile.GetModelCapabilities(modelName, nil)
		assert.Equal(t, int64(4096), caps.MaxContextLength)

		// Should get 8 concurrent requests
		concurrent := profile.GetOptimalConcurrency(modelName)
		assert.Equal(t, 8, concurrent)

		// Should get 30s base + 30s load time
		timeout := profile.GetRequestTimeout(modelName)
		assert.Equal(t, 30*time.Second+30*time.Second, timeout)
	})
}

func TestConfigurableProfile_QuantizationWithNewFeatures(t *testing.T) {
	config := &domain.ProfileConfig{
		Name:    "test-profile",
		Version: "1.0",
	}

	config.Resources.ModelSizes = []domain.ModelSizePattern{
		{
			Patterns:            []string{"70b"},
			MinMemoryGB:         40,
			EstimatedLoadTimeMS: 300000,
		},
	}

	config.Resources.Quantization.Multipliers = map[string]float64{
		"q4": 0.5,
		"q5": 0.625,
	}

	config.Resources.ConcurrencyLimits = []domain.ConcurrencyLimitPattern{
		{MinMemoryGB: 30, MaxConcurrent: 1},
		{MinMemoryGB: 15, MaxConcurrent: 2},
		{MinMemoryGB: 0, MaxConcurrent: 4},
	}

	profile := NewConfigurableProfile(config)

	t.Run("quantized 70B model", func(t *testing.T) {
		// Q4 quantized 70B model should use 20GB (40 * 0.5)
		concurrent := profile.GetOptimalConcurrency("llama3-70b-q4")
		assert.Equal(t, 2, concurrent, "20GB model should get 2 concurrent requests")

		// Q5 quantized 70B model should use 25GB (40 * 0.625)
		concurrent = profile.GetOptimalConcurrency("llama3-70b-q5")
		assert.Equal(t, 2, concurrent, "25GB model should get 2 concurrent requests")
	})
}
