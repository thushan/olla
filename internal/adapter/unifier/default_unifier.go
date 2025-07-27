package unifier

import (
	"context"
	"fmt"
	"time"

	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/core/ports"
)

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

type DeduplicationStats struct {
	LastUpdated   time.Time
	TotalModels   int
	UnifiedModels int
	DigestMatches int
	NameMatches   int
}

// DefaultUnifier handles model deduplication across multiple endpoints
// using digest and name matching strategies
type DefaultUnifier struct {
	store          *CatalogStore
	extractor      *ModelExtractor
	stats          DeduplicationStats
	staleThreshold time.Duration
}

func NewDefaultUnifier() ports.ModelUnifier {
	return &DefaultUnifier{
		store:          NewCatalogStore(5 * time.Minute),
		extractor:      NewModelExtractor(),
		staleThreshold: 24 * time.Hour,
	}
}

func (u *DefaultUnifier) cleanupStaleModels(threshold time.Duration) {
	if threshold <= 0 {
		threshold = u.staleThreshold
	}
	u.store.CleanupStaleModels(threshold)
}

func (u *DefaultUnifier) UnifyModels(ctx context.Context, models []*domain.ModelInfo, endpoint *domain.Endpoint) ([]*domain.UnifiedModel, error) {
	// Empty model lists are valid - they indicate endpoint has no models anymore
	if models == nil {
		models = []*domain.ModelInfo{}
	}

	if u.store.NeedsCleanup() {
		u.store.CleanupStaleModels(u.staleThreshold)
	}

	endpointURL := endpoint.GetURLString()

	// Remove stale models when endpoint updates its model list
	oldModelIDs := u.store.GetEndpointModels(endpointURL)
	for _, modelID := range oldModelIDs {
		u.removeModelFromEndpoint(modelID, endpointURL)
	}

	processedModels := make([]*domain.UnifiedModel, 0, len(models))
	newModelIDs := make([]string, 0, len(models))

	for _, modelInfo := range models {
		if modelInfo == nil {
			continue
		}

		unified := u.processModel(modelInfo, endpoint)
		if unified != nil {
			processedModels = append(processedModels, unified)
			newModelIDs = append(newModelIDs, unified.ID)
		}
	}

	u.store.SetEndpointModels(endpointURL, newModelIDs)
	u.stats.TotalModels = u.store.Size()
	u.stats.UnifiedModels = len(processedModels)
	u.stats.LastUpdated = time.Now()

	// Store returns deep copies to prevent races if caller modifies the models
	result := make([]*domain.UnifiedModel, 0, len(processedModels))
	for _, model := range processedModels {
		if modelCopy, found := u.store.GetModel(model.ID); found {
			result = append(result, modelCopy)
		}
	}

	return result, nil
}

func (u *DefaultUnifier) processModel(modelInfo *domain.ModelInfo, endpoint *domain.Endpoint) *domain.UnifiedModel {
	model := u.convertModelInfoToModel(modelInfo)

	// Digest matching is most reliable - same binary content
	if model.Digest != "" {
		if existingModels, found := u.store.ResolveByDigest(model.Digest); found && len(existingModels) > 0 {
			existing := existingModels[0]
			u.mergeModel(existing, model, endpoint)
			u.stats.DigestMatches++
			updated, _ := u.store.GetModel(existing.ID)
			return updated
		}
	}

	// Fall back to name matching if no digest
	if existing, found := u.store.ResolveByName(model.Name); found {
		if u.canMergeByName(existing, model) {
			u.mergeModel(existing, model, endpoint)
			u.stats.NameMatches++
			updated, _ := u.store.GetModel(existing.ID)
			return updated
		}
	}

	unified := u.createUnifiedModel(model, endpoint)
	u.store.PutModel(unified)
	return unified
}

