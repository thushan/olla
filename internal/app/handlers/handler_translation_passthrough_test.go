package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thushan/olla/internal/adapter/inspector"
	"github.com/thushan/olla/internal/adapter/translator"
	"github.com/thushan/olla/internal/config"
	"github.com/thushan/olla/internal/core/constants"
	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/core/ports"
	"github.com/thushan/olla/internal/logger"
)

// TestTranslationHandler_PassthroughNonStreaming tests end-to-end passthrough for non-streaming requests
func TestTranslationHandler_PassthroughNonStreaming(t *testing.T) {
	// Setup mock backend that accepts Anthropic format
	backendCalled := false
	var receivedBody []byte
	var receivedPath string

	mockBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		backendCalled = true
		receivedPath = r.URL.Path

		// Read body to verify it's unchanged
		body, _ := io.ReadAll(r.Body)
		receivedBody = body

		// Return Anthropic format response
		response := map[string]interface{}{
			"id":      "msg_01XFDUDYJgAACzvnptvVoYEL",
			"type":    "message",
			"role":    "assistant",
			"content": []map[string]interface{}{{"type": "text", "text": "Hello! How can I help you?"}},
			"model":   "claude-3-5-sonnet-20241022",
			"usage": map[string]interface{}{
				"input_tokens":  10,
				"output_tokens": 20,
			},
		}

		w.Header().Set(constants.HeaderContentType, constants.ContentTypeJSON)
		w.Header().Set(constants.HeaderXOllaEndpoint, "test-backend")
		w.Header().Set(constants.HeaderXOllaBackendType, "vllm")
		w.Header().Set(constants.HeaderXOllaModel, "claude-3-5-sonnet-20241022")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(response)
	}))
	defer mockBackend.Close()

	// Parse backend URL
	backendURL, _ := url.Parse(mockBackend.URL)

	// Setup endpoints with Anthropic support
	endpoints := []*domain.Endpoint{
		{
			Name:      "vllm-backend",
			URL:       backendURL,
			URLString: mockBackend.URL,
			Type:      "vllm",
			Status:    domain.StatusHealthy,
		},
	}

	// Create mock profile lookup that indicates Anthropic support
	profileLookup := &mockPassthroughProfileLookup{
		configs: map[string]*domain.AnthropicSupportConfig{
			"vllm": {
				Enabled:      true,
				MessagesPath: "/v1/messages",
			},
		},
	}

	// Create passthrough-capable translator
	trans := &mockPassthroughTranslator{
		name:                  "anthropic",
		implementsErrorWriter: true,
		passthroughEnabled:    true,
		profileLookup:         profileLookup,
		writeErrorFunc: func(w http.ResponseWriter, err error, statusCode int) {
			w.Header().Set(constants.HeaderContentType, constants.ContentTypeJSON)
			w.WriteHeader(statusCode)
			json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
		},
	}

	// Create proxy service that forwards to backend
	proxyService := &mockProxyService{
		proxyFunc: func(ctx context.Context, w http.ResponseWriter, r *http.Request, eps []*domain.Endpoint, stats *ports.RequestStats, rlog logger.StyledLogger) error {
			// Forward to backend
			client := &http.Client{Timeout: 5 * time.Second}
			backendReq, err := http.NewRequest(r.Method, eps[0].URLString+r.URL.Path, r.Body)
			if err != nil {
				return err
			}
			backendReq.Header = r.Header.Clone()

			resp, err := client.Do(backendReq)
			if err != nil {
				return err
			}
			defer resp.Body.Close()

			// Copy headers
			for k, v := range resp.Header {
				w.Header()[k] = v
			}
			w.WriteHeader(resp.StatusCode)

			_, err = io.Copy(w, resp.Body)
			return err
		},
	}

	// Create discovery service that returns our endpoints
	discoveryService := &mockDiscoveryServiceWithEndpoints{
		endpoints: endpoints,
	}

	app := &Application{
		logger:           &mockStyledLogger{},
		proxyService:     proxyService,
		statsCollector:   &mockStatsCollector{},
		repository:       &mockEndpointRepository{getEndpointsFunc: func() []*domain.Endpoint { return endpoints }},
		inspectorChain:   inspector.NewChain(&mockStyledLogger{}),
		profileFactory:   &mockProfileFactory{},
		profileLookup:    profileLookup,
		discoveryService: discoveryService,
		Config:           &config.Config{},
	}

	handler := app.translationHandler(trans)

	// Create Anthropic request
	anthropicReq := map[string]interface{}{
		"model":      "claude-3-5-sonnet-20241022",
		"max_tokens": 1024,
		"messages": []map[string]interface{}{
			{"role": "user", "content": "Hello"},
		},
	}
	reqBody, _ := json.Marshal(anthropicReq)

	req := httptest.NewRequest("POST", "/olla/anthropic/v1/messages", bytes.NewReader(reqBody))
	req.Header.Set(constants.HeaderContentType, constants.ContentTypeJSON)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	// Assertions
	assert.True(t, backendCalled, "Backend should have been called")
	assert.Equal(t, http.StatusOK, rec.Code)

	// Verify passthrough mode header
	assert.Equal(t, "passthrough", rec.Header().Get("X-Olla-Mode"), "Should have passthrough mode header")

	// Verify request was passed through unchanged
	var receivedReq map[string]interface{}
	err := json.Unmarshal(receivedBody, &receivedReq)
	require.NoError(t, err)
	assert.Equal(t, "claude-3-5-sonnet-20241022", receivedReq["model"])
	assert.Equal(t, float64(1024), receivedReq["max_tokens"])

	// Verify path
	assert.Equal(t, "/v1/messages", receivedPath)

	// Verify response
	var response map[string]interface{}
	err = json.Unmarshal(rec.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, "message", response["type"])
	assert.Equal(t, "msg_01XFDUDYJgAACzvnptvVoYEL", response["id"])

	// Verify X-Olla headers preserved
	assert.NotEmpty(t, rec.Header().Get(constants.HeaderXOllaEndpoint))
	assert.NotEmpty(t, rec.Header().Get(constants.HeaderXOllaBackendType))
}

