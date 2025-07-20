package handlers

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/thushan/olla/internal/core/ports"

	"github.com/thushan/olla/internal/logger"

	"github.com/thushan/olla/internal/core/constants"
	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/util"
)

func (a *Application) proxyHandler(w http.ResponseWriter, r *http.Request) {
	stats := ports.RequestStats{
		RequestID: util.GenerateRequestID(),
		StartTime: time.Now(),
	}

	requestLogger := a.logger.WithRequestID(stats.RequestID)
	requestLogger.Debug("Proxy handler called", "path", r.URL.Path, "method", r.Method)

	// Preserve the route prefix from the original context
	ctx := context.WithValue(r.Context(), constants.RequestIDKey, stats.RequestID)
	ctx = context.WithValue(ctx, constants.RequestTimeKey, stats.StartTime)
	r = r.WithContext(ctx)

	rl := a.Config.Server.RateLimits
	clientIP := util.GetClientIP(r, rl.TrustProxyHeaders, rl.TrustedProxyCIDRsParsed)

	pathResolutionStart := time.Now()

	targetPath := a.stripRoutePrefix(ctx, r.URL.Path)

	profile, err := a.inspectorChain.Inspect(ctx, r, targetPath)
	if err != nil {
		requestLogger.Warn("Request inspection failed, continuing with all endpoints", "error", err)
	}

	endpoints, err := a.discoveryService.GetHealthyEndpoints(ctx)
	if err != nil {
		requestLogger.Error("Failed to get healthy endpoints", "error", err)
		http.Error(w, fmt.Sprintf("No healthy endpoints available: %v", err), http.StatusBadGateway)
		return
	}

	compatibleEndpoints := a.filterEndpointsByProfile(endpoints, profile, requestLogger)

	pathResolutionEnd := time.Now()
	stats.PathResolutionMs = pathResolutionEnd.Sub(pathResolutionStart).Milliseconds()

	logFields := []any{
		"client_ip", clientIP,
		"method", r.Method,
		"path", r.URL.Path,
		"target_path", targetPath,
		"compatible_endpoints", len(compatibleEndpoints),
		"path_resolution_ms", stats.PathResolutionMs,
		"query", r.URL.RawQuery,
		"content_type", r.Header.Get("Content-Type"),
		"content_length", r.ContentLength,
	}

	if profile != nil && profile.ModelName != "" {
		logFields = append(logFields, "model", profile.ModelName)
		ctx = context.WithValue(ctx, "model", profile.ModelName)
		r = r.WithContext(ctx)
	}

	requestLogger.Info("Request started", logFields...)

	if err := a.proxyService.ProxyRequestToEndpoints(ctx, w, r, compatibleEndpoints, &stats, requestLogger); err != nil {
		duration := time.Since(stats.StartTime)

		requestLogger.Error("Request failed", "error", err,
			"duration_ms", duration.Milliseconds(),
			"latency_ms", stats.Latency,
			"endpoint", stats.EndpointName,
			"total_bytes", stats.TotalBytes,
			"request_processing_ms", stats.RequestProcessingMs,
			"backend_response_ms", stats.BackendResponseMs,
			"first_data_ms", stats.FirstDataMs,
			"streaming_ms", stats.StreamingMs,
			"header_processing_ms", stats.HeaderProcessingMs,
			"path_resolution_ms", stats.PathResolutionMs,
			"selection_ms", stats.SelectionMs)

		if w.Header().Get("Content-Type") == "" {
			http.Error(w, fmt.Sprintf("Proxy error: %v", err), http.StatusBadGateway)
		}
	} else {
		duration := time.Since(stats.StartTime)
		requestLogger.Info("Request completed",
			"endpoint", stats.EndpointName,
			"total_bytes", stats.TotalBytes,
			"duration_ms", duration.Milliseconds(),
			"latency_ms", stats.Latency,
			"request_processing_ms", stats.RequestProcessingMs,
			"backend_response_ms", stats.BackendResponseMs,
			"first_data_ms", stats.FirstDataMs,
			"streaming_ms", stats.StreamingMs,
			"header_processing_ms", stats.HeaderProcessingMs,
			"path_resolution_ms", stats.PathResolutionMs,
			"selection_ms", stats.SelectionMs)
	}
}