func (u *DefaultUnifier) createUnifiedModel(model *Model, endpoint *domain.Endpoint) *domain.UnifiedModel {
	baseID := model.Name
	if model.ID != "" {
		baseID = model.ID
	}

	// Avoid ID collisions when models have same name but different digests
	if existing, found := u.store.GetModel(baseID); found {
		var existingDigest string
		if existing.Metadata != nil {
			existingDigest, _ = existing.Metadata["digest"].(string)
		}
		baseID = generateUniqueID(baseID, model.Digest, existingDigest)
	}

	platform := u.extractor.DetectPlatform(model.Format, model.Metadata, endpoint.Type)
	paramSize, paramCount := u.extractor.ExtractParameterInfo(model.Metadata, model.Size)
	quantization := u.extractor.ExtractQuantization(model.Metadata)
	family, variant := u.extractor.ExtractFamilyAndVariant(model.Name, model.Family, model.Metadata)
	modelType := u.extractor.ExtractModelType(model.Metadata)
	state := u.extractor.MapModelState(model.Metadata, model.Size)
	publisher := u.extractor.ExtractPublisher(model.Name, model.Metadata)

	capabilities := inferCapabilitiesFromMetadata(modelType, model.Name, model.ContextWindow, model.Metadata)
	confidence := u.extractor.CalculateConfidence(
		model.Digest != "",
		paramSize != "",
		quantization != "",
		family != "",
		model.ContextWindow > 0,
	)

	sourceEndpoint := NewSourceEndpointBuilder().
		WithURL(endpoint.GetURLString()).
		WithName(endpoint.Name).
		WithNativeName(model.Name).
		WithState(state).
		WithDiskSize(model.Size).
		Build()

	builder := NewModelBuilder().
		WithID(baseID).
		WithFamily(family, variant).
		WithParameters(paramSize, paramCount).
		WithQuantization(quantization).
		WithFormat(model.Format).
		WithContextLength(model.ContextWindow).
		WithDiskSize(model.Size).
		AddAlias(model.Name, platform).
		AddSourceEndpoint(sourceEndpoint).
		AddCapabilities(capabilities).
		WithDigest(model.Digest).
		WithPlatform(platform).
		WithPublisher(publisher).
		WithConfidence(confidence)

	if model.Metadata != nil {
		for k, v := range model.Metadata {
			builder.WithMetadata(k, v)
		}
	}

	return builder.Build()
}

func (u *DefaultUnifier) mergeModel(unified *domain.UnifiedModel, model *Model, endpoint *domain.Endpoint) {
	if unified == nil || model == nil {
		return
	}

	// Safe to modify - store returns deep copies

	endpointURL := endpoint.GetURLString()

	updated := false
	for i, source := range unified.SourceEndpoints {
		if source.EndpointURL == endpointURL {
			state := u.extractor.MapModelState(model.Metadata, model.Size)
			unified.SourceEndpoints[i].NativeName = model.Name
			unified.SourceEndpoints[i].State = state
			unified.SourceEndpoints[i].LastSeen = time.Now()
			unified.SourceEndpoints[i].DiskSize = model.Size
			updated = true
			break
		}
	}

	if !updated {
		platform := u.extractor.DetectPlatform(model.Format, model.Metadata, endpoint.Type)
		state := u.extractor.MapModelState(model.Metadata, model.Size)

		sourceEndpoint := NewSourceEndpointBuilder().
			WithURL(endpointURL).
			WithName(endpoint.Name).
			WithNativeName(model.Name).
			WithState(state).
			WithDiskSize(model.Size).
			Build()

		unified.SourceEndpoints = append(unified.SourceEndpoints, sourceEndpoint)

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
	}

	// Endpoints may report different metadata for same model - keep best info
	paramSize, paramCount := u.extractor.ExtractParameterInfo(model.Metadata, model.Size)
	quantization := u.extractor.ExtractQuantization(model.Metadata)
	if paramSize != "" && unified.ParameterSize == "" {
		unified.ParameterSize = paramSize
		unified.ParameterCount = paramCount
	}
	if quantization != "" && unified.Quantization == "" {
		unified.Quantization = quantization
	}
	if model.ContextWindow > 0 && (unified.MaxContextLength == nil || *unified.MaxContextLength == 0) {
		unified.MaxContextLength = &model.ContextWindow
	}
	if model.Format != "" && unified.Format == "" {
		unified.Format = model.Format
	}
	modelType := u.extractor.ExtractModelType(model.Metadata)
	newCaps := inferCapabilitiesFromMetadata(modelType, model.Name, model.ContextWindow, model.Metadata)
	unified.Capabilities = u.mergeCapabilities(unified.Capabilities, newCaps)

	totalSize := int64(0)
	for _, source := range unified.SourceEndpoints {
		totalSize += source.DiskSize
	}
	unified.DiskSize = totalSize

	u.mergeMetadata(unified, model)
	unified.LastSeen = time.Now()
	u.store.PutModel(unified)
}

