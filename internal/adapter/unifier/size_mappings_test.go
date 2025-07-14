package unifier

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExtractSizeFromName(t *testing.T) {
	tests := []struct {
		name               string
		modelName          string
		expectedSize       string
		expectedConfidence int
	}{
		// Numeric patterns
		{
			name:               "explicit B size",
			modelName:          "llama-3-70B",
			expectedSize:       "70",
			expectedConfidence: 100,
		},
		{
			name:               "lowercase b size",
			modelName:          "phi-4-14.7b",
			expectedSize:       "14.7",
			expectedConfidence: 100,
		},
		{
			name:               "million parameters",
			modelName:          "bert-350M",
			expectedSize:       "350",
			expectedConfidence: 90,
		},
		{
			name:               "size in middle with hyphens",
			modelName:          "model-7B-instruct",
			expectedSize:       "7",
			expectedConfidence: 100,
		},
		{
			name:               "size at end with colon",
			modelName:          "mixtral:8x7B",
			expectedSize:       "8x7",
			expectedConfidence: 100,
		},
		
		// Text-based patterns
		{
			name:               "small descriptor",
			modelName:          "devstral-small-2505",
			expectedSize:       "small-2505", // Matches specific pattern
			expectedConfidence: 60,
		},
		{
			name:               "small only",
			modelName:          "magistral-small",
			expectedSize:       "small",
			expectedConfidence: 57,
		},
		{
			name:               "medium model",
			modelName:          "llama-medium-chat",
			expectedSize:       "medium",
			expectedConfidence: 57,
		},
		{
			name:               "large model",
			modelName:          "gpt-large",
			expectedSize:       "large",
			expectedConfidence: 57,
		},
		{
			name:               "mini model",
			modelName:          "phi-mini",
			expectedSize:       "mini",
			expectedConfidence: 58,
		},
		{
			name:               "base model",
			modelName:          "mistral-base",
			expectedSize:       "base",
			expectedConfidence: 56,
		},
		
		// No size found
		{
			name:               "no size info",
			modelName:          "random-model",
			expectedSize:       "",
			expectedConfidence: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			size, confidence := extractSizeFromName(tt.modelName)
			assert.Equal(t, tt.expectedSize, size)
			assert.Equal(t, tt.expectedConfidence, confidence)
		})
	}
}

func TestInferSizeFromContext(t *testing.T) {
	tests := []struct {
		name               string
		family             string
		variant            string
		contextLength      int64
		quantization       string
		expectedSize       string
		expectedConfidence int
	}{
		{
			name:               "very long context",
			contextLength:      131072,
			expectedSize:       "large",
			expectedConfidence: 30,
		},
		{
			name:               "medium context",
			contextLength:      49152,
			expectedSize:       "medium",
			expectedConfidence: 25,
		},
		{
			name:               "mistral magistral",
			family:             "mistral",
			variant:            "magistral",
			contextLength:      8192,
			expectedSize:       "small",
			expectedConfidence: 40,
		},
		{
			name:               "mistral devstral",
			family:             "mistral",
			variant:            "devstral",
			contextLength:      8192,
			expectedSize:       "small",
			expectedConfidence: 40,
		},
		{
			name:               "aggressive quantization",
			quantization:       "q2_k",
			expectedSize:       "large",
			expectedConfidence: 20,
		},
		{
			name:               "no inference possible",
			family:             "unknown",
			variant:            "unknown",
			contextLength:      4096,
			quantization:       "q4_0",
			expectedSize:       "",
			expectedConfidence: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			size, confidence := inferSizeFromContext(tt.family, tt.variant, tt.contextLength, tt.quantization)
			assert.Equal(t, tt.expectedSize, size)
			assert.Equal(t, tt.expectedConfidence, confidence)
		})
	}
}

func TestTextSizeMappings(t *testing.T) {
	// Verify mappings are consistent
	seenPatterns := make(map[string]bool)
	
	for _, mapping := range TextSizeMappings {
		// Check for duplicate patterns
		assert.False(t, seenPatterns[mapping.Pattern], "Duplicate pattern found: %s", mapping.Pattern)
		seenPatterns[mapping.Pattern] = true
		
		// Verify normalized size format
		assert.Regexp(t, `^\d+(?:\.\d+)?b$`, mapping.NormalizedSize, "Invalid normalized size format: %s", mapping.NormalizedSize)
		
		// Verify parameter count is reasonable
		assert.Greater(t, mapping.ParameterCount, int64(0), "Parameter count should be positive")
		assert.Less(t, mapping.ParameterCount, int64(1000000000000), "Parameter count seems too large")
		
		// Verify priority is set
		assert.Greater(t, mapping.Priority, 0, "Priority should be positive")
	}
}