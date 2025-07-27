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

// createProviderProfile creates a RequestProfile configured for a specific provider type
func createProviderProfile(providerType string) *domain.RequestProfile {
	profile := domain.NewRequestProfile("")

	// normalize provider type to handle variations
	providerType = NormaliseProviderType(providerType)

	switch providerType {
	case "openai":
		// openai provider accepts any OpenAI-compatible endpoint
		// including ollama, lm-studio, and actual openai endpoints
		profile.AddSupportedProfile(domain.ProfileOpenAICompatible)
		// also add specific types that should work with openai
		profile.AddSupportedProfile("openai")
		profile.AddSupportedProfile("vllm")
	case "ollama":
		profile.AddSupportedProfile(domain.ProfileOllama)
	case "lm-studio":
		profile.AddSupportedProfile(domain.ProfileLmStudio)
	case "vllm":
		// vllm doesn't have a specific profile constant yet
		profile.AddSupportedProfile("vllm")
	default:
		// for unknown providers, add as-is for exact matching
		profile.AddSupportedProfile(providerType)
	}

	return profile
}

// providerProxyHandler routes requests to endpoints of a specific provider type.
// this enables provider-specific load balancing and failover within a single
// provider ecosystem (eg. multiple ollama instances)
func (a *Application) providerProxyHandler(w http.ResponseWriter, r *http.Request) {
	// extract provider and path from URL
	providerType, _, ok := extractProviderFromPath(r.URL.Path)
	if !ok {
		http.Error(w, "Invalid path format", http.StatusBadRequest)
		return
	}

	// validate provider type (extractProviderFromPath already normalizes)
	if !isProviderSupported(providerType) {
		http.Error(w, fmt.Sprintf("Unknown provider type: %s", providerType), http.StatusBadRequest)
		return
	}

	pr := a.initializeProxyRequest(r)

	// preserve provider constraint through request lifecycle
	ctx := r.Context()
	ctx = context.WithValue(ctx, "provider_type", providerType)

	// set the route prefix in context so proxy can strip it correctly
	// this matches how the router sets it for regular proxy requests
	providerPrefix := getProviderPrefix(providerType)
	ctx = context.WithValue(ctx, constants.ProxyPathPrefix, providerPrefix)
	r = r.WithContext(ctx)

	// standard proxy flow with provider filtering applied
	ctx, r = a.setupRequestContext(r, pr.stats)
	a.analyzeRequest(ctx, r, pr)

	// filter endpoints to requested provider type only
	endpoints, err := a.getProviderEndpoints(ctx, providerType, pr)
	if err != nil {
		a.handleEndpointError(w, pr, err)
		return
	}

	if len(endpoints) == 0 {
		http.Error(w, fmt.Sprintf("No %s endpoints available", providerType), http.StatusNotFound)
		return
	}

	a.logRequestStart(pr, len(endpoints))
	err = a.executeProxyRequest(ctx, w, r, endpoints, pr)
	a.logRequestResult(pr, err)

	if err != nil {
		a.handleProxyError(w, err)
	}
}

// getProviderEndpoints filters healthy endpoints by provider type
func (a *Application) getProviderEndpoints(ctx context.Context, providerType string, pr *proxyRequest) ([]*domain.Endpoint, error) {
	endpoints, err := a.discoveryService.GetHealthyEndpoints(ctx)
	if err != nil {
		pr.requestLogger.Error("Failed to get healthy endpoints", "error", err)
		return nil, fmt.Errorf("no healthy endpoints available: %w", err)
	}

	// create a synthetic profile for provider-based filtering
	// this leverages the existing compatibility logic in RequestProfile
	providerProfile := createProviderProfile(providerType)
	providerProfile.Path = pr.targetPath

	// use the standard filtering logic
	providerEndpoints := a.filterEndpointsByProfile(endpoints, providerProfile, pr.requestLogger)

	// apply additional filtering based on request profile if available
	if pr.profile != nil && len(pr.profile.SupportedBy) > 0 {
		// further refine based on actual request requirements
		providerEndpoints = a.filterEndpointsByProfile(providerEndpoints, pr.profile, pr.requestLogger)
	}

	return providerEndpoints, nil
}

// filterModelsByProvider restricts models to those available on specific provider type
func (a *Application) filterModelsByProvider(ctx context.Context, models []*domain.UnifiedModel, providerType string) ([]*domain.UnifiedModel, error) {
	endpoints, err := a.repository.GetAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get endpoints: %w", err)
	}

	// create a profile for provider compatibility checking
	providerProfile := createProviderProfile(providerType)

	// build endpoint type lookup
	endpointTypes := make(map[string]string)
	for _, ep := range endpoints {
		endpointTypes[ep.URLString] = ep.Type
	}

	providerModels := make([]*domain.UnifiedModel, 0)
	for _, model := range models {
		// check source endpoints for provider match
		hasProvider := false
		for _, source := range model.SourceEndpoints {
			if endpointType, ok := endpointTypes[source.EndpointURL]; ok {
				// normalize endpoint type to handle variations
				normalizedType := NormaliseProviderType(endpointType)
				if providerProfile.IsCompatibleWith(normalizedType) {
					hasProvider = true
					break
				}
			}
		}
		// fallback to alias checking for provider association
		if !hasProvider {
			for _, alias := range model.Aliases {
				// normalize alias source type
				normalizedSource := NormaliseProviderType(alias.Source)
				if providerProfile.IsCompatibleWith(normalizedSource) {
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

// getProviderModels retrieves and filters models for a specific provider type
func (a *Application) getProviderModels(ctx context.Context, providerType string) ([]*domain.UnifiedModel, error) {
	unifiedRegistry, ok := a.modelRegistry.(*registry.UnifiedMemoryModelRegistry)
	if !ok {
		return nil, fmt.Errorf("unified models not supported")
	}

	unifiedModels, err := unifiedRegistry.GetUnifiedModels(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get unified models: %w", err)
	}

	// health filtering ensures only accessible models are returned
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

// convertModelsToProviderFormat transforms unified models to provider-specific format
func (a *Application) convertModelsToProviderFormat(models []*domain.UnifiedModel, format string) (interface{}, error) {
	converter, err := a.converterFactory.GetConverter(format)
	if err != nil {
		return nil, fmt.Errorf("unsupported format: %w", err)
	}

	// no additional filtering needed - provider filtering already applied
	filters := ports.ModelFilters{}
	response, err := converter.ConvertToFormat(models, filters)
	if err != nil {
		return nil, fmt.Errorf("failed to convert models: %w", err)
	}

	return response, nil
}