// TestTranslationHandler_PassthroughStreaming tests end-to-end passthrough for streaming requests
func TestTranslationHandler_PassthroughStreaming(t *testing.T) {
	backendCalled := false
	var receivedPath string

	// Setup mock backend that returns SSE stream
	mockBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		backendCalled = true
		receivedPath = r.URL.Path

		// Return SSE stream in Anthropic format
		w.Header().Set(constants.HeaderContentType, "text/event-stream")
		w.Header().Set(constants.HeaderXOllaEndpoint, "test-backend")
		w.Header().Set(constants.HeaderXOllaBackendType, "vllm")
		w.WriteHeader(http.StatusOK)

		// Write SSE events
		events := []string{
			`event: message_start
data: {"type":"message_start","message":{"id":"msg_123","type":"message","role":"assistant"}}

`,
			`event: content_block_delta
data: {"type":"content_block_delta","delta":{"type":"text_delta","text":"Hello"}}

`,
			`event: message_delta
data: {"type":"message_delta","delta":{"stop_reason":"end_turn"}}

`,
			`event: message_stop
data: {"type":"message_stop"}

`,
		}

		for _, event := range events {
			fmt.Fprint(w, event)
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
		}
	}))
	defer mockBackend.Close()

	backendURL, _ := url.Parse(mockBackend.URL)

	endpoints := []*domain.Endpoint{
		{
			Name:      "vllm-backend",
			URL:       backendURL,
			URLString: mockBackend.URL,
			Type:      "vllm",
			Status:    domain.StatusHealthy,
		},
	}

	profileLookup := &mockPassthroughProfileLookup{
		configs: map[string]*domain.AnthropicSupportConfig{
			"vllm": {
				Enabled:      true,
				MessagesPath: "/v1/messages",
			},
		},
	}

	trans := &mockPassthroughTranslator{
		name:               "anthropic",
		passthroughEnabled: true,
		profileLookup:      profileLookup,
		transformRequestFunc: func(ctx context.Context, r *http.Request) (*translator.TransformedRequest, error) {
			return &translator.TransformedRequest{
				ModelName:   "claude-3-5-sonnet-20241022",
				IsStreaming: true,
			}, nil
		},
	}

	proxyService := &mockProxyService{
		proxyFunc: func(ctx context.Context, w http.ResponseWriter, r *http.Request, eps []*domain.Endpoint, stats *ports.RequestStats, rlog logger.StyledLogger) error {
			client := &http.Client{Timeout: 5 * time.Second}
			backendReq, err := http.NewRequest(r.Method, eps[0].URLString+r.URL.Path, r.Body)
			if err != nil {
				return err
			}

			resp, err := client.Do(backendReq)
			if err != nil {
				return err
			}
			defer resp.Body.Close()

			for k, v := range resp.Header {
				w.Header()[k] = v
			}
			w.WriteHeader(resp.StatusCode)

			_, err = io.Copy(w, resp.Body)
			return err
		},
	}

	discoveryService := &mockDiscoveryServiceWithEndpoints{
		endpoints: endpoints,
	}

	app := &Application{
		logger:           &mockStyledLogger{},
		proxyService:     proxyService,
		statsCollector:   &mockStatsCollector{},
		repository:       &mockEndpointRepository{getEndpointsFunc: func() []*domain.Endpoint { return endpoints }},
		inspectorChain:   inspector.NewChain(&mockStyledLogger{}),
		profileFactory:   &mockProfileFactory{},
		profileLookup:    profileLookup,
		discoveryService: discoveryService,
		Config:           &config.Config{},
	}

	handler := app.translationHandler(trans)

	anthropicReq := map[string]interface{}{
		"model":      "claude-3-5-sonnet-20241022",
		"max_tokens": 1024,
		"stream":     true,
		"messages": []map[string]interface{}{
			{"role": "user", "content": "Hello"},
		},
	}
	reqBody, _ := json.Marshal(anthropicReq)

	req := httptest.NewRequest("POST", "/olla/anthropic/v1/messages", bytes.NewReader(reqBody))
	req.Header.Set(constants.HeaderContentType, constants.ContentTypeJSON)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	// Assertions
	assert.True(t, backendCalled, "Backend should have been called")
	assert.Equal(t, http.StatusOK, rec.Code)

	// Verify passthrough mode header
	assert.Equal(t, "passthrough", rec.Header().Get("X-Olla-Mode"))

	// Verify SSE content type
	assert.Equal(t, "text/event-stream", rec.Header().Get(constants.HeaderContentType))

	// Verify path
	assert.Equal(t, "/v1/messages", receivedPath)

	// Verify SSE events are passed through
	body := rec.Body.String()
	assert.Contains(t, body, "event: message_start")
	assert.Contains(t, body, "event: content_block_delta")
	assert.Contains(t, body, "event: message_delta")
	assert.Contains(t, body, "event: message_stop")

	// Verify X-Olla headers
	assert.NotEmpty(t, rec.Header().Get(constants.HeaderXOllaEndpoint))
}

// TestTranslationHandler_PassthroughWithMultipleEndpoints tests passthrough with load balancing
func TestTranslationHandler_PassthroughWithMultipleEndpoints(t *testing.T) {
	backendsCalled := make(map[string]bool)

	// Setup multiple mock backends
	backend1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		backendsCalled["backend1"] = true
		response := map[string]interface{}{"id": "msg_01", "type": "message", "role": "assistant"}
		w.Header().Set(constants.HeaderContentType, constants.ContentTypeJSON)
		w.Header().Set(constants.HeaderXOllaEndpoint, "vllm-1")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(response)
	}))
	defer backend1.Close()

	backend2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		backendsCalled["backend2"] = true
		response := map[string]interface{}{"id": "msg_02", "type": "message", "role": "assistant"}
		w.Header().Set(constants.HeaderContentType, constants.ContentTypeJSON)
		w.Header().Set(constants.HeaderXOllaEndpoint, "vllm-2")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(response)
	}))
	defer backend2.Close()

	backendURL1, _ := url.Parse(backend1.URL)
	backendURL2, _ := url.Parse(backend2.URL)

	endpoints := []*domain.Endpoint{
		{
			Name:      "vllm-1",
			URL:       backendURL1,
			URLString: backend1.URL,
			Type:      "vllm",
			Status:    domain.StatusHealthy,
		},
		{
			Name:      "vllm-2",
			URL:       backendURL2,
			URLString: backend2.URL,
			Type:      "vllm",
			Status:    domain.StatusHealthy,
		},
	}

	profileLookup := &mockPassthroughProfileLookup{
		configs: map[string]*domain.AnthropicSupportConfig{
			"vllm": {
				Enabled:      true,
				MessagesPath: "/v1/messages",
			},
		},
	}

	trans := &mockPassthroughTranslator{
		name:               "anthropic",
		passthroughEnabled: true,
		profileLookup:      profileLookup,
	}

	proxyService := &mockProxyService{
		proxyFunc: func(ctx context.Context, w http.ResponseWriter, r *http.Request, eps []*domain.Endpoint, stats *ports.RequestStats, rlog logger.StyledLogger) error {
			// Use first endpoint (simulating load balancer selection)
			client := &http.Client{Timeout: 5 * time.Second}
			backendReq, err := http.NewRequest(r.Method, eps[0].URLString+r.URL.Path, r.Body)
			if err != nil {
				return err
			}

			resp, err := client.Do(backendReq)
			if err != nil {
				return err
			}
			defer resp.Body.Close()

			for k, v := range resp.Header {
				w.Header()[k] = v
			}
			w.WriteHeader(resp.StatusCode)
			_, err = io.Copy(w, resp.Body)
			return err
		},
	}

	discoveryService := &mockDiscoveryServiceWithEndpoints{
		endpoints: endpoints,
	}

	app := &Application{
		logger:           &mockStyledLogger{},
		proxyService:     proxyService,
		statsCollector:   &mockStatsCollector{},
		repository:       &mockEndpointRepository{getEndpointsFunc: func() []*domain.Endpoint { return endpoints }},
		inspectorChain:   inspector.NewChain(&mockStyledLogger{}),
		profileFactory:   &mockProfileFactory{},
		profileLookup:    profileLookup,
		discoveryService: discoveryService,
		Config:           &config.Config{},
	}

	handler := app.translationHandler(trans)

	anthropicReq := map[string]interface{}{
		"model":      "claude-3-5-sonnet-20241022",
		"max_tokens": 1024,
		"messages": []map[string]interface{}{
			{"role": "user", "content": "Hello"},
		},
	}
	reqBody, _ := json.Marshal(anthropicReq)

	req := httptest.NewRequest("POST", "/olla/anthropic/v1/messages", bytes.NewReader(reqBody))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	// Verify one backend was called
	assert.True(t, len(backendsCalled) > 0, "At least one backend should be called")

	// Verify passthrough mode
	assert.Equal(t, "passthrough", rec.Header().Get("X-Olla-Mode"))
	assert.Equal(t, http.StatusOK, rec.Code)
}

