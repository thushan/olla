package profile

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/thushan/olla/internal/core/domain"
)

func TestOllamaProfile_InferenceProfile(t *testing.T) {
	profile := NewOllamaProfile()

	t.Run("GetTimeout", func(t *testing.T) {
		timeout := profile.GetTimeout()
		assert.Equal(t, 5*time.Minute, timeout, "Ollama should have 5 minute timeout for large models")
	})

	t.Run("GetMaxConcurrentRequests", func(t *testing.T) {
		max := profile.GetMaxConcurrentRequests()
		assert.Equal(t, 10, max, "Ollama should handle 10 concurrent requests")
	})

	t.Run("GetModelCapabilities", func(t *testing.T) {
		tests := []struct {
			name     string
			model    string
			expected domain.ModelCapabilities
		}{
			{
				name:  "Standard chat model",
				model: "llama3:8b",
				expected: domain.ModelCapabilities{
					ChatCompletion:   true,
					TextGeneration:   true,
					StreamingSupport: true,
					MaxContextLength: 8192,
					MaxOutputTokens:  2048,
					FunctionCalling:  true,
				},
			},
			{
				name:  "Embedding model",
				model: "nomic-embed-text",
				expected: domain.ModelCapabilities{
					ChatCompletion:   false,
					TextGeneration:   false,
					Embeddings:       true,
					StreamingSupport: true,
					MaxContextLength: 4096,
					MaxOutputTokens:  2048,
				},
			},
			{
				name:  "Vision model",
				model: "llava:13b",
				expected: domain.ModelCapabilities{
					ChatCompletion:      true,
					TextGeneration:      true,
					VisionUnderstanding: true,
					StreamingSupport:    true,
					MaxContextLength:    4096,
					MaxOutputTokens:     2048,
				},
			},
			{
				name:  "Code model",
				model: "codellama:34b",
				expected: domain.ModelCapabilities{
					ChatCompletion:   true,
					TextGeneration:   true,
					CodeGeneration:   true,
					StreamingSupport: true,
					MaxContextLength: 4096,
					MaxOutputTokens:  2048,
				},
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				caps := profile.GetModelCapabilities(tt.model, nil)
				assert.Equal(t, tt.expected, caps)
			})
		}
	})

	t.Run("GetResourceRequirements", func(t *testing.T) {
		tests := []struct {
			name   string
			model  string
			minMem float64
			recMem float64
		}{
			{"7B model", "llama3:7b", 6, 8},
			{"13B model", "llama3:13b", 10, 16},
			{"70B model", "llama3:70b", 40, 48},
			{"7B Q4 model", "llama3:7b-q4", 3, 4},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				reqs := profile.GetResourceRequirements(tt.model, nil)
				assert.Equal(t, tt.minMem, reqs.MinMemoryGB)
				assert.Equal(t, tt.recMem, reqs.RecommendedMemoryGB)
			})
		}
	})
}

func TestLMStudioProfile_InferenceProfile(t *testing.T) {
	profile := NewLMStudioProfile()

	t.Run("GetMaxConcurrentRequests", func(t *testing.T) {
		max := profile.GetMaxConcurrentRequests()
		assert.Equal(t, 1, max, "LM Studio is single-threaded")
	})

	t.Run("GetOptimalConcurrency", func(t *testing.T) {
		concurrency := profile.GetOptimalConcurrency("any-model")
		assert.Equal(t, 1, concurrency, "LM Studio should always use 1 concurrent request")
	})

	t.Run("TransformModelName", func(t *testing.T) {
		tests := []struct {
			from     string
			to       string
			expected string
		}{
			{"llama3", "lm_studio", "meta-llama/llama3"},
			{"mistral-7b", "lm_studio", "mistralai/mistral-7b"},
			{"phi-2", "lm_studio", "microsoft/phi-2"},
			{"already/formatted", "lm_studio", "already/formatted"},
		}

		for _, tt := range tests {
			t.Run(tt.from, func(t *testing.T) {
				result := profile.TransformModelName(tt.from, tt.to)
				assert.Equal(t, tt.expected, result)
			})
		}
	})
}

func TestOpenAICompatibleProfile_InferenceProfile(t *testing.T) {
	profile := NewOpenAICompatibleProfile()

	t.Run("GetModelCapabilities", func(t *testing.T) {
		tests := []struct {
			name      string
			model     string
			hasVision bool
			hasFunc   bool
			maxCtx    int64
		}{
			{"GPT-4", "gpt-4", false, true, 8192},
			{"GPT-4 Turbo", "gpt-4-turbo", false, true, 128000},
			{"GPT-4 Vision", "gpt-4-vision-preview", true, true, 8192},
			{"Claude 3", "claude-3-opus", true, true, 200000},
			{"Embeddings", "text-embedding-ada-002", false, false, 4096},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				caps := profile.GetModelCapabilities(tt.model, nil)
				assert.Equal(t, tt.hasVision, caps.VisionUnderstanding)
				assert.Equal(t, tt.hasFunc, caps.FunctionCalling)
				assert.Equal(t, tt.maxCtx, caps.MaxContextLength)
			})
		}
	})

	t.Run("GetResourceRequirements", func(t *testing.T) {
		reqs := profile.GetResourceRequirements("gpt-4", nil)
		assert.Equal(t, float64(0), reqs.MinMemoryGB, "Cloud APIs don't need local resources")
		assert.False(t, reqs.RequiresGPU)
	})
}
