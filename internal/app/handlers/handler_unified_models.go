package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/thushan/olla/internal/adapter/converter"
	"github.com/thushan/olla/internal/adapter/registry"
	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/core/ports"
	"github.com/thushan/olla/pkg/format"
)

// UnifiedModelSummary represents a unified model in the API response
type UnifiedModelSummary struct {
	ID               string   `json:"id"`                // Canonical ID
	Family           string   `json:"family"`            
	Variant          string   `json:"variant"`           
	ParameterSize    string   `json:"parameter_size"`    
	ParameterCount   int64    `json:"parameter_count"`   
	Quantization     string   `json:"quantization"`      
	Format           string   `json:"format"`            
	Aliases          []string `json:"aliases"`           
	Endpoints        []string `json:"endpoints"`         // Endpoint names
	EndpointURLs     []string `json:"endpoint_urls"`     // Actual URLs
	LoadedEndpoints  []string `json:"loaded_endpoints"`  // Where model is loaded
	Capabilities     []string `json:"capabilities"`      
	MaxContextLength *int64   `json:"max_context_length,omitempty"`
	TotalDiskSize    string   `json:"total_disk_size"`   
	LastSeen         string   `json:"last_seen"`
}

// UnifiedModelResponse represents the full unified models API response
type UnifiedModelResponse struct {
	Timestamp         time.Time                      `json:"timestamp"`
	UnifiedModels     []UnifiedModelSummary          `json:"unified_models"`
	ModelsByFamily    map[string][]string            `json:"models_by_family"`
	TotalModels       int                            `json:"total_models"`
	TotalFamilies     int                            `json:"total_families"`
	TotalEndpoints    int                            `json:"total_endpoints"`
	UnificationStats  *domain.UnificationStats       `json:"unification_stats,omitempty"`
}

// unifiedModelsHandler returns unified model information
func (a *Application) unifiedModelsHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Check if registry supports unified models
	unifiedRegistry, ok := a.modelRegistry.(*registry.UnifiedMemoryModelRegistry)
	if !ok {
		// Fall back to regular models handler
		a.modelsStatusHandler(w, r)
		return
	}

	// Parse query parameters
	format := r.URL.Query().Get("format")
	if format == "" {
		format = "unified" // Default format
	}

	// Build filters from query parameters
	filters := ports.ModelFilters{
		Endpoint: r.URL.Query().Get("endpoint"),
		Family:   r.URL.Query().Get("family"),
		Type:     r.URL.Query().Get("type"),
	}

	// Parse available filter
	if availStr := r.URL.Query().Get("available"); availStr != "" {
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
	if capability := r.URL.Query().Get("capability"); capability != "" {
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
		if qpErr, ok := err.(*ports.QueryParameterError); ok {
			http.Error(w, qpErr.Error(), http.StatusBadRequest)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// If endpoint filter is specified by name, resolve to URL
	if filters.Endpoint != "" {
		endpoints, err := a.repository.GetAll(ctx)
		if err == nil {
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
		TotalDiskSize:    format.Bytes(uint64(model.DiskSize)),
		LastSeen:         format.TimeAgo(model.LastSeen),
		Endpoints:        make([]string, 0, len(model.SourceEndpoints)),
		EndpointURLs:     make([]string, 0, len(model.SourceEndpoints)),
		LoadedEndpoints:  make([]string, 0),
	}

	// Process endpoints
	for _, ep := range model.SourceEndpoints {
		endpointName := endpointNames[ep.EndpointURL]
		if endpointName == "" {
			endpointName = ep.EndpointURL
		}
		
		summary.Endpoints = append(summary.Endpoints, endpointName)
		summary.EndpointURLs = append(summary.EndpointURLs, ep.EndpointURL)
		
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

	switch resp := response.(type) {
	case *converter.UnifiedModelResponse:
		for i := range resp.Data {
			if resp.Data[i].Olla != nil {
				for j := range resp.Data[i].Olla.Availability {
					if name, exists := endpointNames[resp.Data[i].Olla.Availability[j].URL]; exists {
						resp.Data[i].Olla.Availability[j].Endpoint = name
					}
				}
			}
		}
	case converter.UnifiedModelResponse:
		// Handle non-pointer case
		respPtr := &resp
		for i := range respPtr.Data {
			if respPtr.Data[i].Olla != nil {
				for j := range respPtr.Data[i].Olla.Availability {
					if name, exists := endpointNames[respPtr.Data[i].Olla.Availability[j].URL]; exists {
						respPtr.Data[i].Olla.Availability[j].Endpoint = name
					}
				}
			}
		}
	}
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