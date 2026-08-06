package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/core/ports"
)

// mockStatusEndpointRepository for status endpoint testing
type mockStatusEndpointRepository struct {
	endpoints []*domain.Endpoint
}

func (m *mockStatusEndpointRepository) GetAll(ctx context.Context) ([]*domain.Endpoint, error) {
	return m.endpoints, nil
}

func (m *mockStatusEndpointRepository) GetHealthy(ctx context.Context) ([]*domain.Endpoint, error) {
	healthy := make([]*domain.Endpoint, 0)
	for _, ep := range m.endpoints {
		if ep.Status == domain.StatusHealthy {
			healthy = append(healthy, ep)
		}
	}
	return healthy, nil
}

func (m *mockStatusEndpointRepository) GetRoutable(ctx context.Context) ([]*domain.Endpoint, error) {
	routable := make([]*domain.Endpoint, 0)
	for _, ep := range m.endpoints {
		if ep.Status == domain.StatusHealthy {
			routable = append(routable, ep)
		}
	}
	return routable, nil
}

func (m *mockStatusEndpointRepository) UpdateEndpoint(ctx context.Context, endpoint *domain.Endpoint) error {
	for i, ep := range m.endpoints {
		if ep.Name == endpoint.Name {
			m.endpoints[i] = endpoint
			return nil
		}
	}
	return nil
}

func (m *mockStatusEndpointRepository) Exists(ctx context.Context, endpointURL *url.URL) bool {
	urlStr := endpointURL.String()
	for _, ep := range m.endpoints {
		if ep.URLString == urlStr {
			return true
		}
	}
	return false
}

// mockStatusStatsCollector for status endpoint testing
type mockStatusStatsCollector struct {
	endpointStats      map[string]ports.EndpointStats
	proxyStats         ports.ProxyStats
	modelStats         map[string]ports.ModelStats
	modelEndpointStats map[string]map[string]ports.EndpointModelStats
	connectionStats    map[string]int64
}

func (m *mockStatusStatsCollector) RecordRequest(endpoint *domain.Endpoint, status string, latency time.Duration, bytes int64) {
}
func (m *mockStatusStatsCollector) RecordConnection(endpoint *domain.Endpoint, delta int)     {}
func (m *mockStatusStatsCollector) RecordSecurityViolation(violation ports.SecurityViolation) {}
func (m *mockStatusStatsCollector) RecordDiscovery(endpoint *domain.Endpoint, success bool, latency time.Duration) {
}
func (m *mockStatusStatsCollector) RecordModelRequest(model string, endpoint *domain.Endpoint, status string, latency time.Duration, bytes int64) {
}
func (m *mockStatusStatsCollector) RecordModelError(model string, endpoint *domain.Endpoint, errorType string) {
}
func (m *mockStatusStatsCollector) GetModelStats() map[string]ports.ModelStats {
	if m.modelStats == nil {
		return make(map[string]ports.ModelStats)
	}
	return m.modelStats
}
func (m *mockStatusStatsCollector) GetModelEndpointStats() map[string]map[string]ports.EndpointModelStats {
	if m.modelEndpointStats == nil {
		return make(map[string]map[string]ports.EndpointModelStats)
	}
	return m.modelEndpointStats
}
func (m *mockStatusStatsCollector) RecordTranslatorRequest(event ports.TranslatorRequestEvent) {}
func (m *mockStatusStatsCollector) GetTranslatorStats() map[string]ports.TranslatorStats {
	return nil
}
func (m *mockStatusStatsCollector) GetProxyStats() ports.ProxyStats { return m.proxyStats }
func (m *mockStatusStatsCollector) GetSecurityStats() ports.SecurityStats {
	return ports.SecurityStats{}
}
func (m *mockStatusStatsCollector) GetConnectionStats() map[string]int64 {
	if m.connectionStats == nil {
		return make(map[string]int64)
	}
	return m.connectionStats
}
func (m *mockStatusStatsCollector) GetConnectionCount(endpoint string) int64 {
	return m.connectionStats[endpoint]
}
func (m *mockStatusStatsCollector) RecordModelTokens(model string, inputTokens, outputTokens int64) {
}