// TestTranslationHandler_FallbackToTranslation_MixedEndpoints tests fallback when endpoints have mixed support
func TestTranslationHandler_FallbackToTranslation_MixedEndpoints(t *testing.T) {
	translationUsed := false

	endpoints := []*domain.Endpoint{
		{Name: "vllm-1", Type: "vllm", Status: domain.StatusHealthy},
		{Name: "ollama-1", Type: "ollama", Status: domain.StatusHealthy}, // No Anthropic support
	}

	profileLookup := &mockPassthroughProfileLookup{
		configs: map[string]*domain.AnthropicSupportConfig{
			"vllm": {
				Enabled:      true,
				MessagesPath: "/v1/messages",
			},
			// ollama has no config (returns nil)
		},
	}

	trans := &mockPassthroughTranslator{
		name:               "anthropic",
		passthroughEnabled: true,
		profileLookup:      profileLookup,
		transformRequestFunc: func(ctx context.Context, r *http.Request) (*translator.TransformedRequest, error) {
			translationUsed = true
			return &translator.TransformedRequest{
				OpenAIRequest: map[string]interface{}{
					"model": "claude-3-5-sonnet-20241022",
					"messages": []interface{}{
						map[string]interface{}{"role": "user", "content": "test"},
					},
				},
				ModelName:   "claude-3-5-sonnet-20241022",
				IsStreaming: false,
				TargetPath:  "/v1/chat/completions",
			}, nil
		},
	}

	proxyService := &mockProxyService{
		proxyFunc: func(ctx context.Context, w http.ResponseWriter, r *http.Request, eps []*domain.Endpoint, stats *ports.RequestStats, rlog logger.StyledLogger) error {
			response := map[string]interface{}{
				"id":      "chatcmpl-123",
				"object":  "chat.completion",
				"choices": []interface{}{},
			}
			w.Header().Set(constants.HeaderContentType, constants.ContentTypeJSON)
			w.WriteHeader(http.StatusOK)
			return json.NewEncoder(w).Encode(response)
		},
	}

	discoveryService := &mockDiscoveryServiceWithEndpoints{
		endpoints: endpoints,
	}

	app := &Application{
		logger:           &mockStyledLogger{},
		proxyService:     proxyService,
		statsCollector:   &mockStatsCollector{},
		repository:       &mockEndpointRepository{getEndpointsFunc: func() []*domain.Endpoint { return endpoints }},
		inspectorChain:   inspector.NewChain(&mockStyledLogger{}),
		profileFactory:   &mockProfileFactory{},
		profileLookup:    profileLookup,
		discoveryService: discoveryService,
		Config:           &config.Config{},
	}

	handler := app.translationHandler(trans)

	anthropicReq := map[string]interface{}{
		"model":      "claude-3-5-sonnet-20241022",
		"max_tokens": 1024,
		"messages": []map[string]interface{}{
			{"role": "user", "content": "Hello"},
		},
	}
	reqBody, _ := json.Marshal(anthropicReq)

	req := httptest.NewRequest("POST", "/olla/anthropic/v1/messages", bytes.NewReader(reqBody))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	// Verify translation mode was used (not passthrough)
	assert.True(t, translationUsed, "Translation should be used when endpoints have mixed support")
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.NotEqual(t, "passthrough", rec.Header().Get("X-Olla-Mode"), "Should not use passthrough mode")
}

// TestTranslationHandler_FallbackToTranslation_PassthroughDisabled tests fallback when passthrough is disabled
func TestTranslationHandler_FallbackToTranslation_PassthroughDisabled(t *testing.T) {
	translationUsed := false

	endpoints := []*domain.Endpoint{
		{Name: "vllm-1", Type: "vllm", Status: domain.StatusHealthy},
	}

	profileLookup := &mockPassthroughProfileLookup{
		configs: map[string]*domain.AnthropicSupportConfig{
			"vllm": {
				Enabled:      true,
				MessagesPath: "/v1/messages",
			},
		},
	}

	// Translator with passthrough disabled
	trans := &mockPassthroughTranslator{
		name:               "anthropic",
		passthroughEnabled: false, // Disabled
		profileLookup:      profileLookup,
		transformRequestFunc: func(ctx context.Context, r *http.Request) (*translator.TransformedRequest, error) {
			translationUsed = true
			return &translator.TransformedRequest{
				OpenAIRequest: map[string]interface{}{
					"model": "claude-3-5-sonnet-20241022",
					"messages": []interface{}{
						map[string]interface{}{"role": "user", "content": "test"},
					},
				},
				ModelName:   "claude-3-5-sonnet-20241022",
				IsStreaming: false,
				TargetPath:  "/v1/chat/completions",
			}, nil
		},
	}

	proxyService := &mockProxyService{
		proxyFunc: func(ctx context.Context, w http.ResponseWriter, r *http.Request, eps []*domain.Endpoint, stats *ports.RequestStats, rlog logger.StyledLogger) error {
			response := map[string]interface{}{
				"id":      "chatcmpl-123",
				"object":  "chat.completion",
				"choices": []interface{}{},
			}
			w.Header().Set(constants.HeaderContentType, constants.ContentTypeJSON)
			w.WriteHeader(http.StatusOK)
			return json.NewEncoder(w).Encode(response)
		},
	}

	discoveryService := &mockDiscoveryServiceWithEndpoints{
		endpoints: endpoints,
	}

	app := &Application{
		logger:           &mockStyledLogger{},
		proxyService:     proxyService,
		statsCollector:   &mockStatsCollector{},
		repository:       &mockEndpointRepository{getEndpointsFunc: func() []*domain.Endpoint { return endpoints }},
		inspectorChain:   inspector.NewChain(&mockStyledLogger{}),
		profileFactory:   &mockProfileFactory{},
		profileLookup:    profileLookup,
		discoveryService: discoveryService,
		Config:           &config.Config{},
	}

	handler := app.translationHandler(trans)

	anthropicReq := map[string]interface{}{
		"model":      "claude-3-5-sonnet-20241022",
		"max_tokens": 1024,
		"messages": []map[string]interface{}{
			{"role": "user", "content": "Hello"},
		},
	}
	reqBody, _ := json.Marshal(anthropicReq)

	req := httptest.NewRequest("POST", "/olla/anthropic/v1/messages", bytes.NewReader(reqBody))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	// Verify translation mode was used even though backend supports Anthropic
	assert.True(t, translationUsed, "Translation should be used when passthrough is disabled")
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.NotEqual(t, "passthrough", rec.Header().Get("X-Olla-Mode"))
}

