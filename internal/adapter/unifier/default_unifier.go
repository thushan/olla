package unifier

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/core/ports"
)

// Model represents a simplified internal representation for processing
type Model struct {
	Metadata      map[string]interface{}
	ID            string
	Name          string
	Family        string
	Digest        string
	Format        string
	Size          int64
	ContextWindow int64
}

func nilIfZero(v int64) *int64 {
	if v == 0 {
		return nil
	}
	return &v
}

// DefaultUnifier implements simple model deduplication based on digest and exact name matching
type DefaultUnifier struct {
	lastCleanup     time.Time
	catalog         map[string]*domain.UnifiedModel // ID -> unified model
	digestIndex     map[string][]string             // digest -> model IDs
	nameIndex       map[string][]string             // lowercase name -> model IDs
	endpointModels  map[string][]string             // endpoint URL -> model IDs
	stats           DeduplicationStats
	cleanupInterval time.Duration
	mu              sync.RWMutex
}

// DeduplicationStats tracks deduplication performance
type DeduplicationStats struct {
	LastUpdated   time.Time
	TotalModels   int
	UnifiedModels int
	DigestMatches int
	NameMatches   int
}

// NewDefaultUnifier creates a new model unifier with simple deduplication
func NewDefaultUnifier() ports.ModelUnifier {
	return &DefaultUnifier{
		catalog:         make(map[string]*domain.UnifiedModel),
		digestIndex:     make(map[string][]string),
		nameIndex:       make(map[string][]string),
		endpointModels:  make(map[string][]string),
		cleanupInterval: 5 * time.Minute,
	}
}

// UnifyModels performs simple deduplication of models from multiple endpoints
func (u *DefaultUnifier) UnifyModels(ctx context.Context, models []*domain.ModelInfo, endpoint *domain.Endpoint) ([]*domain.UnifiedModel, error) {
	if models == nil || len(models) == 0 {
		return nil, nil
	}

	u.mu.Lock()
	defer u.mu.Unlock()

	// Clean up stale models periodically
	if time.Since(u.lastCleanup) > u.cleanupInterval {
		u.cleanupStaleModels()
		u.lastCleanup = time.Now()
	}

	// Use endpoint URL as key internally, but store name for display
	endpointURL := endpoint.GetURLString()

	// Clear previous models from this endpoint
	if oldModels, exists := u.endpointModels[endpointURL]; exists {
		for _, modelID := range oldModels {
			u.removeModelFromEndpoint(modelID, endpointURL, endpoint.Name)
		}
	}
	u.endpointModels[endpointURL] = []string{}

	processedModels := make([]*domain.UnifiedModel, 0, len(models))
	for _, modelInfo := range models {
		if modelInfo == nil {
			continue
		}

		// Convert ModelInfo to Model for processing
		model := u.convertModelInfoToModel(modelInfo)
		unified := u.processModel(model, endpoint)
		if unified != nil {
			processedModels = append(processedModels, unified)
			u.endpointModels[endpointURL] = append(u.endpointModels[endpointURL], unified.ID)
		}
	}

	u.stats.TotalModels = len(u.catalog)
	u.stats.UnifiedModels = len(processedModels)
	u.stats.LastUpdated = time.Now()

	return processedModels, nil
}

// processModel handles deduplication logic for a single model
func (u *DefaultUnifier) processModel(model *Model, endpoint *domain.Endpoint) *domain.UnifiedModel {
	// Try to find existing model by digest first (highest confidence)
	if model.Digest != "" {
		if existingIDs, found := u.digestIndex[model.Digest]; found && len(existingIDs) > 0 {
			// Found existing model with same digest - merge
			existing := u.catalog[existingIDs[0]]
			u.mergeModel(existing, model, endpoint)
			u.stats.DigestMatches++
			return existing
		}
	}

	// Try exact name match (case-insensitive)
	lowercaseName := strings.ToLower(model.Name)
	if existingIDs, found := u.nameIndex[lowercaseName]; found && len(existingIDs) > 0 {
		for _, id := range existingIDs {
			existing := u.catalog[id]
			if u.canMergeByName(existing, model) {
				u.mergeModel(existing, model, endpoint)
				u.stats.NameMatches++
				return existing
			}
		}
		// If we get here, we found name matches but couldn't merge due to conflicts
		// Don't count as name match for stats
	}

	// No match found - create new unified model
	unified := u.createUnifiedModel(model, endpoint)
	u.catalog[unified.ID] = unified

	// Update indices
	if model.Digest != "" {
		u.digestIndex[model.Digest] = append(u.digestIndex[model.Digest], unified.ID)
	}
	u.nameIndex[lowercaseName] = append(u.nameIndex[lowercaseName], unified.ID)

	return unified
}

