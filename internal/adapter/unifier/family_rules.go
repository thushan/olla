package unifier

import (
	"regexp"
	"strconv"
	"strings"
)

// PublisherFamilyRule maps publishers to model families
type PublisherFamilyRule struct {
	Publishers []string
	Family     string
	Priority   int
}

// NamePatternRule maps name patterns to families and variants
type NamePatternRule struct {
	Pattern  string // regex or simple contains
	Family   string
	Variant  string // optional
	Priority int
}

// CapabilityRule maps model characteristics to capabilities
type CapabilityRule struct {
	ModelType    string   // llm, vlm, embeddings
	Architecture string   // nomic-bert, phi3, etc.
	NamePatterns []string // code, embed, vision keywords
	Capabilities []string
}

// FamilyCandidate represents a potential family match with priority
type FamilyCandidate struct {
	Family   string
	Variant  string
	Priority int
	Source   string // publisher, name_pattern, platform, name_extraction
}

// PublisherFamilyRules defines publisher-to-family mappings
var PublisherFamilyRules = []PublisherFamilyRule{
	{
		Publishers: []string{"mistralai", "mistral"},
		Family:     "mistral",
		Priority:   10, // High priority
	},
	{
		Publishers: []string{"microsoft"},
		Family:     "phi",
		Priority:   10,
	},
	{
		Publishers: []string{"google"},
		Family:     "gemma",
		Priority:   9,
	},
	{
		Publishers: []string{"deepseek"},
		Family:     "deepseek",
		Priority:   9,
	},
	{
		Publishers: []string{"nomic-ai", "nomic"},
		Family:     "nomic-bert",
		Priority:   10,
	},
	{
		Publishers: []string{"meta", "facebook"},
		Family:     "llama",
		Priority:   9,
	},
	{
		Publishers: []string{"alibaba", "qwen"},
		Family:     "qwen",
		Priority:   9,
	},
}

// NamePatternRules defines name pattern to family/variant mappings
var NamePatternRules = []NamePatternRule{
	{
		Pattern:  "devstral",
		Family:   "mistral",
		Variant:  "devstral",
		Priority: 8,
	},
	{
		Pattern:  "magistral",
		Family:   "mistral",
		Variant:  "magistral",
		Priority: 8,
	},
	{
		Pattern:  "codegemma",
		Family:   "gemma",
		Variant:  "code",
		Priority: 8,
	},
	{
		Pattern:  "codellama",
		Family:   "llama",
		Variant:  "code",
		Priority: 8,
	},
	{
		Pattern:  "text-embedding",
		Family:   "nomic-bert",
		Priority: 9,
	},
	{
		Pattern:  "nomic-embed",
		Family:   "nomic-bert",
		Variant:  "embed-text",
		Priority: 9,
	},
	{
		Pattern:  "deepseek-coder",
		Family:   "deepseek",
		Variant:  "coder",
		Priority: 8,
	},
	{
		Pattern:  "phi-mini",
		Family:   "phi",
		Variant:  "mini",
		Priority: 8,
	},
}

// CapabilityRules defines capability inference rules
var CapabilityRules = []CapabilityRule{
	{
		ModelType:    "embeddings",
		Capabilities: []string{"embeddings", "text_search"},
	},
	{
		Architecture: "nomic-bert",
		Capabilities: []string{"embeddings", "text_search"},
	},
	{
		ModelType:    "vlm",
		Capabilities: []string{"vision", "multimodal", "chat", "completion"},
	},
	{
		NamePatterns: []string{"code", "coder", "codegen"},
		Capabilities: []string{"code", "completion"},
	},
	{
		NamePatterns: []string{"embed", "embedding", "text-embedding"},
		Capabilities: []string{"embeddings", "text_search"},
	},
	{
		NamePatterns: []string{"vision", "vlm", "multimodal"},
		Capabilities: []string{"vision", "multimodal", "chat", "completion"},
	},
	{
		NamePatterns: []string{"chat", "instruct"},
		Capabilities: []string{"chat", "completion"},
	},
	{
		Architecture: "phi3",
		Capabilities: []string{"chat", "completion"},
	},
	{
		Architecture: "phi",
		Capabilities: []string{"chat", "completion"},
	},
	{
		NamePatterns: []string{"devstral", "magistral"},
		Capabilities: []string{"code", "completion"},
	},
}

