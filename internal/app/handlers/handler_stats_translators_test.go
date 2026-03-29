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

// mockTranslatorStatsCollector implements ports.StatsCollector for translator stats testing
type mockTranslatorStatsCollector struct {
	translatorStats map[string]ports.TranslatorStats
}

func (m *mockTranslatorStatsCollector) RecordRequest(endpoint *domain.Endpoint, status string, latency time.Duration, bytes int64) {
}
func (m *mockTranslatorStatsCollector) RecordConnection(endpoint *domain.Endpoint, delta int) {}
func (m *mockTranslatorStatsCollector) RecordSecurityViolation(violation ports.SecurityViolation) {
}
func (m *mockTranslatorStatsCollector) RecordDiscovery(endpoint *domain.Endpoint, success bool, latency time.Duration) {
}
func (m *mockTranslatorStatsCollector) RecordModelRequest(model string, endpoint *domain.Endpoint, status string, latency time.Duration, bytes int64) {
}
func (m *mockTranslatorStatsCollector) RecordModelError(model string, endpoint *domain.Endpoint, errorType string) {
}
func (m *mockTranslatorStatsCollector) GetModelStats() map[string]ports.ModelStats { return nil }
func (m *mockTranslatorStatsCollector) GetModelEndpointStats() map[string]map[string]ports.EndpointModelStats {
	return nil
}
func (m *mockTranslatorStatsCollector) RecordTranslatorRequest(event ports.TranslatorRequestEvent) {
}
func (m *mockTranslatorStatsCollector) GetProxyStats() ports.ProxyStats { return ports.ProxyStats{} }
func (m *mockTranslatorStatsCollector) GetSecurityStats() ports.SecurityStats {
	return ports.SecurityStats{}
}
func (m *mockTranslatorStatsCollector) GetConnectionStats() map[string]int64 { return nil }
func (m *mockTranslatorStatsCollector) RecordModelTokens(model string, inputTokens, outputTokens int64) {
}
func (m *mockTranslatorStatsCollector) GetEndpointStats() map[string]ports.EndpointStats {
	return nil
}

func (m *mockTranslatorStatsCollector) GetTranslatorStats() map[string]ports.TranslatorStats {
	if m.translatorStats == nil {
		return make(map[string]ports.TranslatorStats)
	}
	return m.translatorStats
}

// mockTranslatorEndpointRepository for translator stats testing
type mockTranslatorEndpointRepository struct{}

func (m *mockTranslatorEndpointRepository) GetAll(ctx context.Context) ([]*domain.Endpoint, error) {
	return nil, nil
}
func (m *mockTranslatorEndpointRepository) GetHealthy(ctx context.Context) ([]*domain.Endpoint, error) {
	return nil, nil
}
func (m *mockTranslatorEndpointRepository) GetRoutable(ctx context.Context) ([]*domain.Endpoint, error) {
	return nil, nil
}
func (m *mockTranslatorEndpointRepository) UpdateEndpoint(ctx context.Context, endpoint *domain.Endpoint) error {
	return nil
}
func (m *mockTranslatorEndpointRepository) Exists(ctx context.Context, endpointURL *url.URL) bool {
	return false
}

// mockTranslatorModelRegistry for translator stats testing
type mockTranslatorModelRegistry struct{}

func (m *mockTranslatorModelRegistry) RegisterModel(ctx context.Context, endpointURL string, model *domain.ModelInfo) error {
	return nil
}
func (m *mockTranslatorModelRegistry) RegisterModels(ctx context.Context, endpointURL string, models []*domain.ModelInfo) error {
	return nil
}
func (m *mockTranslatorModelRegistry) GetModelsForEndpoint(ctx context.Context, endpointURL string) ([]*domain.ModelInfo, error) {
	return nil, nil
}
func (m *mockTranslatorModelRegistry) GetEndpointsForModel(ctx context.Context, modelName string) ([]string, error) {
	return nil, nil
}
func (m *mockTranslatorModelRegistry) IsModelAvailable(ctx context.Context, modelName string) bool {
	return false
}
func (m *mockTranslatorModelRegistry) GetAllModels(ctx context.Context) (map[string][]*domain.ModelInfo, error) {
	return nil, nil
}
func (m *mockTranslatorModelRegistry) RemoveEndpoint(ctx context.Context, endpointURL string) error {
	return nil
}
func (m *mockTranslatorModelRegistry) GetStats(ctx context.Context) (domain.RegistryStats, error) {
	return domain.RegistryStats{}, nil
}
func (m *mockTranslatorModelRegistry) ModelsToString(models []*domain.ModelInfo) string {
	return ""
}
func (m *mockTranslatorModelRegistry) ModelsToStrings(models []*domain.ModelInfo) []string {
	return nil
}
func (m *mockTranslatorModelRegistry) GetModelsByCapability(ctx context.Context, capability string) ([]*domain.UnifiedModel, error) {
	return nil, nil
}
func (m *mockTranslatorModelRegistry) GetRoutableEndpointsForModel(ctx context.Context, modelName string, healthyEndpoints []*domain.Endpoint) ([]*domain.Endpoint, *domain.ModelRoutingDecision, error) {
	return nil, nil, nil
}
func (m *mockTranslatorModelRegistry) GetEndpointModelMap(ctx context.Context) (map[string]*domain.EndpointModels, error) {
	return nil, nil
}