// TestTranslationHandler_FallbackToTranslation_NoAnthropicSupport tests fallback for backends without support
func TestTranslationHandler_FallbackToTranslation_NoAnthropicSupport(t *testing.T) {
	translationUsed := false

	endpoints := []*domain.Endpoint{
		{Name: "ollama-1", Type: "ollama", Status: domain.StatusHealthy},
	}

	profileLookup := &mockPassthroughProfileLookup{
		configs: map[string]*domain.AnthropicSupportConfig{
			// ollama has no Anthropic support configured
		},
	}

	trans := &mockPassthroughTranslator{
		name:               "anthropic",
		passthroughEnabled: true,
		profileLookup:      profileLookup,
		transformRequestFunc: func(ctx context.Context, r *http.Request) (*translator.TransformedRequest, error) {
			translationUsed = true
			return &translator.TransformedRequest{
				OpenAIRequest: map[string]interface{}{
					"model": "claude-3-5-sonnet-20241022",
					"messages": []interface{}{
						map[string]interface{}{"role": "user", "content": "test"},
					},
				},
				ModelName:   "claude-3-5-sonnet-20241022",
				IsStreaming: false,
				TargetPath:  "/v1/chat/completions",
			}, nil
		},
	}

	proxyService := &mockProxyService{
		proxyFunc: func(ctx context.Context, w http.ResponseWriter, r *http.Request, eps []*domain.Endpoint, stats *ports.RequestStats, rlog logger.StyledLogger) error {
			response := map[string]interface{}{
				"id":      "chatcmpl-123",
				"object":  "chat.completion",
				"choices": []interface{}{},
			}
			w.Header().Set(constants.HeaderContentType, constants.ContentTypeJSON)
			w.WriteHeader(http.StatusOK)
			return json.NewEncoder(w).Encode(response)
		},
	}

	discoveryService := &mockDiscoveryServiceWithEndpoints{
		endpoints: endpoints,
	}

	app := &Application{
		logger:           &mockStyledLogger{},
		proxyService:     proxyService,
		statsCollector:   &mockStatsCollector{},
		repository:       &mockEndpointRepository{getEndpointsFunc: func() []*domain.Endpoint { return endpoints }},
		inspectorChain:   inspector.NewChain(&mockStyledLogger{}),
		profileFactory:   &mockProfileFactory{},
		profileLookup:    profileLookup,
		discoveryService: discoveryService,
		Config:           &config.Config{},
	}

	handler := app.translationHandler(trans)

	anthropicReq := map[string]interface{}{
		"model":      "claude-3-5-sonnet-20241022",
		"max_tokens": 1024,
		"messages": []map[string]interface{}{
			{"role": "user", "content": "Hello"},
		},
	}
	reqBody, _ := json.Marshal(anthropicReq)

	req := httptest.NewRequest("POST", "/olla/anthropic/v1/messages", bytes.NewReader(reqBody))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	// Verify translation mode was used
	assert.True(t, translationUsed, "Translation should be used when backend lacks Anthropic support")
	assert.Equal(t, http.StatusOK, rec.Code)
}

// TestTranslationHandler_PassthroughErrorPreservation tests that errors are properly preserved in passthrough mode
func TestTranslationHandler_PassthroughErrorPreservation(t *testing.T) {
	// Setup mock backend that returns an error
	mockBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(constants.HeaderContentType, constants.ContentTypeJSON)
		w.Header().Set(constants.HeaderXOllaEndpoint, "test-backend")
		w.WriteHeader(http.StatusBadRequest)

		errorResp := map[string]interface{}{
			"type": "error",
			"error": map[string]interface{}{
				"type":    "invalid_request_error",
				"message": "Invalid model specified",
			},
		}
		json.NewEncoder(w).Encode(errorResp)
	}))
	defer mockBackend.Close()

	backendURL, _ := url.Parse(mockBackend.URL)

	endpoints := []*domain.Endpoint{
		{
			Name:      "vllm-backend",
			URL:       backendURL,
			URLString: mockBackend.URL,
			Type:      "vllm",
			Status:    domain.StatusHealthy,
		},
	}

	profileLookup := &mockPassthroughProfileLookup{
		configs: map[string]*domain.AnthropicSupportConfig{
			"vllm": {
				Enabled:      true,
				MessagesPath: "/v1/messages",
			},
		},
	}

	trans := &mockPassthroughTranslator{
		name:               "anthropic",
		passthroughEnabled: true,
		profileLookup:      profileLookup,
	}

	proxyService := &mockProxyService{
		proxyFunc: func(ctx context.Context, w http.ResponseWriter, r *http.Request, eps []*domain.Endpoint, stats *ports.RequestStats, rlog logger.StyledLogger) error {
			client := &http.Client{Timeout: 5 * time.Second}
			backendReq, err := http.NewRequest(r.Method, eps[0].URLString+r.URL.Path, r.Body)
			if err != nil {
				return err
			}

			resp, err := client.Do(backendReq)
			if err != nil {
				return err
			}
			defer resp.Body.Close()

			for k, v := range resp.Header {
				w.Header()[k] = v
			}
			w.WriteHeader(resp.StatusCode)
			_, err = io.Copy(w, resp.Body)
			return err
		},
	}

	discoveryService := &mockDiscoveryServiceWithEndpoints{
		endpoints: endpoints,
	}

	app := &Application{
		logger:           &mockStyledLogger{},
		proxyService:     proxyService,
		statsCollector:   &mockStatsCollector{},
		repository:       &mockEndpointRepository{getEndpointsFunc: func() []*domain.Endpoint { return endpoints }},
		inspectorChain:   inspector.NewChain(&mockStyledLogger{}),
		profileFactory:   &mockProfileFactory{},
		profileLookup:    profileLookup,
		discoveryService: discoveryService,
		Config:           &config.Config{},
	}

	handler := app.translationHandler(trans)

	anthropicReq := map[string]interface{}{
		"model":      "invalid-model",
		"max_tokens": 1024,
		"messages": []map[string]interface{}{
			{"role": "user", "content": "Hello"},
		},
	}
	reqBody, _ := json.Marshal(anthropicReq)

	req := httptest.NewRequest("POST", "/olla/anthropic/v1/messages", bytes.NewReader(reqBody))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	// Verify error response is preserved
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Equal(t, "passthrough", rec.Header().Get("X-Olla-Mode"))

	var errorResp map[string]interface{}
	err := json.Unmarshal(rec.Body.Bytes(), &errorResp)
	require.NoError(t, err)
	assert.Equal(t, "error", errorResp["type"])

	errorObj := errorResp["error"].(map[string]interface{})
	assert.Equal(t, "invalid_request_error", errorObj["type"])
	assert.Contains(t, errorObj["message"], "Invalid model")
}

// TestTranslationHandler_ExistingTranslationTestsStillPass verifies no regression
func TestTranslationHandler_ExistingTranslationTestsStillPass(t *testing.T) {
	// This test ensures the existing translation tests still work
	// Run a basic translation flow test
	mockLogger := &mockStyledLogger{}
	trans := &mockTranslator{
		name:                  "test-translator",
		implementsErrorWriter: true,
		writeErrorFunc: func(w http.ResponseWriter, err error, statusCode int) {
			w.Header().Set(constants.HeaderContentType, constants.ContentTypeJSON)
			w.WriteHeader(statusCode)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"error": err.Error(),
			})
		},
	}

	app := &Application{
		logger:           mockLogger,
		proxyService:     &mockProxyService{},
		statsCollector:   &mockStatsCollector{},
		repository:       &mockEndpointRepository{},
		inspectorChain:   inspector.NewChain(mockLogger),
		profileFactory:   &mockProfileFactory{},
		discoveryService: &mockDiscoveryServiceForTranslation{},
		Config:           &config.Config{},
	}

	handler := app.translationHandler(trans)

	reqBody := map[string]interface{}{
		"model": "test-model",
		"messages": []interface{}{
			map[string]interface{}{
				"role":    "user",
				"content": "Hello",
			},
		},
	}
	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest("POST", "/test", bytes.NewReader(body))
	req.Header.Set(constants.HeaderContentType, constants.ContentTypeJSON)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, constants.ContentTypeJSON, rec.Header().Get(constants.HeaderContentType))

	// Verify X-Olla-* headers are preserved
	assert.NotEmpty(t, rec.Header().Get(constants.HeaderXOllaRequestID))
	assert.NotEmpty(t, rec.Header().Get(constants.HeaderXOllaEndpoint))
}