// Helper functions

// extractPublisher extracts the publisher from model name or metadata
func extractPublisher(modelName string) string {
	// Check if name has org/model format
	if strings.Contains(modelName, "/") {
		parts := strings.SplitN(modelName, "/", 2)
		return strings.ToLower(parts[0])
	}
	return ""
}

// matchesPattern checks if a string matches a pattern (simple contains for now)
func matchesPattern(text, pattern string) bool {
	return strings.Contains(strings.ToLower(text), strings.ToLower(pattern))
}

// containsAny checks if text contains any of the patterns
func containsAny(text string, patterns []string) bool {
	textLower := strings.ToLower(text)
	for _, pattern := range patterns {
		if strings.Contains(textLower, strings.ToLower(pattern)) {
			return true
		}
	}
	return false
}

// extractVersionFromName attempts to extract version/variant from model name
func extractVersionFromName(modelName string, family string) string {
	nameLower := strings.ToLower(modelName)
	
	// First check for known patterns that should override number extraction
	knownVariants := []struct {
		pattern string
		variant string
	}{
		{"devstral", "devstral"},
		{"magistral", "magistral"},
		{"codegemma", "code"},
		{"codellama", "code"},
		{"deepseek-coder", "coder"},
		{"phi-mini", "mini"},
		{"phi-medium", "medium"},
		{"embed-text", "embed-text"},
		{"embedding", "embedding"},
	}
	
	for _, kv := range knownVariants {
		if strings.Contains(nameLower, kv.pattern) {
			// Check if there's a version after this pattern
			afterPattern := strings.Split(nameLower, kv.pattern)[1]
			if matches := regexp.MustCompile(`-?v?(\d+(?:\.\d+)?)`).FindStringSubmatch(afterPattern); len(matches) > 1 {
				// Return variant with version if found
				return kv.variant + "-" + matches[1]
			}
			return kv.variant
		}
	}
	
	// Remove family prefix to find variant
	familyLower := strings.ToLower(family)
	
	// Handle cases where the model name contains the family name
	// e.g., "google/gemma-3-12b" -> extract "3" after removing "google/" and "gemma-"
	parts := strings.Split(nameLower, "/")
	nameWithoutPublisher := nameLower
	if len(parts) > 1 {
		nameWithoutPublisher = parts[len(parts)-1]
	}
	
	remaining := strings.ReplaceAll(nameWithoutPublisher, familyLower, "")
	remaining = strings.TrimPrefix(remaining, "-")
	remaining = strings.TrimPrefix(remaining, "_")
	remaining = strings.TrimPrefix(remaining, "/")
	
	// Look for version patterns in the remaining string
	versionPatterns := []struct {
		pattern *regexp.Regexp
		group   int
	}{
		{regexp.MustCompile(`^(\d+(?:\.\d+)?)`), 1},              // Numbers at start: 3, 3.3
		{regexp.MustCompile(`(small|medium|large|xl|xxl)`), 1},   // Size variants
		{regexp.MustCompile(`(mini|micro|nano|tiny)`), 1},        // Small variants
		{regexp.MustCompile(`(code|chat|instruct|base)`), 1},     // Purpose variants
		{regexp.MustCompile(`(embed|embedding)`), 1},             // Embedding variants
		{regexp.MustCompile(`r(\d+)`), 1},                        // r1, r2 versions
		{regexp.MustCompile(`v(\d+(?:\.\d+)?)`), 1},             // v1, v2.0 versions
	}
	
	for _, vp := range versionPatterns {
		if matches := vp.pattern.FindStringSubmatch(remaining); len(matches) > vp.group {
			return matches[vp.group]
		}
	}
	
	// Check for year-based versions (e.g., 2505) only if nothing else matches
	if remaining == "" && !strings.Contains(nameLower, "-") {
		if matches := regexp.MustCompile(`(\d{4})$`).FindStringSubmatch(modelName); len(matches) > 1 {
			year := matches[1]
			yearNum, _ := strconv.Atoi(year)
			if yearNum >= 2020 && yearNum <= 2030 {
				return year
			}
		}
	}
	
	return ""
}