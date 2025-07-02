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

	requestLogger.Info("Request started",
		"client_ip", clientIP,
		"method", r.Method,
		"path", r.URL.Path,
		"target_path", targetPath,
		"compatible_endpoints", len(compatibleEndpoints),
		"path_resolution_ms", stats.PathResolutionMs,
		"query", r.URL.RawQuery,
		"content_type", r.Header.Get("Content-Type"),
		"content_length", r.ContentLength)

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
	if profile == nil || len(profile.SupportedBy) == 0 {
		logger.Info("No profile filtering applied, using all endpoints", "total_endpoints", len(endpoints))
		return endpoints
	}

	compatible := make([]*domain.Endpoint, 0, len(endpoints))
	/*
		for i, endpoint := range endpoints {
			logger.Info("Checking endpoint compatibility",
				"index", i,
				"endpoint_name", endpoint.Name,
				"endpoint_type", endpoint.Type,
				"path", profile.Path,
				"supported_by", profile.SupportedBy)

			if profile.IsCompatibleWith(endpoint.Type) {
				logger.Info("Endpoint is compatible", "endpoint_name", endpoint.Name)
				compatible = append(compatible, endpoint)
			} else {
				logger.Info("Endpoint NOT compatible", "endpoint_name", endpoint.Name)
			}
		}
	*/

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
		return endpoints
	}

	logger.Debug("Filtered endpoints by profile compatibility",
		"path", profile.Path,
		"compatible_count", len(compatible),
		"total_count", len(endpoints),
		"supported_by", profile.SupportedBy)

	return compatible
}