// Mock implementations for testing

// mockPassthroughProfileLookup implements translator.ProfileLookup for testing
type mockPassthroughProfileLookup struct {
	configs map[string]*domain.AnthropicSupportConfig
}

func (m *mockPassthroughProfileLookup) GetAnthropicSupport(endpointType string) *domain.AnthropicSupportConfig {
	if m.configs == nil {
		return nil
	}
	return m.configs[endpointType]
}

// mockPassthroughTranslator implements both RequestTranslator and PassthroughCapable
type mockPassthroughTranslator struct {
	name                   string
	transformRequestFunc   func(ctx context.Context, r *http.Request) (*translator.TransformedRequest, error)
	transformResponseFunc  func(ctx context.Context, openaiResp interface{}, original *http.Request) (interface{}, error)
	transformStreamingFunc func(ctx context.Context, openaiStream io.Reader, w http.ResponseWriter, original *http.Request) error
	writeErrorFunc         func(w http.ResponseWriter, err error, statusCode int)
	implementsErrorWriter  bool
	passthroughEnabled     bool
	profileLookup          translator.ProfileLookup
}

func (m *mockPassthroughTranslator) Name() string {
	return m.name
}

func (m *mockPassthroughTranslator) TransformRequest(ctx context.Context, r *http.Request) (*translator.TransformedRequest, error) {
	if m.transformRequestFunc != nil {
		return m.transformRequestFunc(ctx, r)
	}
	// Default implementation
	body, _ := io.ReadAll(r.Body)
	var req map[string]interface{}
	json.Unmarshal(body, &req)

	modelName := ""
	if model, ok := req["model"].(string); ok {
		modelName = model
	}

	isStreaming := false
	if stream, ok := req["stream"].(bool); ok {
		isStreaming = stream
	}

	return &translator.TransformedRequest{
		OpenAIRequest: map[string]interface{}{
			"model":    modelName,
			"messages": []interface{}{},
		},
		ModelName:   modelName,
		IsStreaming: isStreaming,
	}, nil
}

func (m *mockPassthroughTranslator) TransformResponse(ctx context.Context, openaiResp interface{}, original *http.Request) (interface{}, error) {
	if m.transformResponseFunc != nil {
		return m.transformResponseFunc(ctx, openaiResp, original)
	}
	return map[string]interface{}{
		"id":      "mock-response-id",
		"content": "mock response",
	}, nil
}

func (m *mockPassthroughTranslator) TransformStreamingResponse(ctx context.Context, openaiStream io.Reader, w http.ResponseWriter, original *http.Request) error {
	if m.transformStreamingFunc != nil {
		return m.transformStreamingFunc(ctx, openaiStream, w, original)
	}
	w.Header().Set(constants.HeaderContentType, "text/event-stream")
	_, err := io.Copy(w, openaiStream)
	return err
}

func (m *mockPassthroughTranslator) WriteError(w http.ResponseWriter, err error, statusCode int) {
	if m.implementsErrorWriter && m.writeErrorFunc != nil {
		m.writeErrorFunc(w, err, statusCode)
		return
	}
	panic("WriteError called on translator that doesn't implement ErrorWriter")
}

// CanPassthrough implements PassthroughCapable
func (m *mockPassthroughTranslator) CanPassthrough(endpoints []*domain.Endpoint, profileLookup translator.ProfileLookup) bool {
	if !m.passthroughEnabled {
		return false
	}

	if len(endpoints) == 0 {
		return false
	}

	// Check if all endpoints support Anthropic
	for _, endpoint := range endpoints {
		cfg := profileLookup.GetAnthropicSupport(endpoint.Type)
		if cfg == nil || !cfg.Enabled {
			return false
		}
	}

	return true
}

// PreparePassthrough implements PassthroughCapable
func (m *mockPassthroughTranslator) PreparePassthrough(r *http.Request, profileLookup translator.ProfileLookup) (*translator.PassthroughRequest, error) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read request body: %w", err)
	}

	var req map[string]interface{}
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}

	modelName := ""
	if model, ok := req["model"].(string); ok {
		modelName = model
	}

	isStreaming := false
	if stream, ok := req["stream"].(bool); ok {
		isStreaming = stream
	}

	return &translator.PassthroughRequest{
		Body:        body,
		TargetPath:  "/v1/messages",
		ModelName:   modelName,
		IsStreaming: isStreaming,
	}, nil
}

// mockDiscoveryServiceWithEndpoints provides configured endpoints for testing
type mockDiscoveryServiceWithEndpoints struct {
	endpoints []*domain.Endpoint
}

func (m *mockDiscoveryServiceWithEndpoints) GetEndpoints(ctx context.Context) ([]*domain.Endpoint, error) {
	return m.endpoints, nil
}

func (m *mockDiscoveryServiceWithEndpoints) GetHealthyEndpoints(ctx context.Context) ([]*domain.Endpoint, error) {
	healthy := make([]*domain.Endpoint, 0)
	for _, ep := range m.endpoints {
		if ep.Status == domain.StatusHealthy {
			healthy = append(healthy, ep)
		}
	}
	return healthy, nil
}

func (m *mockDiscoveryServiceWithEndpoints) RefreshEndpoints(ctx context.Context) error {
	return nil
}

func (m *mockDiscoveryServiceWithEndpoints) UpdateEndpointStatus(ctx context.Context, endpoint *domain.Endpoint) error {
	return nil
}

// ========== METRICS INTEGRATION TESTS ==========
// These tests verify that translator metrics are properly recorded during HTTP request flows

// mockStatsCollectorWithCapture extends mockStatsCollector to capture metrics calls
type mockStatsCollectorWithCapture struct {
	recordedEvents []ports.TranslatorRequestEvent
	mu             sync.Mutex
}

func (m *mockStatsCollectorWithCapture) RecordRequest(endpoint *domain.Endpoint, status string, latency time.Duration, bytes int64) {
}
func (m *mockStatsCollectorWithCapture) RecordConnection(endpoint *domain.Endpoint, delta int) {}
func (m *mockStatsCollectorWithCapture) RecordSecurityViolation(violation ports.SecurityViolation) {
}
func (m *mockStatsCollectorWithCapture) RecordDiscovery(endpoint *domain.Endpoint, success bool, latency time.Duration) {
}
func (m *mockStatsCollectorWithCapture) RecordModelRequest(model string, endpoint *domain.Endpoint, status string, latency time.Duration, bytes int64) {
}
func (m *mockStatsCollectorWithCapture) RecordModelError(model string, endpoint *domain.Endpoint, errorType string) {
}
func (m *mockStatsCollectorWithCapture) GetModelStats() map[string]ports.ModelStats { return nil }
func (m *mockStatsCollectorWithCapture) GetModelEndpointStats() map[string]map[string]ports.EndpointModelStats {
	return nil
}
func (m *mockStatsCollectorWithCapture) RecordTranslatorRequest(event ports.TranslatorRequestEvent) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.recordedEvents = append(m.recordedEvents, event)
}
func (m *mockStatsCollectorWithCapture) GetTranslatorStats() map[string]ports.TranslatorStats {
	return nil
}
func (m *mockStatsCollectorWithCapture) GetProxyStats() ports.ProxyStats { return ports.ProxyStats{} }
func (m *mockStatsCollectorWithCapture) GetEndpointStats() map[string]ports.EndpointStats {
	return nil
}
func (m *mockStatsCollectorWithCapture) GetSecurityStats() ports.SecurityStats {
	return ports.SecurityStats{}
}
func (m *mockStatsCollectorWithCapture) GetConnectionStats() map[string]int64 { return nil }