func (u *DefaultUnifier) canMergeByName(existing *domain.UnifiedModel, newModel *Model) bool {
	// Different digest means different model even with same name
	if newModel.Digest != "" && existing.Metadata != nil {
		if existingDigest, ok := existing.Metadata["digest"].(string); ok {
			if existingDigest != "" && existingDigest != newModel.Digest {
				return false
			}
		}
	}
	return true
}

func (u *DefaultUnifier) mergeMetadata(unified *domain.UnifiedModel, model *Model) {
	if unified.Metadata == nil {
		unified.Metadata = make(map[string]interface{})
	}
	if model.Digest != "" {
		unified.Metadata["digest"] = model.Digest
	}

	for k, v := range model.Metadata {
		switch k {
		case "platform":
			// Track all platforms hosting this model
			if vStr, ok := v.(string); ok {
				if platforms, ok := unified.Metadata["platforms"].(map[string]bool); ok {
					platforms[vStr] = true
				} else {
					unified.Metadata["platforms"] = map[string]bool{vStr: true}
				}
			}
		default:
			unified.Metadata[k] = v
		}
	}
}

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

func (u *DefaultUnifier) removeModelFromEndpoint(modelID, endpointURL string) {
	model, exists := u.store.GetModel(modelID)
	if !exists {
		return
	}

	// GetModel returns a deep copy
	newSources := make([]domain.SourceEndpoint, 0, len(model.SourceEndpoints))
	for _, source := range model.SourceEndpoints {
		if source.EndpointURL != endpointURL {
			newSources = append(newSources, source)
		}
	}

	if len(newSources) == 0 {
		u.store.RemoveModel(modelID)
	} else {
		model.SourceEndpoints = newSources
		totalSize := int64(0)
		for _, source := range newSources {
			totalSize += source.DiskSize
		}
		model.DiskSize = totalSize
		u.store.PutModel(model)
	}
}

