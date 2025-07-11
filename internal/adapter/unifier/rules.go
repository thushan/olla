package unifier

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/core/ports"
)

// ollamaPhiFamilyRule handles the phi4 misclassification issue in Ollama
type ollamaPhiFamilyRule struct {
	normalizer ports.ModelNormalizer
}

func (r *ollamaPhiFamilyRule) CanHandle(modelInfo *domain.ModelInfo) bool {
	// Check if this is a phi4 model misclassified as phi3
	nameLower := strings.ToLower(modelInfo.Name)
	if strings.Contains(nameLower, "phi4") || strings.Contains(nameLower, "phi-4") {
		if modelInfo.Details != nil && modelInfo.Details.Family != nil {
			return *modelInfo.Details.Family == "phi3" // Misclassified
		}
	}
	return false
}

func (r *ollamaPhiFamilyRule) Apply(modelInfo *domain.ModelInfo) (*domain.UnifiedModel, error) {
	// Extract correct family and variant
	family := "phi"
	variant := "4"

	// Extract size
	var sizeStr string
	if modelInfo.Details != nil && modelInfo.Details.ParameterSize != nil {
		sizeStr = *modelInfo.Details.ParameterSize
	}
	normalizedSize, paramCount := r.normalizer.NormalizeSize(sizeStr)

	// Extract quantization
	var quantStr string
	if modelInfo.Details != nil && modelInfo.Details.QuantizationLevel != nil {
		quantStr = *modelInfo.Details.QuantizationLevel
	}
	normalizedQuant := r.normalizer.NormalizeQuantization(quantStr)

	// Generate canonical ID
	canonicalID := r.normalizer.GenerateCanonicalID(family, variant, normalizedSize, normalizedQuant)

	// Extract format
	format := "gguf" // Default for Ollama
	if modelInfo.Details != nil && modelInfo.Details.Format != nil {
		format = *modelInfo.Details.Format
	}

	// Build unified model
	unified := &domain.UnifiedModel{
		ID:             canonicalID,
		Family:         family,
		Variant:        variant,
		ParameterSize:  normalizedSize,
		ParameterCount: paramCount,
		Quantization:   normalizedQuant,
		Format:         format,
		Aliases:        []domain.AliasEntry{{Name: modelInfo.Name, Source: "ollama"}},
		Capabilities:   []string{"chat", "completion"}, // Phi models are generally chat-capable
		Metadata:       make(map[string]interface{}),
		PromptTemplateID: "chatml", // Phi models typically use ChatML
	}

	// Add context length if available
	if modelInfo.Details != nil && modelInfo.Details.MaxContextLength != nil {
		unified.MaxContextLength = modelInfo.Details.MaxContextLength
	}

	// Add metadata
	if modelInfo.Details != nil {
		if modelInfo.Details.Digest != nil {
			unified.Metadata["digest"] = *modelInfo.Details.Digest
		}
		if modelInfo.Details.Publisher != nil {
			unified.Metadata["publisher"] = *modelInfo.Details.Publisher
		}
	}

	return unified, nil
}

func (r *ollamaPhiFamilyRule) GetPriority() int {
	return 100 // High priority for specific fixes
}

func (r *ollamaPhiFamilyRule) GetName() string {
	return "ollama_phi_family_fix"
}

// ollamaHuggingFaceRule handles Hugging Face models in Ollama
type ollamaHuggingFaceRule struct {
	normalizer ports.ModelNormalizer
}

func (r *ollamaHuggingFaceRule) CanHandle(modelInfo *domain.ModelInfo) bool {
	return strings.Contains(modelInfo.Name, "hf.co/")
}