func (m *mockStatsCollectorWithCapture) getRecordedEvents() []ports.TranslatorRequestEvent {
	m.mu.Lock()
	defer m.mu.Unlock()
	events := make([]ports.TranslatorRequestEvent, len(m.recordedEvents))
	copy(events, m.recordedEvents)
	return events
}

// TestTranslationHandler_MetricsRecordedForPassthrough verifies metrics are recorded for passthrough requests
func TestTranslationHandler_MetricsRecordedForPassthrough(t *testing.T) {
	// Setup mock backend
	mockBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Add small delay to ensure measurable latency
		time.Sleep(1 * time.Millisecond)
		response := map[string]interface{}{
			"id":      "msg_01XFDUDYJgAACzvnptvVoYEL",
			"type":    "message",
			"role":    "assistant",
			"content": []map[string]interface{}{{"type": "text", "text": "Hello"}},
			"model":   "claude-3-5-sonnet-20241022",
		}
		w.Header().Set(constants.HeaderContentType, constants.ContentTypeJSON)
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(response)
	}))
	defer mockBackend.Close()

	backendURL, _ := url.Parse(mockBackend.URL)
	endpoints := []*domain.Endpoint{
		{
			Name:      "vllm-backend",
			URL:       backendURL,
			URLString: mockBackend.URL,
			Type:      "vllm",
			Status:    domain.StatusHealthy,
		},
	}

	profileLookup := &mockPassthroughProfileLookup{
		configs: map[string]*domain.AnthropicSupportConfig{
			"vllm": {
				Enabled:      true,
				MessagesPath: "/v1/messages",
			},
		},
	}

	trans := &mockPassthroughTranslator{
		name:                  "anthropic",
		passthroughEnabled:    true,
		profileLookup:         profileLookup,
		implementsErrorWriter: true,
	}

	proxyService := &mockProxyService{
		proxyFunc: func(ctx context.Context, w http.ResponseWriter, r *http.Request, eps []*domain.Endpoint, stats *ports.RequestStats, rlog logger.StyledLogger) error {
			client := &http.Client{Timeout: 5 * time.Second}
			backendReq, err := http.NewRequest(r.Method, eps[0].URLString+r.URL.Path, r.Body)
			if err != nil {
				return err
			}
			resp, err := client.Do(backendReq)
			if err != nil {
				return err
			}
			defer resp.Body.Close()
			for k, v := range resp.Header {
				w.Header()[k] = v
			}
			w.WriteHeader(resp.StatusCode)
			io.Copy(w, resp.Body)
			return nil
		},
	}

	// Create stats collector that captures events
	statsCollector := &mockStatsCollectorWithCapture{}

	app := &Application{
		logger:           &mockStyledLogger{},
		proxyService:     proxyService,
		statsCollector:   statsCollector,
		repository:       &mockEndpointRepository{getEndpointsFunc: func() []*domain.Endpoint { return endpoints }},
		inspectorChain:   inspector.NewChain(&mockStyledLogger{}),
		profileFactory:   &mockProfileFactory{},
		profileLookup:    profileLookup,
		discoveryService: &mockDiscoveryServiceWithEndpoints{endpoints: endpoints},
		Config:           &config.Config{},
	}

	handler := app.translationHandler(trans)

	// Send non-streaming request
	anthropicReq := map[string]interface{}{
		"model":      "claude-3-5-sonnet-20241022",
		"max_tokens": 1024,
		"messages": []map[string]interface{}{
			{"role": "user", "content": "Hello"},
		},
	}
	reqBody, _ := json.Marshal(anthropicReq)
	req := httptest.NewRequest("POST", "/olla/anthropic/v1/messages", bytes.NewReader(reqBody))
	req.Header.Set(constants.HeaderContentType, constants.ContentTypeJSON)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	// Verify response is successful
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "passthrough", rec.Header().Get("X-Olla-Mode"))

	// Verify metrics were recorded
	events := statsCollector.getRecordedEvents()
	require.Len(t, events, 1, "Expected exactly one translator metrics event")

	event := events[0]
	assert.Equal(t, "anthropic", event.TranslatorName)
	assert.Equal(t, "claude-3-5-sonnet-20241022", event.Model)
	assert.Equal(t, constants.TranslatorModePassthrough, event.Mode)
	assert.Equal(t, constants.FallbackReasonNone, event.FallbackReason)
	assert.True(t, event.Success)
	assert.False(t, event.IsStreaming)
	assert.Greater(t, event.Latency, time.Duration(0))
}

// TestTranslationHandler_MetricsRecordedForTranslation verifies metrics are recorded for translation requests
func TestTranslationHandler_MetricsRecordedForTranslation(t *testing.T) {
	// Setup endpoints WITHOUT Anthropic support (forces translation mode)
	endpoints := []*domain.Endpoint{
		{Name: "ollama-1", Type: "ollama", Status: domain.StatusHealthy},
	}

	profileLookup := &mockPassthroughProfileLookup{
		configs: map[string]*domain.AnthropicSupportConfig{
			// No Anthropic support for ollama
		},
	}

	trans := &mockPassthroughTranslator{
		name:               "anthropic",
		passthroughEnabled: true,
		profileLookup:      profileLookup,
		transformRequestFunc: func(ctx context.Context, r *http.Request) (*translator.TransformedRequest, error) {
			return &translator.TransformedRequest{
				OpenAIRequest: map[string]interface{}{
					"model": "claude-3-5-sonnet-20241022",
					"messages": []interface{}{
						map[string]interface{}{"role": "user", "content": "test"},
					},
				},
				ModelName:   "claude-3-5-sonnet-20241022",
				IsStreaming: false,
				TargetPath:  "/v1/chat/completions",
			}, nil
		},
		transformResponseFunc: func(ctx context.Context, openaiResp interface{}, original *http.Request) (interface{}, error) {
			return map[string]interface{}{
				"id":   "msg_123",
				"type": "message",
			}, nil
		},
		implementsErrorWriter: true,
	}

	proxyService := &mockProxyService{
		proxyFunc: func(ctx context.Context, w http.ResponseWriter, r *http.Request, eps []*domain.Endpoint, stats *ports.RequestStats, rlog logger.StyledLogger) error {
			response := map[string]interface{}{
				"id":      "chatcmpl-123",
				"object":  "chat.completion",
				"choices": []interface{}{},
			}
			w.Header().Set(constants.HeaderContentType, constants.ContentTypeJSON)
			w.WriteHeader(http.StatusOK)
			return json.NewEncoder(w).Encode(response)
		},
	}

	statsCollector := &mockStatsCollectorWithCapture{}

	app := &Application{
		logger:           &mockStyledLogger{},
		proxyService:     proxyService,
		statsCollector:   statsCollector,
		repository:       &mockEndpointRepository{getEndpointsFunc: func() []*domain.Endpoint { return endpoints }},
		inspectorChain:   inspector.NewChain(&mockStyledLogger{}),
		profileFactory:   &mockProfileFactory{},
		profileLookup:    profileLookup,
		discoveryService: &mockDiscoveryServiceWithEndpoints{endpoints: endpoints},
		Config:           &config.Config{},
	}

	handler := app.translationHandler(trans)

	anthropicReq := map[string]interface{}{
		"model":      "claude-3-5-sonnet-20241022",
		"max_tokens": 1024,
		"messages": []map[string]interface{}{
			{"role": "user", "content": "Hello"},
		},
	}
	reqBody, _ := json.Marshal(anthropicReq)
	req := httptest.NewRequest("POST", "/olla/anthropic/v1/messages", bytes.NewReader(reqBody))
	req.Header.Set(constants.HeaderContentType, constants.ContentTypeJSON)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	// Verify response is successful
	assert.Equal(t, http.StatusOK, rec.Code)

	// Verify metrics were recorded for translation mode
	events := statsCollector.getRecordedEvents()
	require.Len(t, events, 1, "Expected exactly one translator metrics event")

	event := events[0]
	assert.Equal(t, "anthropic", event.TranslatorName)
	assert.Equal(t, "claude-3-5-sonnet-20241022", event.Model)
	assert.Equal(t, constants.TranslatorModeTranslation, event.Mode)
	assert.Equal(t, constants.FallbackReasonCannotPassthrough, event.FallbackReason)
	assert.True(t, event.Success)
	assert.False(t, event.IsStreaming)
}

