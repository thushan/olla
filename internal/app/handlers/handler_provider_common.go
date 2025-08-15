package handlers

import (
	"context"
	"fmt"
	"net/http"

	"github.com/thushan/olla/internal/adapter/registry"
	"github.com/thushan/olla/internal/core/constants"
	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/core/ports"
)

// createProviderProfile builds routing constraints for provider-specific requests.
// OpenAI providers are special - they accept traffic from any OpenAI-compatible
// endpoint to maximise compatibility across different implementations.
func (a *Application) createProviderProfile(providerType string) *domain.RequestProfile {
	profile := domain.NewRequestProfile("")

	// Profile factory has the complete prefix->provider mappings from YAML configs
	if a.profileFactory != nil {
		providerType = a.profileFactory.NormalizeProviderName(providerType)
	} else {
		// Test scenarios may not have full profile loading
		providerType = NormaliseProviderType(providerType)
	}

	// OpenAI routing is inclusive - any OpenAI-compatible endpoint can serve these requests
	if providerType == constants.ProviderTypeOpenAI || providerType == constants.ProviderTypeOpenAICompat {
		profile.AddSupportedProfile(domain.ProfileOpenAICompatible)

		// Dynamically include all providers that advertise OpenAI compatibility
		if a.profileFactory != nil {
			for _, profileName := range a.profileFactory.GetAvailableProfiles() {
				if p, err := a.profileFactory.GetProfile(profileName); err == nil {
					if config := p.GetConfig(); config != nil && config.API.OpenAICompatible {
						profile.AddSupportedProfile(profileName)
					}
				}
			}
		} else {
			// Tests get a minimal set without full profile loading
			profile.AddSupportedProfile(constants.ProviderTypeOpenAI)
			profile.AddSupportedProfile(constants.ProviderTypeVLLM)
		}
	} else {
		// Non-OpenAI providers only route to their specific backend type
		profile.AddSupportedProfile(providerType)
	}

	return profile
}

// providerProxyHandler implements provider-scoped load balancing.
// Unlike the main proxy which balances across all endpoints, this constrains
// traffic to a specific provider type (e.g., only Ollama instances).
func (a *Application) providerProxyHandler(w http.ResponseWriter, r *http.Request) {
	providerType, _, ok := extractProviderFromPath(r.URL.Path)
	if !ok {
		http.Error(w, "Invalid path format", http.StatusBadRequest)
		return
	}

	// extractProviderFromPath already normalises, but we still need to verify
	// it's a provider we actually have endpoints for
	if !a.isProviderSupported(providerType) {
		http.Error(w, fmt.Sprintf("Unknown provider type: %s", providerType), http.StatusBadRequest)
		return
	}

	pr := a.initializeProxyRequest(r)

	// Provider type must flow through the entire request lifecycle to ensure
	// consistent routing decisions
	ctx := r.Context()
	ctx = context.WithValue(ctx, constants.ContextProviderTypeKey, providerType)

	// The proxy needs to know which prefix to strip before forwarding.
	// This mimics the behaviour of the main router for consistency.
	providerPrefix := getProviderPrefix(providerType)
	ctx = context.WithValue(ctx, constants.ContextRoutePrefixKey, providerPrefix)
	r = r.WithContext(ctx)

	ctx, r = a.setupRequestContext(r, pr.stats)
	a.analyzeRequest(ctx, r, pr)

	endpoints, err := a.getProviderEndpoints(ctx, providerType, pr)
	if err != nil {
		a.handleEndpointError(w, pr, err)
		return
	}

	if len(endpoints) == 0 {
		http.Error(w, fmt.Sprintf("No %s endpoints available", providerType), http.StatusNotFound)
		return
	}

	// Update request path to the target path (strip provider prefix)
	r.URL.Path = pr.targetPath

	a.logRequestStart(pr, len(endpoints))
	err = a.executeProxyRequest(ctx, w, r, endpoints, pr)
	a.logRequestResult(pr, err)

	if err != nil {
		a.handleProxyError(w, err)
	}
}

