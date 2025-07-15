package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/thushan/olla/internal/adapter/registry"
	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/core/ports"
	"github.com/thushan/olla/pkg/format"
)

// UnifiedModelSummary represents a unified model in the API response
type UnifiedModelSummary struct {
	MaxContextLength *int64   `json:"max_context_length,omitempty"`
	ID               string   `json:"id"` // Canonical ID
	Family           string   `json:"family"`
	Variant          string   `json:"variant"`
	ParameterSize    string   `json:"parameter_size"`
	Quantization     string   `json:"quantization"`
	Format           string   `json:"format"`
	TotalDiskSize    string   `json:"total_disk_size"`
	LastSeen         string   `json:"last_seen"`
	Aliases          []string `json:"aliases"`
	Endpoints        []string `json:"endpoints"`        // Endpoint names only
	LoadedEndpoints  []string `json:"loaded_endpoints"` // Where model is loaded (names only)
	Capabilities     []string `json:"capabilities"`
	ParameterCount   int64    `json:"parameter_count"`
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

	// Build filters from query parameters
	filters := ports.ModelFilters{
		Endpoint: qry.Get("endpoint"),
		Family:   qry.Get("family"),
		Type:     qry.Get("type"),
	}

	// Parse available filter
	if availStr := qry.Get("available"); availStr != "" {
		switch availStr {
		case "true":
			avail := true
			filters.Available = &avail
		case "false":
			avail := false
			filters.Available = &avail
		default:
			http.Error(w, "Invalid value for 'available' parameter. Use 'true' or 'false'", http.StatusBadRequest)
			return
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

	// Get all unified models
	unifiedModels, err := unifiedRegistry.GetUnifiedModels(ctx)
	if err != nil {
		http.Error(w, "Failed to get unified models", http.StatusInternalServerError)
		return
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

	// For unified and lmstudio formats, replace endpoint URLs with names
	if format == "unified" || format == "lmstudio" {
		a.enrichResponseWithEndpointNames(ctx, &response)
	}

	w.Header().Set(ContentTypeHeader, ContentTypeJSON)
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
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

// unifiedModelByAliasHandler returns a specific unified model by ID or alias
// enrichResponseWithEndpointNames replaces endpoint URLs with names in the response
func (a *Application) enrichResponseWithEndpointNames(ctx context.Context, response interface{}) {
	endpoints, err := a.repository.GetAll(ctx)
	if err != nil {
		return
	}

	endpointNames := make(map[string]string)
	for _, ep := range endpoints {
		endpointNames[ep.URLString] = ep.Name
	}

	// Note: The converter already uses endpoint names directly from SourceEndpoint.EndpointName
	// so no additional enrichment is needed for the availability field
	_ = response // Keep the parameter for potential future use
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
