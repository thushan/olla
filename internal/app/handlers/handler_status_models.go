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
	// Additive (FR-13): absolute form of LastSeen for the dashboard.
	LastSeenAt   *time.Time `json:"last_seen_at,omitempty"`
	Name         string     `json:"name"`
	Type         string     `json:"type,omitempty"`
	Family       string     `json:"family,omitempty"`
	Size         string     `json:"size,omitempty"`
	Params       string     `json:"params,omitempty"`
	Quant        string     `json:"quant,omitempty"`
	LastSeen     string     `json:"last_seen"`
	Endpoints    []string   `json:"endpoints"`
	Capabilities []string   `json:"capabilities,omitempty"`
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

	endpointNames := make(map[string]string, len(endpoints))
	for _, ep := range endpoints {
		endpointNames[ep.URLString] = ep.Name
	}

	allModels := a.buildModelSummaries(modelMap, endpointNames)

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
	uniqueModels := make(map[string]*ModelSummary, maxModelsCapacity)

	for endpointURL, endpointModels := range modelMap {
		endpointName := endpointNames[endpointURL]
		if endpointName == "" {
			// FR-14: an endpoint with no configured name, or a stale model-map
			// entry with no repository match, falls back to the raw URL, which
			// can carry credentials in a query string (e.g. ?api_key=secret).
			// Sanitise it the same way sanitiseDisplayURL does for the other
			// status handlers before it ever reaches a response.
			endpointName = sanitiseDisplayURL(endpointURL)
		}

		for _, model := range endpointModels.Models {
			existing, exists := uniqueModels[model.Name]
			if !exists {
				uniqueModels[model.Name] = a.createModelSummary(model, []string{endpointName})
			} else {
				existing.Endpoints = append(existing.Endpoints, endpointName)

				if model.LastSeen.Unix() > parseTimeAgoOptimised(existing.LastSeen) {
					existing.LastSeen = format.TimeAgo(model.LastSeen)
					if !model.LastSeen.IsZero() {
						ls := model.LastSeen
						existing.LastSeenAt = &ls
					}
				}
			}
		}
	}

	summaries := make([]ModelSummary, 0, len(uniqueModels))
	for _, summary := range uniqueModels {
		// FR-15: endpoint list order depends on map iteration; sort for stable output.
		sort.Strings(summary.Endpoints)
		summaries = append(summaries, *summary)
	}

	return summaries
}

func (a *Application) createModelSummary(model *domain.ModelInfo, endpoints []string) *ModelSummary {
	summary := &ModelSummary{
		Name:      model.Name,
		Type:      model.Type,
		Endpoints: endpoints,
		// EndpointURLs: endpointURLs,
		LastSeen: format.TimeAgo(model.LastSeen),
	}
	if !model.LastSeen.IsZero() {
		ls := model.LastSeen
		summary.LastSeenAt = &ls
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
	familyGroup := make(map[string][]string, 16)

	for i := range models {
		family := models[i].Family
		if family == "" {
			family = familyUnknown
		}
		familyGroup[family] = append(familyGroup[family], models[i].Name)
	}

	for family := range familyGroup {
		sort.Strings(familyGroup[family])
	}

	return familyGroup
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

	modelGroups := make([]ModelGroupSummary, 0, len(familyMap))

	for family, familyModels := range familyMap {
		endpointSet := make(map[string]struct{}, 8)
		for i := range familyModels {
			for j := range familyModels[i].Endpoints {
				endpointSet[familyModels[i].Endpoints[j]] = struct{}{}
			}
		}

		epSlice := make([]string, 0, len(endpointSet))
		for ep := range endpointSet {
			epSlice = append(epSlice, ep)
		}
		sort.Strings(epSlice)

		sort.Slice(familyModels, func(i, j int) bool {
			return familyModels[i].Name < familyModels[j].Name
		})

		modelGroups = append(modelGroups, ModelGroupSummary{
			Family:     family,
			ModelCount: len(familyModels),
			Models:     familyModels,
			Endpoints:  epSlice,
		})
	}

	sort.Slice(modelGroups, func(i, j int) bool {
		if modelGroups[i].Family == familyUnknown {
			return false
		}
		if modelGroups[j].Family == familyUnknown {
			return true
		}
		return modelGroups[i].Family < modelGroups[j].Family
	})

	return modelGroups
}

func (a *Application) getRecentModels(models []ModelSummary, limit int) []ModelSummary {
	sort.Slice(models, func(i, j int) bool {
		ti, tj := parseTimeAgoOptimised(models[i].LastSeen), parseTimeAgoOptimised(models[j].LastSeen)
		if ti != tj {
			return ti > tj
		}
		// FR-15: parseTimeAgoOptimised is coarse (many models share a bucket),
		// so break ties by name for a stable, diffable order across polls.
		return models[i].Name < models[j].Name
	})

	if len(models) > limit {
		return models[:limit]
	}
	return models
}

const modelTypeEmbeddings = "embeddings"

func (a *Application) inferCapabilities(details *domain.ModelDetails) []string {
	caps := make([]string, 0, 4)

	if details.Type != nil {
		switch *details.Type {
		case "vlm":
			caps = append(caps, "vision", "multimodal")
		case modelTypeEmbeddings:
			caps = append(caps, "embeddings", "vector_search")
		case "llm":
			caps = append(caps, "text_generation", "chat")
		}
	}

	if details.MaxContextLength != nil && *details.MaxContextLength > 100000 {
		caps = append(caps, "long_context")
	}

	if details.QuantizationLevel != nil {
		quant := *details.QuantizationLevel
		if strings.Contains(quant, "fp16") || strings.Contains(quant, "bf16") {
			caps = append(caps, "high_precision")
		}
	}

	if len(caps) == 0 {
		return nil
	}
	return caps
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