// canMergeByName determines if two models can be merged based on name
func (u *DefaultUnifier) canMergeByName(existing *domain.UnifiedModel, newModel *Model) bool {
	// Only merge if names match exactly (case-insensitive)
	// Check against aliases
	for _, alias := range existing.Aliases {
		if strings.EqualFold(alias.Name, newModel.Name) {
			// Check for digest conflicts
			if newModel.Digest != "" && existing.Metadata != nil {
				if existingDigest, ok := existing.Metadata["digest"].(string); ok {
					if existingDigest != "" && existingDigest != newModel.Digest {
						// Different digests = different models, don't merge
						return false
					}
				}
			}
			return true
		}
	}
	return false
}

// createUnifiedModel creates a new unified model from a platform model
func (u *DefaultUnifier) createUnifiedModel(model *Model, endpoint *domain.Endpoint) *domain.UnifiedModel {
	// Use original model ID/name as the unified ID
	id := model.Name
	if model.ID != "" {
		id = model.ID
	}

	// Make ID unique if there's already a model with this ID but different digest
	if existing, exists := u.catalog[id]; exists {
		if model.Digest != "" && existing.Metadata != nil {
			if existingDigest, ok := existing.Metadata["digest"].(string); ok {
				if existingDigest != "" && existingDigest != model.Digest {
					// Different digest, need unique ID
					id = fmt.Sprintf("%s-%s", id, model.Digest[len(model.Digest)-8:])
				}
			}
		}
	}

	// Detect platform from model metadata
	platform := u.detectPlatform(model)

	// Create source endpoint - use name for display, URL internally
	source := domain.SourceEndpoint{
		EndpointURL:  endpoint.GetURLString(),
		EndpointName: endpoint.Name, // Use name for display
		NativeName:   model.Name,
		State:        u.mapModelState(model),
		LastSeen:     time.Now(),
		DiskSize:     model.Size,
	}

	// Extract and normalize parameter size
	paramSize := ""
	paramCount := int64(0)
	if sizeStr, ok := model.Metadata["parameter_size"].(string); ok {
		paramSize, paramCount = extractParameterSize(sizeStr)
	}
	// If we didn't get a parameter count, use disk size as fallback
	if paramCount == 0 && model.Size > 0 {
		paramCount = model.Size
	}

	// Extract and normalize quantization
	quantization := ""
	if quantStr, ok := model.Metadata["quantization_level"].(string); ok {
		quantization = normalizeQuantization(quantStr)
	} else if quantStr, ok := model.Metadata["quantization"].(string); ok {
		quantization = normalizeQuantization(quantStr)
	}

	// Extract family and variant
	arch := ""
	if archStr, ok := model.Metadata["arch"].(string); ok {
		arch = archStr
	}
	family, variant := extractFamilyAndVariant(model.Name, arch)
	// Use family from metadata if available, otherwise use extracted
	if model.Family != "" {
		family = model.Family
		// Don't extract variant if family comes from metadata to avoid confusion
		// (e.g., gemma3 from metadata shouldn't produce variant "3")
		variant = ""
	} else if family != "" {
		model.Family = family
	}

	modelType := ""
	if typeStr, ok := model.Metadata["type"].(string); ok {
		modelType = typeStr
	} else if typeStr, ok := model.Metadata["model_type"].(string); ok {
		modelType = typeStr
	}

	// Build comprehensive capabilities list
	capabilities := inferCapabilitiesFromMetadata(modelType, model.Name, model.ContextWindow, model.Metadata)

	// Extract publisher
	publisher := extractPublisher(model.Name, model.Metadata)

	// Prepare enriched metadata
	metadata := make(map[string]interface{})
	if model.Metadata != nil {
		for k, v := range model.Metadata {
			metadata[k] = v
		}
	}
	// Store digest in metadata for conflict detection
	if model.Digest != "" {
		metadata["digest"] = model.Digest
	}
	// Add extracted publisher if we found one
	if publisher != "" && metadata["publisher"] == nil {
		metadata["publisher"] = publisher
	}
	// Add platform info
	metadata["platform"] = platform
	// Add confidence score for extracted metadata
	metadata["metadata_confidence"] = u.calculateMetadataConfidence(model, paramSize, quantization, family)

	contextWindow := model.ContextWindow
	return &domain.UnifiedModel{
		ID:               id,
		Family:           family,
		Variant:          variant,
		ParameterSize:    paramSize,
		ParameterCount:   paramCount,
		Quantization:     quantization,
		Format:           model.Format,
		Aliases:          []domain.AliasEntry{{Name: model.Name, Source: platform}},
		SourceEndpoints:  []domain.SourceEndpoint{source},
		Capabilities:     capabilities,
		MaxContextLength: nilIfZero(contextWindow),
		DiskSize:         model.Size,
		LastSeen:         time.Now(),
		Metadata:         metadata,
	}
}

