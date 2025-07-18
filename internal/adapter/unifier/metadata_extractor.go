package unifier

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
)

var (
	configCache     *ModelUnificationConfig
	configCacheOnce sync.Once
)

// getConfig loads configuration once and caches for performance.
// Thread-safe via sync.Once.
func getConfig() *ModelUnificationConfig {
	configCacheOnce.Do(func() {
		config, err := LoadModelConfig()
		if err != nil {
			config = getDefaultConfig()
		}
		configCache = config
	})
	return configCache
}

// normalizeQuantization converts various quantization formats to a canonical form
func normalizeQuantization(quant string) string {
	if quant == "" {
		return ""
	}

	config := getConfig()
	quant = strings.ToUpper(quant)

	// Use mappings from configuration
	if normalized, exists := config.Quantization.Mappings[quant]; exists {
		return normalized
	}

	// Try to extract quantization from model name patterns
	// e.g., "llama-3.2-1b-instruct-q4_k_m.gguf" -> "q4km"
	patterns := []struct {
		regex       *regexp.Regexp
		replacement string
	}{
		{regexp.MustCompile(`(?i)q(\d+)_k_([msl])`), "q$1k$2"},
		{regexp.MustCompile(`(?i)q(\d+)_(\d+)`), "q$1_$2"},
		{regexp.MustCompile(`(?i)q(\d+)`), "q$1"},
		{regexp.MustCompile(`(?i)f(\d+)`), "f$1"},
		{regexp.MustCompile(`(?i)fp(\d+)`), "f$1"},
		{regexp.MustCompile(`(?i)int(\d+)`), "int$1"},
	}

	lowercaseQuant := strings.ToLower(quant)
	for _, p := range patterns {
		if matches := p.regex.FindStringSubmatch(lowercaseQuant); matches != nil {
			return p.regex.ReplaceAllString(matches[0], p.replacement)
		}
	}

	return strings.ToLower(quant)
}

// extractParameterSize extracts and normalizes parameter size from various formats
func extractParameterSize(sizeStr string) (string, int64) {
	if sizeStr == "" {
		return "", 0
	}

	originalStr := sizeStr
	sizeStr = strings.TrimSpace(strings.ToUpper(sizeStr))

	re := regexp.MustCompile(`^(\d+(?:\.\d+)?)\s*([BM]?)$`)
	matches := re.FindStringSubmatch(sizeStr)

	if len(matches) != 3 {
		return originalStr, 0
	}

	numStr := matches[1]
	unit := matches[2]

	num, err := strconv.ParseFloat(numStr, 64)
	if err != nil {
		return originalStr, 0
	}

	var paramCount int64
	var normalizedSize string

	switch unit {
	case "M":
		paramCount = int64(num * 1_000_000)
		billionValue := num / 1000.0
		if billionValue >= 1.0 {
			normalizedSize = fmt.Sprintf("%.1fb", billionValue)
		} else {
			normalizedSize = fmt.Sprintf("%.0fm", num)
		}
	case "B", "":
		paramCount = int64(num * 1_000_000_000)
		normalizedSize = fmt.Sprintf("%.1fb", num)
	}

	// Remove redundant decimal zeros for cleaner representation
	if strings.HasSuffix(normalizedSize, ".0b") {
		normalizedSize = strings.TrimSuffix(normalizedSize, ".0b") + "b"
	}
	if strings.HasSuffix(normalizedSize, ".0m") {
		normalizedSize = strings.TrimSuffix(normalizedSize, ".0m") + "m"
	}

	return normalizedSize, paramCount
}

// inferCapabilitiesFromMetadata infers model capabilities from various metadata fields
func inferCapabilitiesFromMetadata(modelType, modelName string, contextLength int64, metadata map[string]interface{}) []string {
	capabilities := make(map[string]bool)
	config := getConfig()

	capabilities["text-generation"] = true

	modelTypeLower := strings.ToLower(modelType)
	if typeCaps, exists := config.Capabilities.TypeCapabilities[modelTypeLower]; exists {
		for _, cap := range typeCaps {
			capabilities[cap] = true
		}
		// Embedding models don't generate text, they encode it
		if modelTypeLower == "embeddings" || modelTypeLower == "embedding" {
			delete(capabilities, "text-generation")
		}
	}

	// Name-based inference using patterns from configuration
	nameLower := strings.ToLower(modelName)
	for _, pattern := range config.Capabilities.NamePatterns {
		if pattern.regex != nil && pattern.regex.MatchString(nameLower) {
			for _, cap := range pattern.Capabilities {
				capabilities[cap] = true
			}
		}
	}

	// Context window size determines long-form processing capabilities
	if contextLength > 0 {
		thresholds := config.Capabilities.ContextThresholds
		switch {
		case contextLength >= thresholds["ultra_long_context"]:
			capabilities["ultra-long-context"] = true
			capabilities["long-context"] = true
		case contextLength >= thresholds["long_context"]:
			capabilities["long-context"] = true
		case contextLength >= thresholds["extended_context"]:
			capabilities["extended-context"] = true
		}
	}

	// Preserve explicitly declared capabilities from source metadata
	if metadata != nil {
		if caps, ok := metadata["capabilities"].([]interface{}); ok {
			for _, cap := range caps {
				if capStr, ok := cap.(string); ok {
					capabilities[capStr] = true
				}
			}
		}
	}

	result := make([]string, 0, len(capabilities))
	for cap := range capabilities {
		result = append(result, cap)
	}

	return result
}

