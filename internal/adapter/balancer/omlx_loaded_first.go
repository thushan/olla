package balancer

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/thushan/olla/internal/core/constants"
	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/core/ports"
)

const (
	defaultOMLXStatusPath    = "/v1/models/status"
	defaultOMLXStatusTTL     = 2 * time.Second
	defaultOMLXStatusTimeout = 300 * time.Millisecond
)

// OMLXLoadedFirstSelector prefers endpoints where the requested model is already
// loaded in oMLX memory, then falls back to least-connections across all routable
// compatible endpoints. It intentionally keeps model compatibility filtering in
// the registry/routing layer; this selector only reorders the eligible set.
type OMLXLoadedFirstSelector struct {
	inner      *LeastConnectionsSelector
	httpClient *http.Client
	statusTTL  time.Duration

	cacheMu sync.RWMutex
	cache   map[string]omlxStatusCacheEntry
}

type omlxStatusCacheEntry struct {
	fetchedAt    time.Time
	loadedModels map[string]bool
}

type omlxStatusResponse struct {
	Models []omlxModelStatus `json:"models"`
}

type omlxModelStatus struct {
	ID     string `json:"id"`
	Loaded bool   `json:"loaded"`
}

func NewOMLXLoadedFirstSelector(statsCollector ports.StatsCollector) *OMLXLoadedFirstSelector {
	return &OMLXLoadedFirstSelector{
		inner:      NewLeastConnectionsSelector(statsCollector),
		httpClient: &http.Client{Timeout: defaultOMLXStatusTimeout},
		statusTTL:  defaultOMLXStatusTTL,
		cache:      make(map[string]omlxStatusCacheEntry),
	}
}

func (selector *OMLXLoadedFirstSelector) Name() string {
	return DefaultBalancerOMLXLoadedFirst
}

func (selector *OMLXLoadedFirstSelector) Select(ctx context.Context, endpoints []*domain.Endpoint) (*domain.Endpoint, error) {
	modelName, _ := ctx.Value(constants.ContextModelKey).(string)
	if modelName == "" {
		return selector.inner.Select(ctx, endpoints)
	}

	loadedEndpoints := selector.loadedEndpoints(ctx, endpoints, modelName)
	if len(loadedEndpoints) > 0 {
		return selector.inner.Select(ctx, loadedEndpoints)
	}

	return selector.inner.Select(ctx, endpoints)
}

func (selector *OMLXLoadedFirstSelector) IncrementConnections(endpoint *domain.Endpoint) {
	selector.inner.IncrementConnections(endpoint)
}

func (selector *OMLXLoadedFirstSelector) DecrementConnections(endpoint *domain.Endpoint) {
	selector.inner.DecrementConnections(endpoint)
}

func (selector *OMLXLoadedFirstSelector) loadedEndpoints(ctx context.Context, endpoints []*domain.Endpoint, requestModel string) []*domain.Endpoint {
	loadedEndpoints := make([]*domain.Endpoint, 0, len(endpoints))
	for _, endpoint := range endpoints {
		if !endpoint.Status.IsRoutable() {
			continue
		}

		backendModel := selector.modelForEndpoint(ctx, endpoint, requestModel)
		if backendModel == "" {
			continue
		}

		if selector.isModelLoaded(ctx, endpoint, backendModel) {
			loadedEndpoints = append(loadedEndpoints, endpoint)
		}
	}

	return loadedEndpoints
}

func (selector *OMLXLoadedFirstSelector) modelForEndpoint(ctx context.Context, endpoint *domain.Endpoint, requestModel string) string {
	aliasMap, ok := ctx.Value(constants.ContextModelAliasMapKey).(map[string]string)
	if !ok {
		return requestModel
	}

	if backendModel := aliasMap[endpoint.GetURLString()]; backendModel != "" {
		return backendModel
	}

	return requestModel
}

func (selector *OMLXLoadedFirstSelector) isModelLoaded(ctx context.Context, endpoint *domain.Endpoint, modelName string) bool {
	if cachedStatus, ok := selector.cachedStatus(endpoint.GetURLString()); ok {
		return cachedStatus.loadedModels[modelName]
	}

	fetchedStatus, err := selector.fetchStatus(ctx, endpoint)
	if err != nil {
		return false
	}

	selector.storeStatus(endpoint.GetURLString(), fetchedStatus)
	return fetchedStatus.loadedModels[modelName]
}

func (selector *OMLXLoadedFirstSelector) cachedStatus(endpointURL string) (omlxStatusCacheEntry, bool) {
	selector.cacheMu.RLock()
	defer selector.cacheMu.RUnlock()

	entry, ok := selector.cache[endpointURL]
	if !ok || time.Since(entry.fetchedAt) > selector.statusTTL {
		return omlxStatusCacheEntry{}, false
	}

	return entry, true
}

func (selector *OMLXLoadedFirstSelector) storeStatus(endpointURL string, entry omlxStatusCacheEntry) {
	selector.cacheMu.Lock()
	defer selector.cacheMu.Unlock()

	selector.cache[endpointURL] = entry
}

func (selector *OMLXLoadedFirstSelector) fetchStatus(ctx context.Context, endpoint *domain.Endpoint) (omlxStatusCacheEntry, error) {
	statusURL, err := omlxStatusURL(endpoint)
	if err != nil {
		return omlxStatusCacheEntry{}, err
	}

	statusCtx, cancel := context.WithTimeout(ctx, defaultOMLXStatusTimeout)
	defer cancel()

	request, err := http.NewRequestWithContext(statusCtx, http.MethodGet, statusURL, nil)
	if err != nil {
		return omlxStatusCacheEntry{}, err
	}

	response, err := selector.httpClient.Do(request)
	if err != nil {
		return omlxStatusCacheEntry{}, err
	}
	defer response.Body.Close()

	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return omlxStatusCacheEntry{}, fmt.Errorf("omlx status request failed for %s: %s", endpoint.Name, response.Status)
	}

	var statusResponse omlxStatusResponse
	if err := json.NewDecoder(response.Body).Decode(&statusResponse); err != nil {
		return omlxStatusCacheEntry{}, err
	}

	loadedModels := make(map[string]bool, len(statusResponse.Models))
	for _, model := range statusResponse.Models {
		if model.ID != "" && model.Loaded {
			loadedModels[model.ID] = true
		}
	}

	return omlxStatusCacheEntry{
		fetchedAt:    time.Now(),
		loadedModels: loadedModels,
	}, nil
}

func omlxStatusURL(endpoint *domain.Endpoint) (string, error) {
	baseURL, err := url.Parse(endpoint.GetURLString())
	if err != nil {
		return "", err
	}

	statusURL := *baseURL
	statusURL.Path = strings.TrimRight(statusURL.Path, "/") + defaultOMLXStatusPath
	statusURL.RawQuery = ""
	statusURL.Fragment = ""

	return statusURL.String(), nil
}