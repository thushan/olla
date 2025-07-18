package unifier

import (
	"fmt"
	"strings"
	"time"

	"github.com/thushan/olla/internal/core/domain"
)

// ModelBuilder simplifies the creation of unified models
type ModelBuilder struct {
	model    *domain.UnifiedModel
	metadata map[string]interface{}
}

// NewModelBuilder creates a new model builder
func NewModelBuilder() *ModelBuilder {
	return &ModelBuilder{
		model: &domain.UnifiedModel{
			Aliases:         []domain.AliasEntry{},
			SourceEndpoints: []domain.SourceEndpoint{},
			Capabilities:    []string{},
			LastSeen:        time.Now(),
		},
		metadata: make(map[string]interface{}),
	}
}

// WithID sets the model ID
func (b *ModelBuilder) WithID(id string) *ModelBuilder {
	b.model.ID = id
	return b
}

// WithFamily sets the model family and variant
func (b *ModelBuilder) WithFamily(family, variant string) *ModelBuilder {
	b.model.Family = family
	b.model.Variant = variant
	return b
}

// WithParameters sets the parameter size and count
func (b *ModelBuilder) WithParameters(size string, count int64) *ModelBuilder {
	b.model.ParameterSize = size
	b.model.ParameterCount = count
	return b
}

// WithQuantization sets the quantization level
func (b *ModelBuilder) WithQuantization(quantization string) *ModelBuilder {
	b.model.Quantization = quantization
	return b
}

// WithFormat sets the model format
func (b *ModelBuilder) WithFormat(format string) *ModelBuilder {
	b.model.Format = format
	return b
}

// WithContextLength sets the max context length
func (b *ModelBuilder) WithContextLength(length int64) *ModelBuilder {
	if length > 0 {
		b.model.MaxContextLength = &length
	}
	return b
}

// WithDiskSize sets the disk size
func (b *ModelBuilder) WithDiskSize(size int64) *ModelBuilder {
	b.model.DiskSize = size
	return b
}

// AddAlias adds an alias entry
func (b *ModelBuilder) AddAlias(name, source string) *ModelBuilder {
	b.model.Aliases = append(b.model.Aliases, domain.AliasEntry{
		Name:   name,
		Source: source,
	})
	return b
}

// AddSourceEndpoint adds a source endpoint
func (b *ModelBuilder) AddSourceEndpoint(endpoint domain.SourceEndpoint) *ModelBuilder {
	b.model.SourceEndpoints = append(b.model.SourceEndpoints, endpoint)
	return b
}

// AddCapability adds a capability
func (b *ModelBuilder) AddCapability(capability string) *ModelBuilder {
	b.model.Capabilities = append(b.model.Capabilities, capability)
	return b
}

// AddCapabilities adds multiple capabilities
func (b *ModelBuilder) AddCapabilities(capabilities []string) *ModelBuilder {
	b.model.Capabilities = append(b.model.Capabilities, capabilities...)
	return b
}

// WithMetadata adds a metadata entry
func (b *ModelBuilder) WithMetadata(key string, value interface{}) *ModelBuilder {
	b.metadata[key] = value
	return b
}

// WithDigest sets the digest in metadata
func (b *ModelBuilder) WithDigest(digest string) *ModelBuilder {
	if digest != "" {
		b.metadata["digest"] = digest
	}
	return b
}

// WithPlatform sets the platform in metadata
func (b *ModelBuilder) WithPlatform(platform string) *ModelBuilder {
	b.metadata["platform"] = platform
	return b
}

// WithPublisher sets the publisher in metadata
func (b *ModelBuilder) WithPublisher(publisher string) *ModelBuilder {
	if publisher != "" {
		b.metadata["publisher"] = publisher
	}
	return b
}

// WithConfidence sets the metadata confidence score
func (b *ModelBuilder) WithConfidence(confidence float64) *ModelBuilder {
	b.metadata["metadata_confidence"] = confidence
	return b
}

// Build creates the final unified model
func (b *ModelBuilder) Build() *domain.UnifiedModel {
	b.model.Metadata = b.metadata
	return b.model
}

// ModelExtractor extracts model attributes from various sources
type ModelExtractor struct{}

// NewModelExtractor creates a new model extractor
func NewModelExtractor() *ModelExtractor {
	return &ModelExtractor{}
}

// ExtractParameterInfo extracts parameter size and count from metadata
func (e *ModelExtractor) ExtractParameterInfo(metadata map[string]interface{}, modelSize int64) (string, int64) {
	paramSize := ""
	paramCount := int64(0)

	if sizeStr, ok := metadata["parameter_size"].(string); ok {
		paramSize, paramCount = extractParameterSize(sizeStr)
	}

	// Use disk size as fallback for parameter count
	if paramCount == 0 && modelSize > 0 {
		paramCount = modelSize
	}

	return paramSize, paramCount
}

// ExtractQuantization extracts and normalizes quantization from metadata
func (e *ModelExtractor) ExtractQuantization(metadata map[string]interface{}) string {
	if quantStr, ok := metadata["quantization_level"].(string); ok {
		return normalizeQuantization(quantStr)
	}
	if quantStr, ok := metadata["quantization"].(string); ok {
		return normalizeQuantization(quantStr)
	}
	return ""
}

