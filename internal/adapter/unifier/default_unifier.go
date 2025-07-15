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

// nilIfZero returns nil if value is 0, otherwise returns pointer to value
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

	// Process each model
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

	// Update stats
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
		// Check if any can be merged
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
func (u *DefaultUnifier) canMergeByName(existing *domain.UnifiedModel, new *Model) bool {
	// Only merge if names match exactly (case-insensitive)
	// Check against aliases
	for _, alias := range existing.Aliases {
		if strings.EqualFold(alias.Name, new.Name) {
			// Check for digest conflicts
			if new.Digest != "" && existing.Metadata != nil {
				if existingDigest, ok := existing.Metadata["digest"].(string); ok {
					if existingDigest != "" && existingDigest != new.Digest {
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

	// Build capabilities list
	capabilities := u.extractCapabilities(model)

	// Prepare metadata
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

	contextWindow := model.ContextWindow
	return &domain.UnifiedModel{
		ID:               id,
		Family:           model.Family,
		Variant:          "", // Would need to parse from name
		ParameterSize:    "", // Would need to parse from size
		ParameterCount:   model.Size,
		Quantization:     "", // Would need to parse from name
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

	// Merge capabilities
	newCaps := u.extractCapabilities(model)
	unified.Capabilities = u.mergeCapabilities(unified.Capabilities, newCaps)

	// Update disk size
	unified.DiskSize += model.Size

	// Update digest in metadata if present
	if model.Digest != "" {
		if unified.Metadata == nil {
			unified.Metadata = make(map[string]interface{})
		}
		unified.Metadata["digest"] = model.Digest
	}

	unified.LastSeen = time.Now()
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

// extractCapabilities extracts capabilities from model metadata
func (u *DefaultUnifier) extractCapabilities(model *Model) []string {
	caps := make([]string, 0)

	// Check metadata for explicit capabilities
	if model.Metadata != nil {
		if capabilities, ok := model.Metadata["capabilities"].([]interface{}); ok {
			for _, cap := range capabilities {
				if capStr, ok := cap.(string); ok {
					caps = append(caps, capStr)
				}
			}
		}
	}

	// Infer from model properties
	if model.ContextWindow > 0 {
		caps = append(caps, fmt.Sprintf("context:%d", model.ContextWindow))
	}

	// Always add text-generation as a base capability
	caps = append(caps, "text-generation")

	return caps
}

// mergeCapabilities merges two capability lists, removing duplicates
func (u *DefaultUnifier) mergeCapabilities(existing, new []string) []string {
	capSet := make(map[string]bool)
	for _, cap := range existing {
		capSet[cap] = true
	}
	for _, cap := range new {
		capSet[cap] = true
	}

	merged := make([]string, 0, len(capSet))
	for cap := range capSet {
		merged = append(merged, cap)
	}
	return merged
}

// removeModelFromEndpoint removes a model's association with an endpoint
func (u *DefaultUnifier) removeModelFromEndpoint(modelID, endpointURL, endpointName string) {
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

	// Extract additional fields from Details if available
	if info.Details != nil {
		if info.Details.Digest != nil {
			model.Digest = *info.Details.Digest
		}
		if info.Details.Format != nil {
			model.Format = *info.Details.Format
		}
		if info.Details.Family != nil {
			model.Family = *info.Details.Family
		} else if len(info.Details.Families) > 0 {
			model.Family = info.Details.Families[0]
		}
		if info.Details.MaxContextLength != nil {
			model.ContextWindow = *info.Details.MaxContextLength
		}
		// Store state in metadata
		if info.Details.State != nil {
			model.Metadata["state"] = *info.Details.State
		}
	}

	// Store type in metadata
	if info.Type != "" {
		model.Metadata["type"] = info.Type
	}

	return model
}

// parseSizeString attempts to parse size from string format
func (u *DefaultUnifier) parseSizeString(size string) int64 {
	// Simple implementation - just return 0 for now
	// In a real implementation, this would parse "7B" -> 7000000000, etc.
	return 0
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
