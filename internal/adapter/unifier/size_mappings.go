package unifier

import (
	"regexp"
	"strings"
)

// SizeMapping represents a mapping from text-based size to normalized size
type SizeMapping struct {
	Pattern        string
	NormalizedSize string
	ParameterCount int64
	Priority       int // Higher priority wins in case of conflicts
}

// TextSizeMappings defines common text-based size indicators and their normalized values
var TextSizeMappings = []SizeMapping{
	// Specific numeric patterns that include size indicators
	{Pattern: "small-2505", NormalizedSize: "8b", ParameterCount: 8000000000, Priority: 10}, // Mistral devstral-small-2505
	
	// Common size descriptors
	{Pattern: "nano", NormalizedSize: "0.5b", ParameterCount: 500000000, Priority: 8},
	{Pattern: "tiny", NormalizedSize: "1b", ParameterCount: 1000000000, Priority: 8},
	{Pattern: "mini", NormalizedSize: "3b", ParameterCount: 3000000000, Priority: 8},
	{Pattern: "small", NormalizedSize: "8b", ParameterCount: 8000000000, Priority: 7},
	{Pattern: "medium", NormalizedSize: "13b", ParameterCount: 13000000000, Priority: 7},
	{Pattern: "large", NormalizedSize: "70b", ParameterCount: 70000000000, Priority: 7},
	{Pattern: "xl", NormalizedSize: "175b", ParameterCount: 175000000000, Priority: 7},
	{Pattern: "xxl", NormalizedSize: "405b", ParameterCount: 405000000000, Priority: 7},
	
	// Base models (often medium-sized)
	{Pattern: "base", NormalizedSize: "7b", ParameterCount: 7000000000, Priority: 6},
	
	// Mistral-specific patterns
	{Pattern: "magistral", NormalizedSize: "12b", ParameterCount: 12000000000, Priority: 5}, // Educated guess for magistral
	{Pattern: "devstral", NormalizedSize: "8b", ParameterCount: 8000000000, Priority: 5},    // Devstral is typically small
}

// extractSizeFromName attempts to extract size information from model name
func extractSizeFromName(modelName string) (size string, confidence int) {
	nameLower := strings.ToLower(modelName)
	
	// First, check for numeric patterns (highest confidence)
	numericPatterns := []struct {
		regex      *regexp.Regexp
		confidence int
	}{
		{regexp.MustCompile(`(\d+x\d+)[bB]`), 100},                     // 8x7B (MoE models)
		{regexp.MustCompile(`(\d+(?:\.\d+)?)[bB]`), 100},              // 7B, 13B, 70.6B
		{regexp.MustCompile(`(\d+(?:\.\d+)?)\s*billion`), 100},        // 7 billion
		{regexp.MustCompile(`(\d+)[mM]`), 90},                         // 350M
		{regexp.MustCompile(`(\d+(?:\.\d+)?)\s*million`), 90},         // 350 million
		{regexp.MustCompile(`-(\d+(?:\.\d+)?)[bB](?:-|$)`), 100},      // -7B- in middle or -7B at end
		{regexp.MustCompile(`_(\d+(?:\.\d+)?)[bB]_`), 100},            // _7B_ in middle
		{regexp.MustCompile(`:(\d+x\d+)[bB]`), 100},                    // :8x7B at end (MoE)
		{regexp.MustCompile(`:(\d+(?:\.\d+)?)[bB]`), 100},             // :7B at end
	}
	
	for _, pattern := range numericPatterns {
		if matches := pattern.regex.FindStringSubmatch(modelName); len(matches) > 1 {
			return matches[1], pattern.confidence
		}
	}
	
	// Then check text-based patterns with priority
	var bestMatch *SizeMapping
	var bestPriority int
	
	for _, mapping := range TextSizeMappings {
		if strings.Contains(nameLower, strings.ToLower(mapping.Pattern)) {
			if mapping.Priority > bestPriority {
				bestMatch = &mapping
				bestPriority = mapping.Priority
			}
		}
	}
	
	if bestMatch != nil {
		// Return the raw pattern for now, let NormalizeSize handle the conversion
		return bestMatch.Pattern, 50 + bestMatch.Priority
	}
	
	return "", 0
}

// inferSizeFromContext attempts to infer size from other model properties
func inferSizeFromContext(family string, variant string, contextLength int64, quantization string) (size string, confidence int) {
	// Context-based inference rules
	switch {
	case contextLength > 100000:
		// Very long context usually means larger models
		return "large", 30
	case contextLength > 32000:
		// Medium-long context
		return "medium", 25
	case family == "mistral" && variant == "magistral":
		// Known Mistral model
		return "small", 40
	case family == "mistral" && variant == "devstral":
		// Known Mistral developer model
		return "small", 40
	case strings.Contains(quantization, "q2") || strings.Contains(quantization, "q3"):
		// Very aggressive quantization often used on larger models
		return "large", 20
	default:
		return "", 0
	}
}