// getProviderEndpoints returns only endpoints matching the requested provider type.
// This ensures Ollama requests only go to Ollama backends, etc.
func (a *Application) getProviderEndpoints(ctx context.Context, providerType string, pr *proxyRequest) ([]*domain.Endpoint, error) {
	endpoints, err := a.discoveryService.GetHealthyEndpoints(ctx)
	if err != nil {
		pr.requestLogger.Error("Failed to get healthy endpoints", "error", err)
		return nil, fmt.Errorf("no healthy endpoints available: %w", err)
	}

	// We reuse the existing RequestProfile filtering logic by creating a profile
	// that only accepts the requested provider type
	providerProfile := a.createProviderProfile(providerType)
	providerProfile.Path = pr.targetPath

	providerEndpoints := a.filterEndpointsByProfile(endpoints, providerProfile, pr.requestLogger)

	// If the request has specific requirements (e.g., needs vision support),
	// apply those filters on top of the provider constraint
	if pr.profile != nil && len(pr.profile.SupportedBy) > 0 {
		providerEndpoints = a.filterEndpointsByProfile(providerEndpoints, pr.profile, pr.requestLogger)
	}

	return providerEndpoints, nil
}

// filterModelsByProvider ensures model listings only show what's actually available
// on the requested provider type (e.g., /olla/ollama/models won't show LM Studio models)
func (a *Application) filterModelsByProvider(ctx context.Context, models []*domain.UnifiedModel, providerType string) ([]*domain.UnifiedModel, error) {
	endpoints, err := a.repository.GetAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get endpoints: %w", err)
	}

	providerProfile := a.createProviderProfile(providerType)

	// Need to map endpoint URLs to their types for compatibility checking
	endpointTypes := make(map[string]string)
	for _, ep := range endpoints {
		endpointTypes[ep.URLString] = ep.Type
	}

	providerModels := make([]*domain.UnifiedModel, 0)
	for _, model := range models {
		// Models can be available from multiple sources. Check if any of them
		// match our provider constraint.
		hasProvider := false
		for _, source := range model.SourceEndpoints {
			if endpointType, ok := endpointTypes[source.EndpointURL]; ok {
				normalisedType := NormaliseProviderType(endpointType)
				if providerProfile.IsCompatibleWith(normalisedType) {
					hasProvider = true
					break
				}
			}
		}
		// Model aliases provide another way to determine provider association
		if !hasProvider {
			for _, alias := range model.Aliases {
				normalisedSource := NormaliseProviderType(alias.Source)
				if providerProfile.IsCompatibleWith(normalisedSource) {
					hasProvider = true
					break
				}
			}
		}
		if hasProvider {
			providerModels = append(providerModels, model)
		}
	}

	return providerModels, nil
}

// getProviderModels handles the complete flow of fetching models for a provider-specific
// endpoint (e.g., /olla/ollama/models)
func (a *Application) getProviderModels(ctx context.Context, providerType string) ([]*domain.UnifiedModel, error) {
	unifiedRegistry, ok := a.modelRegistry.(*registry.UnifiedMemoryModelRegistry)
	if !ok {
		return nil, fmt.Errorf("unified models not supported")
	}

	unifiedModels, err := unifiedRegistry.GetUnifiedModels(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get unified models: %w", err)
	}

	// Only show models from endpoints that are currently healthy
	healthyModels, err := a.filterModelsByHealth(ctx, unifiedModels)
	if err != nil {
		return nil, fmt.Errorf("failed to filter models by health: %w", err)
	}

	providerModels, err := a.filterModelsByProvider(ctx, healthyModels, providerType)
	if err != nil {
		return nil, fmt.Errorf("failed to filter models by provider: %w", err)
	}

	return providerModels, nil
}

// convertModelsToProviderFormat handles the final transformation step for model listings,
// converting our internal UnifiedModel format to whatever the client expects (Ollama, OpenAI, etc.)
func (a *Application) convertModelsToProviderFormat(models []*domain.UnifiedModel, format string) (interface{}, error) {
	converter, err := a.converterFactory.GetConverter(format)
	if err != nil {
		return nil, fmt.Errorf("unsupported format: %w", err)
	}

	filters := ports.ModelFilters{}
	response, err := converter.ConvertToFormat(models, filters)
	if err != nil {
		return nil, fmt.Errorf("failed to convert models: %w", err)
	}

	return response, nil
}
