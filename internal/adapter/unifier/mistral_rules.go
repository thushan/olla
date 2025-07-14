package unifier

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/core/ports"
)

// mistralModelRule handles Mistral-specific model patterns
type mistralModelRule struct {
	normalizer ports.ModelNormalizer
}

func (r *mistralModelRule) CanHandle(modelInfo *domain.ModelInfo) bool {
	nameLower := strings.ToLower(modelInfo.Name)
	
	// Check if it's a Mistral model by publisher or known patterns
	if strings.Contains(modelInfo.Name, "mistralai/") {
		return true
	}
	
	// Check for known Mistral model patterns
	mistralPatterns := []string{"devstral", "magistral", "mixtral", "mistral"}
	for _, pattern := range mistralPatterns {
		if strings.Contains(nameLower, pattern) {
			return true
		}
	}
	
	return false
}

func (r *mistralModelRule) Apply(modelInfo *domain.ModelInfo) (*domain.UnifiedModel, error) {
	// Extract model components
	modelName := modelInfo.Name
	nameLower := strings.ToLower(modelName)
	
	// Remove publisher prefix if present
	cleanName := modelName
	if strings.Contains(modelName, "/") {
		parts := strings.Split(modelName, "/")
		cleanName = parts[len(parts)-1]
	}
	
	// Determine variant
	var variant string
	var inferredSize string
	
	switch {
	case strings.Contains(nameLower, "devstral"):
		variant = "devstral"
		// Check for specific devstral versions
		if strings.Contains(nameLower, "small-2505") {
			inferredSize = "small"
		} else if strings.Contains(nameLower, "small") {
			inferredSize = "small"
		}
	case strings.Contains(nameLower, "magistral"):
		variant = "magistral"
		if strings.Contains(nameLower, "small") {
			inferredSize = "small"
		}
	case strings.Contains(nameLower, "mixtral"):
		variant = "mixtral"
		// Mixtral models are typically larger
		if strings.Contains(nameLower, "8x7b") {
			inferredSize = "46.7b" // 8x7B MoE
		} else if strings.Contains(nameLower, "8x22b") {
			inferredSize = "141b" // 8x22B MoE
		}
	default:
		// Extract variant from name (e.g., mistral-7b -> variant="", mistral-instruct -> variant="instruct")
		variant = extractMistralVariant(cleanName)
	}
	
	// Extract size
	var sizeStr string
	var paramCount int64
	
	// First try to get size from model details
	if modelInfo.Details != nil && modelInfo.Details.ParameterSize != nil {
		sizeStr = *modelInfo.Details.ParameterSize
	}
	
	// If no size in details or it's empty, try to extract from name
	if sizeStr == "" || sizeStr == "unknown" {
		extractedSize, _ := extractSizeFromName(modelName)
		if extractedSize != "" {
			sizeStr = extractedSize
		} else if inferredSize != "" {
			sizeStr = inferredSize
		}
	}
	
	// Normalize the size
	normalizedSize, paramCount := r.normalizer.NormalizeSize(sizeStr)
	
	// Extract quantization
	var quantStr string
	if modelInfo.Details != nil && modelInfo.Details.QuantizationLevel != nil {
		quantStr = *modelInfo.Details.QuantizationLevel
	}
	normalizedQuant := r.normalizer.NormalizeQuantization(quantStr)
	
	// Generate canonical ID
	canonicalID := r.normalizer.GenerateCanonicalID("mistral", variant, normalizedSize, normalizedQuant)
	
	// Extract format
	format := "gguf" // Most Mistral models are GGUF
	if modelInfo.Details != nil && modelInfo.Details.Format != nil {
		format = *modelInfo.Details.Format
	}
	
	// Create metadata
	metadata := make(map[string]interface{})
	metadata["source"] = "mistral"
	if strings.Contains(modelName, "mistralai/") {
		metadata["publisher"] = "mistralai"
	}
	
	// Add specific metadata based on variant
	switch variant {
	case "devstral":
		metadata["purpose"] = "code"
		metadata["model_type"] = "code"
	case "magistral":
		metadata["purpose"] = "general"
	case "mixtral":
		metadata["architecture"] = "moe" // Mixture of Experts
	}
	
	if modelInfo.Details != nil {
		if modelInfo.Details.Digest != nil {
			metadata["digest"] = *modelInfo.Details.Digest
		}
		if modelInfo.Details.Type != nil {
			metadata["type"] = *modelInfo.Details.Type
		}
	}
	
	// Determine capabilities based on variant
	capabilities := []string{"completion"}
	switch variant {
	case "devstral":
		capabilities = append(capabilities, "code")
	case "magistral":
		capabilities = append(capabilities, "chat", "reasoning")
	case "mixtral":
		capabilities = append(capabilities, "chat")
	default:
		if strings.Contains(nameLower, "instruct") || strings.Contains(nameLower, "chat") {
			capabilities = append(capabilities, "chat")
		}
	}
	
	// Extract context length
	var maxContext *int64
	if modelInfo.Details != nil && modelInfo.Details.MaxContextLength != nil {
		maxContext = modelInfo.Details.MaxContextLength
	}
	
	unified := &domain.UnifiedModel{
		ID:               canonicalID,
		Family:           "mistral",
		Variant:          variant,
		ParameterSize:    normalizedSize,
		ParameterCount:   paramCount,
		Quantization:     normalizedQuant,
		Format:           format,
		Aliases:          []domain.AliasEntry{{Name: modelInfo.Name, Source: "mistral"}},
		SourceEndpoints:  []domain.SourceEndpoint{},
		Capabilities:     capabilities,
		MaxContextLength: maxContext,
		Metadata:         metadata,
	}
	
	// Generate better aliases
	aliases := r.generateMistralAliases(unified, modelInfo.Name)
	unified.Aliases = append(unified.Aliases, aliases...)
	
	return unified, nil
}

