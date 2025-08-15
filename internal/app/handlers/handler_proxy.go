package handlers

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/thushan/olla/internal/app/middleware"
	"github.com/thushan/olla/internal/core/constants"
	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/core/ports"
	"github.com/thushan/olla/internal/logger"
	"github.com/thushan/olla/internal/util"
)

type proxyRequest struct {
	requestLogger logger.StyledLogger
	stats         *ports.RequestStats
	profile       *domain.RequestProfile
	clientIP      string
	targetPath    string
	model         string
	contentType   string
	method        string
	path          string
	query         string
	userAgent     string
	contentLength int64
}

func (a *Application) proxyHandler(w http.ResponseWriter, r *http.Request) {
	pr := a.initializeProxyRequest(r)

	ctx, r := a.setupRequestContext(r, pr.stats)

	a.analyzeRequest(ctx, r, pr)

	endpoints, err := a.getCompatibleEndpoints(ctx, pr)
	if err != nil {
		a.handleEndpointError(w, pr, err)
		return
	}

	a.logRequestStart(pr, len(endpoints))

	err = a.executeProxyRequest(ctx, w, r, endpoints, pr)

	a.logRequestResult(pr, err)

	if err != nil {
		a.handleProxyError(w, err)
	}
}

func (a *Application) initializeProxyRequest(r *http.Request) *proxyRequest {
	// get the requestID from the middleware context first
	requestID := ""
	if id, ok := r.Context().Value(middleware.RequestIDKey).(string); ok {
		requestID = id
	}

	// fallback to generating a new one otherwise
	if requestID == "" {
		requestID = util.GenerateRequestID()
	}

	stats := &ports.RequestStats{
		RequestID: requestID,
		StartTime: time.Now(),
	}

	return &proxyRequest{
		stats:         stats,
		requestLogger: a.logger.WithRequestID(stats.RequestID),
		contentType:   r.Header.Get(constants.HeaderContentType),
		method:        r.Method,
		path:          r.URL.Path,
		query:         r.URL.RawQuery,
		contentLength: r.ContentLength,
		userAgent:     r.UserAgent(),
	}
}

func (a *Application) setupRequestContext(r *http.Request, stats *ports.RequestStats) (context.Context, *http.Request) {
	ctx := context.WithValue(r.Context(), constants.ContextRequestIdKey, stats.RequestID)
	ctx = context.WithValue(ctx, constants.ContextRequestTimeKey, stats.StartTime)
	return ctx, r.WithContext(ctx)
}

func (a *Application) analyzeRequest(ctx context.Context, r *http.Request, pr *proxyRequest) {
	pr.requestLogger.Debug("Proxy handler called", "path", r.URL.Path, "method", r.Method)

	rl := a.Config.Server.RateLimits
	pr.clientIP = util.GetClientIP(r, rl.TrustProxyHeaders, rl.TrustedProxyCIDRsParsed)

	pathResolutionStart := time.Now()
	pr.targetPath = a.stripRoutePrefix(ctx, r.URL.Path)

	// inspector chain figures out which endpoints can handle this request (ollama vs openai)
	// and extracts model requirements. failures here are non-fatal - we'll spray and pray
	profile, err := a.inspectorChain.Inspect(ctx, r, pr.targetPath)
	if err != nil {
		pr.requestLogger.Warn("Request inspection failed, continuing with all endpoints", "error", err)
	}
	pr.profile = profile

	if profile != nil && profile.ModelName != "" {
		pr.model = profile.ModelName
		pr.stats.Model = pr.model
	}

	pr.stats.PathResolutionMs = time.Since(pathResolutionStart).Milliseconds()
}

func (a *Application) getCompatibleEndpoints(ctx context.Context, pr *proxyRequest) ([]*domain.Endpoint, error) {
	endpoints, err := a.discoveryService.GetHealthyEndpoints(ctx)
	if err != nil {
		pr.requestLogger.Error("Failed to get healthy endpoints", "error", err)
		return nil, fmt.Errorf("no healthy endpoints available: %w", err)
	}

	compatibleEndpoints := a.filterEndpointsByProfile(endpoints, pr.profile, pr.requestLogger)

	return compatibleEndpoints, nil
}