func (m *mockStatusStatsCollector) GetEndpointStats() map[string]ports.EndpointStats {
	if m.endpointStats == nil {
		return make(map[string]ports.EndpointStats)
	}
	return m.endpointStats
}

// mockStatusModelRegistry for status endpoint testing
type mockStatusModelRegistry struct {
	endpointModels map[string]*domain.EndpointModels
}

func (m *mockStatusModelRegistry) RegisterModel(ctx context.Context, endpointURL string, model *domain.ModelInfo) error {
	return nil
}
func (m *mockStatusModelRegistry) RegisterModels(ctx context.Context, endpointURL string, models []*domain.ModelInfo) error {
	return nil
}
func (m *mockStatusModelRegistry) GetModelsForEndpoint(ctx context.Context, endpointURL string) ([]*domain.ModelInfo, error) {
	return nil, nil
}
func (m *mockStatusModelRegistry) GetEndpointsForModel(ctx context.Context, modelName string) ([]string, error) {
	return nil, nil
}
func (m *mockStatusModelRegistry) IsModelAvailable(ctx context.Context, modelName string) bool {
	return false
}
func (m *mockStatusModelRegistry) GetAllModels(ctx context.Context) (map[string][]*domain.ModelInfo, error) {
	return nil, nil
}
func (m *mockStatusModelRegistry) RemoveEndpoint(ctx context.Context, endpointURL string) error {
	return nil
}
func (m *mockStatusModelRegistry) GetStats(ctx context.Context) (domain.RegistryStats, error) {
	return domain.RegistryStats{}, nil
}
func (m *mockStatusModelRegistry) ModelsToString(models []*domain.ModelInfo) string {
	return ""
}
func (m *mockStatusModelRegistry) ModelsToStrings(models []*domain.ModelInfo) []string {
	return nil
}
func (m *mockStatusModelRegistry) GetModelsByCapability(ctx context.Context, capability string) ([]*domain.UnifiedModel, error) {
	return nil, nil
}
func (m *mockStatusModelRegistry) GetRoutableEndpointsForModel(ctx context.Context, modelName string, healthyEndpoints []*domain.Endpoint) ([]*domain.Endpoint, *domain.ModelRoutingDecision, error) {
	return healthyEndpoints, &domain.ModelRoutingDecision{}, nil
}

func (m *mockStatusModelRegistry) GetEndpointModelMap(ctx context.Context) (map[string]*domain.EndpointModels, error) {
	if m.endpointModels == nil {
		return make(map[string]*domain.EndpointModels), nil
	}
	return m.endpointModels, nil
}

// createTestStatusApplication creates a minimal Application for status endpoint testing
func createTestStatusApplication(endpoints []*domain.Endpoint) *Application {
	repo := &mockStatusEndpointRepository{endpoints: endpoints}
	stats := &mockStatusStatsCollector{}
	registry := &mockStatusModelRegistry{}

	return &Application{
		repository:     repo,
		statsCollector: stats,
		modelRegistry:  registry,
		StartTime:      time.Now(),
	}
}

func TestEndpointsStatusHandler_BasicFunctionality(t *testing.T) {
	// Create test endpoints
	endpoints := []*domain.Endpoint{
		{
			Name:      "test-endpoint-1",
			Type:      "ollama",
			URLString: "http://localhost:11434",
			Status:    domain.StatusHealthy,
			Priority:  1,
		},
		{
			Name:      "test-endpoint-2",
			Type:      "openai",
			URLString: "http://localhost:8080",
			Status:    domain.StatusUnhealthy,
			Priority:  2,
		},
	}

	app := createTestStatusApplication(endpoints)

	req := httptest.NewRequest(http.MethodGet, "/internal/status/endpoints", nil)
	w := httptest.NewRecorder()

	app.endpointsStatusHandler(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Header().Get("Content-Type"), "application/json")

	var response EndpointStatusResponse
	err := json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)

	assert.Equal(t, 2, response.TotalCount)
	assert.Equal(t, 1, response.HealthyCount)
	assert.Equal(t, 1, response.RoutableCount)
	assert.Len(t, response.Endpoints, 2)
}

