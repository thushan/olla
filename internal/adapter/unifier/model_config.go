package unifier

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"

	"github.com/thushan/olla/internal/logger"
)

// ModelUnificationConfig represents the model unification configuration.
// This reads from models.yaml and handles model name normalization across
// different platforms (e.g., "llama-3.1-8b" vs "meta-llama-3.1-8b-instruct").
// Not to be confused with profile configs which define platform behavior.
type ModelUnificationConfig struct {
	ModelExtraction struct {
		ArchitectureMappings map[string]string `yaml:"architecture_mappings"`
		FamilyAliases        map[string]string `yaml:"family_aliases"`
		PublisherMappings    map[string]string `yaml:"publisher_mappings"`
		FamilyPatterns       []PatternConfig   `yaml:"family_patterns"`
	} `yaml:"model_extraction"`

	Quantization struct {
		Mappings map[string]string `yaml:"mappings"`
	} `yaml:"quantization"`

	Capabilities struct {
		TypeCapabilities  map[string][]string `yaml:"type_capabilities"`
		ContextThresholds map[string]int64    `yaml:"context_thresholds"`
		NamePatterns      []NamePatternConfig `yaml:"name_patterns"`
	} `yaml:"capabilities"`

	SpecialRules struct {
		PreserveFamily []string `yaml:"preserve_family"`
		GenericNames   []string `yaml:"generic_names"`
	} `yaml:"special_rules"`
}

// PatternConfig represents a pattern configuration
type PatternConfig struct {
	regex       *regexp.Regexp
	Pattern     string `yaml:"pattern"`
	Description string `yaml:"description"`

	FamilyGroup  int `yaml:"family_group"`
	VariantGroup int `yaml:"variant_group"`
}

// NamePatternConfig represents capability name patterns
type NamePatternConfig struct {
	regex        *regexp.Regexp
	Pattern      string   `yaml:"pattern"`
	Capabilities []string `yaml:"capabilities"`
}

var (
	configInstance *ModelUnificationConfig
	configOnce     sync.Once
	errConfig      error

	// configSource is the path models.yaml was loaded from, or "" when
	// embedded defaults are in use. configParseWarnings records any
	// candidate paths that were found but failed to parse, so a startup
	// diagnostic can explain *why* defaults were used rather than leaving
	// it silent (see issue #204).
	configSource        string
	configParseWarnings []string
)

// LoadModelConfig loads the model unification configuration
func LoadModelConfig() (*ModelUnificationConfig, error) {
	configOnce.Do(func() {
		configInstance, configSource, configParseWarnings = loadConfigFromFile()
		if configInstance == nil {
			return
		}

		if err := configInstance.compilePatterns(); err != nil {
			// the candidate parsed as valid YAML but at least one of its
			// patterns isn't a valid regex. Don't hand back an uncompiled
			// config - regex-driven extraction nil-guards on that, so it
			// would silently no-op rather than panic, which is exactly the
			// #204 class of silent failure. Record why and recover onto the
			// embedded defaults, which are known-good.
			if configSource != "" {
				configParseWarnings = append(configParseWarnings, fmt.Sprintf("%s: %v", configSource, err))
			}
			configSource = ""
			configInstance = getDefaultConfig()
			// defaults are hardcoded and covered by TestPatternCompilation,
			// so this should never fail - but never panic on the assumption,
			// so keep whatever it returns rather than assuming success.
			errConfig = configInstance.compilePatterns()
			return
		}

		errConfig = nil
	})
	return configInstance, errConfig
}

// ConfigSource returns the path models.yaml was loaded from, triggering the
// load if it hasn't happened yet. An empty string means embedded defaults
// are in use - check the returned warnings via LogConfigStatus to see why.
func ConfigSource() string {
	_, _ = LoadModelConfig()
	return configSource
}

