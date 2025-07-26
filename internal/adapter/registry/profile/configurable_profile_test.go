package profile

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/thushan/olla/internal/core/domain"
)

func TestConfigurableProfile_GetResourceRequirements(t *testing.T) {
	tests := []struct {
		name      string
		config    *domain.ProfileConfig
		modelName string
		expected  domain.ResourceRequirements
	}{
		{
			name: "70b model with q4 quantization",
			config: &domain.ProfileConfig{
				Resources: struct {
					Quantization struct {
						Multipliers map[string]float64 `yaml:"multipliers"`
					} `yaml:"quantization"`
					ModelSizes        []domain.ModelSizePattern        `yaml:"model_sizes"`
					ConcurrencyLimits []domain.ConcurrencyLimitPattern `yaml:"concurrency_limits"`
					Defaults          domain.ResourceRequirements      `yaml:"defaults"`
					TimeoutScaling    domain.TimeoutScaling            `yaml:"timeout_scaling"`
				}{
					ModelSizes: []domain.ModelSizePattern{
						{
							Patterns:            []string{"70b", "72b"},
							MinMemoryGB:         40,
							RecommendedMemoryGB: 48,
							MinGPUMemoryGB:      40,
							EstimatedLoadTimeMS: 300000,
						},
					},
					Quantization: struct {
						Multipliers map[string]float64 `yaml:"multipliers"`
					}{
						Multipliers: map[string]float64{
							"q4": 0.5,
							"q5": 0.625,
						},
					},
					Defaults: domain.ResourceRequirements{
						MinMemoryGB:         4,
						RecommendedMemoryGB: 8,
						MinGPUMemoryGB:      4,
						RequiresGPU:         false,
						EstimatedLoadTimeMS: 5000,
					},
				},
			},
			modelName: "llama2-70b-q4_K_M",
			expected: domain.ResourceRequirements{
				MinMemoryGB:         20, // 40 * 0.5
				RecommendedMemoryGB: 24, // 48 * 0.5
				MinGPUMemoryGB:      20, // 40 * 0.5
				RequiresGPU:         false,
				EstimatedLoadTimeMS: 300000,
			},
		},
		{
			name: "7b model without quantization",
			config: &domain.ProfileConfig{
				Resources: struct {
					Quantization struct {
						Multipliers map[string]float64 `yaml:"multipliers"`
					} `yaml:"quantization"`
					ModelSizes        []domain.ModelSizePattern        `yaml:"model_sizes"`
					ConcurrencyLimits []domain.ConcurrencyLimitPattern `yaml:"concurrency_limits"`
					Defaults          domain.ResourceRequirements      `yaml:"defaults"`
					TimeoutScaling    domain.TimeoutScaling            `yaml:"timeout_scaling"`
				}{
					ModelSizes: []domain.ModelSizePattern{
						{
							Patterns:            []string{"7b", "8b"},
							MinMemoryGB:         6,
							RecommendedMemoryGB: 8,
							MinGPUMemoryGB:      6,
							EstimatedLoadTimeMS: 30000,
						},
					},
					Defaults: domain.ResourceRequirements{
						MinMemoryGB:         4,
						RecommendedMemoryGB: 8,
						MinGPUMemoryGB:      4,
						RequiresGPU:         false,
						EstimatedLoadTimeMS: 5000,
					},
				},
			},
			modelName: "mistral-7b",
			expected: domain.ResourceRequirements{
				MinMemoryGB:         6,
				RecommendedMemoryGB: 8,
				MinGPUMemoryGB:      6,
				RequiresGPU:         false,
				EstimatedLoadTimeMS: 30000,
			},
		},
		{
			name: "unknown model uses defaults",
			config: &domain.ProfileConfig{
				Resources: struct {
					Quantization struct {
						Multipliers map[string]float64 `yaml:"multipliers"`
					} `yaml:"quantization"`
					ModelSizes        []domain.ModelSizePattern        `yaml:"model_sizes"`
					ConcurrencyLimits []domain.ConcurrencyLimitPattern `yaml:"concurrency_limits"`
					Defaults          domain.ResourceRequirements      `yaml:"defaults"`
					TimeoutScaling    domain.TimeoutScaling            `yaml:"timeout_scaling"`
				}{
					ModelSizes: []domain.ModelSizePattern{
						{
							Patterns:            []string{"7b"},
							MinMemoryGB:         6,
							RecommendedMemoryGB: 8,
							MinGPUMemoryGB:      6,
							EstimatedLoadTimeMS: 30000,
						},
					},
					Defaults: domain.ResourceRequirements{
						MinMemoryGB:         4,
						RecommendedMemoryGB: 8,
						MinGPUMemoryGB:      4,
						RequiresGPU:         true,
						EstimatedLoadTimeMS: 5000,
					},
				},
			},
			modelName: "some-unknown-model",
			expected: domain.ResourceRequirements{
				MinMemoryGB:         4,
				RecommendedMemoryGB: 8,
				MinGPUMemoryGB:      4,
				RequiresGPU:         true,
				EstimatedLoadTimeMS: 5000,
			},
		},
		{
			name: "no resource config returns zero requirements",
			config: &domain.ProfileConfig{
				Name: "cloud-profile",
			},
			modelName: "gpt-4",
			expected: domain.ResourceRequirements{
				MinMemoryGB:         0,
				RecommendedMemoryGB: 0,
				MinGPUMemoryGB:      0,
				RequiresGPU:         false,
				EstimatedLoadTimeMS: 0,
			},
		},
		{
			name: "case insensitive pattern matching",
			config: &domain.ProfileConfig{
				Resources: struct {
					Quantization struct {
						Multipliers map[string]float64 `yaml:"multipliers"`
					} `yaml:"quantization"`
					ModelSizes        []domain.ModelSizePattern        `yaml:"model_sizes"`
					ConcurrencyLimits []domain.ConcurrencyLimitPattern `yaml:"concurrency_limits"`
					Defaults          domain.ResourceRequirements      `yaml:"defaults"`
					TimeoutScaling    domain.TimeoutScaling            `yaml:"timeout_scaling"`
				}{
					ModelSizes: []domain.ModelSizePattern{
						{
							Patterns:            []string{"13b"},
							MinMemoryGB:         10,
							RecommendedMemoryGB: 16,
							MinGPUMemoryGB:      10,
							EstimatedLoadTimeMS: 60000,
						},
					},
					Defaults: domain.ResourceRequirements{
						MinMemoryGB:         4,
						RecommendedMemoryGB: 8,
						MinGPUMemoryGB:      4,
						RequiresGPU:         false,
						EstimatedLoadTimeMS: 5000,
					},
				},
			},
			modelName: "Llama2-13B-Chat",
			expected: domain.ResourceRequirements{
				MinMemoryGB:         10,
				RecommendedMemoryGB: 16,
				MinGPUMemoryGB:      10,
				RequiresGPU:         false,
				EstimatedLoadTimeMS: 60000,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			profile := NewConfigurableProfile(tt.config)
			result := profile.GetResourceRequirements(tt.modelName, nil)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestConfigurableProfile_MultipleQuantizationTypes(t *testing.T) {
	config := &domain.ProfileConfig{
		Resources: struct {
			Quantization struct {
				Multipliers map[string]float64 `yaml:"multipliers"`
			} `yaml:"quantization"`
			ModelSizes        []domain.ModelSizePattern        `yaml:"model_sizes"`
			ConcurrencyLimits []domain.ConcurrencyLimitPattern `yaml:"concurrency_limits"`
			Defaults          domain.ResourceRequirements      `yaml:"defaults"`
			TimeoutScaling    domain.TimeoutScaling            `yaml:"timeout_scaling"`
		}{
			ModelSizes: []domain.ModelSizePattern{
				{
					Patterns:            []string{"13b"},
					MinMemoryGB:         10,
					RecommendedMemoryGB: 16,
					MinGPUMemoryGB:      10,
					EstimatedLoadTimeMS: 60000,
				},
			},
			Quantization: struct {
				Multipliers map[string]float64 `yaml:"multipliers"`
			}{
				Multipliers: map[string]float64{
					"q4": 0.5,
					"q5": 0.625,
					"q6": 0.75,
					"q8": 0.875,
				},
			},
		},
	}

	profile := NewConfigurableProfile(config)

	// Test that only the first matching quantization is applied
	result := profile.GetResourceRequirements("llama-13b-q5_K_M", nil)
	assert.Equal(t, 6.25, result.MinMemoryGB) // 10 * 0.625
}
