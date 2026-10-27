package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thushan/olla/internal/adapter/converter"
	"github.com/thushan/olla/internal/adapter/registry"
	"github.com/thushan/olla/internal/config"
	"github.com/thushan/olla/internal/core/constants"
	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/logger"
)

// TestProviderSpecificModelEndpoints tests the provider-specific model listing endpoints
func TestProviderSpecificModelEndpoints(t *testing.T) {
	// Setup logger
	loggerCfg := &logger.Config{Level: "error", Theme: "default"}
	log, _, _ := logger.New(loggerCfg)
	styledLogger := logger.NewPlainStyledLogger(log)

	// Create test endpoints
	ollamaURL, _ := url.Parse("http://ollama:11434")
	lmstudioURL, _ := url.Parse("http://lmstudio:1234")
	openaiURL, _ := url.Parse("http://openai:8080")
	vllmURL, _ := url.Parse("http://vllm:8000")
	lemonadeURL, _ := url.Parse("http://lemonade:8000")

	endpoints := []*domain.Endpoint{
		{
			Name:      "ollama-1",
			URL:       ollamaURL,
			URLString: ollamaURL.String(),
			Status:    domain.StatusHealthy,
			Type:      "ollama",
		},
		{
			Name:      "lmstudio-1",
			URL:       lmstudioURL,
			URLString: lmstudioURL.String(),
			Status:    domain.StatusHealthy,
			Type:      "lm-studio",
		},
		{
			Name:      "openai-1",
			URL:       openaiURL,
			URLString: openaiURL.String(),
			Status:    domain.StatusHealthy,
			Type:      "openai",
		},
		{
			Name:      "vllm-1",
			URL:       vllmURL,
			URLString: vllmURL.String(),
			Status:    domain.StatusHealthy,
			Type:      "vllm",
		},
		{
			Name:      "lemonade-1",
			URL:       lemonadeURL,
			URLString: lemonadeURL.String(),
			Status:    domain.StatusHealthy,
			Type:      "lemonade",
		},
	}

	// Create registry and register models
	unifiedRegistry := registry.NewUnifiedMemoryModelRegistry(styledLogger, nil, nil, nil)
	ctx := context.Background()

	// Register endpoints
	for _, ep := range endpoints {
		unifiedRegistry.RegisterEndpoint(ep)
	}

	// Register models for each provider
	ollamaModels := []*domain.ModelInfo{{Name: "llama3:latest"}, {Name: "mistral:latest"}}
	lmstudioModels := []*domain.ModelInfo{{Name: "TheBloke/Llama-2-7B-Chat-GGUF"}}
	openaiModels := []*domain.ModelInfo{{Name: "gpt-3.5-turbo"}}
	vllmModels := []*domain.ModelInfo{{Name: "meta-llama/Llama-2-7b-hf"}}
	lemonadeModels := []*domain.ModelInfo{{Name: "Qwen2.5-0.5B-Instruct-CPU"}}

	require.NoError(t, unifiedRegistry.RegisterModelsWithEndpoint(ctx, endpoints[0], ollamaModels))
	require.NoError(t, unifiedRegistry.RegisterModelsWithEndpoint(ctx, endpoints[1], lmstudioModels))
	require.NoError(t, unifiedRegistry.RegisterModelsWithEndpoint(ctx, endpoints[2], openaiModels))
	require.NoError(t, unifiedRegistry.RegisterModelsWithEndpoint(ctx, endpoints[3], vllmModels))
	require.NoError(t, unifiedRegistry.RegisterModelsWithEndpoint(ctx, endpoints[4], lemonadeModels))

	// Wait for async unification (RegisterModels kicks off unifyModelsAsync in
	// a goroutine) to complete for all 5 endpoints - poll for the expected
	// unified model count rather than guessing how long that takes.
	wantModels := len(ollamaModels) + len(lmstudioModels) + len(openaiModels) + len(vllmModels) + len(lemonadeModels)
	require.Eventually(t, func() bool {
		models, err := unifiedRegistry.GetUnifiedModels(ctx)
		return err == nil && len(models) >= wantModels
	}, 5*time.Second, 10*time.Millisecond, "async unification did not complete")

	// Create converter factory
	converterFactory := converter.NewConverterFactory()

	// Create application
	app := &Application{
		modelRegistry:    unifiedRegistry,
		repository:       &mockRepository{endpoints: endpoints},
		converterFactory: converterFactory,
		logger:           styledLogger,
	}

	tests := []struct {
		name           string
		endpoint       string
		handler        http.HandlerFunc
		expectedFormat string
		checkResponse  func(t *testing.T, body []byte)
	}{
		{
			name:           "ollama_native_format",
			endpoint:       "/olla/ollama/api/tags",
			handler:        app.ollamaModelsHandler,
			expectedFormat: "ollama",
			checkResponse: func(t *testing.T, body []byte) {
				var response converter.OllamaModelResponse
				require.NoError(t, json.Unmarshal(body, &response))
				assert.Len(t, response.Models, 2)
				// Check for ollama-specific fields
				assert.NotEmpty(t, response.Models[0].Name)
				assert.NotEmpty(t, response.Models[0].Model)
			},
		},
		{
			name:           "ollama_openai_format",
			endpoint:       "/olla/ollama/v1/models",
			handler:        app.ollamaOpenAIModelsHandler,
			expectedFormat: "openai",
			checkResponse: func(t *testing.T, body []byte) {
				var response converter.OpenAIModelResponse
				require.NoError(t, json.Unmarshal(body, &response))
				assert.Equal(t, "list", response.Object)
				assert.Len(t, response.Data, 2)
				// Check for openai-specific fields
				assert.NotEmpty(t, response.Data[0].ID)
				assert.Equal(t, "model", response.Data[0].Object)
			},
		},
		{
			name:           "lmstudio_openai_format",
			endpoint:       "/olla/lmstudio/v1/models",
			handler:        app.lmstudioOpenAIModelsHandler,
			expectedFormat: "openai",
			checkResponse: func(t *testing.T, body []byte) {
				var response converter.OpenAIModelResponse
				require.NoError(t, json.Unmarshal(body, &response))
				assert.Equal(t, "list", response.Object)
				assert.Len(t, response.Data, 1)
				assert.Equal(t, "TheBloke/Llama-2-7B-Chat-GGUF", response.Data[0].ID)
			},
		},
		{
			name:           "lmstudio_enhanced_format",
			endpoint:       "/olla/lmstudio/api/v0/models",
			handler:        app.lmstudioEnhancedModelsHandler,
			expectedFormat: "lmstudio",
			checkResponse: func(t *testing.T, body []byte) {
				var response converter.LMStudioModelResponse
				require.NoError(t, json.Unmarshal(body, &response))
				assert.Equal(t, "list", response.Object)
				assert.Len(t, response.Data, 1)
				// Check for lmstudio-specific fields
				assert.NotEmpty(t, response.Data[0].ID)
				assert.NotEmpty(t, response.Data[0].Type)
			},
		},
		{
			name:           "openai_format",
			endpoint:       "/olla/openai/v1/models",
			handler:        app.openaiModelsHandler,
			expectedFormat: "openai",
			checkResponse: func(t *testing.T, body []byte) {
				var response converter.OpenAIModelResponse
				require.NoError(t, json.Unmarshal(body, &response))
				assert.Equal(t, "list", response.Object)
				// openai provider accepts all OpenAI-compatible endpoints
				// so we should get models from ollama, lmstudio, openai, vllm, and lemonade
				assert.Len(t, response.Data, 6)
				// verify all expected models are present
				modelIDs := make([]string, len(response.Data))
				for i, model := range response.Data {
					modelIDs[i] = model.ID
				}
				assert.Contains(t, modelIDs, "gpt-3.5-turbo")
				assert.Contains(t, modelIDs, "llama3:latest")
				assert.Contains(t, modelIDs, "TheBloke/Llama-2-7B-Chat-GGUF")
				assert.Contains(t, modelIDs, "meta-llama/Llama-2-7b-hf")
				assert.Contains(t, modelIDs, "Qwen2.5-0.5B-Instruct-CPU")
			},
		},
		{
			name:           "vllm_format",
			endpoint:       "/olla/vllm/v1/models",
			handler:        app.genericProviderModelsHandler("vllm", "openai"),
			expectedFormat: "openai",
			checkResponse: func(t *testing.T, body []byte) {
				var response converter.OpenAIModelResponse
				require.NoError(t, json.Unmarshal(body, &response))
				assert.Equal(t, "list", response.Object)
				assert.Len(t, response.Data, 1)
				assert.Equal(t, "meta-llama/Llama-2-7b-hf", response.Data[0].ID)
			},
		},
		{
			name:           "lemonade_native_format",
			endpoint:       "/olla/lemonade/api/v1/models",
			handler:        app.genericProviderModelsHandler("lemonade", "openai"),
			expectedFormat: "openai",
			checkResponse: func(t *testing.T, body []byte) {
				var response converter.OpenAIModelResponse
				require.NoError(t, json.Unmarshal(body, &response))
				assert.Equal(t, "list", response.Object)
				assert.Len(t, response.Data, 1)
				assert.Equal(t, "Qwen2.5-0.5B-Instruct-CPU", response.Data[0].ID)
			},
		},
		{
			name:           "lemonade_openai_format",
			endpoint:       "/olla/lemonade/v1/models",
			handler:        app.genericProviderModelsHandler("lemonade", "openai"),
			expectedFormat: "openai",
			checkResponse: func(t *testing.T, body []byte) {
				var response converter.OpenAIModelResponse
				require.NoError(t, json.Unmarshal(body, &response))
				assert.Equal(t, "list", response.Object)
				assert.Len(t, response.Data, 1)
				assert.Equal(t, "Qwen2.5-0.5B-Instruct-CPU", response.Data[0].ID)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", tt.endpoint, nil)
			w := httptest.NewRecorder()

			// Call handler
			tt.handler(w, req)

			// Check response
			assert.Equal(t, http.StatusOK, w.Code)
			assert.Equal(t, constants.ContentTypeJSON, w.Header().Get(constants.HeaderContentType))

			// Check format-specific response
			tt.checkResponse(t, w.Body.Bytes())
		})
	}
}

