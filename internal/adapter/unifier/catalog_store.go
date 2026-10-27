package unifier

import (
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/thushan/olla/internal/core/domain"
)

// CatalogStore provides thread-safe storage with fast lookups
// by ID, name, and digest. Uses atomic pointers for zero-copy reads.
type CatalogStore struct {
	lastCleanup     time.Time
	catalog         map[string]*atomic.Pointer[domain.UnifiedModel]
	digestIndex     map[string][]string
	nameIndex       map[string][]string
	endpointModels  map[string][]string
	cleanupInterval time.Duration
	mu              sync.RWMutex
}

func NewCatalogStore(cleanupInterval time.Duration) *CatalogStore {
	return &CatalogStore{
		catalog:         make(map[string]*atomic.Pointer[domain.UnifiedModel]),
		digestIndex:     make(map[string][]string),
		nameIndex:       make(map[string][]string),
		endpointModels:  make(map[string][]string),
		cleanupInterval: cleanupInterval,
		lastCleanup:     time.Now(),
	}
}

func (c *CatalogStore) GetModel(id string) (*domain.UnifiedModel, bool) {
	c.mu.RLock()
	ptr := c.catalog[id]
	c.mu.RUnlock()

	if ptr == nil {
		return nil, false
	}
	// Zero-copy read via atomic load. The stored model is immutable: it is
	// shared with lock-free readers, so callers must never write to it (or to
	// its slices and maps). Writers call Clone, mutate the clone and PutModel it.
	return ptr.Load(), true
}

func (c *CatalogStore) PutModel(model *domain.UnifiedModel) {
	if model == nil {
		return
	}

	// Copy-on-write: deep copy before storing
	// this allows callers to modify their reference while we store immutable version
	c.putOwned(model.Clone())
}

// putOwned stores a model the caller has already cloned and will not touch again,
// saving the second copy PutModel makes for writers that cloned to mutate.
func (c *CatalogStore) putOwned(modelCopy *domain.UnifiedModel) {
	c.mu.Lock()
	defer c.mu.Unlock()

	ptr, exists := c.catalog[modelCopy.ID]
	if !exists {
		ptr = &atomic.Pointer[domain.UnifiedModel]{}
		c.catalog[modelCopy.ID] = ptr
	}
	ptr.Store(modelCopy)

	if modelCopy.Metadata != nil {
		if digest, ok := modelCopy.Metadata["digest"].(string); ok && digest != "" {
			c.addToIndex(c.digestIndex, digest, modelCopy.ID)
		}
	}

	for _, alias := range modelCopy.Aliases {
		c.addToIndex(c.nameIndex, strings.ToLower(alias.Name), modelCopy.ID)
	}
}

func (c *CatalogStore) GetAllModels() []*domain.UnifiedModel {
	c.mu.RLock()
	defer c.mu.RUnlock()

	models := make([]*domain.UnifiedModel, 0, len(c.catalog))
	for _, ptr := range c.catalog {
		if ptr != nil {
			if model := ptr.Load(); model != nil {
				models = append(models, model)
			}
		}
	}
	return models
}

// ResolveByName finds a model by ID or alias. An exact match always wins; only a
// miss on a name without a tag tries "<name>:latest", and that only matches a model
// an Ollama endpoint serves under that native name (see domain.OllamaDefaultTagName).
func (c *CatalogStore) ResolveByName(name string) (*domain.UnifiedModel, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if model, ok := c.lookupLocked(name, true); ok {
		return model, true
	}

	tagged, ok := domain.OllamaDefaultTagName(name)
	if !ok {
		return nil, false
	}
	// The alias index already holds every alias, so the full scan adds nothing here.
	model, ok := c.lookupLocked(tagged, false)
	if !ok || !servedByOllamaAs(model, tagged) {
		return nil, false
	}
	return model, true
}

// ResolveExactName is ResolveByName without the Ollama default-tag fallback. Ingest
// uses it to decide whether a discovered model is one already in the catalogue: a
// vLLM model called "foo" is not the Ollama model "foo:latest" and must not merge
// into it.
func (c *CatalogStore) ResolveExactName(name string) (*domain.UnifiedModel, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.lookupLocked(name, true)
}

// servedByOllamaAs reports whether an Ollama endpoint serves model under nativeName.
// A merged model can also be served by other platforms under the same name, and
// those must not make a bare name resolve.
func servedByOllamaAs(model *domain.UnifiedModel, nativeName string) bool {
	for i := range model.SourceEndpoints {
		src := &model.SourceEndpoints[i]
		if src.Platform == domain.ProfileOllama && strings.EqualFold(src.NativeName, nativeName) {
			return true
		}
	}
	return false
}

// lookupLocked resolves name by ID, then the lower-cased alias index, then (when scan
// is set) an alias scan. Callers hold c.mu.
func (c *CatalogStore) lookupLocked(name string, scan bool) (*domain.UnifiedModel, bool) {
	// Direct ID lookup is fastest
	if ptr, exists := c.catalog[name]; exists && ptr != nil {
		if model := ptr.Load(); model != nil {
			return model, true
		}
	}

	lowercaseName := strings.ToLower(name)
	if modelIDs, exists := c.nameIndex[lowercaseName]; exists && len(modelIDs) > 0 {
		if ptr, exists := c.catalog[modelIDs[0]]; exists && ptr != nil {
			if model := ptr.Load(); model != nil {
				return model, true
			}
		}
	}

	if !scan {
		return nil, false
	}

	// Fallback to full scan for aliases
	for _, ptr := range c.catalog {
		if ptr != nil {
			if model := ptr.Load(); model != nil {
				for _, alias := range model.Aliases {
					if strings.EqualFold(alias.Name, name) {
						return model, true
					}
				}
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
		if ptr, exists := c.catalog[id]; exists && ptr != nil {
			if model := ptr.Load(); model != nil {
				models = append(models, model)
			}
		}
	}

	return models, len(models) > 0
}

func (c *CatalogStore) RemoveModel(id string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	ptr, exists := c.catalog[id]
	if !exists || ptr == nil {
		return false
	}

	model := ptr.Load()
	if model == nil {
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
	for id, ptr := range c.catalog {
		if ptr != nil {
			if model := ptr.Load(); model != nil {
				if now.Sub(model.LastSeen) > staleThreshold {
					toRemove = append(toRemove, id)
				}
			}
		}
	}

	for _, id := range toRemove {
		ptr := c.catalog[id]
		if ptr != nil {
			if model := ptr.Load(); model != nil {
				if model.Metadata != nil {
					if digest, ok := model.Metadata["digest"].(string); ok && digest != "" {
						c.removeFromIndex(c.digestIndex, digest, id)
					}
				}

				for _, alias := range model.Aliases {
					c.removeFromIndex(c.nameIndex, strings.ToLower(alias.Name), id)
				}
			}
		}
		delete(c.catalog, id)
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