// LogConfigStatus reports where the model unification config came from.
// Intended to be called once at startup (rather than relying on the lazy
// first-use load) so a broken config/models.yaml is surfaced immediately
// instead of silently degrading to embedded defaults on first inference
// request.
func LogConfigStatus(log logger.StyledLogger) {
	_, err := LoadModelConfig()
	if log == nil {
		// still trigger the load above so callers that only care about the
		// side effect (warming the singleton) work with a nil logger; there's
		// just nothing to log to
		return
	}
	if err != nil {
		log.Error("model unification config: failed to compile patterns, using embedded defaults", "error", err)
		return
	}

	// Warn about every found-but-unusable candidate even when a later
	// candidate loaded successfully - otherwise a broken ./models.yaml next
	// to a working config/models.yaml goes unmentioned, which is exactly the
	// silence issue #204 complained about. The wording must stay truthful to
	// what actually happened next: only claim "falling back to embedded
	// defaults" when that's genuinely what's active.
	if len(configParseWarnings) > 0 {
		msg := "model unification config: found unusable models.yaml candidate(s), falling back to embedded defaults"
		if configSource != "" {
			msg = fmt.Sprintf("model unification config: found unusable models.yaml candidate(s), using %s instead", configSource)
		}
		log.Warn(msg, "details", strings.Join(configParseWarnings, "; "))
	}

	if configSource != "" {
		log.Info("model unification config loaded", "source", configSource)
		return
	}

	if len(configParseWarnings) == 0 {
		log.Debug("model unification config: no models.yaml found, using embedded defaults")
	}
}

// loadConfigFromFile loads configuration from the YAML file, trying each
// candidate path in turn. Returns the loaded config, the path it came from
// (empty if falling back to defaults), and any parse failures encountered
// along the way for diagnostics.
func loadConfigFromFile() (*ModelUnificationConfig, string, []string) {
	paths := []string{
		"models.yaml",
		"config/models.yaml",
		"config-base/models.yaml",
		"../config/models.yaml",
		"../../config/models.yaml",
	}
	// filepath.Join("", "models.yaml") collapses to "models.yaml", which would
	// silently duplicate the first candidate (and double every warning) when
	// OLLA_CONFIG_DIR is unset - only add it when it's actually set.
	if configDir := os.Getenv("OLLA_CONFIG_DIR"); configDir != "" {
		paths = append(paths, filepath.Join(configDir, "models.yaml"))
	}

	var warnings []string
	seen := make(map[string]bool, len(paths))

	for _, path := range paths {
		if path == "" || seen[path] {
			continue
		}
		seen[path] = true

		data, err := os.ReadFile(path)
		if err != nil {
			// a candidate simply not existing is the overwhelmingly common
			// case (most of the search path is speculative) and not worth a
			// warning. Anything else - permission denied, a directory named
			// models.yaml, and so on - is a real problem the operator should
			// hear about, not silence.
			if !errors.Is(err, fs.ErrNotExist) {
				warnings = append(warnings, fmt.Sprintf("%s: %v", path, err))
			}
			continue
		}

		var config ModelUnificationConfig
		if err := yaml.Unmarshal(data, &config); err != nil {
			// found the file but it doesn't parse - record why so LogConfigStatus
			// can tell the operator instead of quietly using defaults
			warnings = append(warnings, fmt.Sprintf("%s: %v", path, err))
			continue
		}

		if config.isEffectivelyEmpty() {
			// an empty or comment-only file unmarshals without error, but
			// treating that as "loaded" repeats the #204 silent-failure class:
			// LogConfigStatus would report success while every lookup against
			// it returns nothing. Warn and let a later candidate (or defaults)
			// win instead.
			warnings = append(warnings, fmt.Sprintf("%s: parsed but contains no configuration", path))
			continue
		}

		return &config, path, warnings
	}

	return getDefaultConfig(), "", warnings
}

// isEffectivelyEmpty reports whether a successfully-parsed config carries no
// real configuration at all - the signature of an empty or comment-only YAML
// file, which is valid YAML but not a usable models.yaml. Every top-level
// section counts: a file that only sets family_aliases (say) is a legitimate
// partial override and must not be discarded as "empty".
func (c *ModelUnificationConfig) isEffectivelyEmpty() bool {
	return len(c.ModelExtraction.FamilyPatterns) == 0 &&
		len(c.ModelExtraction.FamilyAliases) == 0 &&
		len(c.ModelExtraction.ArchitectureMappings) == 0 &&
		len(c.ModelExtraction.PublisherMappings) == 0 &&
		len(c.Capabilities.NamePatterns) == 0 &&
		len(c.Capabilities.TypeCapabilities) == 0 &&
		len(c.Capabilities.ContextThresholds) == 0 &&
		len(c.Quantization.Mappings) == 0 &&
		len(c.SpecialRules.PreserveFamily) == 0 &&
		len(c.SpecialRules.GenericNames) == 0
}