func (u *DefaultUnifier) convertModelInfoToModel(info *domain.ModelInfo) *Model {
	model := &Model{
		ID:       info.Name,
		Name:     info.Name,
		Size:     info.Size,
		Metadata: make(map[string]interface{}),
	}

	if info.Details != nil {
		if info.Details.Digest != nil {
			model.Digest = *info.Details.Digest
			model.Metadata["digest"] = *info.Details.Digest
		}
		if info.Details.Format != nil {
			model.Format = *info.Details.Format
			model.Metadata["format"] = *info.Details.Format
		}
		if info.Details.Family != nil {
			model.Family = *info.Details.Family
		} else if len(info.Details.Families) > 0 {
			model.Family = info.Details.Families[0]
			model.Metadata["families"] = info.Details.Families
		}
		if info.Details.MaxContextLength != nil {
			model.ContextWindow = *info.Details.MaxContextLength
			model.Metadata["max_context_length"] = *info.Details.MaxContextLength
		}
		if info.Details.State != nil {
			model.Metadata["state"] = *info.Details.State
		}
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

	if info.Type != "" {
		model.Metadata["type"] = info.Type
	}
	if info.Description != "" {
		model.Metadata["description"] = info.Description
	}
	if !info.LastSeen.IsZero() {
		model.Metadata["last_seen"] = info.LastSeen.Format(time.RFC3339)
	}

	return model
}

func (u *DefaultUnifier) UnifyModel(ctx context.Context, sourceModel *domain.ModelInfo, endpoint *domain.Endpoint) (*domain.UnifiedModel, error) {
	if sourceModel == nil {
		return nil, nil
	}
	return u.processModel(sourceModel, endpoint), nil
}

func (u *DefaultUnifier) ResolveModel(ctx context.Context, nameOrID string) (*domain.UnifiedModel, error) {
	model, found := u.store.ResolveByName(nameOrID)
	if !found {
		return nil, fmt.Errorf("model not found: %s", nameOrID)
	}
	return model, nil
}

func (u *DefaultUnifier) ResolveAlias(ctx context.Context, alias string) (*domain.UnifiedModel, error) {
	return u.ResolveModel(ctx, alias)
}

func (u *DefaultUnifier) GetAllModels(ctx context.Context) ([]*domain.UnifiedModel, error) {
	return u.store.GetAllModels(), nil
}

func (u *DefaultUnifier) GetAliases(ctx context.Context, unifiedID string) ([]string, error) {
	model, exists := u.store.GetModel(unifiedID)
	if !exists {
		return []string{}, nil
	}

	aliases := make([]string, 0, len(model.Aliases)+1)
	aliases = append(aliases, unifiedID)
	for _, alias := range model.Aliases {
		aliases = append(aliases, alias.Name)
	}
	return aliases, nil
}

func (u *DefaultUnifier) RegisterCustomRule(platformType string, rule ports.UnificationRule) error {
	return nil
}

func (u *DefaultUnifier) GetStats() domain.UnificationStats {
	return domain.UnificationStats{
		TotalUnified:       int64(u.stats.UnifiedModels),
		TotalErrors:        0,
		CacheHits:          int64(u.stats.DigestMatches + u.stats.NameMatches),
		CacheMisses:        int64(u.stats.TotalModels - u.stats.DigestMatches - u.stats.NameMatches),
		LastUnificationAt:  u.stats.LastUpdated,
		AverageUnifyTimeMs: 0,
	}
}

func (u *DefaultUnifier) MergeUnifiedModels(ctx context.Context, models []*domain.UnifiedModel) (*domain.UnifiedModel, error) {
	if len(models) == 0 {
		return nil, nil
	}
	if len(models) == 1 {
		return models[0], nil
	}

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

	sourceMap := make(map[string]domain.SourceEndpoint)
	aliasMap := make(map[string]domain.AliasEntry)
	capSet := make(map[string]bool)

	for _, model := range models {
		for _, source := range model.SourceEndpoints {
			key := source.EndpointURL
			if existing, exists := sourceMap[key]; exists {
				if source.LastSeen.After(existing.LastSeen) {
					sourceMap[key] = source
				}
			} else {
				sourceMap[key] = source
			}
		}

		for _, alias := range model.Aliases {
			aliasMap[alias.Name] = alias
		}

		for _, cap := range model.Capabilities {
			capSet[cap] = true
		}

		if model.LastSeen.After(merged.LastSeen) {
			merged.LastSeen = model.LastSeen
		}

		for k, v := range model.Metadata {
			merged.Metadata[k] = v
		}
	}

	totalDiskSize := int64(0)
	for _, source := range sourceMap {
		merged.SourceEndpoints = append(merged.SourceEndpoints, source)
		totalDiskSize += source.DiskSize
	}
	merged.DiskSize = totalDiskSize

	for _, alias := range aliasMap {
		merged.Aliases = append(merged.Aliases, alias)
	}

	for cap := range capSet {
		merged.Capabilities = append(merged.Capabilities, cap)
	}

	return merged, nil
}

func (u *DefaultUnifier) Clear(ctx context.Context) error {
	u.store = NewCatalogStore(5 * time.Minute)
	u.stats = DeduplicationStats{}
	return nil
}