// mergeModel merges a new model into an existing unified model
func (u *DefaultUnifier) mergeModel(unified *domain.UnifiedModel, model *Model, endpoint *domain.Endpoint) {
	if unified == nil || model == nil {
		return
	}

	endpointURL := endpoint.GetURLString()

	// Check if this endpoint already exists
	for i, source := range unified.SourceEndpoints {
		if source.EndpointURL == endpointURL {
			// Update existing source
			unified.SourceEndpoints[i].NativeName = model.Name
			unified.SourceEndpoints[i].State = u.mapModelState(model)
			unified.SourceEndpoints[i].LastSeen = time.Now()
			unified.SourceEndpoints[i].DiskSize = model.Size
			unified.LastSeen = time.Now()
			// Still merge metadata even for existing endpoints
			u.mergeMetadata(unified, model)
			return
		}
	}

	// Add new source
	platform := u.detectPlatform(model)
	source := domain.SourceEndpoint{
		EndpointURL:  endpointURL,
		EndpointName: endpoint.Name, // Use name for display
		NativeName:   model.Name,
		State:        u.mapModelState(model),
		LastSeen:     time.Now(),
		DiskSize:     model.Size,
	}
	unified.SourceEndpoints = append(unified.SourceEndpoints, source)

	// Add alias if not present
	hasAlias := false
	for _, alias := range unified.Aliases {
		if alias.Name == model.Name {
			hasAlias = true
			break
		}
	}
	if !hasAlias {
		unified.Aliases = append(unified.Aliases, domain.AliasEntry{Name: model.Name, Source: platform})
	}

	modelType := ""
	if typeStr, ok := model.Metadata["type"].(string); ok {
		modelType = typeStr
	} else if typeStr, ok := model.Metadata["model_type"].(string); ok {
		modelType = typeStr
	}

	// Merge capabilities with new comprehensive extraction
	newCaps := inferCapabilitiesFromMetadata(modelType, model.Name, model.ContextWindow, model.Metadata)
	unified.Capabilities = u.mergeCapabilities(unified.Capabilities, newCaps)

	// Update disk size
	unified.DiskSize += model.Size

	// Merge all metadata
	u.mergeMetadata(unified, model)

	// Update fields if they were previously empty but now have data
	if unified.ParameterSize == "" && model.Metadata["parameter_size"] != nil {
		if sizeStr, ok := model.Metadata["parameter_size"].(string); ok {
			paramSize, paramCount := extractParameterSize(sizeStr)
			unified.ParameterSize = paramSize
			if paramCount > 0 {
				unified.ParameterCount = paramCount
			}
		}
	}

	if unified.Quantization == "" {
		if quantStr, ok := model.Metadata["quantization_level"].(string); ok {
			unified.Quantization = normalizeQuantization(quantStr)
		} else if quantStr, ok := model.Metadata["quantization"].(string); ok {
			unified.Quantization = normalizeQuantization(quantStr)
		}
	}

	if unified.MaxContextLength == nil && model.ContextWindow > 0 {
		unified.MaxContextLength = nilIfZero(model.ContextWindow)
	}

	unified.LastSeen = time.Now()
}