func (a *Application) executeProxyRequest(ctx context.Context, w http.ResponseWriter, r *http.Request, endpoints []*domain.Endpoint, pr *proxyRequest) error {
	if pr.model != "" {
		ctx = context.WithValue(ctx, "model", pr.model)
		r = r.WithContext(ctx)
	}

	// pass routing decision to stats for headers
	if pr.profile != nil && pr.profile.RoutingDecision != nil {
		pr.stats.RoutingDecision = pr.profile.RoutingDecision
	}

	return a.proxyService.ProxyRequestToEndpoints(ctx, w, r, endpoints, pr.stats, pr.requestLogger)
}

func (a *Application) logRequestStart(pr *proxyRequest, endpointCount int) {
	// Log essential operational info at INFO level
	logFields := []any{
		"client_ip", pr.clientIP,
		"method", pr.method,
		"path", pr.path,
		"compatible_endpoints", endpointCount,
	}

	// Add user agent if present
	if pr.userAgent != "" {
		logFields = append(logFields, "user_agent", pr.userAgent)
	}

	// Add model if identified
	if pr.model != "" {
		logFields = append(logFields, "model", pr.model)
	}

	// Add content length if it's a POST/PUT with body
	if pr.contentLength > 0 {
		logFields = append(logFields, "content_length", pr.contentLength)
	}

	pr.requestLogger.Info("Request received", logFields...)

	// Log additional details at DEBUG level
	debugFields := []any{
		"target_path", pr.targetPath,
		"path_resolution_ms", pr.stats.PathResolutionMs,
		"query", pr.query,
		"content_type", pr.contentType,
	}

	pr.requestLogger.Debug("Request details", debugFields...)
}

func (a *Application) logRequestResult(pr *proxyRequest, err error) {
	duration := time.Since(pr.stats.StartTime)

	if err != nil {
		logFields := a.buildLogFields(pr, duration)
		pr.requestLogger.Error("Request failed", append([]any{"error", err}, logFields...)...)
	} else {
		// Log essential completion info at INFO level
		infoFields := []any{
			"endpoint", pr.stats.EndpointName,
			"duration_ms", duration.Milliseconds(),
			"status", "completed",
		}

		if pr.model != "" {
			infoFields = append(infoFields, "model", pr.model)
		}

		if pr.stats.TotalBytes > 0 {
			infoFields = append(infoFields, "total_bytes", pr.stats.TotalBytes)
		}

		pr.requestLogger.Info("Request completed", infoFields...)

		// Log detailed metrics at DEBUG level
		debugFields := a.buildLogFields(pr, duration)
		pr.requestLogger.Debug("Request metrics", debugFields...)
	}
}

func (a *Application) buildLogFields(pr *proxyRequest, duration time.Duration) []any {
	fields := []any{
		"endpoint", pr.stats.EndpointName,
		"model", pr.model,
		"client_ip", pr.clientIP,
		"total_bytes", pr.stats.TotalBytes,
		"duration_ms", duration.Milliseconds(),
		"latency_ms", pr.stats.Latency,
		"request_processing_ms", pr.stats.RequestProcessingMs,
		"backend_response_ms", pr.stats.BackendResponseMs,
		"first_data_ms", pr.stats.FirstDataMs,
		"streaming_ms", pr.stats.StreamingMs,
		"header_processing_ms", pr.stats.HeaderProcessingMs,
		"path_resolution_ms", pr.stats.PathResolutionMs,
		"selection_ms", pr.stats.SelectionMs,
	}

	if pr.stats.EndpointName == "" {
		fields = append(fields, "target_path", pr.targetPath)
	}

	return fields
}

func (a *Application) handleEndpointError(w http.ResponseWriter, pr *proxyRequest, err error) {
	pr.requestLogger.Error("Failed to get endpoints", "error", err)
	http.Error(w, fmt.Sprintf("Service unavailable: %v", err), http.StatusBadGateway)
}

// only send error response if we haven't started streaming yet.
// content-type check prevents double-writing response after partial stream
// (learned this the hard way when users got html error messages appended to their json)
func (a *Application) handleProxyError(w http.ResponseWriter, err error) {
	if w.Header().Get(constants.HeaderContentType) == "" {
		http.Error(w, fmt.Sprintf("Proxy error: %v", err), http.StatusBadGateway)
	}
}

