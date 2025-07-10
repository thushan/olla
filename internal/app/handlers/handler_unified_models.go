package handlers

import (
	"encoding/json"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/thushan/olla/internal/adapter/registry"
	"github.com/thushan/olla/internal/core/domain"
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

	// Get query parameters
	family := r.URL.Query().Get("family")
	capability := r.URL.Query().Get("capability")
	format := r.URL.Query().Get("format")
	// TODO: Implement size filtering
	// minSize := r.URL.Query().Get("min_size")
	// maxSize := r.URL.Query().Get("max_size")

	// Get all unified models
	unifiedModels, err := unifiedRegistry.GetUnifiedModels(ctx)
	if err != nil {
		http.Error(w, "Failed to get unified models", http.StatusInternalServerError)
		return
	}

	// Get endpoint names for display
	endpoints, err := a.repository.GetAll(ctx)
	if err != nil {
		http.Error(w, "Failed to get endpoints", http.StatusInternalServerError)
		return
	}

	endpointNames := make(map[string]string)
	for _, ep := range endpoints {
		endpointNames[ep.URLString] = ep.Name
	}

	// Apply filters and build summaries
	var filteredModels []UnifiedModelSummary
	familyMap := make(map[string][]string)

	for _, model := range unifiedModels {
		// Apply filters
		if family != "" && !strings.EqualFold(model.Family, family) {
			continue
		}

		if capability != "" {
			hasCapability := false
			for _, cap := range model.Capabilities {
				if strings.EqualFold(cap, capability) {
					hasCapability = true
					break
				}
			}
			if !hasCapability {
				continue
			}
		}

		if format != "" && !strings.EqualFold(model.Format, format) {
			continue
		}

		// TODO: Add size filtering based on parameter count

		// Build summary
		summary := a.buildUnifiedModelSummary(model, endpointNames)
		filteredModels = append(filteredModels, summary)

		// Track families
		familyMap[model.Family] = append(familyMap[model.Family], model.ID)
	}

	// Sort models by family/variant/size
	sort.Slice(filteredModels, func(i, j int) bool {
		if filteredModels[i].Family != filteredModels[j].Family {
			return filteredModels[i].Family < filteredModels[j].Family
		}
		if filteredModels[i].Variant != filteredModels[j].Variant {
			return filteredModels[i].Variant < filteredModels[j].Variant
		}
		return filteredModels[i].ParameterCount < filteredModels[j].ParameterCount
	})

	// Sort family names
	for family := range familyMap {
		sort.Strings(familyMap[family])
	}

	// Get unification stats
	stats, err := unifiedRegistry.GetUnifiedStats(ctx)
	if err != nil {
		a.logger.Error("Failed to get unification stats", err)
	}

	response := UnifiedModelResponse{
		Timestamp:        time.Now(),
		UnifiedModels:    filteredModels,
		ModelsByFamily:   familyMap,
		TotalModels:      len(filteredModels),
		TotalFamilies:    len(familyMap),
		TotalEndpoints:   len(endpoints),
	}

	if stats.UnificationStats.TotalUnified > 0 {
		response.UnificationStats = &stats.UnificationStats
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
		Aliases:          model.Aliases,
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