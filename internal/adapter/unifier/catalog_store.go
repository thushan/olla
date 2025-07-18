package unifier

import (
	"strings"
	"sync"
	"time"

	"github.com/thushan/olla/internal/core/domain"
)

// CatalogStore provides thread-safe storage with fast lookups
// by ID, name, and digest
type CatalogStore struct {
	lastCleanup     time.Time
	catalog         map[string]*domain.UnifiedModel
	digestIndex     map[string][]string
	nameIndex       map[string][]string
	endpointModels  map[string][]string
	cleanupInterval time.Duration
	mu              sync.RWMutex
}

func NewCatalogStore(cleanupInterval time.Duration) *CatalogStore {
	return &CatalogStore{
		catalog:         make(map[string]*domain.UnifiedModel),
		digestIndex:     make(map[string][]string),
		nameIndex:       make(map[string][]string),
		endpointModels:  make(map[string][]string),
		cleanupInterval: cleanupInterval,
		lastCleanup:     time.Now(),
	}
}

func (c *CatalogStore) GetModel(id string) (*domain.UnifiedModel, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	model, exists := c.catalog[id]
	if !exists {
		return nil, false
	}
	return c.deepCopyModel(model), true
}

func (c *CatalogStore) PutModel(model *domain.UnifiedModel) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.catalog[model.ID] = model

	if model.Metadata != nil {
		if digest, ok := model.Metadata["digest"].(string); ok && digest != "" {
			c.addToIndex(c.digestIndex, digest, model.ID)
		}
	}

	for _, alias := range model.Aliases {
		c.addToIndex(c.nameIndex, strings.ToLower(alias.Name), model.ID)
	}
}

func (c *CatalogStore) GetAllModels() []*domain.UnifiedModel {
	c.mu.RLock()
	defer c.mu.RUnlock()

	models := make([]*domain.UnifiedModel, 0, len(c.catalog))
	for _, model := range c.catalog {
		// Deep copy prevents races when caller modifies returned models
		modelCopy := c.deepCopyModel(model)
		models = append(models, modelCopy)
	}
	return models
}

func (c *CatalogStore) ResolveByName(name string) (*domain.UnifiedModel, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// Direct ID lookup is fastest
	if model, exists := c.catalog[name]; exists {
		return c.deepCopyModel(model), true
	}

	lowercaseName := strings.ToLower(name)
	if modelIDs, exists := c.nameIndex[lowercaseName]; exists && len(modelIDs) > 0 {
		if model, exists := c.catalog[modelIDs[0]]; exists {
			return c.deepCopyModel(model), true
		}
	}

	// Fallback to full scan for aliases
	for _, model := range c.catalog {
		for _, alias := range model.Aliases {
			if strings.EqualFold(alias.Name, name) {
				return c.deepCopyModel(model), true
			}
		}
	}

	return nil, false
}

func (c *CatalogStore) ResolveByDigest(digest string) ([]*domain.UnifiedModel, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	modelIDs, exists := c.digestIndex[digest]
	if !exists || len(modelIDs) == 0 {
		return nil, false
	}

	models := make([]*domain.UnifiedModel, 0, len(modelIDs))
	for _, id := range modelIDs {
		if model, exists := c.catalog[id]; exists {
			models = append(models, c.deepCopyModel(model))
		}
	}

	return models, len(models) > 0
}

func (c *CatalogStore) RemoveModel(id string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	model, exists := c.catalog[id]
	if !exists {
		return false
	}
	if model.Metadata != nil {
		if digest, ok := model.Metadata["digest"].(string); ok && digest != "" {
			c.removeFromIndex(c.digestIndex, digest, id)
		}
	}

	for _, alias := range model.Aliases {
		c.removeFromIndex(c.nameIndex, strings.ToLower(alias.Name), id)
	}

	delete(c.catalog, id)
	return true
}

func (c *CatalogStore) GetEndpointModels(endpointURL string) []string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	models := c.endpointModels[endpointURL]
	result := make([]string, len(models))
	copy(result, models)
	return result
}

