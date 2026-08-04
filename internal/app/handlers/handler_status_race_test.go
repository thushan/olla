package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thushan/olla/internal/config"
	"github.com/thushan/olla/internal/core/domain"
)

// TestModelsStatusHandler_Concurrent verifies that concurrent requests to
// /internal/status/models produce valid, independent responses with no data
// races. The race detector must flag any shared-state violation on the slices
// and maps that were previously package-level globals.
func TestModelsStatusHandler_Concurrent(t *testing.T) {
	t.Parallel()

	endpoints := []*domain.Endpoint{
		{
			Name:      "ollama-local",
			Type:      "ollama",
			URLString: "http://localhost:11434",
			Status:    domain.StatusHealthy,
			Priority:  1,
		},
		{
			Name:      "lmstudio-local",
			Type:      "lm-studio",
			URLString: "http://localhost:1234",
			Status:    domain.StatusHealthy,
			Priority:  2,
		},
	}

	// Populate the model registry with a couple of models so the code paths
	// that build summaries and group by family are exercised.
	family := "llama"
	paramSize := "7B"
	quant := "Q4_K_M"
	modelInfo := &domain.ModelInfo{
		Name: "llama3:7b",
		Type: modelTypeLLM,
		Size: 4_000_000_000,
		Details: &domain.ModelDetails{
			Family:            &family,
			ParameterSize:     &paramSize,
			QuantizationLevel: &quant,
		},
	}

	registry := &mockStatusModelRegistry{
		endpointModels: map[string]*domain.EndpointModels{
			"http://localhost:11434": {
				Models: []*domain.ModelInfo{modelInfo},
			},
			"http://localhost:1234": {
				Models: []*domain.ModelInfo{modelInfo},
			},
		},
	}

	repo := &mockStatusEndpointRepository{endpoints: endpoints}
	stats := &mockStatusStatsCollector{}

	app := &Application{
		repository:     repo,
		statsCollector: stats,
		modelRegistry:  registry,
	}

	const goroutines = 40
	var wg sync.WaitGroup
	errs := make(chan error, goroutines)

	for range goroutines {
		wg.Add(1)
		go func() {
			defer wg.Done()

			req := httptest.NewRequest(http.MethodGet, "/internal/status/models?detailed=true&group=family", nil)
			w := httptest.NewRecorder()

			app.modelsStatusHandler(w, req)

			if w.Code != http.StatusOK {
				errs <- assert.AnError
				return
			}

			var resp ModelStatusResponse
			if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
				errs <- err
				return
			}

			// Basic sanity: both endpoints expose the same model so we expect 1 unique entry.
			if resp.TotalModels != 1 {
				errs <- assert.AnError
			}
		}()
	}

	wg.Wait()
	close(errs)

	for err := range errs {
		require.NoError(t, err, "concurrent /internal/status/models request failed")
	}
}

// TestStatusHandler_Concurrent verifies that concurrent requests to
// /internal/status produce valid independent responses. The issuesPool was a
// package-level var that could be corrupted across goroutines.
func TestStatusHandler_Concurrent(t *testing.T) {
	t.Parallel()

	endpoints := []*domain.Endpoint{
		{
			Name:                "ep-healthy",
			Type:                "ollama",
			URLString:           "http://localhost:11434",
			Status:              domain.StatusHealthy,
			Priority:            1,
			ConsecutiveFailures: 0,
		},
		{
			Name:                "ep-unhealthy",
			Type:                "openai",
			URLString:           "http://localhost:8080",
			Status:              domain.StatusUnhealthy,
			Priority:            2,
			ConsecutiveFailures: 5,
		},
	}

	app := &Application{
		repository:     &mockStatusEndpointRepository{endpoints: endpoints},
		statsCollector: &mockStatusStatsCollector{},
		modelRegistry:  &mockStatusModelRegistry{},
		StartTime:      time.Now(),
		Config:         &config.Config{},
	}

	const goroutines = 40
	var wg sync.WaitGroup
	errs := make(chan error, goroutines)

	for range goroutines {
		wg.Add(1)
		go func() {
			defer wg.Done()

			req := httptest.NewRequest(http.MethodGet, "/internal/status", nil)
			w := httptest.NewRecorder()

			app.statusHandler(w, req)

			if w.Code != http.StatusOK {
				errs <- assert.AnError
				return
			}

			var resp StatusResponse
			if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
				errs <- err
				return
			}

			if len(resp.Endpoints) != 2 {
				errs <- assert.AnError
			}
		}()
	}

	wg.Wait()
	close(errs)

	for err := range errs {
		require.NoError(t, err, "concurrent /internal/status request failed")
	}
}