func TestEndpointsStatusHandler_Concurrent(t *testing.T) {
	// Create test endpoints
	endpoints := []*domain.Endpoint{
		{
			Name:      "test-endpoint-1",
			Type:      "ollama",
			URLString: "http://localhost:11434",
			Status:    domain.StatusHealthy,
			Priority:  1,
		},
		{
			Name:      "test-endpoint-2",
			Type:      "openai",
			URLString: "http://localhost:8080",
			Status:    domain.StatusHealthy,
			Priority:  2,
		},
		{
			Name:      "test-endpoint-3",
			Type:      "lm-studio",
			URLString: "http://localhost:1234",
			Status:    domain.StatusUnhealthy,
			Priority:  3,
		},
	}

	app := createTestStatusApplication(endpoints)

	// Run 20 concurrent requests to stress test for race conditions
	const numRequests = 20
	var wg sync.WaitGroup
	errors := make(chan error, numRequests)
	results := make(chan int, numRequests)

	for range numRequests {
		wg.Add(1)
		go func() {
			defer wg.Done()

			req := httptest.NewRequest(http.MethodGet, "/internal/status/endpoints", nil)
			w := httptest.NewRecorder()

			app.endpointsStatusHandler(w, req)

			if w.Code != http.StatusOK {
				errors <- assert.AnError
				return
			}

			var response EndpointStatusResponse
			if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
				errors <- err
				return
			}

			// Verify response integrity
			if response.TotalCount != 3 {
				errors <- assert.AnError
				return
			}

			if len(response.Endpoints) != 3 {
				errors <- assert.AnError
				return
			}

			results <- w.Code
		}()
	}

	wg.Wait()
	close(errors)
	close(results)

	// Check for any errors
	for err := range errors {
		require.NoError(t, err, "Concurrent request failed")
	}

	// Verify all requests succeeded
	successCount := 0
	for range results {
		successCount++
	}
	assert.Equal(t, numRequests, successCount, "All concurrent requests should succeed")
}

func TestEndpointsStatusHandler_SortingPriority(t *testing.T) {
	// Create test endpoints with different priorities
	endpoints := []*domain.Endpoint{
		{
			Name:      "low-priority",
			Type:      "ollama",
			URLString: "http://localhost:11434",
			Status:    domain.StatusHealthy,
			Priority:  1,
		},
		{
			Name:      "high-priority",
			Type:      "openai",
			URLString: "http://localhost:8080",
			Status:    domain.StatusHealthy,
			Priority:  10,
		},
		{
			Name:      "medium-priority",
			Type:      "lm-studio",
			URLString: "http://localhost:1234",
			Status:    domain.StatusHealthy,
			Priority:  5,
		},
	}

	app := createTestStatusApplication(endpoints)

	req := httptest.NewRequest(http.MethodGet, "/internal/status/endpoints", nil)
	w := httptest.NewRecorder()

	app.endpointsStatusHandler(w, req)

	var response EndpointStatusResponse
	err := json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)

	// Verify sorting: highest priority first
	assert.Equal(t, "high-priority", response.Endpoints[0].Name)
	assert.Equal(t, 10, response.Endpoints[0].Priority)
	assert.Equal(t, "medium-priority", response.Endpoints[1].Name)
	assert.Equal(t, 5, response.Endpoints[1].Priority)
	assert.Equal(t, "low-priority", response.Endpoints[2].Name)
	assert.Equal(t, 1, response.Endpoints[2].Priority)
}

func TestEndpointsStatusHandler_SortingHealthStatus(t *testing.T) {
	// Create test endpoints with same priority but different health status
	endpoints := []*domain.Endpoint{
		{
			Name:      "unhealthy-endpoint",
			Type:      "ollama",
			URLString: "http://localhost:11434",
			Status:    domain.StatusUnhealthy,
			Priority:  5,
		},
		{
			Name:      "healthy-endpoint",
			Type:      "openai",
			URLString: "http://localhost:8080",
			Status:    domain.StatusHealthy,
			Priority:  5,
		},
	}

	app := createTestStatusApplication(endpoints)

	req := httptest.NewRequest(http.MethodGet, "/internal/status/endpoints", nil)
	w := httptest.NewRecorder()

	app.endpointsStatusHandler(w, req)

	var response EndpointStatusResponse
	err := json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)

	// Verify sorting: with same priority, healthy comes first
	assert.Equal(t, "healthy-endpoint", response.Endpoints[0].Name)
	assert.Equal(t, "healthy", response.Endpoints[0].Status)
	assert.Equal(t, "unhealthy-endpoint", response.Endpoints[1].Name)
}