func (a *Application) stripRoutePrefix(ctx context.Context, path string) string {
	return util.StripRoutePrefix(ctx, path, constants.ContextRoutePrefixKey)
}

// three-stage filtering pipeline that progressively narrows down endpoints.
// starts broad (platform compatibility), then capabilities (vision, embeddings),
// finally specific model availability. each stage falls back gracefully.
func (a *Application) filterEndpointsByProfile(endpoints []*domain.Endpoint, profile *domain.RequestProfile, logger logger.StyledLogger) []*domain.Endpoint {
	var profileFiltered []*domain.Endpoint

	// stage 1: platform compatibility (ollama can't handle openai requests etc)
	if profile == nil || len(profile.SupportedBy) == 0 {
		logger.Debug("No profile filtering applied", "total_endpoints", len(endpoints))
		profileFiltered = endpoints
	} else {
		compatible := make([]*domain.Endpoint, 0, len(endpoints))
		for _, endpoint := range endpoints {
			// Normalise endpoint type to handle variations (e.g., lmstudio -> lm-studio)
			normalizedType := NormaliseProviderType(endpoint.Type)
			if profile.IsCompatibleWith(normalizedType) {
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

	// stage 2: capability filtering (vision requests need vision models)
	if profile != nil && profile.ModelCapabilities != nil && a.modelRegistry != nil {
		capabilityFiltered := a.filterEndpointsByCapabilities(profileFiltered, profile, logger)
		if len(capabilityFiltered) > 0 {
			profileFiltered = capabilityFiltered
		}
	}

	// stage 3: specific model filtering using routing strategy
	if profile != nil && profile.ModelName != "" && a.modelRegistry != nil {
		ctx := context.Background()

		// use new routing strategy method
		routableEndpoints, decision, err := a.modelRegistry.GetRoutableEndpointsForModel(ctx, profile.ModelName, profileFiltered)

		// store routing decision for headers and metrics
		if decision != nil {
			profile.RoutingDecision = decision
		}

		if err != nil {
			// handle routing errors based on decision
			if decision != nil && decision.StatusCode > 0 {
				logger.Warn("Model routing rejected request",
					"model", profile.ModelName,
					"strategy", decision.Strategy,
					"reason", decision.Reason,
					"status", decision.StatusCode)
				// return empty to trigger appropriate error response
				return []*domain.Endpoint{}
			}

			logger.Warn("Model routing failed, using all compatible endpoints",
				"model", profile.ModelName,
				"error", err)
			return profileFiltered
		}

		logger.Debug("Model routing decision",
			"model", profile.ModelName,
			"strategy", decision.Strategy,
			"action", decision.Action,
			"routable", len(routableEndpoints),
			"compatible", len(profileFiltered))

		return routableEndpoints
	}

	return profileFiltered
}

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
			"model", profile.ModelName,
			"capabilities", requiredCapabilities)
		return endpoints
	}

	return a.filterEndpointsByCapableModels(endpoints, capableModels, requiredCapabilities, logger)
}

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

// intersects capability sets to find models that support ALL requested features.
// uses nil return to signal "no capability support" vs empty map for "no matches"
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

		if len(models) > 0 {
			hasCapabilitySupport = true
		}

		if i == 0 {
			a.addModelsToMap(models, capableModels)
		} else {
			// set intersection - only keep models that have all capabilities
			capableModels = a.intersectModels(models, capableModels)
		}
	}

	// nil means "don't filter", empty map means "no matches found"
	if !hasCapabilitySupport {
		return nil
	}

	return capableModels
}

func (a *Application) addModelsToMap(models []*domain.UnifiedModel, capableModels map[string]bool) {
	for _, model := range models {
		for _, sourceEndpoint := range model.SourceEndpoints {
			capableModels[sourceEndpoint.EndpointURL] = true
		}
	}
}

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

func (a *Application) filterEndpointsByCapableModels(endpoints []*domain.Endpoint, capableModels map[string]bool, requiredCapabilities []string, logger logger.StyledLogger) []*domain.Endpoint {
	// nil check differentiates "no capability support" from "no matches"
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
