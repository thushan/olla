package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/thushan/olla/internal/adapter/registry"
	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/core/ports"
	"github.com/thushan/olla/pkg/format"
)

// ModelAvailability represents the availability status of a model
type ModelAvailability struct {
	Status             string   `json:"status"`              // "available", "unavailable", "partial"
	HealthyEndpoints   []string `json:"healthy_endpoints"`   // Endpoint names where model is available
	UnhealthyEndpoints []string `json:"unhealthy_endpoints"` // Endpoint names where model exists but endpoint is unhealthy
	TotalEndpoints     int      `json:"total_endpoints"`     // Total number of endpoints with this model
	HealthyCount       int      `json:"healthy_count"`       // Number of healthy endpoints
}

// UnifiedModelSummary represents a unified model in the API response
type UnifiedModelSummary struct {
	MaxContextLength *int64             `json:"max_context_length,omitempty"`
	Availability     *ModelAvailability `json:"availability,omitempty"` // Only included when include_unavailable=true
	ID               string             `json:"id"`                     // Canonical ID
	Family           string             `json:"family"`
	Variant          string             `json:"variant"`
	ParameterSize    string             `json:"parameter_size"`
	Quantization     string             `json:"quantization"`
	Format           string             `json:"format"`
	TotalDiskSize    string             `json:"total_disk_size"`
	LastSeen         string             `json:"last_seen"`
	Aliases          []string           `json:"aliases"`
	Endpoints        []string           `json:"endpoints"`        // Endpoint names only
	LoadedEndpoints  []string           `json:"loaded_endpoints"` // Where model is loaded (names only)
	Capabilities     []string           `json:"capabilities"`
	ParameterCount   int64              `json:"parameter_count"`
}

// UnifiedModelResponse represents the full unified models API response
type UnifiedModelResponse struct {
	Timestamp        time.Time                `json:"timestamp"`
	ModelsByFamily   map[string][]string      `json:"models_by_family"`
	UnificationStats *domain.UnificationStats `json:"unification_stats,omitempty"`
	UnifiedModels    []UnifiedModelSummary    `json:"unified_models"`
	TotalModels      int                      `json:"total_models"`
	TotalFamilies    int                      `json:"total_families"`
	TotalEndpoints   int                      `json:"total_endpoints"`
}

