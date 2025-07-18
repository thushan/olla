package unifier

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sync"

	"gopkg.in/yaml.v3"
)

// ModelUnificationConfig represents the model unification configuration
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
)

// LoadModelConfig loads the model unification configuration
func LoadModelConfig() (*ModelUnificationConfig, error) {
	configOnce.Do(func() {
		configInstance = loadConfigFromFile()
		if configInstance != nil {
			errConfig = configInstance.compilePatterns()
		}
	})
	return configInstance, errConfig
}

// loadConfigFromFile loads configuration from the YAML file
func loadConfigFromFile() *ModelUnificationConfig {
	paths := []string{
		"models.yml",
		"config/models.yml",
		"../config/models.yml",
		"../../config/models.yml",
		filepath.Join(os.Getenv("OLLA_CONFIG_DIR"), "models.yml"),
	}

	for _, path := range paths {
		if path == "" {
			continue
		}

		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}

		var config ModelUnificationConfig
		if err := yaml.Unmarshal(data, &config); err != nil {
			continue
		}

		return &config
	}

	return getDefaultConfig()
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
			Description:  "Common families with versions",
		},
		{
			Pattern:      `^[^/]+/(phi|llama|gemma|qwen|mistral)[-_]?(\d+(?:\.\d+)?)`,
			FamilyGroup:  1,
			VariantGroup: 2,
			Description:  "Publisher-prefixed models",
		},
		{
			Pattern:      `^(codellama|starcoder|vicuna|falcon|yi)[-_]?(\d+[bB]?)`,
			FamilyGroup:  1,
			VariantGroup: 2,
			Description:  "Code and specialized models",
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
	}

	config.ModelExtraction.ArchitectureMappings = map[string]string{
		"phi3":      "phi",
		"phi3.5":    "phi",
		"phi4":      "phi",
		"llama":     "llama",
		"llama2":    "llama",
		"llama3":    "llama",
		"llama3.1":  "llama",
		"llama3.2":  "llama",
		"llama3.3":  "llama",
		"llama4":    "llama",
		"gemma":     "gemma",
		"gemma2":    "gemma",
		"gemma3":    "gemma",
		"mistral":   "mistral",
		"mixtral":   "mixtral",
		"qwen":      "qwen",
		"qwen2":     "qwen",
		"qwen2.5":   "qwen",
		"qwen3":     "qwen",
		"deepseek":  "deepseek",
		"yi":        "yi",
		"starcoder": "starcoder",
		"codellama": "codellama",
		"vicuna":    "vicuna",
		"falcon":    "falcon",
		"gpt2":      "gpt2",
		"gptj":      "gptj",
		"gptneox":   "gptneox",
		"bloom":     "bloom",
		"opt":       "opt",
		"mpt":       "mpt",
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

	config.SpecialRules.GenericNames = []string{"model", "unknown", "test", "temp", "default"}

	return config
}