// mergeMetadata intelligently merges metadata from a new model into the unified model
func (u *DefaultUnifier) mergeMetadata(unified *domain.UnifiedModel, model *Model) {
	if unified.Metadata == nil {
		unified.Metadata = make(map[string]interface{})
	}

	// Update digest if present
	if model.Digest != "" {
		unified.Metadata["digest"] = model.Digest
	}

	// Merge all metadata from the new model
	for k, v := range model.Metadata {
		// For certain fields, only update if not already present
		switch k {
		case "metadata_confidence":
			// Recalculate confidence based on merged data
			continue
		case "platform":
			// Store platform-specific data separately
			if vStr, ok := v.(string); ok {
				if platforms, ok := unified.Metadata["platforms"].(map[string]bool); ok {
					platforms[vStr] = true
				} else {
					unified.Metadata["platforms"] = map[string]bool{vStr: true}
				}
			}
		default:
			// For most fields, newer data overwrites older data
			unified.Metadata[k] = v
		}
	}

	// Recalculate confidence score
	arch := ""
	if archStr, ok := model.Metadata["arch"].(string); ok {
		arch = archStr
	}
	family, _ := extractFamilyAndVariant(model.Name, arch)
	unified.Metadata["metadata_confidence"] = u.calculateMetadataConfidence(model, unified.ParameterSize, unified.Quantization, family)
}

// detectPlatform detects the platform from model metadata
func (u *DefaultUnifier) detectPlatform(model *Model) string {
	// Check metadata for platform hints
	if model.Metadata != nil {
		if platform, ok := model.Metadata["platform"].(string); ok {
			return strings.ToLower(platform)
		}
		if _, ok := model.Metadata["ollama.version"]; ok {
			return "ollama"
		}
		if _, ok := model.Metadata["lmstudio.version"]; ok {
			return "lmstudio"
		}
	}

	// Check model properties
	if model.Format != "" && strings.Contains(strings.ToLower(model.Format), "gguf") {
		return "ollama"
	}

	// Default to openai-compatible
	return "openai"
}

// mapModelState maps model properties to a state string
func (u *DefaultUnifier) mapModelState(model *Model) string {
	// Check various state indicators
	if model.Metadata != nil {
		if state, ok := model.Metadata["state"].(string); ok {
			return state
		}
		if loaded, ok := model.Metadata["loaded"].(bool); ok && loaded {
			return "loaded"
		}
	}

	// Check model size to infer if it's loaded
	if model.Size > 0 {
		return "available"
	}

	return "unknown"
}

// mergeCapabilities merges two capability lists, removing duplicates
func (u *DefaultUnifier) mergeCapabilities(existing, newCaps []string) []string {
	capSet := make(map[string]bool)
	for _, cap := range existing {
		capSet[cap] = true
	}
	for _, cap := range newCaps {
		capSet[cap] = true
	}

	merged := make([]string, 0, len(capSet))
	for cap := range capSet {
		merged = append(merged, cap)
	}
	return merged
}

// removeModelFromEndpoint removes a model's association with an endpoint
func (u *DefaultUnifier) removeModelFromEndpoint(modelID, endpointURL, _ string) {
	unified, exists := u.catalog[modelID]
	if !exists {
		return
	}

	// Remove the source for this endpoint
	newSources := make([]domain.SourceEndpoint, 0, len(unified.SourceEndpoints))
	for _, source := range unified.SourceEndpoints {
		if source.EndpointURL != endpointURL {
			newSources = append(newSources, source)
		}
	}

	if len(newSources) == 0 {
		// No more sources - remove the model entirely
		// First remove from indices while we still have the model
		if unified.Metadata != nil {
			if digest, ok := unified.Metadata["digest"].(string); ok && digest != "" {
				u.removeFromIndex(u.digestIndex, digest, modelID)
			}
		}
		// Remove all name aliases from index
		for _, alias := range unified.Aliases {
			u.removeFromIndex(u.nameIndex, strings.ToLower(alias.Name), modelID)
		}

		// Now remove from catalog
		delete(u.catalog, modelID)
	} else {
		unified.SourceEndpoints = newSources
	}
}

