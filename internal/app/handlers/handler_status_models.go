package handlers

import (
	"encoding/json"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/thushan/olla/internal/core/constants"

	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/pkg/format"
)

const (
	// sized for a typical deployment
	maxModelsCapacity      = 128
	maxEndpointNamesLength = 32

	familyUnknown = "unknown"
)

type ModelSummary struct {
	Name         string   `json:"name"`
	Type         string   `json:"type,omitempty"`
	Family       string   `json:"family,omitempty"`
	Size         string   `json:"size,omitempty"`
	Params       string   `json:"params,omitempty"`
	Quant        string   `json:"quant,omitempty"`
	Endpoints    []string `json:"endpoints"`
	LastSeen     string   `json:"last_seen"`
	Capabilities []string `json:"capabilities,omitempty"`
}

type ModelGroupSummary struct {
	Family     string         `json:"family"`
	Models     []ModelSummary `json:"models"`
	Endpoints  []string       `json:"endpoints"`
	ModelCount int            `json:"model_count"`
}

type ModelStatusResponse struct {
	Timestamp      time.Time           `json:"timestamp"`
	ModelsByFamily map[string][]string `json:"models_by_family"`
	RecentModels   []ModelSummary      `json:"recent_models"`
	ModelGroups    []ModelGroupSummary `json:"model_groups,omitempty"`
	TotalModels    int                 `json:"total_models"`
	TotalFamilies  int                 `json:"total_families"`
	TotalEndpoints int                 `json:"total_endpoints"`
}

// these are pooled to avoid allocations
var (
	modelSummaryPool  = make([]ModelSummary, 0, maxModelsCapacity)
	endpointNamesPool = make(map[string]string, maxEndpointNamesLength)
	familyGroupPool   = make(map[string][]string, 16)
	uniqueModelsPool  = make(map[string]*ModelSummary, maxModelsCapacity)
	endpointSetPool   = make(map[string]struct{}, 8)
	capabilitiesPool  = make([]string, 0, 4)
	stringSlicePool   = make([]string, 0, 8)
	modelGroupPool    = make([]ModelGroupSummary, 0, 16)
)

func (a *Application) modelsStatusHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	detailed := r.URL.Query().Get("detailed") == queryValueTrue
	groupBy := r.URL.Query().Get("group")

	modelMap, err := a.modelRegistry.GetEndpointModelMap(ctx)
	if err != nil {
		http.Error(w, "Failed to get models", http.StatusInternalServerError)
		return
	}

	endpoints, err := a.repository.GetAll(ctx)
	if err != nil {
		http.Error(w, "Failed to get endpoints", http.StatusInternalServerError)
		return
	}

	for k := range endpointNamesPool {
		delete(endpointNamesPool, k)
	}
	for _, ep := range endpoints {
		endpointNamesPool[ep.URLString] = ep.Name
	}

	allModels := a.buildModelSummaries(modelMap, endpointNamesPool)

	response := ModelStatusResponse{
		Timestamp:      time.Now(),
		TotalModels:    len(allModels),
		TotalEndpoints: len(modelMap),
		ModelsByFamily: a.groupModelsByFamily(allModels),
		RecentModels:   a.getRecentModels(allModels, 10),
	}

	response.TotalFamilies = len(response.ModelsByFamily)

	if detailed && groupBy == "family" {
		response.ModelGroups = a.groupModelsByFamilyWithDetails(allModels)
	}

	w.Header().Set(constants.HeaderContentType, constants.ContentTypeJSON)
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

func (a *Application) buildModelSummaries(modelMap map[string]*domain.EndpointModels, endpointNames map[string]string) []ModelSummary {
	for k := range uniqueModelsPool {
		delete(uniqueModelsPool, k)
	}

	for endpointURL, endpointModels := range modelMap {
		endpointName := endpointNames[endpointURL]
		if endpointName == "" {
			endpointName = endpointURL
		}

		for _, model := range endpointModels.Models {
			existing, exists := uniqueModelsPool[model.Name]
			if !exists {
				stringSlicePool = stringSlicePool[:0]
				stringSlicePool = append(stringSlicePool, endpointName)
				endpoints := make([]string, len(stringSlicePool))
				copy(endpoints, stringSlicePool)

				stringSlicePool = stringSlicePool[:0]
				stringSlicePool = append(stringSlicePool, endpointURL)
				endpointURLs := make([]string, len(stringSlicePool))
				copy(endpointURLs, stringSlicePool)

				uniqueModelsPool[model.Name] = a.createModelSummary(model, endpointURLs)
			} else {
				existing.Endpoints = append(existing.Endpoints, endpointName)
				// existing.EndpointURLs = append(existing.EndpointURLs, endpointURL)

				if model.LastSeen.Unix() > parseTimeAgoOptimised(existing.LastSeen) {
					existing.LastSeen = format.TimeAgo(model.LastSeen)
				}
			}
		}
	}

	modelSummaryPool = modelSummaryPool[:0]
	if cap(modelSummaryPool) < len(uniqueModelsPool) {
		modelSummaryPool = make([]ModelSummary, 0, len(uniqueModelsPool))
	}

	for _, summary := range uniqueModelsPool {
		modelSummaryPool = append(modelSummaryPool, *summary)
	}

	return modelSummaryPool
}