// compilePatterns compiles all regex patterns in the configuration
func (c *ModelUnificationConfig) compilePatterns() error {
	for i := range c.ModelExtraction.FamilyPatterns {
		regex, err := regexp.Compile(c.ModelExtraction.FamilyPatterns[i].Pattern)
		if err != nil {
			return fmt.Errorf("failed to compile family pattern %s: %w",
				c.ModelExtraction.FamilyPatterns[i].Pattern, err)
		}
		c.ModelExtraction.FamilyPatterns[i].regex = regex
	}

	for i := range c.Capabilities.NamePatterns {
		regex, err := regexp.Compile("(?i)" + c.Capabilities.NamePatterns[i].Pattern)
		if err != nil {
			return fmt.Errorf("failed to compile capability pattern %s: %w",
				c.Capabilities.NamePatterns[i].Pattern, err)
		}
		c.Capabilities.NamePatterns[i].regex = regex
	}

	return nil
}

// getDefaultConfig returns a default configuration if file is not found
func getDefaultConfig() *ModelUnificationConfig {
	config := &ModelUnificationConfig{}

	config.ModelExtraction.FamilyPatterns = []PatternConfig{
		{
			Pattern:      `^(mistral|mixtral)[-_]?(.+)`,
			FamilyGroup:  1,
			VariantGroup: 2,
			Description:  "Mistral and Mixtral models",
		},
		{
			Pattern:      `^(llama|gemma|phi|qwen)[-_]?(\d+(?:\.\d+)?)`,
			FamilyGroup:  1,
			VariantGroup: 2,
			Description:  "Common families with version numbers",
		},
		{
			Pattern:      `^[^/]+/(phi|llama|gemma|qwen|mistral|glm|granite|nemotron|exaone|kimi|minimax|hunyuan|olmo|internlm|smollm)[-_]?(\d+(?:\.\d+)?)?`,
			FamilyGroup:  1,
			VariantGroup: 2,
			Description:  "Models with publisher prefix",
		},
		{
			Pattern:      `^(codellama|starcoder|vicuna|falcon|yi)[-_]?(\d+[bB]?)`,
			FamilyGroup:  1,
			VariantGroup: 2,
			Description:  "Code and specialised models",
		},
		{
			Pattern:      `^(gpt-oss)[-_:]?(.+)?`,
			FamilyGroup:  1,
			VariantGroup: 2,
			Description:  "OpenAI open-weight gpt-oss models",
		},
		{
			Pattern:      `^(gpt)[-_]?(2|j|neox)?`,
			FamilyGroup:  1,
			VariantGroup: 2,
			Description:  "GPT variants",
		},
		{
			Pattern:      `^(deepseek)[-_]?(.+)?`,
			FamilyGroup:  1,
			VariantGroup: 2,
			Description:  "DeepSeek models",
		},
		{
			Pattern:      `^(devstral|magistral|codestral|ministral)[-_]?(.+)?`,
			FamilyGroup:  1,
			VariantGroup: 2,
			Description:  "Mistral AI code/agent/edge variants (Devstral, Magistral, Codestral, Ministral)",
		},
		{
			Pattern:      `^(glm)[-_]?(\d+(?:\.\d+)?)?`,
			FamilyGroup:  1,
			VariantGroup: 2,
			Description:  "GLM models (Zhipu / Z.ai)",
		},
		{
			Pattern:      `^(kimi)[-_]?(.+)?`,
			FamilyGroup:  1,
			VariantGroup: 2,
			Description:  "Kimi models (Moonshot AI)",
		},
		{
			Pattern:      `^(granite)[-_]?(\d+(?:\.\d+)?)?`,
			FamilyGroup:  1,
			VariantGroup: 2,
			Description:  "IBM Granite models",
		},
		{
			Pattern:      `^(nemotron)[-_]?(.+)?`,
			FamilyGroup:  1,
			VariantGroup: 2,
			Description:  "NVIDIA Nemotron models",
		},
		{
			Pattern:      `^(exaone)[-_]?(\d+(?:\.\d+)?)?`,
			FamilyGroup:  1,
			VariantGroup: 2,
			Description:  "LG AI EXAONE models",
		},
		{
			Pattern:      `^(hunyuan)[-_]?(.+)?`,
			FamilyGroup:  1,
			VariantGroup: 2,
			Description:  "Tencent Hunyuan models",
		},
		{
			Pattern:      `^(minimax)[-_]?(.+)?`,
			FamilyGroup:  1,
			VariantGroup: 2,
			Description:  "MiniMax models",
		},
		{
			Pattern:      `^(seed-oss)[-_]?(.+)?`,
			FamilyGroup:  1,
			VariantGroup: 2,
			Description:  "ByteDance Seed-OSS models",
		},
		{
			Pattern:      `^(olmo)[-_]?(\d+(?:\.\d+)?)?`,
			FamilyGroup:  1,
			VariantGroup: 2,
			Description:  "Allen Institute for AI OLMo models",
		},
		{
			Pattern:      `^(internlm)[-_]?(\d+(?:\.\d+)?)?`,
			FamilyGroup:  1,
			VariantGroup: 2,
			Description:  "Shanghai AI Lab InternLM models",
		},
		{
			Pattern:      `^(smollm)[-_]?(\d+(?:\.\d+)?)?`,
			FamilyGroup:  1,
			VariantGroup: 2,
			Description:  "Hugging Face SmolLM models",
		},
		{
			// Command-R is genuinely locally runnable (GGUF exists) but CC-BY-NC
			// licensed - non-commercial only. Olla doesn't gate on licence, this
			// is just so a future reader isn't surprised.
			Pattern:      `^(command-r)[-_]?(.+)?`,
			FamilyGroup:  1,
			VariantGroup: 2,
			Description:  "Cohere Command-R models (CC-BY-NC licence - non-commercial only)",
		},
		{
			Pattern:      `^(?:[^/]+/)?(forge)[-_]?(.+)?`,
			FamilyGroup:  1,
			VariantGroup: 2,
			Description:  "TensorFoundry Forge models",
		},
	}

	config.ModelExtraction.FamilyAliases = map[string]string{
		"llama3":        "llama",
		"llama3.2":      "llama",
		"llama3.3":      "llama",
		"llama4":        "llama",
		"gemma2":        "gemma",
		"gemma3":        "gemma",
		"phi3":          "phi",
		"phi3.5":        "phi",
		"phi4":          "phi",
		"qwen2":         "qwen",
		"qwen2.5":       "qwen",
		"qwen3":         "qwen",
		"deepseek2":     "deepseek",
		"deepseek3":     "deepseek",
		"deepseek4":     "deepseek",
		"devstral":      "mistral",
		"qwen3.5":       "qwen",
		"gemma4":        "gemma",
		"glm4":          "glm",
		"glm4moe":       "glm",
		"granitemoe":    "granite",
		"granitehybrid": "granite",
		"exaone4":       "exaone",
		"magistral":     "mistral",
		"codestral":     "mistral",
		"ministral":     "mistral",
		"seed-oss":      "seed-oss",
		"seed_oss":      "seed-oss",
		"olmo2":         "olmo",
		"internlm2":     "internlm",
		"internlm3":     "internlm",
		"smollm2":       "smollm",
		"smollm3":       "smollm",
	}

	config.ModelExtraction.ArchitectureMappings = map[string]string{
		"phi3":          "phi",
		"phi3.5":        "phi",
		"phi4":          "phi",
		"llama":         "llama",
		"llama2":        "llama",
		"llama3":        "llama",
		"llama3.1":      "llama",
		"llama3.2":      "llama",
		"llama3.3":      "llama",
		"llama4":        "llama",
		"gemma":         "gemma",
		"gemma2":        "gemma",
		"gemma3":        "gemma",
		"gemma4":        "gemma",
		"mistral":       "mistral",
		"mixtral":       "mixtral",
		"qwen":          "qwen",
		"qwen2":         "qwen",
		"qwen2.5":       "qwen",
		"qwen3":         "qwen",
		"deepseek":      "deepseek",
		"yi":            "yi",
		"starcoder":     "starcoder",
		"codellama":     "codellama",
		"vicuna":        "vicuna",
		"falcon":        "falcon",
		"gpt2":          "gpt2",
		"gptj":          "gptj",
		"gptneox":       "gptneox",
		"bloom":         "bloom",
		"opt":           "opt",
		"mpt":           "mpt",
		"qwen3.5":       "qwen",
		"qwen3.6":       "qwen",
		"glm4":          "glm",
		"glm5":          "glm",
		"glm4moe":       "glm",
		"glm":           "glm",
		"granite":       "granite",
		"granitemoe":    "granite",
		"granitehybrid": "granite",
		"exaone":        "exaone",
		"exaone4":       "exaone",
		"kimi":          "kimi",
		"nemotron":      "nemotron",
		"hunyuan":       "hunyuan",
		"minimax":       "minimax",
		"seed-oss":      "seed-oss",
		"seed_oss":      "seed-oss",
		"olmo":          "olmo",
		"olmo2":         "olmo",
		"internlm":      "internlm",
		"internlm2":     "internlm",
		"internlm3":     "internlm",
		"smollm":        "smollm",
		"smollm2":       "smollm",
		"smollm3":       "smollm",
		"command-r":     "command-r",
	}

	config.Quantization.Mappings = map[string]string{
		"Q4_K_M":    "q4km",
		"Q4_K_S":    "q4ks",
		"Q3_K_L":    "q3kl",
		"Q3_K_M":    "q3km",
		"Q3_K_S":    "q3ks",
		"Q5_K_M":    "q5km",
		"Q5_K_S":    "q5ks",
		"Q6_K":      "q6k",
		"Q2_K":      "q2k",
		"Q4_0":      "q4",
		"Q4_1":      "q4_1",
		"Q5_0":      "q5",
		"Q5_1":      "q5_1",
		"Q8_0":      "q8",
		"F16":       "f16",
		"FP16":      "f16",
		"F32":       "f32",
		"FP32":      "f32",
		"BF16":      "bf16",
		"GPTQ_4BIT": "gptq4",
		"GPTQ-4BIT": "gptq4",
		"AWQ_4BIT":  "awq4",
		"AWQ-4BIT":  "awq4",
		"INT8":      "int8",
		"INT4":      "int4",
		"Q4_K_XL":   "q4kxl",
		"MXFP4":     "mxfp4",
		"NVFP4":     "nvfp4",
	}

	config.ModelExtraction.PublisherMappings = map[string]string{
		"llama":     "meta",
		"codellama": "meta",
		"gemma":     "google",
		"phi":       "microsoft",
		"mistral":   "mistral",
		"mixtral":   "mistral",
		"qwen":      "alibaba",
		"deepseek":  "deepseek",
		"yi":        "01-ai",
		"starcoder": "bigcode",
		"falcon":    "tii",
		"vicuna":    "lmsys",
		"bloom":     "bigscience",
		"opt":       "meta",
		"gpt2":      "openai",
		"gptj":      "eleutherai",
		"gptneox":   "eleutherai",
		"devstral":  "mistral",
		"magistral": "mistral",
		"codestral": "mistral",
		"ministral": "mistral",
		"glm":       "zai-org",
		"gpt-oss":   "openai",
		"kimi":      "moonshotai",
		"granite":   "ibm",
		"nemotron":  "nvidia",
		"exaone":    "lg-ai",
		"hunyuan":   "tencent",
		"minimax":   "minimax",
		"seed-oss":  "bytedance",
		"olmo":      "allenai",
		"internlm":  "internlm",
		"smollm":    "huggingfacetb",
		"command-r": "cohere",
		"forge":     "tensorfoundry",
	}

	config.Capabilities.TypeCapabilities = map[string][]string{
		"llm":        {"text-generation", "chat", "completion"},
		"vlm":        {"text-generation", "vision", "multimodal", "image-understanding"},
		"embeddings": {"embeddings", "similarity", "vector-search"},
		"embedding":  {"embeddings", "similarity", "vector-search"},
	}

	config.Capabilities.NamePatterns = []NamePatternConfig{
		{
			Pattern:      "(code|coder|codegen|starcoder)",
			Capabilities: []string{"code-generation", "programming", "code-completion"},
		},
		{
			Pattern:      "(instruct|chat|assistant)",
			Capabilities: []string{"instruction-following", "chat"},
		},
		{
			Pattern:      "(reasoning|think)",
			Capabilities: []string{"reasoning", "logic"},
		},
		{
			Pattern:      "(math|mathstral)",
			Capabilities: []string{"mathematics", "problem-solving"},
		},
		{
			Pattern:      "(vision|vlm|llava|bakllava)",
			Capabilities: []string{"vision", "multimodal", "image-understanding"},
		},
	}

	config.Capabilities.ContextThresholds = map[string]int64{
		"extended_context":   32000,
		"long_context":       100000,
		"ultra_long_context": 1000000,
	}

	config.SpecialRules.PreserveFamily = []string{"nomic-bert", "deepseek-coder-v2"}
	config.SpecialRules.GenericNames = []string{"model", "unknown", "test", "temp", "default"}

	return config
}