// removeFromIndex removes a value from an index
func (u *DefaultUnifier) removeFromIndex(index map[string][]string, key, value string) {
	if values, exists := index[key]; exists {
		newValues := make([]string, 0, len(values))
		for _, v := range values {
			if v != value {
				newValues = append(newValues, v)
			}
		}
		if len(newValues) == 0 {
			delete(index, key)
		} else {
			index[key] = newValues
		}
	}
}

// cleanupStaleModels removes models that haven't been seen recently
func (u *DefaultUnifier) cleanupStaleModels() {
	staleThreshold := 24 * time.Hour
	now := time.Now()

	toRemove := []string{}
	for id, model := range u.catalog {
		if now.Sub(model.LastSeen) > staleThreshold {
			toRemove = append(toRemove, id)
		}
	}

	for _, id := range toRemove {
		model := u.catalog[id]
		delete(u.catalog, id)

		// Remove from indices
		if model.Metadata != nil {
			if digest, ok := model.Metadata["digest"].(string); ok && digest != "" {
				u.removeFromIndex(u.digestIndex, digest, id)
			}
		}
		// Remove by aliases
		for _, alias := range model.Aliases {
			u.removeFromIndex(u.nameIndex, strings.ToLower(alias.Name), id)
		}
	}

	// Silent cleanup - no logging in simplified implementation
}

// ResolveModel finds a model by name or ID
func (u *DefaultUnifier) ResolveModel(ctx context.Context, nameOrID string) (*domain.UnifiedModel, error) {
	u.mu.RLock()
	defer u.mu.RUnlock()

	// Try direct ID lookup first
	if model, exists := u.catalog[nameOrID]; exists {
		return model, nil
	}

	// Try case-insensitive name lookup
	lowercaseName := strings.ToLower(nameOrID)
	if modelIDs, exists := u.nameIndex[lowercaseName]; exists && len(modelIDs) > 0 {
		return u.catalog[modelIDs[0]], nil
	}

	// Try alias lookup
	for _, model := range u.catalog {
		for _, alias := range model.Aliases {
			if strings.EqualFold(alias.Name, nameOrID) {
				return model, nil
			}
		}
	}

	return nil, fmt.Errorf("model not found: %s", nameOrID)
}

// GetAllModels returns all unified models
func (u *DefaultUnifier) GetAllModels(ctx context.Context) ([]*domain.UnifiedModel, error) {
	u.mu.RLock()
	defer u.mu.RUnlock()

	models := make([]*domain.UnifiedModel, 0, len(u.catalog))
	for _, model := range u.catalog {
		models = append(models, model)
	}
	return models, nil
}

// GetDeduplicationStats returns internal deduplication statistics
func (u *DefaultUnifier) GetDeduplicationStats() DeduplicationStats {
	u.mu.RLock()
	defer u.mu.RUnlock()
	return u.stats
}

// Clear removes all cached data
func (u *DefaultUnifier) Clear(ctx context.Context) error {
	u.mu.Lock()
	defer u.mu.Unlock()

	u.catalog = make(map[string]*domain.UnifiedModel)
	u.digestIndex = make(map[string][]string)
	u.nameIndex = make(map[string][]string)
	u.endpointModels = make(map[string][]string)
	u.stats = DeduplicationStats{}

	return nil
}

// UnifyModel converts a single model to unified format
func (u *DefaultUnifier) UnifyModel(ctx context.Context, sourceModel *domain.ModelInfo, endpoint *domain.Endpoint) (*domain.UnifiedModel, error) {
	if sourceModel == nil {
		return nil, nil
	}

	u.mu.Lock()
	defer u.mu.Unlock()

	model := u.convertModelInfoToModel(sourceModel)
	return u.processModel(model, endpoint), nil
}