func TestEndpointsStatusHandler_EmptyEndpoints(t *testing.T) {
	app := createTestStatusApplication([]*domain.Endpoint{})

	req := httptest.NewRequest(http.MethodGet, "/internal/status/endpoints", nil)
	w := httptest.NewRecorder()

	app.endpointsStatusHandler(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response EndpointStatusResponse
	err := json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)

	assert.Equal(t, 0, response.TotalCount)
	assert.Equal(t, 0, response.HealthyCount)
	assert.Equal(t, 0, response.RoutableCount)
	assert.Empty(t, response.Endpoints)
}

// C5: equal-priority same-health endpoints must produce a deterministic order
// across polls. The previous comparator sorted by priority then health class
// only, and its input came from map iteration, so two endpoints with equal
// priority in the same health class reordered between polls purely from
// map-iteration randomisation. The name (then URL) tie-breaker makes the
// order stable for ties without disturbing the priority/health ordering.
// Input here is deliberately reverse-alphabetical: against the current
// comparator (no tie-breaker) insertion sort preserves this input order, so
// the alphabetical assertion fails until the tie-breaker is added.
func TestEndpointsStatusHandler_TieBreakerStableOrder(t *testing.T) {
	endpoints := []*domain.Endpoint{
		{Name: "zebra", Type: "ollama", URLString: "http://z:11434", Status: domain.StatusHealthy, Priority: 5},
		{Name: "mango", Type: "ollama", URLString: "http://m:11434", Status: domain.StatusHealthy, Priority: 5},
		{Name: "alpha", Type: "ollama", URLString: "http://a:11434", Status: domain.StatusHealthy, Priority: 5},
	}

	app := createTestStatusApplication(endpoints)
	req := httptest.NewRequest(http.MethodGet, "/internal/status/endpoints", nil)
	w := httptest.NewRecorder()
	app.endpointsStatusHandler(w, req)

	var response EndpointStatusResponse
	err := json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)

	require.Len(t, response.Endpoints, 3)
	assert.Equal(t, "alpha", response.Endpoints[0].Name, "tie-breaker must sort equal-priority same-health endpoints by name")
	assert.Equal(t, "mango", response.Endpoints[1].Name)
	assert.Equal(t, "zebra", response.Endpoints[2].Name)
}

// Regression for the B2/C1 conflict: sanitiseDisplayURL strips query and
// fragment, so two endpoints that differ only by query string arrive with an
// identical display URL. Before the ID tie-breaker, the URL-based comparator
// tied completely on same name+URL and the sort was unstable across polls
// (input order decided it, and input order comes from map iteration). The ID
// tie-breaker must still order deterministically because it is derived from
// the raw, pre-sanitisation URL.
func TestEndpointsStatusHandler_TieBreakerDeterministicOnCollidingDisplayURL(t *testing.T) {
	a := &domain.Endpoint{Name: "twin", Type: "ollama", URLString: "http://host:11434?v=a", Status: domain.StatusHealthy, Priority: 5}
	b := &domain.Endpoint{Name: "twin", Type: "ollama", URLString: "http://host:11434?v=b", Status: domain.StatusHealthy, Priority: 5}

	// The repository's endpoint map is keyed on the raw URL (B2), so a and b
	// are genuinely distinct entries; GetAll's returned slice order is not
	// guaranteed the same across polls (real repository backs onto a map). A
	// correct tie-breaker must pick the same winner regardless of which
	// order they arrive in - that's what "stable across polls" means. Feed
	// both input orders and compare the winner by identity (ID), not index.
	orderings := [][]*domain.Endpoint{{a, b}, {b, a}}

	var firstWinner string
	for i, endpoints := range orderings {
		app := createTestStatusApplication(endpoints)
		req := httptest.NewRequest(http.MethodGet, "/internal/status/endpoints", nil)
		w := httptest.NewRecorder()
		app.endpointsStatusHandler(w, req)

		var response EndpointStatusResponse
		require.NoError(t, json.NewDecoder(w.Body).Decode(&response))
		require.Len(t, response.Endpoints, 2)

		// The two rows must remain individually identifiable and their
		// display URLs really do collide - that is the bug this ID exists to
		// route around.
		assert.NotEqual(t, response.Endpoints[0].ID, response.Endpoints[1].ID)
		assert.Equal(t, response.Endpoints[0].URL, response.Endpoints[1].URL, "test setup: display URLs should collide after sanitisation")

		winner := response.Endpoints[0].ID
		if i == 0 {
			firstWinner = winner
		} else {
			assert.Equal(t, firstWinner, winner, "the same endpoint must sort first regardless of input order (map-iteration order varies across polls)")
		}
	}
}

