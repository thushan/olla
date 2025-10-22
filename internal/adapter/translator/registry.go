package translator

import (
	"fmt"
	"sort"
	"sync"

	"github.com/thushan/olla/internal/logger"
)

// manages registered translators, scales to unlimited formats without hardcoded deps
type Registry struct {
	translators map[string]RequestTranslator
	logger      logger.StyledLogger
	mu          sync.RWMutex
}

// NewRegistry creates a new translator registry instance
func NewRegistry(logger logger.StyledLogger) *Registry {
	return &Registry{
		translators: make(map[string]RequestTranslator),
		logger:      logger,
	}
}

// Register adds a translator to the registry
// uses format name (from Name()) to keep things consistent
func (r *Registry) Register(name string, translator RequestTranslator) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if translator == nil {
		r.logger.Error("Attempted to register nil translator, ignoring")
		return
	}

	if name == "" {
		r.logger.Warn("Attempted to register translator with empty name, using translator.Name() instead")
		name = translator.Name()
		if name == "" {
			r.logger.Error("Translator has empty name, cannot register")
			return
		}
	}

	// Log if we're overwriting an existing translator - might indicate a config issue
	if existing, exists := r.translators[name]; exists {
		r.logger.Warn("Overwriting existing translator",
			"name", name,
			"old", fmt.Sprintf("%T", existing),
			"new", fmt.Sprintf("%T", translator))
	}

	r.translators[name] = translator
	r.logger.Debug("Registered translator", "name", name, "type", fmt.Sprintf("%T", translator))
}

// Get retrieves a translator by name
// returns error if not found (go convention)
func (r *Registry) Get(name string) (RequestTranslator, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	translator, exists := r.translators[name]
	if !exists {
		return nil, fmt.Errorf("translator not found: %s (available: %v)", name, r.getAvailableNames())
	}

	return translator, nil
}

// GetAll returns all registered translators as a map
// returns copy to prevent modification
func (r *Registry) GetAll() map[string]RequestTranslator {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Return a copy to prevent concurrent modification issues
	result := make(map[string]RequestTranslator, len(r.translators))
	for name, translator := range r.translators {
		result[name] = translator
	}

	return result
}

// GetAvailableNames returns sorted list of registered translator names
// helpful for error messages and debugging
func (r *Registry) GetAvailableNames() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.getAvailableNames()
}

// getAvailableNames is the internal version without locking
// avoids extra lock calls for internal use
func (r *Registry) getAvailableNames() []string {
	names := make([]string, 0, len(r.translators))
	for name := range r.translators {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