// createTestTranslatorStatsApplication creates a minimal Application for translator stats testing
func createTestTranslatorStatsApplication(translatorStats map[string]ports.TranslatorStats) *Application {
	repo := &mockTranslatorEndpointRepository{}
	stats := &mockTranslatorStatsCollector{translatorStats: translatorStats}
	registry := &mockTranslatorModelRegistry{}

	return &Application{
		repository:     repo,
		statsCollector: stats,
		modelRegistry:  registry,
		StartTime:      time.Now(),
	}
}

func TestTranslatorStatsHandler_BasicFunctionality(t *testing.T) {
	translatorStats := map[string]ports.TranslatorStats{
		"anthropic": {
			TranslatorName:                              "anthropic",
			TotalRequests:                               100,
			SuccessfulRequests:                          95,
			FailedRequests:                              5,
			PassthroughRequests:                         80,
			TranslationRequests:                         20,
			StreamingRequests:                           60,
			NonStreamingRequests:                        40,
			AverageLatency:                              245,
			FallbackNoCompatibleEndpoints:               10,
			FallbackTranslatorDoesNotSupportPassthrough: 5,
			FallbackCannotPassthrough:                   5,
		},
		"openai": {
			TranslatorName:                              "openai",
			TotalRequests:                               50,
			SuccessfulRequests:                          48,
			FailedRequests:                              2,
			PassthroughRequests:                         50,
			TranslationRequests:                         0,
			StreamingRequests:                           30,
			NonStreamingRequests:                        20,
			AverageLatency:                              150,
			FallbackNoCompatibleEndpoints:               0,
			FallbackTranslatorDoesNotSupportPassthrough: 0,
			FallbackCannotPassthrough:                   0,
		},
	}

	app := createTestTranslatorStatsApplication(translatorStats)

	req := httptest.NewRequest(http.MethodGet, "/internal/stats/translators", nil)
	w := httptest.NewRecorder()

	app.translatorStatsHandler(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Header().Get("Content-Type"), "application/json")

	var response TranslatorStatsResponse
	err := json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)

	// Verify response structure
	assert.Len(t, response.Translators, 2)
	assert.Equal(t, 2, response.Summary.TotalTranslators)
	assert.Equal(t, 2, response.Summary.ActiveTranslators)
	assert.Equal(t, int64(150), response.Summary.TotalRequests)
	assert.Equal(t, int64(130), response.Summary.TotalPassthrough)
	assert.Equal(t, int64(20), response.Summary.TotalTranslations)

	// Verify translators are sorted by request count (anthropic first with 100 requests)
	assert.Equal(t, "anthropic", response.Translators[0].TranslatorName)
	assert.Equal(t, int64(100), response.Translators[0].TotalRequests)
	assert.Equal(t, "openai", response.Translators[1].TranslatorName)
	assert.Equal(t, int64(50), response.Translators[1].TotalRequests)

	// Verify formatted values
	assert.Equal(t, "95.0%", response.Translators[0].SuccessRate)
	assert.Equal(t, "80.0%", response.Translators[0].PassthroughRate)
	assert.Equal(t, "245ms", response.Translators[0].AverageLatency)
}

func TestTranslatorStatsHandler_EmptyTranslators(t *testing.T) {
	app := createTestTranslatorStatsApplication(map[string]ports.TranslatorStats{})

	req := httptest.NewRequest(http.MethodGet, "/internal/stats/translators", nil)
	w := httptest.NewRecorder()

	app.translatorStatsHandler(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response TranslatorStatsResponse
	err := json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)

	// Verify empty response
	assert.Empty(t, response.Translators)
	assert.Equal(t, 0, response.Summary.TotalTranslators)
	assert.Equal(t, 0, response.Summary.ActiveTranslators)
	assert.Equal(t, int64(0), response.Summary.TotalRequests)

	// Verify zero values are formatted correctly
	assert.Equal(t, "0%", response.Summary.OverallSuccessRate)
	assert.Equal(t, "0%", response.Summary.OverallPassthrough)
}