// TestProviderModelFiltering tests that endpoints only return models from their provider type
func TestProviderModelFiltering(t *testing.T) {
	// Setup logger
	loggerCfg := &logger.Config{Level: "error", Theme: "default"}
	log, _, _ := logger.New(loggerCfg)
	styledLogger := logger.NewPlainStyledLogger(log)

	// Create mixed endpoints
	ollamaURL1, _ := url.Parse("http://ollama1:11434")
	ollamaURL2, _ := url.Parse("http://ollama2:11434")
	lmstudioURL, _ := url.Parse("http://lmstudio:1234")

	endpoints := []*domain.Endpoint{
		{
			Name:      "ollama-1",
			URL:       ollamaURL1,
			URLString: ollamaURL1.String(),
			Status:    domain.StatusHealthy,
			Type:      "ollama",
		},
		{
			Name:      "ollama-2",
			URL:       ollamaURL2,
			URLString: ollamaURL2.String(),
			Status:    domain.StatusHealthy,
			Type:      "ollama",
		},
		{
			Name:      "lmstudio-1",
			URL:       lmstudioURL,
			URLString: lmstudioURL.String(),
			Status:    domain.StatusHealthy,
			Type:      "lm-studio",
		},
	}

	// Create registry
	unifiedRegistry := registry.NewUnifiedMemoryModelRegistry(styledLogger, nil, nil, nil)
	ctx := context.Background()

	// Register endpoints
	for _, ep := range endpoints {
		unifiedRegistry.RegisterEndpoint(ep)
	}

	// Register different models on different providers
	require.NoError(t, unifiedRegistry.RegisterModelsWithEndpoint(ctx, endpoints[0], []*domain.ModelInfo{{Name: "llama3:latest"}}))
	require.NoError(t, unifiedRegistry.RegisterModelsWithEndpoint(ctx, endpoints[1], []*domain.ModelInfo{{Name: "mistral:latest"}}))
	require.NoError(t, unifiedRegistry.RegisterModelsWithEndpoint(ctx, endpoints[2], []*domain.ModelInfo{{Name: "TheBloke/Llama-2-7B-Chat-GGUF"}}))

	// Wait for async unification to complete
	require.Eventually(t, func() bool {
		models, err := unifiedRegistry.GetUnifiedModels(ctx)
		return err == nil && len(models) >= 3
	}, 5*time.Second, 10*time.Millisecond, "async unification did not complete")

	// Create application
	app := &Application{
		modelRegistry:    unifiedRegistry,
		repository:       &mockRepository{endpoints: endpoints},
		converterFactory: converter.NewConverterFactory(),
		logger:           styledLogger,
	}

	// Test Ollama endpoint returns only Ollama models
	req := httptest.NewRequest("GET", "/olla/ollama/api/tags", nil)
	w := httptest.NewRecorder()
	app.ollamaModelsHandler(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var ollamaResponse converter.OllamaModelResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &ollamaResponse))
	assert.Len(t, ollamaResponse.Models, 2) // llama3 and mistral from both ollama endpoints

	// Test LM Studio endpoint returns only LM Studio models
	req = httptest.NewRequest("GET", "/olla/lmstudio/v1/models", nil)
	w = httptest.NewRecorder()
	app.lmstudioOpenAIModelsHandler(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var lmstudioResponse converter.OpenAIModelResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &lmstudioResponse))
	assert.Len(t, lmstudioResponse.Data, 1) // Only TheBloke model
	assert.Equal(t, "TheBloke/Llama-2-7B-Chat-GGUF", lmstudioResponse.Data[0].ID)
}

