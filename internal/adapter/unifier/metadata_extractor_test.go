package unifier

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGetConfig_ReflectsResetSingleton is the regression test for the
// double-cache bug: getConfig() used to keep its own sync.Once independent of
// LoadModelConfig's, so resetConfigSingleton() (which only clears the latter)
// never actually reached extraction - normalizeQuantization and friends kept
// serving whatever config they saw on first use for the rest of the test
// process. getConfig() now delegates straight to LoadModelConfig(), so a
// reset must be visible here too.
func TestGetConfig_ReflectsResetSingleton(t *testing.T) {
	// Latch getConfig() onto the embedded defaults first, same as any other
	// test in this process would.
	resetConfigSingleton()
	t.Cleanup(resetConfigSingleton)
	require.NotContains(t, getDefaultConfig().Quantization.Mappings, "ZZTESTMARKER", "sanity: pick a mapping key the embedded defaults don't already define")

	dir := t.TempDir()
	customYAML := `
quantization:
  mappings:
    ZZTESTMARKER: "custom-marker"
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "models.yaml"), []byte(customYAML), 0644))
	// Select the config via OLLA_CONFIG_DIR rather than t.Chdir, so this test
	// stays isolated from others that use unqualified models.yaml paths.
	t.Setenv("OLLA_CONFIG_DIR", dir)

	resetConfigSingleton()

	assert.Equal(t, "custom-marker", normalizeQuantization("ZZTESTMARKER"), "getConfig() must observe the reset, not a stale cached config")
}

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
		// "kimi" is in preserve_family (fixes the deepseek arch collision
		// below), so matchPreserveFamily short-circuits on the name-token
		// prefix and returns no variant - same trade-off preserve_family
		// already makes for deepseek-coder-v2 and nomic-bert.
		{"kimi-k2", "kimi-k2", "", "kimi", ""},
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

		// Kimi-K2 GGUF files often report general.architecture "deepseek2" or
		// "deepseek" (Kimi-K2 and DeepSeek-V3 share the same underlying
		// transformer architecture), which used to misclassify a Kimi model
		// as "deepseek" since architecture-based lookup ran before
		// name-pattern matching. Fixed via special_rules.preserve_family:
		// matchPreserveFamily checks the name-token prefix ahead of the arch
		// stage, so the name wins whenever it actually says kimi.
		{"kimi model with deepseek arch (fixed via preserve_family)", "kimi-k2-instruct", "deepseek", "kimi", ""},
		{"kimi model with deepseek2 arch (fixed via preserve_family)", "kimi-k2", "deepseek2", "kimi", ""},

		// Publisher-prefixed names (LM Studio/vLLM commonly report these)
		// used to skip matchPreserveFamily entirely because the token-prefix
		// check ran against the whole "publisher/model" string, so the arch
		// stage won regardless of preserve_family. Fixed by stripping up to
		// the last '/' before the check.
		{"publisher-prefixed kimi with deepseek arch", "moonshotai/Kimi-K2-Instruct", "deepseek", "kimi", ""},
		{"publisher-prefixed kimi with deepseek2 arch", "moonshotai/Kimi-K2-Instruct", "deepseek2", "kimi", ""},
		{"publisher-prefixed deepseek-coder-v2 preserved", "deepseek-ai/DeepSeek-Coder-V2-Instruct", "", "deepseek-coder-v2", ""},

		// A publisher segment that happens to share a preserve entry's name
		// must not fool the check - the stripped, matched part is the model
		// name after the last '/', not the publisher.
		{"publisher segment matching a preserve entry must not match", "kimi/llama-3-8b", "", "llama", "3"},

		// FamilyAliases delimiter-fallback for name-based extraction is covered
		// separately in TestExtractFromDelimiters_UsesConfiguredAliases: the
		// 2026 refresh (main) gave devstral its own family_pattern, so
		// "devstral-24b" now resolves at the pattern stage as family
		// "devstral" (see devstral-small-2505 above) and never reaches the
		// delimiter fallback this alias mechanism exists for.

		// FamilyAliases: architecture fallback for arch strings not in ArchitectureMappings
		{"arch alias deepseek2", "model", "deepseek2", "deepseek", "2"},

		// PreserveFamily: wins outright, ahead of the deepseek pattern
		{"deepseek-coder-v2 preserved", "deepseek-coder-v2", "", "deepseek-coder-v2", ""},
		{"deepseek-coder-v2 with suffix preserved", "deepseek-coder-v2-lite", "", "deepseek-coder-v2", ""},
		{"nomic-bert preserved", "nomic-bert", "", "nomic-bert", ""},
		{"nomic-bert with suffix preserved", "nomic-bert-v1.5", "", "nomic-bert", ""},

		// PreserveFamily must NOT affect deepseek-coder (no -v2 suffix)
		{"deepseek-coder still splits", "deepseek-coder", "", "deepseek", "coder"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			family, variant := extractFamilyAndVariant(tt.modelName, tt.arch)
			assert.Equal(t, tt.expectedFamily, family, "Family mismatch")
			assert.Equal(t, tt.expectedVariant, variant, "Variant mismatch")
		})
	}
}

// TestFamilyPatternBoundaries is the regression guard for the campaign-found
// family_patterns bug: the 2026-refresh patterns used an optional delimiter
// plus an optional catch-all suffix (e.g. kimi's old `^(kimi)[-_]?(.+)?`),
// which is unanchored - "kimichef-70b" matched at position 0 with the
// delimiter and suffix both empty, misclassifying it as family "kimi". Every
// family listed here must now require the character immediately following
// the family literal to be a delimiter ([-_.:]), a version digit, or the end
// of the string - never a bare letter - while genuine delimited/versioned
// names keep resolving correctly. Exercised directly against
// extractFromPatterns (not extractFamilyAndVariant) so preserve_family
// (which already has its own correct boundary logic for "kimi") can't mask a
// regression in the pattern itself.
func TestFamilyPatternBoundaries(t *testing.T) {
	config := getDefaultConfig()
	require.NoError(t, config.compilePatterns())

	tests := []struct {
		name           string
		modelName      string
		expectedFamily string
	}{
		{"kimi: unrelated name sharing the prefix must not match", "kimichef-70b", ""},
		{"kimi: hyphen boundary", "kimi-k2", "kimi"},
		{"kimi: dot boundary", "kimi.k2", "kimi"},
		{"kimi: colon boundary", "kimi:latest", "kimi"},

		{"nemotron: unrelated name sharing the prefix must not match", "nemotronian-model", ""},
		{"nemotron: hyphen boundary", "nemotron-70b", "nemotron"},

		{"hunyuan: unrelated name sharing the prefix must not match", "hunyuanite-1b", ""},
		{"hunyuan: hyphen boundary", "hunyuan-a13b", "hunyuan"},

		{"minimax: unrelated name sharing the prefix must not match", "minimaxwell-2", ""},
		{"minimax: hyphen boundary", "minimax-m2", "minimax"},

		{"gpt-oss: colon boundary", "gpt-oss:20b", "gpt-oss"},

		{"seed-oss: unrelated name sharing the prefix must not match", "seed-ossify-1b", ""},
		{"seed-oss: hyphen boundary", "seed-oss-36b-instruct", "seed-oss"},

		{"command-r: unrelated name sharing the prefix must not match", "command-rocket-1b", ""},
		{"command-r: hyphen boundary", "command-r-plus", "command-r"},

		{"devstral family: unrelated name sharing the prefix must not match", "devstralia-1b", ""},
		{"devstral family: hyphen boundary", "devstral-small-2505", "devstral"},

		{"glm: unrelated name sharing the prefix must not match", "glmania-1b", ""},
		{"glm: hyphen boundary", "glm-4.5-air", "glm"},
		{"glm: bare digit boundary", "glm4", "glm"},

		{"granite: unrelated name sharing the prefix must not match", "granitehybrid", ""},
		{"granite: bare digit boundary", "granite3.3:8b", "granite"},

		{"exaone: unrelated name sharing the prefix must not match", "exaonerous-1b", ""},
		{"exaone: hyphen boundary", "exaone-4.0-32b", "exaone"},

		{"olmo: unrelated name sharing the prefix must not match", "olmossy-1b", ""},
		{"olmo: bare digit boundary", "olmo2-13b", "olmo"},

		{"internlm: unrelated name sharing the prefix must not match", "internlmish-1b", ""},
		{"internlm: bare digit boundary", "internlm3-8b", "internlm"},

		{"smollm: unrelated name sharing the prefix must not match", "smollmish-1b", ""},
		{"smollm: bare digit boundary", "smollm3-3b", "smollm"},

		{"forge: unrelated name sharing the prefix must not match", "forgery-1b", ""},
		{"forge: hyphen boundary", "forge-code-2.5-code", "forge"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			family, _ := extractFromPatterns(tt.modelName, config)
			assert.Equal(t, tt.expectedFamily, family)
		})
	}

	// gpt-oss's own boundary is checked against its compiled regex directly,
	// not via extractFromPatterns: "gpt-ossify-1b" fails the gpt-oss pattern
	// correctly, but then falls through to the separate, unrelated, and
	// out-of-scope "gpt" pattern (`^(gpt)[-_]?(2|j|neox)?`, not covered by
	// this fix), which has the identical unbounded-suffix bug and would
	// misreport family "gpt" - a false failure attributable to a different
	// pattern, not this one.
	var gptOssRegex *regexp.Regexp
	for _, p := range config.ModelExtraction.FamilyPatterns {
		if p.Description == "OpenAI open-weight gpt-oss models" {
			gptOssRegex = p.regex
			break
		}
	}
	require.NotNil(t, gptOssRegex, "gpt-oss family pattern must exist in default config")
	if gptOssRegex.MatchString("gpt-ossify-1b") {
		t.Error("gpt-oss pattern must not match \"gpt-ossify-1b\" - letter immediately follows the family literal")
	}
}

// TestMatchPreserveFamily_KimiTokenPrefixSemantics is a direct, isolated
// sanity check for the kimi preserve_family fix (issue: PR #207 CodeRabbit
// finding). matchPreserveFamily uses exact-or-token-prefix matching, not a
// bare substring contains, so it must not fire for names that merely start
// with the same letters, or arch strings that merely contain "kimi".
// Exercised directly against matchPreserveFamily (rather than through
// extractFamilyAndVariant) so this stays a pure unit test of that one
// function, independent of the kimi family_pattern regex (now separately
// boundary-tightened and guarded by TestFamilyPatternBoundaries above).
func TestMatchPreserveFamily_KimiTokenPrefixSemantics(t *testing.T) {
	config := &ModelUnificationConfig{}
	config.SpecialRules.PreserveFamily = []string{"kimi"}

	tests := []struct {
		name      string
		modelName string
		arch      string
		want      string
	}{
		{"exact name match", "kimi", "", "kimi"},
		{"name with hyphen boundary", "kimi-k2-instruct", "", "kimi"},
		{"name with dot boundary", "kimi.k2", "", "kimi"},
		{"arch exact match", "unrelated-name", "kimi", "kimi"},
		{"unrelated name sharing a prefix must not match", "kimichef-70b", "", ""},
		{"arch that merely contains kimi as a substring must not match", "unrelated", "not-kimi-arch", ""},

		// Publisher-prefixed names strip everything up to and including the
		// last '/' before the token-prefix check runs.
		{"publisher-prefixed name matches on the model part", "moonshotai/kimi-k2-instruct", "", "kimi"},
		{"publisher-prefixed name with deepseek arch still matches on name", "moonshotai/kimi-k2-instruct", "deepseek", "kimi"},
		{"publisher segment sharing the entry name must not match", "kimi/llama-3-8b", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := matchPreserveFamily(tt.modelName, tt.arch, config)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestExtractFamilyAndVariant_PlainDeepseekUnaffectedByKimiPreserve confirms
// adding "kimi" to preserve_family didn't collaterally change plain
// deepseek model classification - the two entries are independent.
func TestExtractFamilyAndVariant_PlainDeepseekUnaffectedByKimiPreserve(t *testing.T) {
	tests := []struct {
		name            string
		modelName       string
		arch            string
		expectedFamily  string
		expectedVariant string
	}{
		{"deepseek-v3", "deepseek-v3", "", "deepseek", "v3"},
		{"deepseek-r1-distill-qwen-7b", "deepseek-r1-distill-qwen-7b", "", "deepseek", "r1-distill-qwen-7b"},
		{"arch deepseek", "model", "deepseek", "deepseek", ""},
		{"arch deepseek2 via alias fallback", "model", "deepseek2", "deepseek", "2"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			family, variant := extractFamilyAndVariant(tt.modelName, tt.arch)
			assert.Equal(t, tt.expectedFamily, family, "Family mismatch")
			assert.Equal(t, tt.expectedVariant, variant, "Variant mismatch")
		})
	}
}

// TestExtractFromDelimiters_UsesConfiguredAliases exercises the FamilyAliases
// delimiter fallback directly, with a synthetic config, rather than through
// the full extractFamilyAndVariant path against production config. Every
// alias in the shipped config now also has its own family_pattern (that's
// how the 2026 model families were added), so any real model name that
// starts with an alias key gets resolved at the pattern stage first and
// never reaches this fallback - a synthetic config keeps the fallback itself
// under test regardless of how the shipped patterns evolve.
func TestExtractFromDelimiters_UsesConfiguredAliases(t *testing.T) {
	config := &ModelUnificationConfig{}
	config.ModelExtraction.FamilyAliases = map[string]string{
		"widgetcoder": "widget",
	}

	family := extractFromDelimiters("widgetcoder-24b", config)
	assert.Equal(t, "widget", family)
}

// TestExtractFromDelimiters_NilConfigFallsBackToKnownFamilies confirms the
// nil-config guard added when this became config-driven: a nil config must
// not panic, and the hardcoded knownDelimiterFamilies list still resolves.
func TestExtractFromDelimiters_NilConfigFallsBackToKnownFamilies(t *testing.T) {
	family := extractFromDelimiters("llama-3.2-1b", nil)
	assert.Equal(t, "llama", family)
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
