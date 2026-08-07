package profile

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

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

// TestConfigurableProfile_GetResourceRequirements_SizeTokenBoundaries guards
// against the bare strings.Contains bug where a size token like "7b" matched
// inside "17b" or "27b", and "1b" matched inside "11b". Patterns must match
// on a token boundary (start/end of string or a non-alphanumeric neighbour).
func TestConfigurableProfile_GetResourceRequirements_SizeTokenBoundaries(t *testing.T) {
	sevenBReqs := domain.ResourceRequirements{
		MinMemoryGB:         6,
		RecommendedMemoryGB: 8,
		MinGPUMemoryGB:      6,
		EstimatedLoadTimeMS: 30000,
	}
	oneBReqs := domain.ResourceRequirements{
		MinMemoryGB:         2,
		RecommendedMemoryGB: 3,
		MinGPUMemoryGB:      2,
		EstimatedLoadTimeMS: 10000,
	}
	defaults := domain.ResourceRequirements{
		MinMemoryGB:         4,
		RecommendedMemoryGB: 8,
		MinGPUMemoryGB:      4,
		EstimatedLoadTimeMS: 5000,
	}

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
					Patterns:            []string{"7b", "8b"},
					MinMemoryGB:         sevenBReqs.MinMemoryGB,
					RecommendedMemoryGB: sevenBReqs.RecommendedMemoryGB,
					MinGPUMemoryGB:      sevenBReqs.MinGPUMemoryGB,
					EstimatedLoadTimeMS: 30000,
				},
				{
					Patterns:            []string{"1b", "1.5b"},
					MinMemoryGB:         oneBReqs.MinMemoryGB,
					RecommendedMemoryGB: oneBReqs.RecommendedMemoryGB,
					MinGPUMemoryGB:      oneBReqs.MinGPUMemoryGB,
					EstimatedLoadTimeMS: 10000,
				},
			},
			Defaults: defaults,
		},
	}

	tests := []struct {
		name      string
		modelName string
		expected  domain.ResourceRequirements
	}{
		// "7b" must not match inside a larger number.
		{"17b model does not match 7b token", "llama-17b", defaults},
		{"27b model does not match 7b token", "gemma-27b", defaults},
		{"70b model does not match 7b token", "llama-70b", defaults},
		{"7b8 model does not match 7b token", "weird-7b8-variant", defaults},
		// "7b" must still match legitimate boundary forms.
		{"llama-7b matches 7b token", "llama-7b", sevenBReqs},
		{"7b-chat matches 7b token", "7b-chat", sevenBReqs},
		{"codellama:7b matches 7b token", "codellama:7b", sevenBReqs},
		// "1b" must not match inside "11b".
		{"11b model does not match 1b token", "llama-11b", defaults},
		// "1b" must still match legitimate boundary forms.
		{"llama-1b matches 1b token", "llama-1b", oneBReqs},
		{"1b-instruct matches 1b token", "1b-instruct", oneBReqs},
		{"tinyllama-1.5b matches 1.5b token", "tinyllama-1.5b", oneBReqs},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			profile := NewConfigurableProfile(config)
			result := profile.GetResourceRequirements(tt.modelName, nil)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestContainsSizeToken exercises the boundary-matching helper directly,
// independent of the profile config plumbing.
func TestContainsSizeToken(t *testing.T) {
	tests := []struct {
		name     string
		s        string
		token    string
		expected bool
	}{
		{"exact match", "7b", "7b", true},
		{"prefixed with delimiter", "llama-7b", "7b", true},
		{"suffixed with delimiter", "7b-chat", "7b", true},
		{"colon delimiter", "codellama:7b", "7b", true},
		{"surrounded by delimiters", "llama-7b-chat", "7b", true},
		{"does not match inside larger number (prefix digit)", "17b", "7b", false},
		{"does not match inside larger number (prefix digit 2)", "27b", "7b", false},
		{"does not match when followed by digit", "70b", "7b", false},
		{"does not match when followed by another digit-letter run", "7b8", "7b", false},
		{"1b does not match inside 11b", "11b", "1b", false},
		{"1b matches at start with delimiter after", "1b-instruct", "1b", true},
		{"empty token never matches", "7b", "", false},
		{"token longer than string", "7b", "codellama-7b", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, containsSizeToken(tt.s, tt.token))
		})
	}
}