// TestModelAliasesMode tests the model_aliases_mode configuration
func TestModelAliasesMode(t *testing.T) {
	// Setup logger
	loggerCfg := &logger.Config{Level: "error", Theme: "default"}
	log, _, _ := logger.New(loggerCfg)
	styledLogger := logger.NewPlainStyledLogger(log)

	// Create test endpoints
	ollamaURL, _ := url.Parse("http://ollama:11434")
	openaiURL, _ := url.Parse("http://openai:8080")

	endpoints := []*domain.Endpoint{
		{
			Name:      "ollama-1",
			URL:       ollamaURL,
			URLString: ollamaURL.String(),
			Status:    domain.StatusHealthy,
			Type:      "ollama",
		},
		{
			Name:      "openai-1",
			URL:       openaiURL,
			URLString: openaiURL.String(),
			Status:    domain.StatusHealthy,
			Type:      "openai",
		},
	}

	// Create registry and register models
	unifiedRegistry := registry.NewUnifiedMemoryModelRegistry(styledLogger, nil, nil, nil)
	ctx := context.Background()

	for _, ep := range endpoints {
		unifiedRegistry.RegisterEndpoint(ep)
	}

	// Register models: llama3 on ollama, gpt-3.5-turbo on openai
	require.NoError(t, unifiedRegistry.RegisterModelsWithEndpoint(ctx, endpoints[0], []*domain.ModelInfo{{Name: "llama3:latest"}}))
	require.NoError(t, unifiedRegistry.RegisterModelsWithEndpoint(ctx, endpoints[1], []*domain.ModelInfo{{Name: "gpt-3.5-turbo"}}))

	// Wait for async unification to complete
	require.Eventually(t, func() bool {
		models, err := unifiedRegistry.GetUnifiedModels(ctx)
		return err == nil && len(models) >= 2
	}, 5*time.Second, 10*time.Millisecond, "async unification did not complete")

	// Create converter factory
	converterFactory := converter.NewConverterFactory()

	tests := []struct {
		name               string
		modelAliases       map[string][]string
		modelAliasesMode   string
		expectedModelCount int
		expectedIDs        []string
		notExpectedIDs     []string
	}{
		{
			name: "disabled_mode_only_targets",
			modelAliases: map[string][]string{
				"my-llama": {"llama3:latest"},
				"my-gpt":   {"gpt-3.5-turbo"},
			},
			modelAliasesMode:   constants.StickyOutcomeResultDisabled,
			expectedModelCount: 2, // only llama3:latest and gpt-3.5-turbo
			expectedIDs:        []string{"llama3:latest", "gpt-3.5-turbo"},
			notExpectedIDs:     []string{"my-llama", "my-gpt"},
		},
		{
			name: "append_mode_both_aliases_and_targets",
			modelAliases: map[string][]string{
				"my-llama": {"llama3:latest"},
				"my-gpt":   {"gpt-3.5-turbo"},
			},
			modelAliasesMode:   "append",
			expectedModelCount: 4, // 2 targets + 2 aliases
			expectedIDs:        []string{"llama3:latest", "gpt-3.5-turbo", "my-llama", "my-gpt"},
			notExpectedIDs:     []string{},
		},
		{
			name: "hidden_mode_only_aliases",
			modelAliases: map[string][]string{
				"my-llama": {"llama3:latest"},
				"my-gpt":   {"gpt-3.5-turbo"},
			},
			modelAliasesMode:   "hidden",
			expectedModelCount: 2, // only my-llama and my-gpt
			expectedIDs:        []string{"my-llama", "my-gpt"},
			notExpectedIDs:     []string{"llama3:latest", "gpt-3.5-turbo"},
		},
		{
			name: "hidden_mode_multiple_targets_per_alias",
			modelAliases: map[string][]string{
				"my-gpt": {"gpt-3.5-turbo", "gpt-4"}, // gpt-4 not registered, should still work
			},
			modelAliasesMode:   "hidden",
			expectedModelCount: 2, // my-gpt (alias) + llama3:latest (not a target of any alias)
			expectedIDs:        []string{"my-gpt", "llama3:latest"},
			notExpectedIDs:     []string{"gpt-3.5-turbo"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create application with alias config
			cfg := config.DefaultConfig()
			cfg.ModelAliases = tt.modelAliases
			cfg.ModelAliasesMode = tt.modelAliasesMode

			aliasResolver := registry.NewAliasResolver(cfg.ModelAliases, styledLogger)

			app := &Application{
				modelRegistry:    unifiedRegistry,
				repository:       &mockRepository{endpoints: endpoints},
				converterFactory: converterFactory,
				logger:           styledLogger,
				aliasResolver:    aliasResolver,
				Config:           cfg,
			}

			// Test /olla/proxy/v1/models (openai format)
			req := httptest.NewRequest("GET", "/olla/proxy/v1/models", nil)
			w := httptest.NewRecorder()
			app.openaiModelsHandler(w, req)

			assert.Equal(t, http.StatusOK, w.Code)
			assert.Equal(t, constants.ContentTypeJSON, w.Header().Get(constants.HeaderContentType))

			var response converter.OpenAIModelResponse
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
			assert.Equal(t, "list", response.Object)

			modelIDs := make([]string, len(response.Data))
			for i, model := range response.Data {
				modelIDs[i] = model.ID
			}

			// Check expected models are present
			for _, id := range tt.expectedIDs {
				assert.Contains(t, modelIDs, id, "expected model %s not found in %v", id, modelIDs)
			}

			// Check unexpected models are NOT present
			for _, id := range tt.notExpectedIDs {
				assert.NotContains(t, modelIDs, id, "unexpected model %s found in %v", id, modelIDs)
			}

			assert.Len(t, response.Data, tt.expectedModelCount, "model count mismatch for %s: got %d, want %d", tt.name, len(response.Data), tt.expectedModelCount)
		})
	}
}