// ExtractFamilyAndVariant extracts model family and variant
func (e *ModelExtractor) ExtractFamilyAndVariant(modelName, modelFamily string, metadata map[string]interface{}) (string, string) {
	arch := ""
	if archStr, ok := metadata["arch"].(string); ok {
		arch = archStr
	}

	family, variant := extractFamilyAndVariant(modelName, arch)

	// Use provided family if available
	if modelFamily != "" {
		family = modelFamily
		variant = "" // Don't extract variant if family is provided
	}

	return family, variant
}

// ExtractModelType extracts the model type from metadata
func (e *ModelExtractor) ExtractModelType(metadata map[string]interface{}) string {
	if typeStr, ok := metadata["type"].(string); ok {
		return typeStr
	}
	if typeStr, ok := metadata["model_type"].(string); ok {
		return typeStr
	}
	return ""
}

// ExtractPublisher extracts publisher from model name and metadata
func (e *ModelExtractor) ExtractPublisher(modelName string, metadata map[string]interface{}) string {
	// Check metadata first
	if publisher, ok := metadata["publisher"].(string); ok {
		return publisher
	}

	// Try to extract from model name
	return extractPublisher(modelName, metadata)
}

// DetectPlatform detects the platform from model metadata
func (e *ModelExtractor) DetectPlatform(format string, metadata map[string]interface{}) string {
	// Check metadata for platform hints
	if metadata != nil {
		if platform, ok := metadata["platform"].(string); ok {
			return strings.ToLower(platform)
		}
		if _, ok := metadata["ollama.version"]; ok {
			return "ollama"
		}
		if _, ok := metadata["lmstudio.version"]; ok {
			return "lmstudio"
		}
	}

	// Check format
	if format != "" && strings.Contains(strings.ToLower(format), "gguf") {
		return "ollama"
	}

	// Default to openai-compatible
	return "openai"
}

// MapModelState maps model properties to a state string
func (e *ModelExtractor) MapModelState(metadata map[string]interface{}, modelSize int64) string {
	// Check various state indicators
	if metadata != nil {
		if state, ok := metadata["state"].(string); ok {
			return state
		}
		if loaded, ok := metadata["loaded"].(bool); ok && loaded {
			return "loaded"
		}
	}

	// Check model size to infer if it's loaded
	if modelSize > 0 {
		return "available"
	}

	return "unknown"
}

// CalculateConfidence calculates a confidence score for extracted metadata
func (e *ModelExtractor) CalculateConfidence(hasDigest, hasParameterSize, hasQuantization, hasFamily, hasContextWindow bool) float64 {
	confidence := 0.0
	fields := 0.0

	if hasDigest {
		confidence += 1.0
		fields++
	}
	if hasParameterSize {
		confidence += 1.0
		fields++
	}
	if hasQuantization {
		confidence += 1.0
		fields++
	}
	if hasFamily {
		confidence += 1.0
		fields++
	}
	if hasContextWindow {
		confidence += 1.0
		fields++
	}

	if fields > 0 {
		return confidence / fields
	}
	return 0.0
}

// SourceEndpointBuilder builds source endpoints
type SourceEndpointBuilder struct {
	endpoint domain.SourceEndpoint
}

// NewSourceEndpointBuilder creates a new source endpoint builder
func NewSourceEndpointBuilder() *SourceEndpointBuilder {
	return &SourceEndpointBuilder{
		endpoint: domain.SourceEndpoint{
			LastSeen: time.Now(),
		},
	}
}

// WithURL sets the endpoint URL
func (b *SourceEndpointBuilder) WithURL(url string) *SourceEndpointBuilder {
	b.endpoint.EndpointURL = url
	return b
}

// WithName sets the endpoint name
func (b *SourceEndpointBuilder) WithName(name string) *SourceEndpointBuilder {
	b.endpoint.EndpointName = name
	return b
}

// WithNativeName sets the native model name
func (b *SourceEndpointBuilder) WithNativeName(name string) *SourceEndpointBuilder {
	b.endpoint.NativeName = name
	return b
}

// WithState sets the model state
func (b *SourceEndpointBuilder) WithState(state string) *SourceEndpointBuilder {
	b.endpoint.State = state
	return b
}

// WithDiskSize sets the disk size
func (b *SourceEndpointBuilder) WithDiskSize(size int64) *SourceEndpointBuilder {
	b.endpoint.DiskSize = size
	return b
}

// Build creates the source endpoint
func (b *SourceEndpointBuilder) Build() domain.SourceEndpoint {
	return b.endpoint
}

// generateUniqueID generates a unique ID considering digest conflicts
func generateUniqueID(baseID, digest string, existingDigest string) string {
	if digest != "" && existingDigest != "" && existingDigest != digest {
		// Different digest, need unique ID
		if len(digest) >= 8 {
			return fmt.Sprintf("%s-%s", baseID, digest[len(digest)-8:])
		}
		return fmt.Sprintf("%s-%s", baseID, digest)
	}
	return baseID
}