// convertModelInfoToModel converts ModelInfo to Model for internal processing
func (u *DefaultUnifier) convertModelInfoToModel(info *domain.ModelInfo) *Model {
	model := &Model{
		ID:       info.Name, // Use name as ID since ModelInfo doesn't have ID
		Name:     info.Name,
		Size:     info.Size,
		Metadata: make(map[string]interface{}),
	}

	// Extract ALL available metadata from Details
	if info.Details != nil {
		// Core fields
		if info.Details.Digest != nil {
			model.Digest = *info.Details.Digest
			model.Metadata["digest"] = *info.Details.Digest
		}
		if info.Details.Format != nil {
			model.Format = *info.Details.Format
			model.Metadata["format"] = *info.Details.Format
		}

		// Family extraction with fallback
		if info.Details.Family != nil {
			model.Family = *info.Details.Family
		} else if len(info.Details.Families) > 0 {
			model.Family = info.Details.Families[0]
			model.Metadata["families"] = info.Details.Families
		}

		// Context length
		if info.Details.MaxContextLength != nil {
			model.ContextWindow = *info.Details.MaxContextLength
			model.Metadata["max_context_length"] = *info.Details.MaxContextLength
		}

		// State information
		if info.Details.State != nil {
			model.Metadata["state"] = *info.Details.State
		}

		// Extract rich metadata that was being ignored
		if info.Details.ParameterSize != nil {
			model.Metadata["parameter_size"] = *info.Details.ParameterSize
		}
		if info.Details.QuantizationLevel != nil {
			model.Metadata["quantization_level"] = *info.Details.QuantizationLevel
		}
		if info.Details.Publisher != nil {
			model.Metadata["publisher"] = *info.Details.Publisher
		}
		if info.Details.Type != nil {
			model.Metadata["model_type"] = *info.Details.Type
		}
		if info.Details.ParentModel != nil {
			model.Metadata["parent_model"] = *info.Details.ParentModel
		}
		if info.Details.ModifiedAt != nil {
			model.Metadata["modified_at"] = info.Details.ModifiedAt.Format(time.RFC3339)
		}
	}

	// Store type in metadata (from LM Studio)
	if info.Type != "" {
		model.Metadata["type"] = info.Type
	}

	// Store description if available
	if info.Description != "" {
		model.Metadata["description"] = info.Description
	}

	// Store last seen time
	if !info.LastSeen.IsZero() {
		model.Metadata["last_seen"] = info.LastSeen.Format(time.RFC3339)
	}

	return model
}

// calculateMetadataConfidence calculates a confidence score for extracted metadata
func (u *DefaultUnifier) calculateMetadataConfidence(model *Model, paramSize, quantization, family string) float64 {
	confidence := 0.0
	fields := 0.0

	// Direct field mappings (high confidence)
	if paramSize != "" && model.Metadata["parameter_size"] != nil {
		confidence += 1.0
		fields++
	}
	if quantization != "" && (model.Metadata["quantization_level"] != nil || model.Metadata["quantization"] != nil) {
		confidence += 1.0
		fields++
	}
	if family != "" && model.Family != "" {
		confidence += 1.0
		fields++
	}
	if model.Digest != "" {
		confidence += 1.0
		fields++
	}
	if model.ContextWindow > 0 {
		confidence += 1.0
		fields++
	}

	// Inferred fields (medium confidence)
	if paramSize != "" && model.Metadata["parameter_size"] == nil {
		confidence += 0.5
		fields++
	}
	if quantization != "" && model.Metadata["quantization_level"] == nil && model.Metadata["quantization"] == nil {
		confidence += 0.5
		fields++
	}
	if family != "" && model.Family == "" {
		confidence += 0.5
		fields++
	}

	// Calculate overall confidence
	if fields > 0 {
		return confidence / fields
	}
	return 0.0
}

// ResolveAlias finds a model by alias (same as ResolveModel for simplified implementation)
func (u *DefaultUnifier) ResolveAlias(ctx context.Context, alias string) (*domain.UnifiedModel, error) {
	return u.ResolveModel(ctx, alias)
}