func (a *Application) stripRoutePrefix(ctx context.Context, path string) string {
	return util.StripRoutePrefix(ctx, path, constants.ProxyPathPrefix)
}

func (a *Application) filterEndpointsByProfile(endpoints []*domain.Endpoint, profile *domain.RequestProfile, logger logger.StyledLogger) []*domain.Endpoint {
	var profileFiltered []*domain.Endpoint

	// Stage 1: Filter by profile compatibility (endpoint type)
	if profile == nil || len(profile.SupportedBy) == 0 {
		logger.Debug("No profile filtering applied", "total_endpoints", len(endpoints))
		profileFiltered = endpoints
	} else {
		compatible := make([]*domain.Endpoint, 0, len(endpoints))
		for _, endpoint := range endpoints {
			if profile.IsCompatibleWith(endpoint.Type) {
				compatible = append(compatible, endpoint)
			}
		}

		if len(compatible) == 0 {
			logger.Warn("No compatible endpoints found for path, falling back to all endpoints",
				"path", profile.Path,
				"supported_by", profile.SupportedBy,
				"total_endpoints", len(endpoints))
			profileFiltered = endpoints
		} else {
			logger.Debug("Filtered endpoints by profile compatibility",
				"path", profile.Path,
				"compatible_count", len(compatible),
				"total_count", len(endpoints),
				"supported_by", profile.SupportedBy)
			profileFiltered = compatible
		}
	}

	// Stage 2: Filter by capabilities if present
	if profile != nil && profile.ModelCapabilities != nil && a.modelRegistry != nil {
		capabilityFiltered := a.filterEndpointsByCapabilities(profileFiltered, profile, logger)
		if len(capabilityFiltered) > 0 {
			profileFiltered = capabilityFiltered
		}
	}

	// Stage 3: Filter by specific model if present
	if profile != nil && profile.ModelName != "" && a.modelRegistry != nil {
		ctx := context.Background()
		modelEndpoints, err := a.modelRegistry.GetEndpointsForModel(ctx, profile.ModelName)
		if err != nil {
			logger.Warn("Failed to get endpoints for model, skipping model filtering",
				"model", profile.ModelName,
				"error", err)
			return profileFiltered
		}

		if len(modelEndpoints) == 0 {
			logger.Warn("No endpoints have the requested model, using profile-filtered endpoints",
				"model", profile.ModelName,
				"available_endpoints", len(profileFiltered))
			return profileFiltered
		}

		modelEndpointMap := make(map[string]bool)
		for _, endpointURL := range modelEndpoints {
			modelEndpointMap[endpointURL] = true
		}

		modelFiltered := make([]*domain.Endpoint, 0, len(profileFiltered))
		for _, endpoint := range profileFiltered {
			if modelEndpointMap[endpoint.URLString] {
				modelFiltered = append(modelFiltered, endpoint)
			}
		}

		if len(modelFiltered) == 0 {
			logger.Warn("No profile-compatible endpoints have the requested model, falling back",
				"model", profile.ModelName,
				"model_endpoints", len(modelEndpoints),
				"compatible_endpoints", len(profileFiltered))
			return profileFiltered
		}

		logger.Debug("Filtered endpoints by model availability",
			"model", profile.ModelName,
			"model_filtered_count", len(modelFiltered),
			"profile_filtered_count", len(profileFiltered),
			"total_count", len(endpoints))

		return modelFiltered
	}

	return profileFiltered
}

// filterEndpointsByCapabilities filters endpoints based on required capabilities
func (a *Application) filterEndpointsByCapabilities(endpoints []*domain.Endpoint, profile *domain.RequestProfile, logger logger.StyledLogger) []*domain.Endpoint {
	if profile.ModelCapabilities == nil {
		return endpoints
	}

	requiredCapabilities := a.extractRequiredCapabilities(profile.ModelCapabilities)
	if len(requiredCapabilities) == 0 {
		return endpoints
	}

	capableModels := a.findCapableModels(requiredCapabilities, logger)
	if len(capableModels) == 0 {
		logger.Warn("No models found with required capabilities",
			"capabilities", requiredCapabilities)
		return endpoints
	}

	return a.filterEndpointsByCapableModels(endpoints, capableModels, requiredCapabilities, logger)
}