// TestModelAliasesModeProviderSpecific tests that aliases work with provider-specific endpoints
func TestModelAliasesModeProviderSpecific(t *testing.T) {
	loggerCfg := &logger.Config{Level: "error", Theme: "default"}
	log, _, _ := logger.New(loggerCfg)
	styledLogger := logger.NewPlainStyledLogger(log)

	ollamaURL, _ := url.Parse("http://ollama:11434")
	lmstudioURL, _ := url.Parse("http://lmstudio:1234")

	endpoints := []*domain.Endpoint{
		{
			Name:      "ollama-1",
			URL:       ollamaURL,
			URLString: ollamaURL.String(),
			Status:    domain.StatusHealthy,
			Type:      "ollama",
		},
		{
			Name:      "lmstudio-1",
			URL:       lmstudioURL,
			URLString: lmstudioURL.String(),
			Status:    domain.StatusHealthy,
			Type:      "lm-studio",
		},
	}

	unifiedRegistry := registry.NewUnifiedMemoryModelRegistry(styledLogger, nil, nil, nil)
	ctx := context.Background()

	for _, ep := range endpoints {
		unifiedRegistry.RegisterEndpoint(ep)
	}

	// Register different models on different providers
	require.NoError(t, unifiedRegistry.RegisterModelsWithEndpoint(ctx, endpoints[0], []*domain.ModelInfo{{Name: "llama3:latest"}}))
	require.NoError(t, unifiedRegistry.RegisterModelsWithEndpoint(ctx, endpoints[1], []*domain.ModelInfo{{Name: "TheBloke/Llama-2-7B-Chat-GGUF"}}))

	require.Eventually(t, func() bool {
		models, err := unifiedRegistry.GetUnifiedModels(ctx)
		return err == nil && len(models) >= 2
	}, 5*time.Second, 10*time.Millisecond, "async unification did not complete")

	converterFactory := converter.NewConverterFactory()

	cfg := config.DefaultConfig()
	cfg.ModelAliases = map[string][]string{
		"my-llama": {"llama3:latest", "TheBloke/Llama-2-7B-Chat-GGUF"},
	}
	cfg.ModelAliasesMode = "hidden"

	aliasResolver := registry.NewAliasResolver(cfg.ModelAliases, styledLogger)

	app := &Application{
		modelRegistry:    unifiedRegistry,
		repository:       &mockRepository{endpoints: endpoints},
		converterFactory: converterFactory,
		logger:           styledLogger,
		aliasResolver:    aliasResolver,
		Config:           cfg,
	}

	// Test /olla/ollama/v1/models - should show my-llama since it has targets on ollama
	req := httptest.NewRequest("GET", "/olla/ollama/v1/models", nil)
	w := httptest.NewRecorder()
	app.ollamaOpenAIModelsHandler(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var ollamaResponse converter.OpenAIModelResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &ollamaResponse))

	ollamaIDs := make([]string, len(ollamaResponse.Data))
	for i, m := range ollamaResponse.Data {
		ollamaIDs[i] = m.ID
	}
	assert.Contains(t, ollamaIDs, "my-llama", "alias should appear in ollama endpoint")
	assert.NotContains(t, ollamaIDs, "llama3:latest", "target should be hidden in hidden mode")
	assert.NotContains(t, ollamaIDs, "TheBloke/Llama-2-7B-Chat-GGUF", "target should be hidden in hidden mode")

	// Test /olla/lmstudio/v1/models - should show my-llama since it has targets on lmstudio
	req = httptest.NewRequest("GET", "/olla/lmstudio/v1/models", nil)
	w = httptest.NewRecorder()
	app.lmstudioOpenAIModelsHandler(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var lmstudioResponse converter.OpenAIModelResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &lmstudioResponse))

	lmstudioIDs := make([]string, len(lmstudioResponse.Data))
	for i, m := range lmstudioResponse.Data {
		lmstudioIDs[i] = m.ID
	}
	assert.Contains(t, lmstudioIDs, "my-llama", "alias should appear in lmstudio endpoint")
	assert.NotContains(t, lmstudioIDs, "llama3:latest", "target should be hidden in hidden mode")
	assert.NotContains(t, lmstudioIDs, "TheBloke/Llama-2-7B-Chat-GGUF", "target should be hidden in hidden mode")
}