func TestBuildEndpointSummaryOptimised(t *testing.T) {
	endpoint := &domain.Endpoint{
		Name:      "test-endpoint",
		Type:      "ollama",
		URLString: "http://localhost:11434",
		Status:    domain.StatusHealthy,
		Priority:  5,
	}

	statsMap := map[string]ports.EndpointStats{
		"http://localhost:11434": {
			TotalRequests:      100,
			SuccessfulRequests: 95,
		},
	}

	modelMap := map[string]*domain.EndpointModels{
		"http://localhost:11434": {
			Models:      []*domain.ModelInfo{{Name: "llama2"}},
			LastUpdated: time.Now(),
		},
	}

	app := createTestStatusApplication([]*domain.Endpoint{endpoint})

	endpointIDs := buildEndpointIDs([]*domain.Endpoint{endpoint})
	summary := app.buildEndpointSummaryOptimised(endpoint, statsMap, nil, modelMap, endpointIDs[endpoint.URLString])

	assert.Equal(t, "test-endpoint", summary.Name)
	assert.Equal(t, "ollama", summary.Type)
	assert.Equal(t, "healthy", summary.Status)
	assert.Equal(t, 5, summary.Priority)
	assert.Equal(t, int64(100), summary.RequestCount)
	assert.Equal(t, "95.0%", summary.SuccessRate)
	assert.Equal(t, 1, summary.ModelCount)
}

func TestGetEndpointIssuesSummaryOptimised(t *testing.T) {
	app := createTestStatusApplication([]*domain.Endpoint{})

	tests := []struct {
		name           string
		endpoint       *domain.Endpoint
		stats          ports.EndpointStats
		hasStats       bool
		expectedIssues string
	}{
		{
			name: "healthy endpoint no failures",
			endpoint: &domain.Endpoint{
				Status:              domain.StatusHealthy,
				ConsecutiveFailures: 0,
			},
			stats:          ports.EndpointStats{},
			hasStats:       false,
			expectedIssues: "",
		},
		{
			name: "offline endpoint",
			endpoint: &domain.Endpoint{
				Status: domain.StatusOffline,
			},
			stats:          ports.EndpointStats{},
			hasStats:       false,
			expectedIssues: "unavailable",
		},
		{
			name: "unhealthy endpoint",
			endpoint: &domain.Endpoint{
				Status: domain.StatusUnhealthy,
			},
			stats:          ports.EndpointStats{},
			hasStats:       false,
			expectedIssues: "unavailable",
		},
		{
			name: "unstable endpoint",
			endpoint: &domain.Endpoint{
				Status:              domain.StatusHealthy,
				ConsecutiveFailures: 5,
			},
			stats:          ports.EndpointStats{},
			hasStats:       false,
			expectedIssues: "unstable",
		},
		{
			name: "low success rate with some failures",
			endpoint: &domain.Endpoint{
				Status:              domain.StatusHealthy,
				ConsecutiveFailures: 2, // has some failures, but not enough to be unstable
			},
			stats: ports.EndpointStats{
				TotalRequests:      100,
				SuccessfulRequests: 80, // 80% success rate
			},
			hasStats:       true,
			expectedIssues: "low success rate",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			issues := app.getEndpointIssuesSummaryOptimised(tt.endpoint, tt.stats, tt.hasStats)
			assert.Equal(t, tt.expectedIssues, issues)
		})
	}
}
