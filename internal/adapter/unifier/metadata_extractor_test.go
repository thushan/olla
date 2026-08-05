package unifier

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNormalizeQuantization(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		// K-quants
		{"Q4_K_M uppercase", "Q4_K_M", "q4km"},
		{"Q4_K_M lowercase", "q4_k_m", "q4km"},
		{"Q4_K_S", "Q4_K_S", "q4ks"},
		{"Q3_K_L", "Q3_K_L", "q3kl"},
		{"Q3_K_M", "Q3_K_M", "q3km"},
		{"Q3_K_S", "Q3_K_S", "q3ks"},
		{"Q5_K_M", "Q5_K_M", "q5km"},
		{"Q5_K_S", "Q5_K_S", "q5ks"},
		{"Q6_K", "Q6_K", "q6k"},
		{"Q2_K", "Q2_K", "q2k"},

		// Regular quants
		{"Q4_0", "Q4_0", "q4"},
		{"Q4_1", "Q4_1", "q4_1"},
		{"Q5_0", "Q5_0", "q5"},
		{"Q5_1", "Q5_1", "q5_1"},
		{"Q8_0", "Q8_0", "q8"},

		// Float formats
		{"F16", "F16", "f16"},
		{"FP16", "FP16", "f16"},
		{"F32", "F32", "f32"},
		{"FP32", "FP32", "f32"},
		{"BF16", "BF16", "bf16"},

		// GPTQ/AWQ formats
		{"GPTQ_4BIT", "GPTQ_4BIT", "gptq4"},
		{"GPTQ-4BIT", "GPTQ-4BIT", "gptq4"},
		{"AWQ_4BIT", "AWQ_4BIT", "awq4"},
		{"AWQ-4BIT", "AWQ-4BIT", "awq4"},

		// Other formats
		{"INT8", "INT8", "int8"},
		{"INT4", "INT4", "int4"},

		// Unknown/unmapped
		{"CUSTOM", "CUSTOM", "custom"},
		{"", "", ""},

		// Mixed case
		{"q4_K_m", "q4_K_m", "q4km"},
		{"Q4_k_M", "Q4_k_M", "q4km"},

		// gpt-oss / Nemotron FP4 formats
		{"MXFP4", "MXFP4", "mxfp4"},
		{"NVFP4", "NVFP4", "nvfp4"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizeQuantization(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestExtractParameterSize(t *testing.T) {
	tests := []struct {
		name          string
		input         string
		expectedSize  string
		expectedCount int64
	}{
		// Standard formats
		{"7B", "7B", "7b", 7_000_000_000},
		{"7b lowercase", "7b", "7b", 7_000_000_000},
		{"13B", "13B", "13b", 13_000_000_000},
		{"70B", "70B", "70b", 70_000_000_000},
		{"175B", "175B", "175b", 175_000_000_000},

		// Decimal formats
		{"1.5B", "1.5B", "1.5b", 1_500_000_000},
		{"3.8B", "3.8B", "3.8b", 3_800_000_000},
		{"12.2B", "12.2B", "12.2b", 12_200_000_000},
		{"0.5B", "0.5B", "0.5b", 500_000_000},

		// Million parameters
		{"540M", "540M", "540m", 540_000_000},
		{"125M", "125M", "125m", 125_000_000},

		// With spaces
		{"7 B", "7 B", "7b", 7_000_000_000},
		{"1.5 B", "1.5 B", "1.5b", 1_500_000_000},

		// Edge cases
		{"", "", "", 0},
		{"unknown", "unknown", "unknown", 0},
		{"7", "7", "7b", 7_000_000_000},
		{"NaN", "NaN", "NaN", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			size, count := extractParameterSize(tt.input)
			assert.Equal(t, tt.expectedSize, size)
			assert.Equal(t, tt.expectedCount, count)
		})
	}
}

func TestInferCapabilitiesFromMetadata(t *testing.T) {
	tests := []struct {
		name                 string
		modelType            string
		modelName            string
		contextLength        int64
		metadata             map[string]interface{}
		expectedCapabilities []string
	}{
		{
			name:                 "LLM type",
			modelType:            "llm",
			modelName:            "llama-3.2-1b",
			expectedCapabilities: []string{"text-generation", "chat", "completion"},
		},
		{
			name:                 "VLM type",
			modelType:            "vlm",
			modelName:            "llava-v1.6",
			expectedCapabilities: []string{"text-generation", "vision", "multimodal", "image-understanding"},
		},
		{
			name:                 "Embeddings type",
			modelType:            "embeddings",
			modelName:            "all-minilm",
			expectedCapabilities: []string{"embeddings", "similarity", "vector-search"},
		},
		{
			name:                 "Code model by name",
			modelType:            "llm",
			modelName:            "codellama-34b",
			expectedCapabilities: []string{"text-generation", "chat", "completion", "code-generation", "programming", "code-completion"},
		},
		{
			name:                 "Instruct model",
			modelType:            "llm",
			modelName:            "phi-3-mini-instruct",
			expectedCapabilities: []string{"text-generation", "chat", "completion", "instruction-following"},
		},
		{
			name:                 "Reasoning model",
			modelType:            "llm",
			modelName:            "phi-4-mini-reasoning",
			expectedCapabilities: []string{"text-generation", "chat", "completion", "reasoning", "logic"},
		},
		{
			name:                 "Math model",
			modelType:            "llm",
			modelName:            "mathstral-7b",
			expectedCapabilities: []string{"text-generation", "chat", "completion", "mathematics", "problem-solving"},
		},
		{
			name:                 "Long context",
			modelType:            "llm",
			modelName:            "claude-3",
			contextLength:        200_000,
			expectedCapabilities: []string{"text-generation", "chat", "completion", "long-context"},
		},
		{
			name:                 "Ultra long context",
			modelType:            "llm",
			modelName:            "gemini-1.5-pro",
			contextLength:        2_000_000,
			expectedCapabilities: []string{"text-generation", "chat", "completion", "ultra-long-context", "long-context"},
		},
		{
			name:                 "Vision model by name",
			modelType:            "",
			modelName:            "bakllava-7b",
			expectedCapabilities: []string{"text-generation", "vision", "multimodal", "image-understanding"},
		},
		{
			name:          "Multiple capabilities",
			modelType:     "vlm",
			modelName:     "codellama-instruct-vision",
			contextLength: 128_000,
			expectedCapabilities: []string{
				"text-generation", "vision", "multimodal", "image-understanding",
				"code-generation", "programming", "code-completion",
				"instruction-following", "chat", "long-context",
			},
		},
		{
			name:                 "Forge code model by name",
			modelType:            "llm",
			modelName:            "tf/forge-code-2.1-code",
			expectedCapabilities: []string{"text-generation", "chat", "completion", "code-generation", "programming", "code-completion"},
		},
		{
			name:      "Metadata capabilities",
			modelType: "llm",
			modelName: "custom-model",
			metadata: map[string]interface{}{
				"capabilities": []interface{}{"custom-cap1", "custom-cap2"},
			},
			expectedCapabilities: []string{"text-generation", "chat", "completion", "custom-cap1", "custom-cap2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			caps := inferCapabilitiesFromMetadata(tt.modelType, tt.modelName, tt.contextLength, tt.metadata)

			// Check that all expected capabilities are present
			for _, expected := range tt.expectedCapabilities {
				assert.Contains(t, caps, expected, "Expected capability %s not found", expected)
			}

			// For embedding models, ensure text-generation is not present
			if tt.modelType == "embeddings" || tt.modelType == "embedding" {
				assert.NotContains(t, caps, "text-generation")
			}
		})
	}
}

func TestExtractFamilyAndVariant(t *testing.T) {
	tests := []struct {
		name            string
		modelName       string
		arch            string
		expectedFamily  string
		expectedVariant string
	}{
		// Basic patterns
		{"llama3.2:1b", "llama3.2:1b", "", "llama", "3.2"},
		{"llama-3.2-1b", "llama-3.2-1b", "", "llama", "3.2"},
		{"gemma3:12b", "gemma3:12b", "", "gemma", "3"},
		{"phi-4-mini", "phi-4-mini", "", "phi", "4"},
		{"qwen2.5-coder", "qwen2.5-coder", "", "qwen", "2.5"},
		{"mistral-7b", "mistral-7b", "", "mistral", "7b"},
		{"mixtral-8x7b", "mixtral-8x7b", "", "mixtral", "8x7b"},

		// With publisher prefix
		{"microsoft/phi-4-mini-reasoning", "microsoft/phi-4-mini-reasoning", "", "phi", "4"},
		{"google/gemma-2-9b", "google/gemma-2-9b", "", "gemma", "2"},

		// Architecture mappings
		{"custom-name", "custom-name", "phi3", "phi", ""},
		{"model", "model", "llama3.2", "llama", "3.2"},
		{"unknown", "unknown", "gemma3", "gemma", "3"},

		// Special models
		{"codellama-34b", "codellama-34b", "", "codellama", "34b"},
		{"starcoder-15b", "starcoder-15b", "", "starcoder", "15b"},
		{"vicuna-13b", "vicuna-13b", "", "vicuna", "13b"},
		{"falcon-40b", "falcon-40b", "", "falcon", "40b"},

		// GPT variants
		{"gpt2", "gpt2", "", "gpt", "2"},
		{"gpt-j-6b", "gpt-j-6b", "", "gpt", "j"},
		{"gpt-neox-20b", "gpt-neox-20b", "", "gpt", "neox"},

		// Edge cases
		{"", "", "", "", ""},
		{"unknown-model", "unknown-model", "", "", ""},
		{"yi-34b", "yi-34b", "", "yi", "34b"},
		{"deepseek-coder", "deepseek-coder", "", "deepseek", "coder"},

		// 2026 model families (issue: models.yaml refresh)
		{"qwen3:32b", "qwen3:32b", "", "qwen", "3"},    // already worked pre-change; regression proof
		{"gemma4:12b", "gemma4:12b", "", "gemma", "4"}, // already worked pre-change; regression proof
		{"glm-4.5-air", "glm-4.5-air", "", "glm", "4.5"},
		{"gpt-oss:20b", "gpt-oss:20b", "", "gpt-oss", "20b"},
		{"kimi-k2", "kimi-k2", "", "kimi", "k2"},
		{"granite3.3:8b", "granite3.3:8b", "", "granite", "3.3"},
		{"devstral-small-2505", "devstral-small-2505", "", "devstral", "small-2505"},
		{"codestral-22b", "codestral-22b", "", "codestral", "22b"},
		{"magistral-small", "magistral-small", "", "magistral", "small"},
		{"ministral-8b", "ministral-8b", "", "ministral", "8b"},
		{"nemotron-70b", "nemotron-70b", "", "nemotron", "70b"},
		{"exaone-4.0-32b", "exaone-4.0-32b", "", "exaone", "4.0"},
		{"hunyuan-a13b", "hunyuan-a13b", "", "hunyuan", "a13b"},
		{"minimax-m2", "minimax-m2", "", "minimax", "m2"},
		{"seed-oss-36b-instruct", "seed-oss-36b-instruct", "", "seed-oss", "36b-instruct"},
		{"olmo2-13b", "olmo2-13b", "", "olmo", "2"},
		{"internlm3-8b", "internlm3-8b", "", "internlm", "3"},
		{"smollm3-3b", "smollm3-3b", "", "smollm", "3"},
		{"command-r-plus", "command-r-plus", "", "command-r", "plus"},
		{"hf publisher prefix - glm", "zai-org/glm-4.6", "", "glm", "4.6"},
		{"hf publisher prefix - kimi", "moonshotai/Kimi-K2-Instruct", "", "kimi", ""},

		// TensorFoundry Forge models (models.yaml 2026 refresh)
		{"forge with publisher prefix", "tf/forge-code-2.1-code", "", "forge", "code-2.1-code"},
		{"forge without publisher prefix", "forge-code-2.5-code", "", "forge", "code-2.5-code"},

		// architecture-mapping path (arch supplied, name generic ("model" is
		// in SpecialRules.GenericNames)). extractFromArchitecture doesn't stop
		// at the first non-digit run after the family prefix - it takes the
		// entire trimmed remainder as the variant once the first character is
		// numeric, so "glm4moe" and "qwen3.5" yield "4moe" and "3.5" rather
		// than just the leading digits.
		{"model with glm4moe arch", "model", "glm4moe", "glm", "4moe"},
		{"model with granitemoe arch", "model", "granitemoe", "granite", ""},
		{"model with qwen3.5 arch", "model", "qwen3.5", "qwen", "3.5"},

		// Known limitation, not a bug to fix here: Kimi-K2 GGUF files often
		// report general.architecture "deepseek2" (Kimi-K2 and DeepSeek-V3
		// share the same underlying transformer architecture), so
		// architecture-based extraction misclassifies a Kimi model as
		// "deepseek" whenever arch metadata is present - architecture-based
		// lookup is tried before name-pattern matching in
		// extractFamilyAndVariant, so it wins over the "kimi" pattern.
		// ArchitectureMappings is a flat 1:1 table, so this ambiguity is
		// unavoidable without a redesign - out of scope for this pass.
		{"kimi model with deepseek arch (known collision)", "kimi-k2-instruct", "deepseek", "deepseek", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			family, variant := extractFamilyAndVariant(tt.modelName, tt.arch)
			assert.Equal(t, tt.expectedFamily, family, "Family mismatch")
			assert.Equal(t, tt.expectedVariant, variant, "Variant mismatch")
		})
	}
}

func TestExtractPublisher(t *testing.T) {
	tests := []struct {
		name              string
		modelName         string
		metadata          map[string]interface{}
		expectedPublisher string
	}{
		// From metadata
		{
			name:              "Publisher in metadata",
			modelName:         "phi-4",
			metadata:          map[string]interface{}{"publisher": "microsoft"},
			expectedPublisher: "microsoft",
		},
		// From model name prefix
		{
			name:              "Publisher prefix",
			modelName:         "microsoft/phi-4-mini",
			metadata:          nil,
			expectedPublisher: "microsoft",
		},
		{
			name:              "Google prefix",
			modelName:         "google/gemma-2",
			metadata:          nil,
			expectedPublisher: "google",
		},
		// Known patterns
		{
			name:              "Known Llama",
			modelName:         "llama3.2:1b",
			metadata:          nil,
			expectedPublisher: "meta",
		},
		{
			name:              "Known CodeLlama",
			modelName:         "codellama-34b",
			metadata:          nil,
			expectedPublisher: "meta",
		},
		{
			name:              "Known Gemma",
			modelName:         "gemma-7b",
			metadata:          nil,
			expectedPublisher: "google",
		},
		{
			name:              "Known Phi",
			modelName:         "phi-3-mini",
			metadata:          nil,
			expectedPublisher: "microsoft",
		},
		{
			name:              "Known Mistral",
			modelName:         "mistral-7b",
			metadata:          nil,
			expectedPublisher: "mistral",
		},
		{
			name:              "Known Qwen",
			modelName:         "qwen2.5-coder",
			metadata:          nil,
			expectedPublisher: "alibaba",
		},
		{
			name:              "Known Yi",
			modelName:         "yi-34b",
			metadata:          nil,
			expectedPublisher: "01-ai",
		},
		{
			name:              "Known StarCoder",
			modelName:         "starcoder2-15b",
			metadata:          nil,
			expectedPublisher: "bigcode",
		},
		// Unknown
		{
			name:              "Unknown model",
			modelName:         "custom-model",
			metadata:          nil,
			expectedPublisher: "",
		},
		// 2026 model families
		{
			name:              "Known Devstral",
			modelName:         "devstral-small-2505",
			metadata:          nil,
			expectedPublisher: "mistral",
		},
		{
			name:              "Known GLM",
			modelName:         "glm-4.5-air",
			metadata:          nil,
			expectedPublisher: "zai-org",
		},
		{
			name:              "Known Kimi",
			modelName:         "kimi-k2",
			metadata:          nil,
			expectedPublisher: "moonshotai",
		},
		{
			name:              "Known Granite",
			modelName:         "granite3.3:8b",
			metadata:          nil,
			expectedPublisher: "ibm",
		},
		{
			name:              "Known Command-R",
			modelName:         "command-r-plus",
			metadata:          nil,
			expectedPublisher: "cohere",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			publisher := extractPublisher(tt.modelName, tt.metadata)
			assert.Equal(t, tt.expectedPublisher, publisher)
		})
	}
}

func TestParseContextLength(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		expected int64
	}{
		{"int64", int64(4096), 4096},
		{"int", 8192, 8192},
		{"float64", float64(16384), 16384},
		{"string valid", "32768", 32768},
		{"string invalid", "invalid", 0},
		{"nil", nil, 0},
		{"other type", []int{1, 2, 3}, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseContextLength(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}