func (r *ollamaHuggingFaceRule) Apply(modelInfo *domain.ModelInfo) (*domain.UnifiedModel, error) {
	// Parse HF model name: hf.co/org/model:quantization
	parts := strings.Split(modelInfo.Name, "/")
	if len(parts) < 3 {
		return nil, fmt.Errorf("invalid huggingface model name: %s", modelInfo.Name)
	}

	// Extract organization and model name
	org := parts[1]
	modelPart := strings.Join(parts[2:], "/")

	// Split model and quantization
	modelParts := strings.Split(modelPart, ":")
	modelName := modelParts[0]
	
	// Try to extract family and variant from model name
	var platformFamily string
	if modelInfo.Details != nil && modelInfo.Details.Family != nil {
		platformFamily = *modelInfo.Details.Family
	}
	family, variant := r.normalizer.NormalizeFamily(modelName, platformFamily)

	// Extract size
	var sizeStr string
	if modelInfo.Details != nil && modelInfo.Details.ParameterSize != nil {
		sizeStr = *modelInfo.Details.ParameterSize
	}
	normalizedSize, paramCount := r.normalizer.NormalizeSize(sizeStr)

	// Extract quantization
	var quantStr string
	if len(modelParts) > 1 {
		quantStr = modelParts[1]
	} else if modelInfo.Details != nil && modelInfo.Details.QuantizationLevel != nil {
		quantStr = *modelInfo.Details.QuantizationLevel
	}
	normalizedQuant := r.normalizer.NormalizeQuantization(quantStr)

	// Generate canonical ID
	canonicalID := r.normalizer.GenerateCanonicalID(family, variant, normalizedSize, normalizedQuant)

	// Build unified model
	unified := &domain.UnifiedModel{
		ID:             canonicalID,
		Family:         family,
		Variant:        variant,
		ParameterSize:  normalizedSize,
		ParameterCount: paramCount,
		Quantization:   normalizedQuant,
		Format:         "gguf",
		Aliases:        []domain.AliasEntry{
			{Name: modelInfo.Name, Source: "ollama"},
			{Name: fmt.Sprintf("%s/%s", org, modelName), Source: "huggingface"},
		},
		Capabilities:   inferCapabilitiesFromName(modelName),
		Metadata: map[string]interface{}{
			"organization": org,
			"source":       "huggingface",
		},
	}

	// Add additional metadata
	if modelInfo.Details != nil {
		if modelInfo.Details.MaxContextLength != nil {
			unified.MaxContextLength = modelInfo.Details.MaxContextLength
		}
		if modelInfo.Details.Digest != nil {
			unified.Metadata["digest"] = *modelInfo.Details.Digest
		}
	}

	return unified, nil
}

func (r *ollamaHuggingFaceRule) GetPriority() int {
	return 90
}

func (r *ollamaHuggingFaceRule) GetName() string {
	return "ollama_huggingface"
}

// lmstudioVendorPrefixRule handles LM Studio vendor-prefixed models
type lmstudioVendorPrefixRule struct {
	normalizer ports.ModelNormalizer
}

func (r *lmstudioVendorPrefixRule) CanHandle(modelInfo *domain.ModelInfo) bool {
	// LM Studio models often have vendor/model format
	return strings.Count(modelInfo.Name, "/") >= 1
}