// GetAliases returns empty list as we don't maintain separate aliases
func (u *DefaultUnifier) GetAliases(ctx context.Context, unifiedID string) ([]string, error) {
	u.mu.RLock()
	defer u.mu.RUnlock()

	if _, exists := u.catalog[unifiedID]; exists {
		// Return the ID itself as the only alias
		return []string{unifiedID}, nil
	}
	return []string{}, nil
}

// RegisterCustomRule is a no-op for simplified implementation
func (u *DefaultUnifier) RegisterCustomRule(platformType string, rule ports.UnificationRule) error {
	// Simplified implementation doesn't support custom rules
	return nil
}

// GetStats returns unification statistics in the expected format
func (u *DefaultUnifier) GetStats() domain.UnificationStats {
	u.mu.RLock()
	defer u.mu.RUnlock()

	return domain.UnificationStats{
		TotalUnified:       int64(u.stats.UnifiedModels),
		TotalErrors:        0, // Simplified implementation doesn't track errors
		CacheHits:          int64(u.stats.DigestMatches + u.stats.NameMatches),
		CacheMisses:        int64(u.stats.TotalModels - u.stats.DigestMatches - u.stats.NameMatches),
		LastUnificationAt:  u.stats.LastUpdated,
		AverageUnifyTimeMs: 0, // Not tracked in simplified implementation
	}
}

// MergeUnifiedModels merges multiple unified models into one
func (u *DefaultUnifier) MergeUnifiedModels(ctx context.Context, models []*domain.UnifiedModel) (*domain.UnifiedModel, error) {
	if len(models) == 0 {
		return nil, nil
	}
	if len(models) == 1 {
		return models[0], nil
	}

	// Start with the first model as base
	merged := &domain.UnifiedModel{
		ID:               models[0].ID,
		Family:           models[0].Family,
		Variant:          models[0].Variant,
		ParameterSize:    models[0].ParameterSize,
		ParameterCount:   models[0].ParameterCount,
		Quantization:     models[0].Quantization,
		Format:           models[0].Format,
		Aliases:          make([]domain.AliasEntry, 0),
		SourceEndpoints:  make([]domain.SourceEndpoint, 0),
		Capabilities:     make([]string, 0),
		MaxContextLength: models[0].MaxContextLength,
		DiskSize:         0,
		LastSeen:         models[0].LastSeen,
		Metadata:         make(map[string]interface{}),
	}

	// Merge all sources
	sourceMap := make(map[string]domain.SourceEndpoint)
	totalDiskSize := int64(0)
	for _, model := range models {
		for _, source := range model.SourceEndpoints {
			key := source.EndpointURL
			if existing, exists := sourceMap[key]; exists {
				// Update if newer
				if source.LastSeen.After(existing.LastSeen) {
					sourceMap[key] = source
				}
			} else {
				sourceMap[key] = source
			}
		}

		// Update last seen
		if model.LastSeen.After(merged.LastSeen) {
			merged.LastSeen = model.LastSeen
		}
	}

	// Convert map back to slice and calculate total disk size
	for _, source := range sourceMap {
		merged.SourceEndpoints = append(merged.SourceEndpoints, source)
		totalDiskSize += source.DiskSize
	}
	merged.DiskSize = totalDiskSize

	// Merge aliases
	aliasMap := make(map[string]domain.AliasEntry)
	for _, model := range models {
		for _, alias := range model.Aliases {
			aliasMap[alias.Name] = alias
		}
	}
	for _, alias := range aliasMap {
		merged.Aliases = append(merged.Aliases, alias)
	}

	// Merge capabilities
	capSet := make(map[string]bool)
	for _, model := range models {
		for _, cap := range model.Capabilities {
			capSet[cap] = true
		}
	}
	for cap := range capSet {
		merged.Capabilities = append(merged.Capabilities, cap)
	}

	// Merge metadata (last one wins)
	for _, model := range models {
		for k, v := range model.Metadata {
			merged.Metadata[k] = v
		}
	}

	return merged, nil
}