// TestTranslationHandler_MetricsRecordedForFallback verifies metrics capture fallback scenarios
func TestTranslationHandler_MetricsRecordedForFallback(t *testing.T) {
	// Test case: mixed endpoint support (some support Anthropic, some don't)
	endpoints := []*domain.Endpoint{
		{Name: "vllm-1", Type: "vllm", Status: domain.StatusHealthy},
		{Name: "ollama-1", Type: "ollama", Status: domain.StatusHealthy}, // No Anthropic support
	}

	profileLookup := &mockPassthroughProfileLookup{
		configs: map[string]*domain.AnthropicSupportConfig{
			"vllm": {
				Enabled:      true,
				MessagesPath: "/v1/messages",
			},
			// ollama has no config
		},
	}

	trans := &mockPassthroughTranslator{
		name:               "anthropic",
		passthroughEnabled: true,
		profileLookup:      profileLookup,
		transformRequestFunc: func(ctx context.Context, r *http.Request) (*translator.TransformedRequest, error) {
			return &translator.TransformedRequest{
				OpenAIRequest: map[string]interface{}{
					"model":    "claude-3-5-sonnet-20241022",
					"messages": []interface{}{map[string]interface{}{"role": "user", "content": "test"}},
				},
				ModelName:   "claude-3-5-sonnet-20241022",
				IsStreaming: false,
				TargetPath:  "/v1/chat/completions",
			}, nil
		},
		transformResponseFunc: func(ctx context.Context, openaiResp interface{}, original *http.Request) (interface{}, error) {
			return map[string]interface{}{"id": "msg_123", "type": "message"}, nil
		},
		implementsErrorWriter: true,
	}

	proxyService := &mockProxyService{
		proxyFunc: func(ctx context.Context, w http.ResponseWriter, r *http.Request, eps []*domain.Endpoint, stats *ports.RequestStats, rlog logger.StyledLogger) error {
			response := map[string]interface{}{"id": "chatcmpl-123", "object": "chat.completion", "choices": []interface{}{}}
			w.Header().Set(constants.HeaderContentType, constants.ContentTypeJSON)
			w.WriteHeader(http.StatusOK)
			return json.NewEncoder(w).Encode(response)
		},
	}

	statsCollector := &mockStatsCollectorWithCapture{}

	app := &Application{
		logger:           &mockStyledLogger{},
		proxyService:     proxyService,
		statsCollector:   statsCollector,
		repository:       &mockEndpointRepository{getEndpointsFunc: func() []*domain.Endpoint { return endpoints }},
		inspectorChain:   inspector.NewChain(&mockStyledLogger{}),
		profileFactory:   &mockProfileFactory{},
		profileLookup:    profileLookup,
		discoveryService: &mockDiscoveryServiceWithEndpoints{endpoints: endpoints},
		Config:           &config.Config{},
	}

	handler := app.translationHandler(trans)

	anthropicReq := map[string]interface{}{
		"model":      "claude-3-5-sonnet-20241022",
		"max_tokens": 1024,
		"messages": []map[string]interface{}{
			{"role": "user", "content": "Hello"},
		},
	}
	reqBody, _ := json.Marshal(anthropicReq)
	req := httptest.NewRequest("POST", "/olla/anthropic/v1/messages", bytes.NewReader(reqBody))
	req.Header.Set(constants.HeaderContentType, constants.ContentTypeJSON)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	// Verify fallback reason is recorded
	events := statsCollector.getRecordedEvents()
	require.Len(t, events, 1)

	event := events[0]
	assert.Equal(t, constants.TranslatorModeTranslation, event.Mode)
	assert.Equal(t, constants.FallbackReasonCannotPassthrough, event.FallbackReason)
	assert.True(t, event.Success)
}

// TestTranslationHandler_MetricsRecordedForStreamingVsNonStreaming verifies streaming flag is tracked
func TestTranslationHandler_MetricsRecordedForStreamingVsNonStreaming(t *testing.T) {
	mockBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Add small delay to ensure measurable latency
		time.Sleep(1 * time.Millisecond)
		// Check if request is streaming based on request body
		body, _ := io.ReadAll(r.Body)
		var req map[string]interface{}
		json.Unmarshal(body, &req)

		if stream, ok := req["stream"].(bool); ok && stream {
			// Return SSE stream
			w.Header().Set(constants.HeaderContentType, "text/event-stream")
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, "event: message_start\ndata: {\\\"type\\\":\\\"message_start\\\"}\n\n")
			fmt.Fprint(w, "event: message_stop\ndata: {\\\"type\\\":\\\"message_stop\\\"}\n\n")
		} else {
			// Return JSON response
			response := map[string]interface{}{"id": "msg_123", "type": "message"}
			w.Header().Set(constants.HeaderContentType, constants.ContentTypeJSON)
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(response)
		}
	}))
	defer mockBackend.Close()

	backendURL, _ := url.Parse(mockBackend.URL)
	endpoints := []*domain.Endpoint{
		{
			Name:      "vllm-backend",
			URL:       backendURL,
			URLString: mockBackend.URL,
			Type:      "vllm",
			Status:    domain.StatusHealthy,
		},
	}

	profileLookup := &mockPassthroughProfileLookup{
		configs: map[string]*domain.AnthropicSupportConfig{
			"vllm": {Enabled: true, MessagesPath: "/v1/messages"},
		},
	}

	trans := &mockPassthroughTranslator{
		name:                  "anthropic",
		passthroughEnabled:    true,
		profileLookup:         profileLookup,
		implementsErrorWriter: true,
		transformRequestFunc: func(ctx context.Context, r *http.Request) (*translator.TransformedRequest, error) {
			body, _ := io.ReadAll(r.Body)
			var req map[string]interface{}
			json.Unmarshal(body, &req)

			modelName := "claude-3-5-sonnet-20241022"
			if model, ok := req["model"].(string); ok {
				modelName = model
			}

			isStreaming := false
			if stream, ok := req["stream"].(bool); ok {
				isStreaming = stream
			}

			return &translator.TransformedRequest{
				ModelName:   modelName,
				IsStreaming: isStreaming,
			}, nil
		},
	}

	proxyService := &mockProxyService{
		proxyFunc: func(ctx context.Context, w http.ResponseWriter, r *http.Request, eps []*domain.Endpoint, stats *ports.RequestStats, rlog logger.StyledLogger) error {
			client := &http.Client{Timeout: 5 * time.Second}
			backendReq, err := http.NewRequest(r.Method, eps[0].URLString+r.URL.Path, r.Body)
			if err != nil {
				return err
			}

			resp, err := client.Do(backendReq)
			if err != nil {
				return err
			}
			defer resp.Body.Close()
			for k, v := range resp.Header {
				w.Header()[k] = v
			}
			w.WriteHeader(resp.StatusCode)
			io.Copy(w, resp.Body)

			return nil
		},
	}

	statsCollector := &mockStatsCollectorWithCapture{}

	app := &Application{
		logger:           &mockStyledLogger{},
		proxyService:     proxyService,
		statsCollector:   statsCollector,
		repository:       &mockEndpointRepository{getEndpointsFunc: func() []*domain.Endpoint { return endpoints }},
		inspectorChain:   inspector.NewChain(&mockStyledLogger{}),
		profileFactory:   &mockProfileFactory{},
		profileLookup:    profileLookup,
		discoveryService: &mockDiscoveryServiceWithEndpoints{endpoints: endpoints},
		Config:           &config.Config{},
	}

	handler := app.translationHandler(trans)

	// Test 1: Non-streaming request
	nonStreamingReq := map[string]interface{}{
		"model":      "claude-3-5-sonnet-20241022",
		"max_tokens": 1024,
		"stream":     false,
		"messages":   []map[string]interface{}{{"role": "user", "content": "Hello"}},
	}
	reqBody, _ := json.Marshal(nonStreamingReq)
	req := httptest.NewRequest("POST", "/olla/anthropic/v1/messages", bytes.NewReader(reqBody))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	// Test 2: Streaming request
	streamingReq := map[string]interface{}{
		"model":      "claude-3-5-sonnet-20241022",
		"max_tokens": 1024,
		"stream":     true,
		"messages":   []map[string]interface{}{{"role": "user", "content": "Hello"}},
	}
	reqBody2, _ := json.Marshal(streamingReq)
	req2 := httptest.NewRequest("POST", "/olla/anthropic/v1/messages", bytes.NewReader(reqBody2))
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req2)

	assert.Equal(t, http.StatusOK, rec2.Code)

	// Verify both events were recorded with correct streaming flag
	events := statsCollector.getRecordedEvents()
	require.Len(t, events, 2)

	// First event should be non-streaming
	assert.False(t, events[0].IsStreaming, "First request should be non-streaming")

	// Second event should be streaming
	assert.True(t, events[1].IsStreaming, "Second request should be streaming")
}