func (c *CatalogStore) SetEndpointModels(endpointURL string, modelIDs []string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.endpointModels[endpointURL] = modelIDs
}

func (c *CatalogStore) ClearEndpointModels(endpointURL string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.endpointModels, endpointURL)
}

func (c *CatalogStore) CleanupStaleModels(staleThreshold time.Duration) []string {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	c.lastCleanup = now

	toRemove := []string{}
	for id, model := range c.catalog {
		if now.Sub(model.LastSeen) > staleThreshold {
			toRemove = append(toRemove, id)
		}
	}

	for _, id := range toRemove {
		model := c.catalog[id]
		delete(c.catalog, id)

		if model.Metadata != nil {
			if digest, ok := model.Metadata["digest"].(string); ok && digest != "" {
				c.removeFromIndex(c.digestIndex, digest, id)
			}
		}

		for _, alias := range model.Aliases {
			c.removeFromIndex(c.nameIndex, strings.ToLower(alias.Name), id)
		}
	}

	return toRemove
}

func (c *CatalogStore) NeedsCleanup() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return time.Since(c.lastCleanup) > c.cleanupInterval
}

func (c *CatalogStore) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return len(c.catalog)
}

func (c *CatalogStore) addToIndex(index map[string][]string, key, value string) {
	if values, exists := index[key]; exists {
		for _, v := range values {
			if v == value {
				return
			}
		}
		index[key] = append(values, value)
	} else {
		index[key] = []string{value}
	}
}

func (c *CatalogStore) removeFromIndex(index map[string][]string, key, value string) {
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

// deepCopyModel prevents race conditions when models are accessed concurrently
func (c *CatalogStore) deepCopyModel(model *domain.UnifiedModel) *domain.UnifiedModel {
	if model == nil {
		return nil
	}
	modelCopy := &domain.UnifiedModel{
		ID:               model.ID,
		Family:           model.Family,
		Variant:          model.Variant,
		ParameterSize:    model.ParameterSize,
		Quantization:     model.Quantization,
		Format:           model.Format,
		PromptTemplateID: model.PromptTemplateID,
		ParameterCount:   model.ParameterCount,
		DiskSize:         model.DiskSize,
		LastSeen:         model.LastSeen,
		MaxContextLength: model.MaxContextLength,
	}

	if model.Aliases != nil {
		modelCopy.Aliases = make([]domain.AliasEntry, len(model.Aliases))
		copy(modelCopy.Aliases, model.Aliases)
	}

	if model.Capabilities != nil {
		modelCopy.Capabilities = make([]string, len(model.Capabilities))
		copy(modelCopy.Capabilities, model.Capabilities)
	}

	// Source endpoints need special handling to avoid data races
	if model.SourceEndpoints != nil {
		modelCopy.SourceEndpoints = make([]domain.SourceEndpoint, len(model.SourceEndpoints))
		for i, endpoint := range model.SourceEndpoints {
			modelCopy.SourceEndpoints[i] = domain.SourceEndpoint{
				EndpointURL:    endpoint.EndpointURL,
				EndpointName:   endpoint.EndpointName,
				NativeName:     endpoint.NativeName,
				State:          endpoint.State,
				ModelState:     endpoint.ModelState,
				DiskSize:       endpoint.DiskSize,
				LastSeen:       endpoint.LastSeen,
				LastStateCheck: endpoint.LastStateCheck,
			}
			if endpoint.StateInfo != nil {
				modelCopy.SourceEndpoints[i].StateInfo = &domain.EndpointStateInfo{
					State:               endpoint.StateInfo.State,
					ConsecutiveFailures: endpoint.StateInfo.ConsecutiveFailures,
					LastStateChange:     endpoint.StateInfo.LastStateChange,
					LastError:           endpoint.StateInfo.LastError,
					Metadata:            endpoint.StateInfo.Metadata,
				}
			}
		}
	}

	if model.Metadata != nil {
		modelCopy.Metadata = make(map[string]interface{}, len(model.Metadata))
		for k, v := range model.Metadata {
			modelCopy.Metadata[k] = v
		}
	}

	return modelCopy
}