// extractRequiredCapabilities builds a list of required capability strings from ModelCapabilities
func (a *Application) extractRequiredCapabilities(caps *domain.ModelCapabilities) []string {
	requiredCapabilities := make([]string, 0)

	if caps.VisionUnderstanding {
		requiredCapabilities = append(requiredCapabilities, "vision")
	}
	if caps.FunctionCalling {
		requiredCapabilities = append(requiredCapabilities, "function_calling", "tools")
	}
	if caps.Embeddings {
		requiredCapabilities = append(requiredCapabilities, "embeddings")
	}
	if caps.CodeGeneration {
		requiredCapabilities = append(requiredCapabilities, "code")
	}

	return requiredCapabilities
}

// findCapableModels returns endpoints that have models supporting all required capabilities
func (a *Application) findCapableModels(requiredCapabilities []string, logger logger.StyledLogger) map[string]bool {
	ctx := context.Background()
	capableModels := make(map[string]bool)
	hasCapabilitySupport := false

	for i, capability := range requiredCapabilities {
		models, err := a.modelRegistry.GetModelsByCapability(ctx, capability)
		if err != nil {
			logger.Warn("Failed to get models by capability",
				"capability", capability,
				"error", err)
			continue
		}

		// Check if we have any models - empty result means registry doesn't support capabilities
		if len(models) > 0 {
			hasCapabilitySupport = true
		}

		if i == 0 {
			// For first capability, add all models
			a.addModelsToMap(models, capableModels)
		} else {
			// For subsequent capabilities, keep only models that have all capabilities
			capableModels = a.intersectModels(models, capableModels)
		}
	}

	// If registry doesn't support capability queries, return nil to skip filtering
	if !hasCapabilitySupport {
		return nil
	}

	return capableModels
}

// addModelsToMap adds all model endpoints to the map
func (a *Application) addModelsToMap(models []*domain.UnifiedModel, capableModels map[string]bool) {
	for _, model := range models {
		for _, sourceEndpoint := range model.SourceEndpoints {
			capableModels[sourceEndpoint.EndpointURL] = true
		}
	}
}

// intersectModels keeps only models that exist in both the new models and existing capable models
func (a *Application) intersectModels(models []*domain.UnifiedModel, existingCapableModels map[string]bool) map[string]bool {
	newCapableModels := make(map[string]bool)
	for _, model := range models {
		for _, sourceEndpoint := range model.SourceEndpoints {
			if existingCapableModels[sourceEndpoint.EndpointURL] {
				newCapableModels[sourceEndpoint.EndpointURL] = true
			}
		}
	}
	return newCapableModels
}

// filterEndpointsByCapableModels filters endpoints to only those with capable models
func (a *Application) filterEndpointsByCapableModels(endpoints []*domain.Endpoint, capableModels map[string]bool, requiredCapabilities []string, logger logger.StyledLogger) []*domain.Endpoint {
	// If capableModels is nil, it means the registry doesn't support capability queries
	// Skip filtering in this case
	if capableModels == nil {
		logger.Debug("Registry doesn't support capability queries, skipping capability filtering",
			"capabilities", requiredCapabilities)
		return endpoints
	}

	capableEndpoints := make([]*domain.Endpoint, 0, len(endpoints))
	for _, endpoint := range endpoints {
		if capableModels[endpoint.URLString] {
			capableEndpoints = append(capableEndpoints, endpoint)
		}
	}

	if len(capableEndpoints) == 0 {
		logger.Warn("No endpoints have models with required capabilities, using unfiltered",
			"capabilities", requiredCapabilities,
			"available_endpoints", len(endpoints))
		return endpoints
	}

	logger.Debug("Filtered endpoints by capabilities",
		"capabilities", requiredCapabilities,
		"capable_count", len(capableEndpoints),
		"total_count", len(endpoints))

	return capableEndpoints
}