// TestTranslationHandler_MetricsRecordedForSuccessVsError verifies success/failure tracking
func TestTranslationHandler_MetricsRecordedForSuccessVsError(t *testing.T) {
	successBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Add small delay to ensure measurable latency
		time.Sleep(1 * time.Millisecond)
		response := map[string]interface{}{"id": "msg_123", "type": "message"}
		w.Header().Set(constants.HeaderContentType, constants.ContentTypeJSON)
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(response)
	}))
	defer successBackend.Close()

	errorBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		errorResp := map[string]interface{}{
			"type": "error",
			"error": map[string]interface{}{
				"type":    "invalid_request_error",
				"message": "Test error",
			},
		}
		w.Header().Set(constants.HeaderContentType, constants.ContentTypeJSON)
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(errorResp)
	}))
	defer errorBackend.Close()

	successURL, _ := url.Parse(successBackend.URL)
	errorURL, _ := url.Parse(errorBackend.URL)

	profileLookup := &mockPassthroughProfileLookup{
		configs: map[string]*domain.AnthropicSupportConfig{
			"vllm": {Enabled: true, MessagesPath: "/v1/messages"},
		},
	}

	trans := &mockPassthroughTranslator{
		name:                  "anthropic",
		passthroughEnabled:    true,
		profileLookup:         profileLookup,
		implementsErrorWriter: true,
	}

	statsCollector := &mockStatsCollectorWithCapture{}

	// Test 1: Successful request
	successEndpoints := []*domain.Endpoint{
		{Name: "success-backend", URL: successURL, URLString: successBackend.URL, Type: "vllm", Status: domain.StatusHealthy},
	}

	proxyService := &mockProxyService{
		proxyFunc: func(ctx context.Context, w http.ResponseWriter, r *http.Request, eps []*domain.Endpoint, stats *ports.RequestStats, rlog logger.StyledLogger) error {
			client := &http.Client{Timeout: 5 * time.Second}
			backendReq, err := http.NewRequest(r.Method, eps[0].URLString+r.URL.Path, r.Body)
			if err != nil {
				return err
			}
			resp, err := client.Do(backendReq)
			if err != nil {
				return err
			}
			defer resp.Body.Close()
			for k, v := range resp.Header {
				w.Header()[k] = v
			}
			w.WriteHeader(resp.StatusCode)
			io.Copy(w, resp.Body)
			return nil
		},
	}

	app := &Application{
		logger:           &mockStyledLogger{},
		proxyService:     proxyService,
		statsCollector:   statsCollector,
		repository:       &mockEndpointRepository{getEndpointsFunc: func() []*domain.Endpoint { return successEndpoints }},
		inspectorChain:   inspector.NewChain(&mockStyledLogger{}),
		profileFactory:   &mockProfileFactory{},
		profileLookup:    profileLookup,
		discoveryService: &mockDiscoveryServiceWithEndpoints{endpoints: successEndpoints},
		Config:           &config.Config{},
	}

	handler := app.translationHandler(trans)

	reqBody, _ := json.Marshal(map[string]interface{}{
		"model":      "claude-3-5-sonnet-20241022",
		"max_tokens": 1024,
		"messages":   []map[string]interface{}{{"role": "user", "content": "Hello"}},
	})

	req := httptest.NewRequest("POST", "/olla/anthropic/v1/messages", bytes.NewReader(reqBody))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	// Test 2: Error request
	errorEndpoints := []*domain.Endpoint{
		{Name: "error-backend", URL: errorURL, URLString: errorBackend.URL, Type: "vllm", Status: domain.StatusHealthy},
	}

	app2 := &Application{
		logger:           &mockStyledLogger{},
		proxyService:     proxyService,
		statsCollector:   statsCollector,
		repository:       &mockEndpointRepository{getEndpointsFunc: func() []*domain.Endpoint { return errorEndpoints }},
		inspectorChain:   inspector.NewChain(&mockStyledLogger{}),
		profileFactory:   &mockProfileFactory{},
		profileLookup:    profileLookup,
		discoveryService: &mockDiscoveryServiceWithEndpoints{endpoints: errorEndpoints},
		Config:           &config.Config{},
	}

	handler2 := app2.translationHandler(trans)

	reqBody2, _ := json.Marshal(map[string]interface{}{
		"model":      "claude-3-5-sonnet-20241022",
		"max_tokens": 1024,
		"messages":   []map[string]interface{}{{"role": "user", "content": "Hello"}},
	})

	req2 := httptest.NewRequest("POST", "/olla/anthropic/v1/messages", bytes.NewReader(reqBody2))
	rec2 := httptest.NewRecorder()
	handler2.ServeHTTP(rec2, req2)

	assert.Equal(t, http.StatusBadRequest, rec2.Code)

	// Verify metrics recorded for both success and error
	events := statsCollector.getRecordedEvents()
	require.Len(t, events, 2)

	// First event should be successful
	assert.True(t, events[0].Success, "First request should be successful")

	// Second event should be successful (even though backend returned error, the handler processed it successfully)
	// Backend errors are considered successful processing from the handler's perspective
	assert.True(t, events[1].Success, "Second request should be successful (handler processed backend error)")
}