// unifiedModelsHandler returns unified model information
func (a *Application) unifiedModelsHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	qry := r.URL.Query()

	// Check if registry supports unified models
	unifiedRegistry, ok := a.modelRegistry.(*registry.UnifiedMemoryModelRegistry)
	if !ok {
		// Fall back to regular models handler
		a.modelsStatusHandler(w, r)
		return
	}

	// Parse query parameters
	format := qry.Get("format")
	if format == "" {
		format = "unified" // Default format
	}

	// Parse query parameters
	filters, includeUnavailable, err := a.parseUnifiedModelsQuery(qry)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Get all unified models
	unifiedModels, err := unifiedRegistry.GetUnifiedModels(ctx)
	if err != nil {
		http.Error(w, "Failed to get unified models", http.StatusInternalServerError)
		return
	}

	// Filter models based on endpoint health if needed
	if !includeUnavailable {
		unifiedModels, err = a.filterModelsByHealth(ctx, unifiedModels)
		if err != nil {
			http.Error(w, "Failed to filter models by health", http.StatusInternalServerError)
			return
		}
	}

	// Get converter for the requested format
	converter, err := a.converterFactory.GetConverter(format)
	if err != nil {
		var qpErr *ports.QueryParameterError
		if errors.As(err, &qpErr) {
			http.Error(w, qpErr.Error(), http.StatusBadRequest)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// If endpoint filter is specified by name, resolve to URL
	if filters.Endpoint != "" {
		endpoints, epErr := a.repository.GetAll(ctx)
		if epErr == nil {
			for _, ep := range endpoints {
				if ep.Name == filters.Endpoint {
					filters.Endpoint = ep.URLString
					break
				}
			}
		}
	}

	// Convert models to the requested format
	response, err := converter.ConvertToFormat(unifiedModels, filters)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Note: When include_unavailable=true, the converter already includes
	// availability info showing endpoint state. The filtering above ensures
	// only models from healthy endpoints are shown when include_unavailable=false.

	w.Header().Set(ContentTypeHeader, ContentTypeJSON)
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

// parseUnifiedModelsQuery parses query parameters for unified models endpoint
func (a *Application) parseUnifiedModelsQuery(qry url.Values) (ports.ModelFilters, bool, error) {
	filters := ports.ModelFilters{
		Endpoint: qry.Get("endpoint"),
		Family:   qry.Get("family"),
		Type:     qry.Get("type"),
	}

	// Parse include_unavailable parameter
	includeUnavailable := false
	if includeStr := qry.Get("include_unavailable"); includeStr == queryValueTrue {
		includeUnavailable = true
	}

	// Parse available filter
	if availStr := qry.Get("available"); availStr != "" {
		switch availStr {
		case queryValueTrue:
			avail := true
			filters.Available = &avail
		case "false":
			avail := false
			filters.Available = &avail
		default:
			return filters, false, errors.New("invalid value for 'available' parameter. Use 'true' or 'false'")
		}
	}

	// Handle legacy capability parameter by mapping to type
	if capability := qry.Get("capability"); capability != "" {
		// Map capabilities to types
		switch strings.ToLower(capability) {
		case "vision", "multimodal":
			filters.Type = "vlm"
		case "embedding", "embeddings", "vector_search":
			filters.Type = "embeddings"
		case "chat", "text_generation", "completion":
			filters.Type = "llm"
		}
	}

	return filters, includeUnavailable, nil
}

// filterModelsByHealth filters models to only include those from healthy endpoints
func (a *Application) filterModelsByHealth(ctx context.Context, models []*domain.UnifiedModel) ([]*domain.UnifiedModel, error) {
	// Get healthy endpoints
	healthyEndpoints, err := a.repository.GetHealthy(ctx)
	if err != nil {
		return nil, err
	}

	// Create a map of healthy endpoint URLs for fast lookup
	healthyMap := make(map[string]bool)
	for _, ep := range healthyEndpoints {
		healthyMap[ep.URLString] = true
	}

	// Filter models to only include those from healthy endpoints
	filteredModels := make([]*domain.UnifiedModel, 0)
	for _, model := range models {
		hasHealthyEndpoint := false
		for _, source := range model.SourceEndpoints {
			if healthyMap[source.EndpointURL] {
				hasHealthyEndpoint = true
				break
			}
		}
		if hasHealthyEndpoint {
			filteredModels = append(filteredModels, model)
		}
	}

	return filteredModels, nil
}

// safeDiskSize safely converts int64 to uint64, handling negative values
func safeDiskSize(size int64) uint64 {
	if size < 0 {
		return 0
	}
	return uint64(size)
}

// buildUnifiedModelSummary converts a UnifiedModel to API summary format
func (a *Application) buildUnifiedModelSummary(model *domain.UnifiedModel, endpointNames map[string]string) UnifiedModelSummary {
	summary := UnifiedModelSummary{
		ID:               model.ID,
		Family:           model.Family,
		Variant:          model.Variant,
		ParameterSize:    model.ParameterSize,
		ParameterCount:   model.ParameterCount,
		Quantization:     model.Quantization,
		Format:           model.Format,
		Aliases:          model.GetAliasStrings(),
		Capabilities:     model.Capabilities,
		MaxContextLength: model.MaxContextLength,
		TotalDiskSize:    format.Bytes(safeDiskSize(model.DiskSize)),
		LastSeen:         format.TimeAgo(model.LastSeen),
		Endpoints:        make([]string, 0, len(model.SourceEndpoints)),
		LoadedEndpoints:  make([]string, 0),
	}

	// Process endpoints
	for _, ep := range model.SourceEndpoints {
		endpointName := endpointNames[ep.EndpointURL]
		if endpointName == "" {
			endpointName = ep.EndpointName // Use the endpoint name from the model
		}

		summary.Endpoints = append(summary.Endpoints, endpointName)

		if ep.State == "loaded" {
			summary.LoadedEndpoints = append(summary.LoadedEndpoints, endpointName)
		}
	}

	return summary
}

func (a *Application) unifiedModelByAliasHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Extract model ID/alias from path
	// This assumes the route is something like /olla/models/{id}
	pathParts := strings.Split(r.URL.Path, "/")
	if len(pathParts) < 4 {
		http.Error(w, "Invalid path", http.StatusBadRequest)
		return
	}

	modelAlias := pathParts[3]
	if modelAlias == "" {
		http.Error(w, "Model ID or alias required", http.StatusBadRequest)
		return
	}

	// Check if registry supports unified models
	unifiedRegistry, ok := a.modelRegistry.(*registry.UnifiedMemoryModelRegistry)
	if !ok {
		http.Error(w, "Unified models not supported", http.StatusNotImplemented)
		return
	}

	// Get the unified model
	model, err := unifiedRegistry.GetUnifiedModel(ctx, modelAlias)
	if err != nil {
		http.Error(w, "Model not found", http.StatusNotFound)
		return
	}

	// Get endpoint names
	endpoints, _ := a.repository.GetAll(ctx)
	endpointNames := make(map[string]string)
	for _, ep := range endpoints {
		endpointNames[ep.URLString] = ep.Name
	}

	// Build response
	summary := a.buildUnifiedModelSummary(model, endpointNames)

	w.Header().Set(ContentTypeHeader, ContentTypeJSON)
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(summary)
}