func TestTranslatorStatsHandler_NilStatsCollector(t *testing.T) {
	app := &Application{
		statsCollector: nil,
	}

	req := httptest.NewRequest(http.MethodGet, "/internal/stats/translators", nil)
	w := httptest.NewRecorder()

	app.translatorStatsHandler(w, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
	assert.Contains(t, w.Body.String(), "Stats collector not initialized")
}

func TestTranslatorStatsHandler_SortingByRequestCount(t *testing.T) {
	translatorStats := map[string]ports.TranslatorStats{
		"low-usage": {
			TranslatorName:     "low-usage",
			TotalRequests:      10,
			SuccessfulRequests: 10,
		},
		"high-usage": {
			TranslatorName:     "high-usage",
			TotalRequests:      1000,
			SuccessfulRequests: 950,
		},
		"medium-usage": {
			TranslatorName:     "medium-usage",
			TotalRequests:      100,
			SuccessfulRequests: 95,
		},
	}

	app := createTestTranslatorStatsApplication(translatorStats)

	req := httptest.NewRequest(http.MethodGet, "/internal/stats/translators", nil)
	w := httptest.NewRecorder()

	app.translatorStatsHandler(w, req)

	var response TranslatorStatsResponse
	err := json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)

	// Verify sorting: highest request count first
	assert.Equal(t, "high-usage", response.Translators[0].TranslatorName)
	assert.Equal(t, int64(1000), response.Translators[0].TotalRequests)
	assert.Equal(t, "medium-usage", response.Translators[1].TranslatorName)
	assert.Equal(t, int64(100), response.Translators[1].TotalRequests)
	assert.Equal(t, "low-usage", response.Translators[2].TranslatorName)
	assert.Equal(t, int64(10), response.Translators[2].TotalRequests)
}

func TestTranslatorStatsHandler_SuccessRateCalculation(t *testing.T) {
	translatorStats := map[string]ports.TranslatorStats{
		"perfect": {
			TranslatorName:     "perfect",
			TotalRequests:      100,
			SuccessfulRequests: 100,
			FailedRequests:     0,
		},
		"mixed": {
			TranslatorName:     "mixed",
			TotalRequests:      100,
			SuccessfulRequests: 75,
			FailedRequests:     25,
		},
		"zero": {
			TranslatorName:     "zero",
			TotalRequests:      0,
			SuccessfulRequests: 0,
			FailedRequests:     0,
		},
	}

	app := createTestTranslatorStatsApplication(translatorStats)

	req := httptest.NewRequest(http.MethodGet, "/internal/stats/translators", nil)
	w := httptest.NewRecorder()

	app.translatorStatsHandler(w, req)

	var response TranslatorStatsResponse
	err := json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)

	// Find each translator in response
	statsMap := make(map[string]TranslatorStatsEntry)
	for _, entry := range response.Translators {
		statsMap[entry.TranslatorName] = entry
	}

	// Verify success rate formatting
	assert.Equal(t, "100%", statsMap["perfect"].SuccessRate)
	assert.Equal(t, "75.0%", statsMap["mixed"].SuccessRate)
	assert.Equal(t, "0%", statsMap["zero"].SuccessRate)
}

func TestTranslatorStatsHandler_LatencyFormatting(t *testing.T) {
	translatorStats := map[string]ports.TranslatorStats{
		"fast": {
			TranslatorName: "fast",
			TotalRequests:  1,
			AverageLatency: 5, // 5ms
		},
		"medium": {
			TranslatorName: "medium",
			TotalRequests:  1,
			AverageLatency: 245, // 245ms
		},
		"slow": {
			TranslatorName: "slow",
			TotalRequests:  1,
			AverageLatency: 367500, // 367.5s
		},
		"zero": {
			TranslatorName: "zero",
			TotalRequests:  1,
			AverageLatency: 0,
		},
	}

	app := createTestTranslatorStatsApplication(translatorStats)

	req := httptest.NewRequest(http.MethodGet, "/internal/stats/translators", nil)
	w := httptest.NewRecorder()

	app.translatorStatsHandler(w, req)

	var response TranslatorStatsResponse
	err := json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)

	// Find each translator in response
	statsMap := make(map[string]TranslatorStatsEntry)
	for _, entry := range response.Translators {
		statsMap[entry.TranslatorName] = entry
	}

	// Verify latency formatting
	assert.Equal(t, "5ms", statsMap["fast"].AverageLatency)
	assert.Equal(t, "245ms", statsMap["medium"].AverageLatency)
	assert.Equal(t, "367.5s", statsMap["slow"].AverageLatency)
	assert.Equal(t, "0ms", statsMap["zero"].AverageLatency)
}