func (r *lmstudioVendorPrefixRule) Apply(modelInfo *domain.ModelInfo) (*domain.UnifiedModel, error) {
	// Parse vendor/model format
	parts := strings.SplitN(modelInfo.Name, "/", 2)
	vendor := parts[0]
	modelName := parts[1]

	// Extract family and variant
	var platformFamily string
	if modelInfo.Details != nil && modelInfo.Details.Family != nil {
		platformFamily = *modelInfo.Details.Family
	}
	family, variant := r.normalizer.NormalizeFamily(modelName, platformFamily)

	// For LM Studio, parameter size is often in the model ID
	sizeRegex := regexp.MustCompile(`(?i)(\d+(?:\.\d+)?)b`)
	var normalizedSize string
	var paramCount int64

	// Special handling for known models
	if strings.Contains(strings.ToLower(modelName), "phi-4") || strings.Contains(strings.ToLower(modelName), "phi4") {
		// Phi-4 is known to be 14.7B
		normalizedSize = "14.7b"
		paramCount = 14700000000
	} else if matches := sizeRegex.FindStringSubmatch(modelName); len(matches) > 1 {
		// matches[0] is the full match like "8b"
		normalizedSize, paramCount = r.normalizer.NormalizeSize(matches[0])
	} else if modelInfo.Details != nil && modelInfo.Details.ParameterSize != nil {
		normalizedSize, paramCount = r.normalizer.NormalizeSize(*modelInfo.Details.ParameterSize)
	} else {
		normalizedSize = "unknown"
		paramCount = 0
	}

	// Extract quantization
	var quantStr string
	if modelInfo.Details != nil && modelInfo.Details.QuantizationLevel != nil {
		quantStr = *modelInfo.Details.QuantizationLevel
	}
	normalizedQuant := r.normalizer.NormalizeQuantization(quantStr)

	// Generate canonical ID
	canonicalID := r.normalizer.GenerateCanonicalID(family, variant, normalizedSize, normalizedQuant)

	// Determine format
	format := "gguf" // Default
	if modelInfo.Details != nil && modelInfo.Details.Format != nil {
		format = *modelInfo.Details.Format
	}

	// Build unified model
	unified := &domain.UnifiedModel{
		ID:             canonicalID,
		Family:         family,
		Variant:        variant,
		ParameterSize:  normalizedSize,
		ParameterCount: paramCount,
		Quantization:   normalizedQuant,
		Format:         format,
		Aliases:        []domain.AliasEntry{
			{Name: modelInfo.Name, Source: "lmstudio"},
			{Name: modelName, Source: "lmstudio"},
		},
		Capabilities:   inferCapabilitiesFromName(modelName),
		Metadata: map[string]interface{}{
			"vendor":    vendor,
			"publisher": vendor,
		},
	}

	// Add LM Studio specific metadata
	if modelInfo.Details != nil {
		if modelInfo.Details.MaxContextLength != nil {
			unified.MaxContextLength = modelInfo.Details.MaxContextLength
		}
		if modelInfo.Details.Type != nil {
			unified.Metadata["model_type"] = *modelInfo.Details.Type
			if *modelInfo.Details.Type == "vlm" {
				unified.Capabilities = append(unified.Capabilities, "vision")
			}
		}
		if modelInfo.Details.State != nil {
			unified.Metadata["initial_state"] = *modelInfo.Details.State
		}
	}

	return unified, nil
}

func (r *lmstudioVendorPrefixRule) GetPriority() int {
	return 80
}

func (r *lmstudioVendorPrefixRule) GetName() string {
	return "lmstudio_vendor_prefix"
}

// genericModelRule is a fallback rule for any model
type genericModelRule struct {
	normalizer ports.ModelNormalizer
}

func (r *genericModelRule) CanHandle(modelInfo *domain.ModelInfo) bool {
	return true // Can handle any model
}