// TestConfigurableProfile_ShippedProfiles_SizeBucketsResolve is the
// regression guard for the incident where six shipped profiles (vllm,
// sglang, lmdeploy, lemonade, vllm-mlx, dmr) carried glob-wrapped size
// patterns like "*70b*" left over from before the ollama/lmstudio/llamacpp
// profiles switched to bare-token matching. containsSizeToken (like the
// strings.Contains it replaced) can never match a literal "*" character, so
// those six profiles silently fell through to resources.defaults for every
// model, for months, without a single test noticing.
//
// This loads the real shipped YAML from config/profiles/ (not a fixture) and
// exercises the real GetResourceRequirements path, so a reintroduced
// glob-wrapped pattern - in any profile, not just the six named above - fails
// loudly instead of quietly degrading to defaults.
func TestConfigurableProfile_ShippedProfiles_SizeBucketsResolve(t *testing.T) {
	entries, err := os.ReadDir(shippedProfilesDir)
	require.NoError(t, err)

	var yamlFiles []string
	for _, e := range entries {
		if !e.IsDir() && len(e.Name()) > 5 && e.Name()[len(e.Name())-5:] == ".yaml" {
			yamlFiles = append(yamlFiles, e.Name())
		}
	}
	require.NotEmpty(t, yamlFiles, "expected shipped profile YAML files under %s", shippedProfilesDir)

	for _, filename := range yamlFiles {
		t.Run(filename, func(t *testing.T) {
			data, err := os.ReadFile(shippedProfilesDir + filename)
			require.NoError(t, err)

			var cfg domain.ProfileConfig
			require.NoError(t, yaml.Unmarshal(data, &cfg))

			if len(cfg.Resources.ModelSizes) == 0 {
				t.Skipf("%s has no resources.model_sizes block (defaults-only or cloud profile) - nothing to resolve", filename)
				return
			}

			profile := NewConfigurableProfile(&cfg)

			// Every size bucket must be reachable from a realistic model name
			// built around its own first pattern - if any bucket is dead, the
			// pattern is glob-wrapped (or otherwise broken) again.
			for _, bucket := range cfg.Resources.ModelSizes {
				require.NotEmpty(t, bucket.Patterns, "%s: model_sizes bucket has no patterns", filename)
				token := bucket.Patterns[0]
				modelName := "test-model-" + token + "-instruct"

				got := profile.GetResourceRequirements(modelName, nil)
				assert.Equalf(t, bucket.MinMemoryGB, got.MinMemoryGB,
					"%s: model %q (pattern %q) resolved to MinMemoryGB=%v, want the %q bucket's %v - pattern is likely glob-wrapped and dead",
					filename, modelName, token, got.MinMemoryGB, token, bucket.MinMemoryGB)
			}
		})
	}
}

// TestConfigurableProfile_VLLM_SizeTokenBoundary loads the real shipped
// vllm.yaml (one of the six profiles fixed alongside this test) and checks
// the 7b/17b boundary case directly, mirroring
// TestConfigurableProfile_GetResourceRequirements_SizeTokenBoundaries but
// against production YAML rather than an inline fixture.
func TestConfigurableProfile_VLLM_SizeTokenBoundary(t *testing.T) {
	data, err := os.ReadFile(shippedProfilesDir + "vllm.yaml")
	require.NoError(t, err)

	var cfg domain.ProfileConfig
	require.NoError(t, yaml.Unmarshal(data, &cfg))

	profile := NewConfigurableProfile(&cfg)

	sevenB := profile.GetResourceRequirements("Meta-Llama-3.1-7B-Instruct", nil)
	seventeenB := profile.GetResourceRequirements("Some-Custom-17B-Instruct", nil)

	assert.Equal(t, 16.0, sevenB.MinMemoryGB, "7B model must resolve to the 7b/8b bucket, not fall through")
	assert.NotEqual(t, sevenB.MinMemoryGB, seventeenB.MinMemoryGB, "17B model must not match the 7b token")
	assert.Equal(t, cfg.Resources.Defaults.MinMemoryGB, seventeenB.MinMemoryGB, "17B model has no matching bucket, so it must fall through to defaults")
}