func TestTranslatorStatsHandler_SummaryPassthroughRate(t *testing.T) {
	translatorStats := map[string]ports.TranslatorStats{
		"translator1": {
			TranslatorName:      "translator1",
			TotalRequests:       100,
			PassthroughRequests: 80,
			TranslationRequests: 20,
		},
		"translator2": {
			TranslatorName:      "translator2",
			TotalRequests:       50,
			PassthroughRequests: 50,
			TranslationRequests: 0,
		},
	}

	app := createTestTranslatorStatsApplication(translatorStats)

	req := httptest.NewRequest(http.MethodGet, "/internal/stats/translators", nil)
	w := httptest.NewRecorder()

	app.translatorStatsHandler(w, req)

	var response TranslatorStatsResponse
	err := json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)

	// Total: 150 requests, 130 passthrough = 86.666...% ≈ 86.7%
	assert.Equal(t, int64(150), response.Summary.TotalRequests)
	assert.Equal(t, int64(130), response.Summary.TotalPassthrough)
	assert.Equal(t, "86.7%", response.Summary.OverallPassthrough)

	// Individual passthrough rates
	statsMap := make(map[string]TranslatorStatsEntry)
	for _, entry := range response.Translators {
		statsMap[entry.TranslatorName] = entry
	}
	assert.Equal(t, "80.0%", statsMap["translator1"].PassthroughRate)
	assert.Equal(t, "100%", statsMap["translator2"].PassthroughRate)
}

func TestTranslatorStatsHandler_ActiveTranslatorCount(t *testing.T) {
	translatorStats := map[string]ports.TranslatorStats{
		"active1": {
			TranslatorName: "active1",
			TotalRequests:  100,
		},
		"active2": {
			TranslatorName: "active2",
			TotalRequests:  50,
		},
		"inactive": {
			TranslatorName: "inactive",
			TotalRequests:  0,
		},
	}

	app := createTestTranslatorStatsApplication(translatorStats)

	req := httptest.NewRequest(http.MethodGet, "/internal/stats/translators", nil)
	w := httptest.NewRecorder()

	app.translatorStatsHandler(w, req)

	var response TranslatorStatsResponse
	err := json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)

	// Verify total vs active count
	assert.Equal(t, 3, response.Summary.TotalTranslators)
	assert.Equal(t, 2, response.Summary.ActiveTranslators) // Only active1 and active2 have requests
}

func TestTranslatorStatsHandler_FallbackReasonBreakdown(t *testing.T) {
	translatorStats := map[string]ports.TranslatorStats{
		"translator": {
			TranslatorName:                              "translator",
			TotalRequests:                               100,
			FallbackNoCompatibleEndpoints:               15,
			FallbackTranslatorDoesNotSupportPassthrough: 10,
			FallbackCannotPassthrough:                   5,
		},
	}

	app := createTestTranslatorStatsApplication(translatorStats)

	req := httptest.NewRequest(http.MethodGet, "/internal/stats/translators", nil)
	w := httptest.NewRecorder()

	app.translatorStatsHandler(w, req)

	var response TranslatorStatsResponse
	err := json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)

	// Verify all fallback fields are present
	assert.Len(t, response.Translators, 1)
	entry := response.Translators[0]
	assert.Equal(t, int64(15), entry.FallbackNoCompatibleEndpoints)
	assert.Equal(t, int64(10), entry.FallbackTranslatorDoesNotSupportPassthrough)
	assert.Equal(t, int64(5), entry.FallbackCannotPassthrough)
}

func TestTranslatorStatsHandler_Concurrent(t *testing.T) {
	translatorStats := map[string]ports.TranslatorStats{
		"translator1": {
			TranslatorName:      "translator1",
			TotalRequests:       100,
			SuccessfulRequests:  95,
			PassthroughRequests: 80,
			AverageLatency:      150,
		},
		"translator2": {
			TranslatorName:      "translator2",
			TotalRequests:       50,
			SuccessfulRequests:  48,
			PassthroughRequests: 50,
			AverageLatency:      200,
		},
		"translator3": {
			TranslatorName:      "translator3",
			TotalRequests:       25,
			SuccessfulRequests:  20,
			PassthroughRequests: 15,
			AverageLatency:      300,
		},
	}

	app := createTestTranslatorStatsApplication(translatorStats)

	// Run 20 concurrent requests to stress test for race conditions
	const numRequests = 20
	var wg sync.WaitGroup
	errors := make(chan error, numRequests)
	results := make(chan int, numRequests)

	for i := 0; i < numRequests; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			req := httptest.NewRequest(http.MethodGet, "/internal/stats/translators", nil)
			w := httptest.NewRecorder()

			app.translatorStatsHandler(w, req)

			if w.Code != http.StatusOK {
				errors <- assert.AnError
				return
			}

			var response TranslatorStatsResponse
			if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
				errors <- err
				return
			}

			// Verify response integrity
			if len(response.Translators) != 3 {
				errors <- assert.AnError
				return
			}

			if response.Summary.TotalTranslators != 3 {
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