// TestUnifiedModelsFormatFiltering tests that format parameter filters models by provider
func TestUnifiedModelsFormatFiltering(t *testing.T) {
	// Setup logger
	loggerCfg := &logger.Config{Level: "error", Theme: "default"}
	log, _, _ := logger.New(loggerCfg)
	styledLogger := logger.NewPlainStyledLogger(log)

	// Create test endpoints with different providers
	ollamaURL, _ := url.Parse("http://ollama:11434")
	lmstudioURL, _ := url.Parse("http://lmstudio:1234")
	openaiURL, _ := url.Parse("http://openai:8080")

	endpoints := []*domain.Endpoint{
		{
			Name:      "ollama-1",
			URL:       ollamaURL,
			URLString: ollamaURL.String(),
			Status:    domain.StatusHealthy,
			Type:      "ollama",
		},
		{
			Name:      "lmstudio-1",
			URL:       lmstudioURL,
			URLString: lmstudioURL.String(),
			Status:    domain.StatusHealthy,
			Type:      "lm-studio",
		},
		{
			Name:      "openai-1",
			URL:       openaiURL,
			URLString: openaiURL.String(),
			Status:    domain.StatusHealthy,
			Type:      "openai",
		},
	}

	// Create registry
	unifiedRegistry := registry.NewUnifiedMemoryModelRegistry(styledLogger, nil, nil, nil)
	ctx := context.Background()

	// Register endpoints
	for _, ep := range endpoints {
		unifiedRegistry.RegisterEndpoint(ep)
	}

	// Register different models on different providers
	require.NoError(t, unifiedRegistry.RegisterModelsWithEndpoint(ctx, endpoints[0],
		[]*domain.ModelInfo{{Name: "llama3:latest"}, {Name: "mistral:latest"}}))
	require.NoError(t, unifiedRegistry.RegisterModelsWithEndpoint(ctx, endpoints[1],
		[]*domain.ModelInfo{{Name: "TheBloke/Llama-2-7B-Chat-GGUF"}}))
	require.NoError(t, unifiedRegistry.RegisterModelsWithEndpoint(ctx, endpoints[2],
		[]*domain.ModelInfo{{Name: "gpt-3.5-turbo"}, {Name: "gpt-4"}}))

	// Wait for async unification to complete
	require.Eventually(t, func() bool {
		models, err := unifiedRegistry.GetUnifiedModels(ctx)
		return err == nil && len(models) >= 5
	}, 5*time.Second, 10*time.Millisecond, "async unification did not complete")

	// Create application
	app := &Application{
		modelRegistry:    unifiedRegistry,
		repository:       &mockRepository{endpoints: endpoints},
		converterFactory: converter.NewConverterFactory(),
		logger:           styledLogger,
	}

	tests := []struct {
		name          string
		query         string
		expectedCount int
		expectedIDs   []string
	}{
		{
			name:          "no_filter_returns_all",
			query:         "",
			expectedCount: 5,
			expectedIDs:   []string{"llama3:latest", "mistral:latest", "TheBloke/Llama-2-7B-Chat-GGUF", "gpt-3.5-turbo", "gpt-4"},
		},
		{
			name:          "format_ollama_filters_models",
			query:         "?format=ollama",
			expectedCount: 2,
			expectedIDs:   []string{"llama3:latest", "mistral:latest"},
		},
		{
			name:          "format_lmstudio_filters_models",
			query:         "?format=lmstudio",
			expectedCount: 1,
			expectedIDs:   []string{"TheBloke/Llama-2-7B-Chat-GGUF"},
		},
		{
			name:          "format_openai_returns_all",
			query:         "?format=openai",
			expectedCount: 5,
			expectedIDs:   []string{"llama3:latest", "mistral:latest", "TheBloke/Llama-2-7B-Chat-GGUF", "gpt-3.5-turbo", "gpt-4"},
		},
		{
			name:          "format_unified_returns_all",
			query:         "?format=unified",
			expectedCount: 5,
			expectedIDs:   []string{"llama3:latest", "mistral:latest", "TheBloke/Llama-2-7B-Chat-GGUF", "gpt-3.5-turbo", "gpt-4"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/olla/models"+tt.query, nil)
			w := httptest.NewRecorder()

			app.unifiedModelsHandler(w, req)

			assert.Equal(t, http.StatusOK, w.Code)

			// Check response based on format
			if tt.query == "?format=ollama" {
				// Ollama format returns a different structure
				var ollamaResponse converter.OllamaModelResponse
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &ollamaResponse))
				assert.Len(t, ollamaResponse.Models, tt.expectedCount)
			} else if tt.query == "?format=lmstudio" {
				// LM Studio format returns a different structure
				var lmstudioResponse converter.LMStudioModelResponse
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &lmstudioResponse))
				assert.Len(t, lmstudioResponse.Data, tt.expectedCount)
			} else if tt.query == "?format=openai" {
				// OpenAI format
				var openaiResponse converter.OpenAIModelResponse
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &openaiResponse))
				assert.Len(t, openaiResponse.Data, tt.expectedCount)
			} else {
				// Unified format (default or explicit)
				var response converter.UnifiedModelResponse
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
				assert.Len(t, response.Data, tt.expectedCount)
			}
		})
	}
}