func (r *genericModelRule) Apply(modelInfo *domain.ModelInfo) (*domain.UnifiedModel, error) {
	// Extract family and variant
	var platformFamily string
	if modelInfo.Details != nil && modelInfo.Details.Family != nil {
		platformFamily = *modelInfo.Details.Family
	}
	family, variant := r.normalizer.NormalizeFamily(modelInfo.Name, platformFamily)

	// Extract size
	var sizeStr string
	var normalizedSize string
	var paramCount int64
	
	// First try to get from details
	if modelInfo.Details != nil && modelInfo.Details.ParameterSize != nil {
		sizeStr = *modelInfo.Details.ParameterSize
		normalizedSize, paramCount = r.normalizer.NormalizeSize(sizeStr)
	} else {
		// Try to extract from model name
		sizeRegex := regexp.MustCompile(`(?i)(\d+(?:\.\d+)?)[bmk]`)
		if matches := sizeRegex.FindStringSubmatch(modelInfo.Name); len(matches) > 0 {
			normalizedSize, paramCount = r.normalizer.NormalizeSize(matches[0])
		} else {
			normalizedSize = "unknown"
			paramCount = 0
		}
	}

	// Extract quantization
	var quantStr string
	if modelInfo.Details != nil && modelInfo.Details.QuantizationLevel != nil {
		quantStr = *modelInfo.Details.QuantizationLevel
	}
	normalizedQuant := r.normalizer.NormalizeQuantization(quantStr)

	// Generate canonical ID
	canonicalID := r.normalizer.GenerateCanonicalID(family, variant, normalizedSize, normalizedQuant)

	// Extract format
	format := "unknown"
	if modelInfo.Details != nil && modelInfo.Details.Format != nil {
		format = *modelInfo.Details.Format
	}

	// Build unified model
	unified := &domain.UnifiedModel{
		ID:             canonicalID,
		Family:         family,
		Variant:        variant,
		ParameterSize:  normalizedSize,
		ParameterCount: paramCount,
		Quantization:   normalizedQuant,
		Format:         format,
		Aliases:        []domain.AliasEntry{{Name: modelInfo.Name, Source: "*"}},
		Capabilities:   inferCapabilitiesFromName(modelInfo.Name),
		Metadata:       make(map[string]interface{}),
	}

	// Add all available metadata
	if modelInfo.Details != nil {
		if modelInfo.Details.MaxContextLength != nil {
			unified.MaxContextLength = modelInfo.Details.MaxContextLength
		}
		if modelInfo.Details.Digest != nil {
			unified.Metadata["digest"] = *modelInfo.Details.Digest
		}
		if modelInfo.Details.Publisher != nil {
			unified.Metadata["publisher"] = *modelInfo.Details.Publisher
		}
		if modelInfo.Details.Type != nil {
			unified.Metadata["model_type"] = *modelInfo.Details.Type
		}
		if modelInfo.Details.State != nil {
			unified.Metadata["state"] = *modelInfo.Details.State
		}
		if modelInfo.Details.ParentModel != nil {
			unified.Metadata["parent_model"] = *modelInfo.Details.ParentModel
		}
	}

	return unified, nil
}

func (r *genericModelRule) GetPriority() int {
	return 10 // Low priority - fallback rule
}

func (r *genericModelRule) GetName() string {
	return "generic_model"
}

// Helper function to infer capabilities from model name
func inferCapabilitiesFromName(modelName string) []string {
	capabilitySet := make(map[string]bool)
	
	// Apply capability rules
	for _, rule := range CapabilityRules {
		matches := false
		
		// Check name patterns
		if len(rule.NamePatterns) > 0 && containsAny(modelName, rule.NamePatterns) {
			matches = true
		}
		
		// Check architecture in name
		if rule.Architecture != "" && matchesPattern(modelName, rule.Architecture) {
			matches = true
		}
		
		// Add capabilities if rule matches
		if matches {
			for _, cap := range rule.Capabilities {
				capabilitySet[cap] = true
			}
		}
	}
	
	// Legacy pattern matching for backward compatibility
	nameLower := strings.ToLower(modelName)
	patterns := map[string]string{
		"chat":     "chat",
		"instruct": "chat",
		"code":     "code",
		"coder":    "code",
		"vision":   "vision",
		"vlm":      "vision",
		"embed":    "embeddings",
		"embedding": "embeddings",
		"rerank":   "reranking",
	}

	for pattern, capability := range patterns {
		if strings.Contains(nameLower, pattern) {
			capabilitySet[capability] = true
		}
	}

	// Default to completion if no specific capabilities found
	if len(capabilitySet) == 0 {
		capabilitySet["completion"] = true
	}

	// Convert set to slice
	var capabilities []string
	for cap := range capabilitySet {
		capabilities = append(capabilities, cap)
	}

	return capabilities
}