func (r *mistralModelRule) GetPriority() int {
	return 95 // High priority for Mistral-specific handling
}

func (r *mistralModelRule) GetName() string {
	return "mistral_model_rule"
}

// extractMistralVariant extracts the variant from a Mistral model name
func extractMistralVariant(name string) string {
	nameLower := strings.ToLower(name)
	
	// Known Mistral variants
	variants := []string{
		"devstral",
		"magistral", 
		"mixtral",
		"instruct",
		"chat",
		"base",
	}
	
	for _, v := range variants {
		if strings.Contains(nameLower, v) {
			return v
		}
	}
	
	// Check for version patterns (e.g., mistral-7b-v0.3)
	if matches := regexp.MustCompile(`v(\d+(?:\.\d+)?)`).FindStringSubmatch(nameLower); len(matches) > 1 {
		return "v" + matches[1]
	}
	
	return ""
}

// generateMistralAliases generates appropriate aliases for Mistral models
func (r *mistralModelRule) generateMistralAliases(model *domain.UnifiedModel, originalName string) []domain.AliasEntry {
	aliases := []domain.AliasEntry{}
	
	// Clean name without publisher
	cleanName := originalName
	if strings.Contains(originalName, "/") {
		parts := strings.Split(originalName, "/")
		cleanName = parts[len(parts)-1]
		// Also add the clean name as an alias
		aliases = append(aliases, domain.AliasEntry{Name: cleanName, Source: "generated"})
	}
	
	// Generate standard aliases based on variant
	baseAlias := model.Family
	if model.Variant != "" && model.Variant != "unknown" {
		baseAlias = fmt.Sprintf("%s-%s", model.Family, model.Variant)
	}
	
	// Add size-based aliases
	if model.ParameterSize != "unknown" {
		sizeAlias := fmt.Sprintf("%s:%s", baseAlias, model.ParameterSize)
		aliases = append(aliases, domain.AliasEntry{Name: sizeAlias, Source: "generated"})
		
		// Add quantization variant
		if model.Quantization != "unk" {
			fullAlias := fmt.Sprintf("%s:%s-%s", baseAlias, model.ParameterSize, model.Quantization)
			aliases = append(aliases, domain.AliasEntry{Name: fullAlias, Source: "generated"})
		}
	}
	
	// Add publisher variants if originally had publisher
	if strings.Contains(originalName, "mistralai/") {
		if model.Variant != "" && model.Variant != "unknown" {
			publisherAlias := fmt.Sprintf("mistralai/%s-%s", model.Family, model.Variant)
			aliases = append(aliases, domain.AliasEntry{Name: publisherAlias, Source: "generated"})
			
			if model.ParameterSize != "unknown" {
				publisherSizeAlias := fmt.Sprintf("mistralai/%s-%s-%s", model.Family, model.Variant, model.ParameterSize)
				aliases = append(aliases, domain.AliasEntry{Name: publisherSizeAlias, Source: "generated"})
			}
		}
	}
	
	return aliases
}