func (a *Application) createModelSummary(model *domain.ModelInfo, endpoints []string) *ModelSummary {
	summary := &ModelSummary{
		Name:      model.Name,
		Type:      model.Type,
		Endpoints: endpoints,
		// EndpointURLs: endpointURLs,
		LastSeen: format.TimeAgo(model.LastSeen),
	}

	if model.Details != nil {
		if model.Details.Family != nil {
			summary.Family = *model.Details.Family
		}
		if model.Details.ParameterSize != nil {
			summary.Params = *model.Details.ParameterSize
		}
		if model.Details.QuantizationLevel != nil {
			summary.Quant = *model.Details.QuantizationLevel
		}
		summary.Capabilities = a.inferCapabilities(model.Details)
	}

	if model.Size > 0 {
		summary.Size = format.Bytes(uint64(model.Size))
	}
	return summary
}

func (a *Application) groupModelsByFamily(models []ModelSummary) map[string][]string {
	for k := range familyGroupPool {
		delete(familyGroupPool, k)
	}

	for i := range models {
		family := models[i].Family
		if family == "" {
			family = familyUnknown
		}
		familyGroupPool[family] = append(familyGroupPool[family], models[i].Name)
	}

	for family := range familyGroupPool {
		sort.Strings(familyGroupPool[family])
	}

	return familyGroupPool
}

func (a *Application) groupModelsByFamilyWithDetails(models []ModelSummary) []ModelGroupSummary {
	familyMap := make(map[string][]ModelSummary)

	for i := range models {
		family := models[i].Family
		if family == "" {
			family = familyUnknown
		}
		familyMap[family] = append(familyMap[family], models[i])
	}

	modelGroupPool = modelGroupPool[:0]
	if cap(modelGroupPool) < len(familyMap) {
		modelGroupPool = make([]ModelGroupSummary, 0, len(familyMap))
	}

	for family, familyModels := range familyMap {
		for k := range endpointSetPool {
			delete(endpointSetPool, k)
		}

		for i := range familyModels {
			for j := range familyModels[i].Endpoints {
				endpointSetPool[familyModels[i].Endpoints[j]] = struct{}{}
			}
		}

		stringSlicePool = stringSlicePool[:0]
		for ep := range endpointSetPool {
			stringSlicePool = append(stringSlicePool, ep)
		}
		sort.Strings(stringSlicePool)

		endpoints := make([]string, len(stringSlicePool))
		copy(endpoints, stringSlicePool)

		sort.Slice(familyModels, func(i, j int) bool {
			return familyModels[i].Name < familyModels[j].Name
		})

		group := ModelGroupSummary{
			Family:     family,
			ModelCount: len(familyModels),
			Models:     familyModels,
			Endpoints:  endpoints,
		}

		modelGroupPool = append(modelGroupPool, group)
	}

	sort.Slice(modelGroupPool, func(i, j int) bool {
		if modelGroupPool[i].Family == familyUnknown {
			return false
		}
		if modelGroupPool[j].Family == familyUnknown {
			return true
		}
		return modelGroupPool[i].Family < modelGroupPool[j].Family
	})

	return modelGroupPool
}

func (a *Application) getRecentModels(models []ModelSummary, limit int) []ModelSummary {
	sort.Slice(models, func(i, j int) bool {
		return parseTimeAgoOptimised(models[i].LastSeen) > parseTimeAgoOptimised(models[j].LastSeen)
	})

	if len(models) > limit {
		return models[:limit]
	}
	return models
}

const modelTypeEmbeddings = "embeddings"

func (a *Application) inferCapabilities(details *domain.ModelDetails) []string {
	capabilitiesPool = capabilitiesPool[:0]

	if details.Type != nil {
		switch *details.Type {
		case "vlm":
			capabilitiesPool = append(capabilitiesPool, "vision", "multimodal")
		case modelTypeEmbeddings:
			capabilitiesPool = append(capabilitiesPool, "embeddings", "vector_search")
		case "llm":
			capabilitiesPool = append(capabilitiesPool, "text_generation", "chat")
		}
	}

	if details.MaxContextLength != nil && *details.MaxContextLength > 100000 {
		capabilitiesPool = append(capabilitiesPool, "long_context")
	}

	if details.QuantizationLevel != nil {
		quant := *details.QuantizationLevel
		if strings.Contains(quant, "fp16") || strings.Contains(quant, "bf16") {
			capabilitiesPool = append(capabilitiesPool, "high_precision")
		}
	}

	if len(capabilitiesPool) == 0 {
		return nil
	}
	result := make([]string, len(capabilitiesPool))
	copy(result, capabilitiesPool)
	return result
}

// from Scout
func parseTimeAgoOptimised(timeAgo string) int64 {
	if strings.Contains(timeAgo, "second") {
		return time.Now().Unix() - 30
	}
	if strings.Contains(timeAgo, "minute") {
		if len(timeAgo) > 2 && timeAgo[0] >= '0' && timeAgo[0] <= '9' {
			if num, err := strconv.Atoi(string(timeAgo[0])); err == nil {
				return time.Now().Unix() - int64(num*60)
			}
		}
		return time.Now().Unix() - 300
	}
	if strings.Contains(timeAgo, "hour") {
		if len(timeAgo) > 2 && timeAgo[0] >= '0' && timeAgo[0] <= '9' {
			if num, err := strconv.Atoi(string(timeAgo[0])); err == nil {
				return time.Now().Unix() - int64(num*3600)
			}
		}
		return time.Now().Unix() - 7200
	}
	if strings.Contains(timeAgo, "day") {
		return time.Now().Unix() - 43200
	}
	return time.Now().Unix() - 86400
}