// extractFamilyAndVariant extracts family and variant from model name
func extractFamilyAndVariant(modelName string, arch string) (family, variant string) {
	modelName = strings.TrimSpace(strings.ToLower(modelName))
	config := getConfig()

	// Try architecture-based extraction first
	if arch != "" {
		family, variant = extractFromArchitecture(arch, modelName, config)
		if family != "" {
			return family, variant
		}
	}

	// Try pattern-based extraction
	family, variant = extractFromPatterns(modelName, config)
	if family != "" {
		return family, variant
	}

	// Fallback to delimiter-based extraction
	family = extractFromDelimiters(modelName)
	return family, variant
}

// extractFromArchitecture extracts family and variant from architecture info
func extractFromArchitecture(arch, modelName string, config *ModelUnificationConfig) (family, variant string) {
	archLower := strings.ToLower(arch)
	mappedFamily, exists := config.ModelExtraction.ArchitectureMappings[archLower]
	if !exists {
		return "", ""
	}

	family = mappedFamily

	// Extract numeric version suffix from architecture string
	if !strings.Contains(archLower, family) || len(archLower) <= len(family) {
		return family, ""
	}

	possibleVariant := strings.TrimPrefix(archLower, family)
	if len(possibleVariant) == 0 || possibleVariant[0] < '0' || possibleVariant[0] > '9' {
		return family, ""
	}

	// Check if model name is generic or contains the family
	if isGenericModelName(modelName, config) || strings.Contains(strings.ToLower(modelName), family) {
		variant = possibleVariant
	}

	return family, variant
}

// extractFromPatterns extracts family and variant using regex patterns
func extractFromPatterns(modelName string, config *ModelUnificationConfig) (family, variant string) {
	for _, pattern := range config.ModelExtraction.FamilyPatterns {
		if pattern.regex == nil {
			continue
		}

		matches := pattern.regex.FindStringSubmatch(modelName)
		if matches == nil {
			continue
		}

		// Special handling for mistral/mixtral
		if isMistralPattern(pattern, matches) {
			family, variant = extractMistralFamily(matches)
		} else {
			family, variant = extractStandardFamily(pattern, matches)
		}

		if family != "" {
			return family, variant
		}
	}

	return "", ""
}

// extractFromDelimiters extracts family from first part of delimited name
func extractFromDelimiters(modelName string) string {
	parts := regexp.MustCompile(`[-_:/]`).Split(modelName, -1)
	if len(parts) == 0 {
		return ""
	}

	firstPart := strings.ToLower(parts[0])
	knownFamilies := []string{
		"llama", "gemma", "phi", "qwen", "mistral", "mixtral",
		"yi", "deepseek", "codellama", "starcoder", "vicuna",
		"falcon", "gpt2", "gptj", "gptneox", "bloom", "opt", "mpt",
	}

	for _, known := range knownFamilies {
		if strings.HasPrefix(firstPart, known) {
			return known
		}
	}

	return ""
}

// isGenericModelName checks if model name is generic
func isGenericModelName(modelName string, config *ModelUnificationConfig) bool {
	modelNameLower := strings.ToLower(modelName)
	for _, generic := range config.SpecialRules.GenericNames {
		if modelNameLower == generic {
			return true
		}
	}
	return false
}

// isMistralPattern checks if pattern is for mistral/mixtral models
func isMistralPattern(pattern PatternConfig, matches []string) bool {
	return pattern.FamilyGroup == 1 && pattern.VariantGroup == 2 &&
		len(matches) > 1 && (matches[1] == "mistral" || matches[1] == "mixtral")
}

// extractMistralFamily extracts family and variant for mistral models
func extractMistralFamily(matches []string) (family, variant string) {
	family = matches[1]
	if len(matches) > 2 {
		variant = matches[2]
	}
	return family, variant
}

// extractStandardFamily extracts family and variant using standard pattern groups
func extractStandardFamily(pattern PatternConfig, matches []string) (family, variant string) {
	if pattern.FamilyGroup > 0 && pattern.FamilyGroup < len(matches) {
		family = matches[pattern.FamilyGroup]
	}
	if pattern.VariantGroup > 0 && pattern.VariantGroup < len(matches) && matches[pattern.VariantGroup] != "" {
		variant = matches[pattern.VariantGroup]
	}
	return family, variant
}

// parseContextLength handles type-polymorphic context length values from diverse platforms.
func parseContextLength(value interface{}) int64 {
	switch v := value.(type) {
	case int64:
		return v
	case int:
		return int64(v)
	case float64:
		return int64(v)
	case string:
		if num, err := strconv.ParseInt(v, 10, 64); err == nil {
			return num
		}
	}
	return 0
}

// extractPublisher identifies model creators through metadata, naming conventions,
// and family-to-publisher mappings.
func extractPublisher(modelName string, metadata map[string]interface{}) string {
	if metadata != nil {
		if publisher, ok := metadata["publisher"].(string); ok && publisher != "" {
			return publisher
		}
	}

	// HuggingFace-style namespace extraction
	if idx := strings.Index(modelName, "/"); idx > 0 {
		return modelName[:idx]
	}

	config := getConfig()
	nameLower := strings.ToLower(modelName)

	for modelFamily, publisher := range config.ModelExtraction.PublisherMappings {
		if strings.HasPrefix(nameLower, modelFamily) {
			return publisher
		}
	}

	return ""
}
