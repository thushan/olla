package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
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
	"github.com/thushan/olla/internal/util"
)

// mockTranslator implements RequestTranslator for testing
type mockTranslator struct {
	name                      string
	transformRequestFunc      func(ctx context.Context, r *http.Request) (*translator.TransformedRequest, error)
	transformResponseFunc     func(ctx context.Context, openaiResp interface{}, original *http.Request) (interface{}, error)
	transformStreamingFunc    func(ctx context.Context, openaiStream io.Reader, w http.ResponseWriter, original *http.Request) error
	writeErrorFunc            func(w http.ResponseWriter, err error, statusCode int)
	countTokensFunc           func(ctx context.Context, r *http.Request) (*translator.TokenCountResponse, error)
	serialiseCountTokensFunc  func(resp *translator.TokenCountResponse) ([]byte, error)
	pathProvider              string
	implementsErrorWriter     bool
	implementsPathProvider    bool
	implementsTokenCounter    bool
	implementsTokenSerializer bool
}

func (m *mockTranslator) Name() string {
	return m.name
}

func (m *mockTranslator) TransformRequest(ctx context.Context, r *http.Request) (*translator.TransformedRequest, error) {
	if m.transformRequestFunc != nil {
		return m.transformRequestFunc(ctx, r)
	}
	return &translator.TransformedRequest{
		OpenAIRequest: map[string]interface{}{
			"model": "test-model",
			"messages": []interface{}{
				map[string]interface{}{
					"role":    "user",
					"content": "test",
				},
			},
		},
		ModelName:   "test-model",
		IsStreaming: false,
	}, nil
}

func (m *mockTranslator) TransformResponse(ctx context.Context, openaiResp interface{}, original *http.Request) (interface{}, error) {
	if m.transformResponseFunc != nil {
		return m.transformResponseFunc(ctx, openaiResp, original)
	}
	return map[string]interface{}{
		"id":      "mock-response-id",
		"content": "mock response",
	}, nil
}

func (m *mockTranslator) TransformStreamingResponse(ctx context.Context, openaiStream io.Reader, w http.ResponseWriter, original *http.Request) error {
	if m.transformStreamingFunc != nil {
		return m.transformStreamingFunc(ctx, openaiStream, w, original)
	}
	w.Header().Set(constants.HeaderContentType, "text/event-stream")
	_, err := io.Copy(w, openaiStream)
	return err
}

func (m *mockTranslator) GetAPIPath() string {
	if m.implementsPathProvider {
		return m.pathProvider
	}
	panic("GetAPIPath called on translator that doesn't implement PathProvider")
}

func (m *mockTranslator) WriteError(w http.ResponseWriter, err error, statusCode int) {
	if m.implementsErrorWriter && m.writeErrorFunc != nil {
		m.writeErrorFunc(w, err, statusCode)
		return
	}
	panic("WriteError called on translator that doesn't implement ErrorWriter")
}

func (m *mockTranslator) CountTokens(ctx context.Context, r *http.Request) (*translator.TokenCountResponse, error) {
	if m.implementsTokenCounter && m.countTokensFunc != nil {
		return m.countTokensFunc(ctx, r)
	}
	panic("CountTokens called on translator that doesn't implement TokenCounter")
}

func (m *mockTranslator) SerialiseCountTokens(resp *translator.TokenCountResponse) ([]byte, error) {
	if m.implementsTokenSerializer && m.serialiseCountTokensFunc != nil {
		return m.serialiseCountTokensFunc(resp)
	}
	panic("SerialiseCountTokens called on translator that doesn't implement TokenCountSerializer")
}

// mockProxyService implements ProxyService for testing
type mockProxyService struct {
	proxyFunc func(ctx context.Context, w http.ResponseWriter, r *http.Request, endpoints []*domain.Endpoint, stats *ports.RequestStats, logger logger.StyledLogger) error
}

func (m *mockProxyService) ProxyRequestToEndpoints(
	ctx context.Context,
	w http.ResponseWriter,
	r *http.Request,
	endpoints []*domain.Endpoint,
	stats *ports.RequestStats,
	logger logger.StyledLogger,
) error {
	if m.proxyFunc != nil {
		return m.proxyFunc(ctx, w, r, endpoints, stats, logger)
	}
	response := map[string]interface{}{
		"id":      "chatcmpl-123",
		"object":  "chat.completion",
		"created": 1677652288,
		"model":   "test-model",
		"choices": []interface{}{
			map[string]interface{}{
				"index": 0,
				"message": map[string]interface{}{
					"role":    "assistant",
					"content": "Hello! How can I help you?",
				},
				"finish_reason": "stop",
			},
		},
	}

	w.Header().Set(constants.HeaderContentType, constants.ContentTypeJSON)
	w.Header().Set(constants.HeaderXOllaRequestID, "test-request-id")
	w.Header().Set(constants.HeaderXOllaEndpoint, "test-endpoint")
	w.Header().Set(constants.HeaderXOllaBackendType, "openai")
	w.Header().Set(constants.HeaderXOllaModel, "test-model")
	w.WriteHeader(http.StatusOK)
	return json.NewEncoder(w).Encode(response)
}

// TestTranslationHandler_PlainTextBackendError verifies that when a backend returns
// a non-JSON 4xx/5xx body (e.g. a plain-text 429 from a rate-limiter), the handler
// preserves the upstream status code and emits a structured Anthropic error rather
// than surfacing a misleading "failed to parse OpenAI response" 502.
func TestTranslationHandler_PlainTextBackendError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		backendStatus  int
		backendBody    string
		backendCT      string
		expectedStatus int
		expectedType   string
	}{
		{
			name:           "plain_text_429",
			backendStatus:  http.StatusTooManyRequests,
			backendBody:    "Rate limit exceeded. Please try again later.",
			backendCT:      "text/plain",
			expectedStatus: http.StatusTooManyRequests,
			expectedType:   "rate_limit_error",
		},
		{
			name:           "html_503",
			backendStatus:  http.StatusServiceUnavailable,
			backendBody:    "<html><body>Service Unavailable</body></html>",
			backendCT:      "text/html",
			expectedStatus: http.StatusServiceUnavailable,
			expectedType:   "overloaded_error", // mock writeErrorFunc maps 503 → overloaded_error
		},
		{
			name:          "json_400_preserved",
			backendStatus: http.StatusBadRequest,
			backendBody:   `{"error":{"message":"Invalid temperature value","type":"invalid_request_error"}}`,
			backendCT:     "application/json",
			// Existing behaviour must be preserved: message extracted from JSON.
			expectedStatus: http.StatusBadRequest,
			expectedType:   "invalid_request_error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			mockLogger := &mockStyledLogger{}

			trans := &mockTranslator{
				name:                  "anthropic",
				implementsErrorWriter: true,
				writeErrorFunc: func(w http.ResponseWriter, err error, statusCode int) {
					errorType := "api_error"
					switch statusCode {
					case http.StatusBadRequest:
						errorType = "invalid_request_error"
					case http.StatusTooManyRequests:
						errorType = "rate_limit_error"
					case http.StatusServiceUnavailable:
						errorType = "overloaded_error"
					}
					w.Header().Set(constants.HeaderContentType, constants.ContentTypeJSON)
					w.WriteHeader(statusCode)
					_ = json.NewEncoder(w).Encode(map[string]interface{}{
						"type": "error",
						"error": map[string]interface{}{
							"type":    errorType,
							"message": err.Error(),
						},
					})
				},
			}

			proxyService := &mockProxyService{
				proxyFunc: func(ctx context.Context, w http.ResponseWriter, r *http.Request, endpoints []*domain.Endpoint, stats *ports.RequestStats, rlog logger.StyledLogger) error {
					w.Header().Set(constants.HeaderContentType, tt.backendCT)
					w.WriteHeader(tt.backendStatus)
					_, err := w.Write([]byte(tt.backendBody))
					return err
				},
			}

			app := &Application{
				logger:           mockLogger,
				proxyService:     proxyService,
				statsCollector:   &mockStatsCollector{},
				repository:       &mockEndpointRepository{},
				inspectorChain:   inspector.NewChain(mockLogger),
				profileFactory:   &mockProfileFactory{},
				discoveryService: &mockDiscoveryServiceForTranslation{},
				Config:           &config.Config{},
			}

			reqBody, _ := json.Marshal(map[string]interface{}{
				"model":      "test-model",
				"max_tokens": 100,
				"messages":   []map[string]interface{}{{"role": "user", "content": "hello"}},
			})

			req := httptest.NewRequest("POST", "/olla/anthropic/v1/messages", bytes.NewReader(reqBody))
			req.Header.Set(constants.HeaderContentType, constants.ContentTypeJSON)
			rec := httptest.NewRecorder()

			handler := app.translationHandler(trans)
			handler.ServeHTTP(rec, req)

			assert.Equal(t, tt.expectedStatus, rec.Code,
				"upstream status code must be preserved, not flattened to 502")

			var errBody map[string]interface{}
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &errBody),
				"error body must be valid JSON")
			assert.Equal(t, "error", errBody["type"])

			errObj, ok := errBody["error"].(map[string]interface{})
			require.True(t, ok)
			assert.Equal(t, tt.expectedType, errObj["type"],
				"error type must match the expected Anthropic error type for this status")
		})
	}
}

func TestTranslationHandler_NonStreaming(t *testing.T) {
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

	// Verify response was transformed
	var response map[string]interface{}
	err := json.Unmarshal(rec.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, "mock-response-id", response["id"])
}

func TestTranslationHandler_Streaming(t *testing.T) {
	mockLogger := &mockStyledLogger{}

	streamingTrans := &mockTranslator{
		name:                  "streaming-translator",
		implementsErrorWriter: true,
		writeErrorFunc: func(w http.ResponseWriter, err error, statusCode int) {
			w.Header().Set(constants.HeaderContentType, constants.ContentTypeJSON)
			w.WriteHeader(statusCode)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"error": err.Error(),
			})
		},
		transformRequestFunc: func(ctx context.Context, r *http.Request) (*translator.TransformedRequest, error) {
			return &translator.TransformedRequest{
				OpenAIRequest: map[string]interface{}{
					"model":  "test-model",
					"stream": true,
					"messages": []interface{}{
						map[string]interface{}{
							"role":    "user",
							"content": "test",
						},
					},
				},
				ModelName:   "test-model",
				IsStreaming: true,
			}, nil
		},
		transformStreamingFunc: func(ctx context.Context, openaiStream io.Reader, w http.ResponseWriter, original *http.Request) error {
			// Read from stream and write transformed output
			w.Header().Set(constants.HeaderContentType, "text/event-stream")
			data, _ := io.ReadAll(openaiStream)
			_, err := w.Write([]byte("data: transformed-" + string(data) + "\n\n"))
			return err
		},
	}

	proxyService := &mockProxyService{
		proxyFunc: func(ctx context.Context, w http.ResponseWriter, r *http.Request, endpoints []*domain.Endpoint, stats *ports.RequestStats, logger logger.StyledLogger) error {
			w.Header().Set(constants.HeaderContentType, "text/event-stream")
			w.Header().Set(constants.HeaderXOllaRequestID, "streaming-test-id")
			w.WriteHeader(http.StatusOK)
			_, err := w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"Hello\"}}]}\n\n"))
			return err
		},
	}

	app := &Application{
		logger:           mockLogger,
		proxyService:     proxyService,
		statsCollector:   &mockStatsCollector{},
		repository:       &mockEndpointRepository{},
		inspectorChain:   inspector.NewChain(mockLogger),
		profileFactory:   &mockProfileFactory{},
		converterFactory: nil,
		discoveryService: &mockDiscoveryServiceForTranslation{},
		Config:           &config.Config{},
	}

	handler := app.translationHandler(streamingTrans)

	reqBody := map[string]interface{}{
		"model":  "test-model",
		"stream": true,
		"messages": []interface{}{
			map[string]interface{}{
				"role":    "user",
				"content": "Hello",
			},
		},
	}
	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest("POST", "/test", bytes.NewReader(body))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "text/event-stream", rec.Header().Get(constants.HeaderContentType))

	// Verify streaming response was transformed
	assert.Contains(t, rec.Body.String(), "transformed-")
}

func TestTranslationHandler_TransformRequestError(t *testing.T) {
	mockLogger := &mockStyledLogger{}

	trans := &mockTranslator{
		name: "error-translator",
		transformRequestFunc: func(ctx context.Context, r *http.Request) (*translator.TransformedRequest, error) {
			return nil, errors.New("invalid request format")
		},
		implementsErrorWriter: true,
		writeErrorFunc: func(w http.ResponseWriter, err error, statusCode int) {
			w.Header().Set(constants.HeaderContentType, constants.ContentTypeJSON)
			w.WriteHeader(statusCode)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"type": "error",
				"error": map[string]interface{}{
					"type":    "invalid_request_error",
					"message": err.Error(),
				},
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

	req := httptest.NewRequest("POST", "/test", bytes.NewReader([]byte("{}")))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)

	var response map[string]interface{}
	err := json.Unmarshal(rec.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, "error", response["type"])
}

func TestTranslationHandler_NoHealthyEndpoints(t *testing.T) {
	mockLogger := &mockStyledLogger{}

	trans := &mockTranslator{
		name:                  "test-translator",
		implementsErrorWriter: true,
		writeErrorFunc: func(w http.ResponseWriter, err error, statusCode int) {
			w.Header().Set(constants.HeaderContentType, constants.ContentTypeJSON)
			w.WriteHeader(statusCode)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"error": map[string]interface{}{
					"message": err.Error(),
					"type":    "service_unavailable",
				},
			})
		},
	}

	noEndpointsDiscovery := &mockDiscoveryServiceWithFunc{
		getHealthyEndpointsFunc: func(ctx context.Context) ([]*domain.Endpoint, error) {
			return nil, errors.New("no healthy endpoints available")
		},
	}

	app := &Application{
		logger:           mockLogger,
		proxyService:     &mockProxyService{},
		statsCollector:   &mockStatsCollector{},
		repository:       &mockEndpointRepository{},
		inspectorChain:   inspector.NewChain(mockLogger),
		profileFactory:   &mockProfileFactory{},
		converterFactory: nil,
		discoveryService: noEndpointsDiscovery,
		Config:           &config.Config{},
	}

	handler := app.translationHandler(trans)

	reqBody := map[string]interface{}{"model": "test-model"}
	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest("POST", "/test", bytes.NewReader(body))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
}

func TestTranslationHandler_HeaderPreservation(t *testing.T) {
	mockLogger := &mockStyledLogger{}

	trans := &mockTranslator{
		name: "test-translator",
	}

	proxyService := &mockProxyService{
		proxyFunc: func(ctx context.Context, w http.ResponseWriter, r *http.Request, endpoints []*domain.Endpoint, stats *ports.RequestStats, logger logger.StyledLogger) error {
			// Set all X-Olla-* headers
			w.Header().Set(constants.HeaderXOllaRequestID, "test-request-id")
			w.Header().Set(constants.HeaderXOllaEndpoint, "test-endpoint")
			w.Header().Set(constants.HeaderXOllaBackendType, "openai")
			w.Header().Set(constants.HeaderXOllaModel, "test-model")
			w.Header().Set(constants.HeaderXOllaResponseTime, "123ms")
			w.Header().Set(constants.HeaderXOllaRoutingStrategy, "priority")
			w.Header().Set(constants.HeaderXOllaRoutingDecision, "selected")
			w.Header().Set(constants.HeaderXOllaRoutingReason, "health check passed")

			response := map[string]interface{}{
				"id":      "test-id",
				"choices": []interface{}{},
			}
			return json.NewEncoder(w).Encode(response)
		},
	}

	app := &Application{
		logger:           mockLogger,
		proxyService:     proxyService,
		statsCollector:   &mockStatsCollector{},
		repository:       &mockEndpointRepository{},
		inspectorChain:   inspector.NewChain(mockLogger),
		profileFactory:   &mockProfileFactory{},
		converterFactory: nil,
		discoveryService: &mockDiscoveryServiceForTranslation{},
		Config:           &config.Config{},
	}

	handler := app.translationHandler(trans)

	reqBody := map[string]interface{}{"model": "test-model"}
	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest("POST", "/test", bytes.NewReader(body))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	// Verify all X-Olla-* headers are preserved
	assert.Equal(t, "test-request-id", rec.Header().Get(constants.HeaderXOllaRequestID))
	assert.Equal(t, "test-endpoint", rec.Header().Get(constants.HeaderXOllaEndpoint))
	assert.Equal(t, "openai", rec.Header().Get(constants.HeaderXOllaBackendType))
	assert.Equal(t, "test-model", rec.Header().Get(constants.HeaderXOllaModel))
	assert.Equal(t, "123ms", rec.Header().Get(constants.HeaderXOllaResponseTime))
	assert.Equal(t, "priority", rec.Header().Get(constants.HeaderXOllaRoutingStrategy))
	assert.Equal(t, "selected", rec.Header().Get(constants.HeaderXOllaRoutingDecision))
	assert.Equal(t, "health check passed", rec.Header().Get(constants.HeaderXOllaRoutingReason))
}

func TestWriteTranslatorError_WithErrorWriter(t *testing.T) {
	mockLogger := &mockStyledLogger{}

	trans := &mockTranslator{
		name:                  "test-translator",
		implementsErrorWriter: true,
		writeErrorFunc: func(w http.ResponseWriter, err error, statusCode int) {
			w.Header().Set(constants.HeaderContentType, constants.ContentTypeJSON)
			w.WriteHeader(statusCode)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"custom_error": err.Error(),
			})
		},
	}

	app := &Application{
		logger: mockLogger,
		Config: &config.Config{},
	}

	rec := httptest.NewRecorder()
	pr := &proxyRequest{
		requestLogger: mockLogger,
	}

	app.writeTranslatorError(rec, trans, pr, errors.New("test error"), http.StatusBadRequest)

	assert.Equal(t, http.StatusBadRequest, rec.Code)

	var response map[string]interface{}
	err := json.Unmarshal(rec.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, "test error", response["custom_error"])
}

func TestWriteTranslatorError_WithoutErrorWriter(t *testing.T) {
	mockLogger := &mockStyledLogger{}

	trans := &mockTranslatorWithoutErrorWriter{
		name: "test-translator",
	}

	app := &Application{
		logger: mockLogger,
		Config: &config.Config{},
	}

	rec := httptest.NewRecorder()
	pr := &proxyRequest{
		requestLogger: mockLogger,
	}

	app.writeTranslatorError(rec, trans, pr, errors.New("test error"), http.StatusInternalServerError)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)

	var response map[string]interface{}
	err := json.Unmarshal(rec.Body.Bytes(), &response)
	require.NoError(t, err)

	errorObj, ok := response["error"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "test error", errorObj["message"])
	assert.Equal(t, "translation_error", errorObj["type"])
}

func (m *mockProxyService) GetStats(ctx context.Context) (ports.ProxyStats, error) {
	return ports.ProxyStats{}, nil
}
func (m *mockProxyService) UpdateConfig(configuration ports.ProxyConfiguration) {}
func (m *mockProxyService) ProxyRequest(ctx context.Context, w http.ResponseWriter, r *http.Request, stats *ports.RequestStats, rlog logger.StyledLogger) error {
	return nil
}

type mockStatsCollector struct{}

func (m *mockStatsCollector) RecordRequest(endpoint *domain.Endpoint, status string, latency time.Duration, bytes int64) {
}
func (m *mockStatsCollector) RecordConnection(endpoint *domain.Endpoint, delta int) {}
func (m *mockStatsCollector) RecordSecurityViolation(violation ports.SecurityViolation) {
}
func (m *mockStatsCollector) RecordDiscovery(endpoint *domain.Endpoint, success bool, latency time.Duration) {
}
func (m *mockStatsCollector) RecordModelRequest(model string, endpoint *domain.Endpoint, status string, latency time.Duration, bytes int64) {
}
func (m *mockStatsCollector) RecordModelError(model string, endpoint *domain.Endpoint, errorType string) {
}
func (m *mockStatsCollector) GetModelStats() map[string]ports.ModelStats { return nil }
func (m *mockStatsCollector) GetModelEndpointStats() map[string]map[string]ports.EndpointModelStats {
	return nil
}
func (m *mockStatsCollector) RecordTranslatorRequest(event ports.TranslatorRequestEvent) {}
func (m *mockStatsCollector) GetTranslatorStats() map[string]ports.TranslatorStats {
	return nil
}
func (m *mockStatsCollector) GetProxyStats() ports.ProxyStats                  { return ports.ProxyStats{} }
func (m *mockStatsCollector) GetEndpointStats() map[string]ports.EndpointStats { return nil }
func (m *mockStatsCollector) GetSecurityStats() ports.SecurityStats            { return ports.SecurityStats{} }
func (m *mockStatsCollector) GetConnectionStats() map[string]int64             { return nil }
func (m *mockStatsCollector) GetConnectionCount(endpoint string) int64         { return 0 }

type mockEndpointRepository struct {
	getEndpointsFunc func() []*domain.Endpoint
}

func (m *mockEndpointRepository) GetEndpoints() []*domain.Endpoint {
	if m.getEndpointsFunc != nil {
		return m.getEndpointsFunc()
	}
	u, _ := url.Parse("http://localhost:8080")
	return []*domain.Endpoint{
		{
			Name:      "test-endpoint",
			URL:       u,
			URLString: "http://localhost:8080",
			Type:      "openai",
			Status:    domain.StatusHealthy,
		},
	}
}

func (m *mockEndpointRepository) GetEndpoint(name string) (*domain.Endpoint, error) {
	return nil, nil
}

func (m *mockEndpointRepository) AddEndpoint(endpoint *domain.Endpoint) error {
	return nil
}

func (m *mockEndpointRepository) RemoveEndpoint(name string) error {
	return nil
}

func (m *mockEndpointRepository) Exists(ctx context.Context, endpointURL *url.URL) bool {
	return false
}

func (m *mockEndpointRepository) GetAll(ctx context.Context) ([]*domain.Endpoint, error) {
	if m.getEndpointsFunc != nil {
		return m.getEndpointsFunc(), nil
	}
	u, _ := url.Parse("http://localhost:8080")
	return []*domain.Endpoint{
		{
			Name:      "test-endpoint",
			URL:       u,
			URLString: "http://localhost:8080",
			Type:      "openai",
			Status:    domain.StatusHealthy,
		},
	}, nil
}

func (m *mockEndpointRepository) GetRoutable(ctx context.Context) ([]*domain.Endpoint, error) {
	return m.GetHealthy(ctx)
}

func (m *mockEndpointRepository) UpdateEndpoint(ctx context.Context, endpoint *domain.Endpoint) error {
	return nil
}

func (m *mockEndpointRepository) GetHealthy(ctx context.Context) ([]*domain.Endpoint, error) {
	if m.getEndpointsFunc != nil {
		return m.getEndpointsFunc(), nil
	}
	u, _ := url.Parse("http://localhost:8080")
	return []*domain.Endpoint{
		{
			Name:      "test-endpoint",
			URL:       u,
			URLString: "http://localhost:8080",
			Type:      "openai",
			Status:    domain.StatusHealthy,
		},
	}, nil
}

// provides discovery service that returns healthy endpoints for translation tests
type mockDiscoveryServiceForTranslation struct{}

func (m *mockDiscoveryServiceForTranslation) GetEndpoints(ctx context.Context) ([]*domain.Endpoint, error) {
	u, _ := url.Parse("http://localhost:8080")
	return []*domain.Endpoint{
		{
			Name:      "test-endpoint",
			URL:       u,
			URLString: "http://localhost:8080",
			Type:      "openai",
			Status:    domain.StatusHealthy,
		},
	}, nil
}

func (m *mockDiscoveryServiceForTranslation) GetHealthyEndpoints(ctx context.Context) ([]*domain.Endpoint, error) {
	return m.GetEndpoints(ctx)
}

func (m *mockDiscoveryServiceForTranslation) RefreshEndpoints(ctx context.Context) error {
	return nil
}

func (m *mockDiscoveryServiceForTranslation) UpdateEndpointStatus(ctx context.Context, endpoint *domain.Endpoint) error {
	return nil
}

type mockDiscoveryServiceWithFunc struct {
	getEndpointsFunc        func(ctx context.Context) ([]*domain.Endpoint, error)
	getHealthyEndpointsFunc func(ctx context.Context) ([]*domain.Endpoint, error)
}

func (m *mockDiscoveryServiceWithFunc) GetEndpoints(ctx context.Context) ([]*domain.Endpoint, error) {
	if m.getEndpointsFunc != nil {
		return m.getEndpointsFunc(ctx)
	}
	return nil, nil
}

func (m *mockDiscoveryServiceWithFunc) GetHealthyEndpoints(ctx context.Context) ([]*domain.Endpoint, error) {
	if m.getHealthyEndpointsFunc != nil {
		return m.getHealthyEndpointsFunc(ctx)
	}
	return nil, nil
}

func (m *mockDiscoveryServiceWithFunc) RefreshEndpoints(ctx context.Context) error {
	return nil
}

func (m *mockDiscoveryServiceWithFunc) UpdateEndpointStatus(ctx context.Context, endpoint *domain.Endpoint) error {
	return nil
}

// translator that doesn't implement ErrorWriter interface
type mockTranslatorWithoutErrorWriter struct {
	name string
}

func (m *mockTranslatorWithoutErrorWriter) Name() string {
	return m.name
}

func (m *mockTranslatorWithoutErrorWriter) TransformRequest(ctx context.Context, r *http.Request) (*translator.TransformedRequest, error) {
	return &translator.TransformedRequest{
		OpenAIRequest: map[string]interface{}{
			"model": "test-model",
			"messages": []interface{}{
				map[string]interface{}{
					"role":    "user",
					"content": "test",
				},
			},
		},
		ModelName:   "test-model",
		IsStreaming: false,
	}, nil
}

func (m *mockTranslatorWithoutErrorWriter) TransformResponse(ctx context.Context, openaiResp interface{}, original *http.Request) (interface{}, error) {
	return map[string]interface{}{
		"id":      "mock-response-id",
		"content": "mock response",
	}, nil
}

func (m *mockTranslatorWithoutErrorWriter) TransformStreamingResponse(ctx context.Context, openaiStream io.Reader, w http.ResponseWriter, original *http.Request) error {
	w.Header().Set(constants.HeaderContentType, "text/event-stream")
	_, err := io.Copy(w, openaiStream)
	return err
}

func TestTranslationHandler_StreamingPanicRecovery(t *testing.T) {
	t.Parallel()

	mockLogger := &mockStyledLogger{}

	panicTrans := &mockTranslator{
		name:                  "panic-translator",
		implementsErrorWriter: true,
		writeErrorFunc: func(w http.ResponseWriter, err error, statusCode int) {
			w.Header().Set(constants.HeaderContentType, constants.ContentTypeJSON)
			w.WriteHeader(statusCode)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"error": err.Error(),
			})
		},
		transformRequestFunc: func(ctx context.Context, r *http.Request) (*translator.TransformedRequest, error) {
			return &translator.TransformedRequest{
				OpenAIRequest: map[string]interface{}{
					"model":  "test-model",
					"stream": true,
					"messages": []interface{}{
						map[string]interface{}{
							"role":    "user",
							"content": "test",
						},
					},
				},
				ModelName:   "test-model",
				IsStreaming: true,
			}, nil
		},
		transformStreamingFunc: func(ctx context.Context, openaiStream io.Reader, w http.ResponseWriter, original *http.Request) error {
			panic("simulated panic during stream transformation")
		},
	}

	proxyService := &mockProxyService{
		proxyFunc: func(ctx context.Context, w http.ResponseWriter, r *http.Request, endpoints []*domain.Endpoint, stats *ports.RequestStats, logger logger.StyledLogger) error {
			w.Header().Set(constants.HeaderContentType, "text/event-stream")
			w.Header().Set(constants.HeaderXOllaRequestID, "panic-test-id")
			w.WriteHeader(http.StatusOK)

			_, err := w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"test\"}}]}\n\n"))
			if err != nil {
				return err
			}

			time.Sleep(10 * time.Millisecond)
			return nil
		},
	}

	app := &Application{
		logger:           mockLogger,
		proxyService:     proxyService,
		statsCollector:   &mockStatsCollector{},
		repository:       &mockEndpointRepository{},
		inspectorChain:   inspector.NewChain(mockLogger),
		profileFactory:   &mockProfileFactory{},
		converterFactory: nil,
		discoveryService: &mockDiscoveryServiceForTranslation{},
		Config:           &config.Config{},
	}

	handler := app.translationHandler(panicTrans)

	reqBody := map[string]interface{}{
		"model":  "test-model",
		"stream": true,
		"messages": []interface{}{
			map[string]interface{}{
				"role":    "user",
				"content": "test",
			},
		},
	}
	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest("POST", "/test", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	// The handler must NOT re-panic. A re-panic resets the TCP connection,
	// giving the client no indication of what went wrong.
	//
	// The proxy writes one SSE event into the pipe, but the translator panics
	// immediately without reading from it and writing anything to the real client
	// ResponseWriter. The committedResponseWriter therefore has committed=false,
	// and the handler correctly falls back to a plain HTTP 502 rather than an SSE
	// error event. This is the correct behaviour: no bytes reached the client, so
	// the response is not committed and a proper error status can be sent.
	handler.ServeHTTP(rec, req)

	// The panic must not propagate as a connection reset. Because the translator
	// panicked before writing to the client, the response is uncommitted and a
	// 502 with a structured error body is the correct outcome.
	assert.Equal(t, http.StatusBadGateway, rec.Code,
		"status must be 502 when panic fires before any bytes reach the client")
	assert.NotContains(t, rec.Body.String(), "event: error",
		"no SSE event should appear when the response was not committed")
}

// Verifies that:
// 1. Backend errors (404, 500, 429, etc.) are correctly translated to Anthropic format
func TestTranslationHandler_BackendErrorTranslation(t *testing.T) {
	tests := []struct {
		name               string
		backendStatus      int
		backendError       map[string]interface{}
		expectedErrorType  string
		expectedStatusCode int
		expectedMessage    string
	}{
		{
			name:          "404_model_not_found",
			backendStatus: 404,
			backendError: map[string]interface{}{
				"error": map[string]interface{}{
					"message": "Model not found",
					"type":    "invalid_request_error",
					"code":    "model_not_found",
				},
			},
			expectedErrorType:  "not_found_error",
			expectedStatusCode: 404,
			expectedMessage:    "Model not found",
		},
		{
			name:          "500_internal_error",
			backendStatus: 500,
			backendError: map[string]interface{}{
				"error": map[string]interface{}{
					"message": "Internal server error",
					"type":    "api_error",
				},
			},
			expectedErrorType:  "api_error",
			expectedStatusCode: 500,
			expectedMessage:    "Internal server error",
		},
		{
			name:          "429_rate_limit",
			backendStatus: 429,
			backendError: map[string]interface{}{
				"error": map[string]interface{}{
					"message": "Rate limit exceeded",
					"type":    "rate_limit_error",
				},
			},
			expectedErrorType:  "rate_limit_error",
			expectedStatusCode: 429,
			expectedMessage:    "Rate limit exceeded",
		},
		{
			name:          "400_bad_request",
			backendStatus: 400,
			backendError: map[string]interface{}{
				"error": map[string]interface{}{
					"message": "Invalid temperature value",
					"type":    "invalid_request_error",
				},
			},
			expectedErrorType:  "invalid_request_error",
			expectedStatusCode: 400,
			expectedMessage:    "Invalid temperature value",
		},
		{
			name:          "401_authentication_error",
			backendStatus: 401,
			backendError: map[string]interface{}{
				"error": map[string]interface{}{
					"message": "Invalid API key",
					"type":    "authentication_error",
				},
			},
			expectedErrorType:  "authentication_error",
			expectedStatusCode: 401,
			expectedMessage:    "Invalid API key",
		},
		{
			name:          "403_permission_error",
			backendStatus: 403,
			backendError: map[string]interface{}{
				"error": map[string]interface{}{
					"message": "Access denied",
					"type":    "permission_error",
				},
			},
			expectedErrorType:  "permission_error",
			expectedStatusCode: 403,
			expectedMessage:    "Access denied",
		},
		{
			name:          "503_service_unavailable",
			backendStatus: 503,
			backendError: map[string]interface{}{
				"error": map[string]interface{}{
					"message": "Service overloaded",
					"type":    "overloaded_error",
				},
			},
			expectedErrorType:  "overloaded_error",
			expectedStatusCode: 503,
			expectedMessage:    "Service overloaded",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockLogger := &mockStyledLogger{}

			// Create Anthropic translator with error writing capability
			trans := &mockTranslator{
				name:                  "anthropic",
				implementsErrorWriter: true,
				writeErrorFunc: func(w http.ResponseWriter, err error, statusCode int) {
					// Simulate Anthropic error formatting
					errorType := "api_error"
					switch statusCode {
					case http.StatusBadRequest:
						errorType = "invalid_request_error"
					case http.StatusUnauthorized:
						errorType = "authentication_error"
					case http.StatusForbidden:
						errorType = "permission_error"
					case http.StatusNotFound:
						errorType = "not_found_error"
					case http.StatusTooManyRequests:
						errorType = "rate_limit_error"
					case http.StatusServiceUnavailable:
						errorType = "overloaded_error"
					}

					errorResp := map[string]interface{}{
						"type": "error",
						"error": map[string]interface{}{
							"type":    errorType,
							"message": err.Error(),
						},
					}

					w.Header().Set(constants.HeaderContentType, constants.ContentTypeJSON)
					w.WriteHeader(statusCode)
					json.NewEncoder(w).Encode(errorResp)
				},
			}

			// Create mock proxy that returns backend error
			mockProxy := &mockProxyService{
				proxyFunc: func(ctx context.Context, w http.ResponseWriter, r *http.Request, endpoints []*domain.Endpoint, stats *ports.RequestStats, logger logger.StyledLogger) error {
					// Set X-Olla headers before error
					w.Header().Set(constants.HeaderXOllaRequestID, "test-request-123")
					w.Header().Set(constants.HeaderXOllaEndpoint, "test-backend")
					w.Header().Set(constants.HeaderXOllaBackendType, "openai")
					w.Header().Set(constants.HeaderXOllaModel, "test-model")
					w.Header().Set(constants.HeaderXOllaResponseTime, "50ms")
					w.Header().Set(constants.HeaderContentType, constants.ContentTypeJSON)
					w.WriteHeader(tt.backendStatus)
					return json.NewEncoder(w).Encode(tt.backendError)
				},
			}

			app := &Application{
				logger:           mockLogger,
				proxyService:     mockProxy,
				statsCollector:   &mockStatsCollector{},
				repository:       &mockEndpointRepository{},
				inspectorChain:   inspector.NewChain(mockLogger),
				profileFactory:   &mockProfileFactory{},
				discoveryService: &mockDiscoveryServiceForTranslation{},
				Config:           &config.Config{},
			}

			// Create Anthropic request
			anthropicReq := map[string]interface{}{
				"model":      "claude-3-5-sonnet-20241022",
				"max_tokens": 1024,
				"messages": []map[string]interface{}{
					{"role": "user", "content": "hello"},
				},
			}
			reqBody, _ := json.Marshal(anthropicReq)

			req := httptest.NewRequest("POST", "/olla/anthropic/v1/messages", bytes.NewReader(reqBody))
			req.Header.Set(constants.HeaderContentType, constants.ContentTypeJSON)

			rec := httptest.NewRecorder()

			// Execute translation handler
			handler := app.translationHandler(trans)
			handler.ServeHTTP(rec, req)

			// 1. Check status code matches backend error
			assert.Equal(t, tt.expectedStatusCode, rec.Code, "Status code should match backend error")

			// 2. Parse response body
			var anthropicError map[string]interface{}
			err := json.Unmarshal(rec.Body.Bytes(), &anthropicError)
			require.NoError(t, err, "Response should be valid JSON")

			// 3. Verify Anthropic error format
			assert.Equal(t, "error", anthropicError["type"], "Response type should be 'error'")

			errorObj, ok := anthropicError["error"].(map[string]interface{})
			require.True(t, ok, "Error object should exist")

			// 4. Verify error type mapping
			assert.Equal(t, tt.expectedErrorType, errorObj["type"], "Error type should match expected")

			// 5. Verify error message preserved
			assert.Equal(t, tt.expectedMessage, errorObj["message"], "Error message should be preserved")

			// 6. Verify X-Olla headers preserved during error response
			assert.Equal(t, "test-request-123", rec.Header().Get(constants.HeaderXOllaRequestID), "X-Olla-Request-ID should be preserved")
			assert.Equal(t, "test-backend", rec.Header().Get(constants.HeaderXOllaEndpoint), "X-Olla-Endpoint should be preserved")
			assert.Equal(t, "openai", rec.Header().Get(constants.HeaderXOllaBackendType), "X-Olla-Backend-Type should be preserved")
			assert.Equal(t, "test-model", rec.Header().Get(constants.HeaderXOllaModel), "X-Olla-Model should be preserved")
			assert.Equal(t, "50ms", rec.Header().Get(constants.HeaderXOllaResponseTime), "X-Olla-Response-Time should be preserved")

			// 7. Verify content type is JSON
			assert.Equal(t, constants.ContentTypeJSON, rec.Header().Get(constants.HeaderContentType), "Content-Type should be application/json")
		})
	}
}

// Verifies that:
// 1. Streaming errors from backend are handled correctly
func TestTranslationHandler_StreamingErrorTranslation(t *testing.T) {
	tests := []struct {
		name          string
		backendStatus int
		errorMessage  string
	}{
		{
			name:          "streaming_404_error",
			backendStatus: 404,
			errorMessage:  "Model not found for streaming",
		},
		{
			name:          "streaming_500_error",
			backendStatus: 500,
			errorMessage:  "Backend streaming error",
		},
		{
			name:          "streaming_503_error",
			backendStatus: 503,
			errorMessage:  "Service temporarily unavailable",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockLogger := &mockStyledLogger{}

			// Track that TransformStreamingResponse was called with error data
			transformStreamingCalled := false
			var receivedErrorData string

			// Create Anthropic translator with streaming support
			trans := &mockTranslator{
				name:                  "anthropic",
				implementsErrorWriter: true,
				writeErrorFunc: func(w http.ResponseWriter, err error, statusCode int) {
					w.Header().Set(constants.HeaderContentType, constants.ContentTypeJSON)
					w.WriteHeader(statusCode)
					json.NewEncoder(w).Encode(map[string]interface{}{
						"error": err.Error(),
					})
				},
				transformRequestFunc: func(ctx context.Context, r *http.Request) (*translator.TransformedRequest, error) {
					return &translator.TransformedRequest{
						OpenAIRequest: map[string]interface{}{
							"model":  "test-model",
							"stream": true,
							"messages": []interface{}{
								map[string]interface{}{
									"role":    "user",
									"content": "test",
								},
							},
						},
						ModelName:   "test-model",
						IsStreaming: true,
					}, nil
				},
				transformStreamingFunc: func(ctx context.Context, openaiStream io.Reader, w http.ResponseWriter, original *http.Request) error {
					transformStreamingCalled = true

					// Read the error data from the stream (in real scenario this would be SSE error events)
					data, _ := io.ReadAll(openaiStream)
					receivedErrorData = string(data)

					// In a real translator, this would parse the error and write Anthropic format
					// For this test, we just verify the error data was received
					w.Header().Set(constants.HeaderContentType, "text/event-stream")
					w.WriteHeader(http.StatusOK)

					// Write error event in streaming format
					_, err := w.Write([]byte("event: error\ndata: " + receivedErrorData + "\n\n"))
					return err
				},
			}

			// Mock proxy that writes error to stream (simulating SSE error event)
			mockProxy := &mockProxyService{
				proxyFunc: func(ctx context.Context, w http.ResponseWriter, r *http.Request, endpoints []*domain.Endpoint, stats *ports.RequestStats, logger logger.StyledLogger) error {
					// Set X-Olla headers first
					w.Header().Set(constants.HeaderXOllaRequestID, "streaming-error-test")
					w.Header().Set(constants.HeaderXOllaEndpoint, "streaming-backend")
					w.Header().Set(constants.HeaderXOllaBackendType, "openai")
					w.Header().Set(constants.HeaderContentType, "text/event-stream")

					// In streaming mode, we write the error as an SSE event to the stream
					// The translator will then receive this through the pipe
					errorData := fmt.Sprintf(`{"error":{"message":"%s","type":"api_error"}}`, tt.errorMessage)
					_, err := w.Write([]byte(errorData))
					return err
				},
			}

			app := &Application{
				logger:           mockLogger,
				proxyService:     mockProxy,
				statsCollector:   &mockStatsCollector{},
				repository:       &mockEndpointRepository{},
				inspectorChain:   inspector.NewChain(mockLogger),
				profileFactory:   &mockProfileFactory{},
				discoveryService: &mockDiscoveryServiceForTranslation{},
				Config:           &config.Config{},
			}

			// Create streaming Anthropic request
			anthropicReq := map[string]interface{}{
				"model":      "claude-3-5-sonnet-20241022",
				"max_tokens": 1024,
				"stream":     true,
				"messages": []map[string]interface{}{
					{"role": "user", "content": "hello"},
				},
			}
			reqBody, _ := json.Marshal(anthropicReq)

			req := httptest.NewRequest("POST", "/olla/anthropic/v1/messages", bytes.NewReader(reqBody))
			req.Header.Set(constants.HeaderContentType, constants.ContentTypeJSON)

			rec := httptest.NewRecorder()

			handler := app.translationHandler(trans)
			handler.ServeHTTP(rec, req)

			// 1. Verify TransformStreamingResponse was called
			assert.True(t, transformStreamingCalled, "TransformStreamingResponse should be called")

			// 2. Verify error data was passed through the stream
			assert.Contains(t, receivedErrorData, tt.errorMessage, "Error message should be in stream data")

			// 3. Verify X-Olla headers preserved during streaming
			assert.Equal(t, "streaming-error-test", rec.Header().Get(constants.HeaderXOllaRequestID), "X-Olla-Request-ID should be preserved")
			assert.Equal(t, "streaming-backend", rec.Header().Get(constants.HeaderXOllaEndpoint), "X-Olla-Endpoint should be preserved")
			assert.Equal(t, "openai", rec.Header().Get(constants.HeaderXOllaBackendType), "X-Olla-Backend-Type should be preserved")

			// 4. Verify streaming response was written
			assert.Equal(t, http.StatusOK, rec.Code, "Streaming should return 200 with error in stream")
			assert.Contains(t, rec.Body.String(), "event: error", "Should contain error event in stream")
		})
	}
}

// Verifies that:
// 1. Debug log is emitted when TargetPath is set
func TestTranslationHandler_PathValidationLogging(t *testing.T) {
	t.Run("with_target_path", func(t *testing.T) {
		mockLogger := &mockStyledLogger{}

		trans := &mockTranslator{
			name: "test-translator-with-path",
			transformRequestFunc: func(ctx context.Context, r *http.Request) (*translator.TransformedRequest, error) {
				return &translator.TransformedRequest{
					OpenAIRequest: map[string]interface{}{
						"model": "test-model",
						"messages": []interface{}{
							map[string]interface{}{
								"role":    "user",
								"content": "test",
							},
						},
					},
					ModelName:   "test-model",
					IsStreaming: false,
					TargetPath:  "/v1/chat/completions", // Set target path
				}, nil
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

		reqBody := map[string]interface{}{"model": "test-model"}
		body, _ := json.Marshal(reqBody)
		req := httptest.NewRequest("POST", "/v1/messages", bytes.NewReader(body))
		req.Header.Set(constants.HeaderContentType, constants.ContentTypeJSON)

		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		// For this test, we're verifying the handler completes successfully with TargetPath set
	})

	t.Run("without_target_path_non_passthrough", func(t *testing.T) {
		mockLogger := &mockStyledLogger{}

		trans := &mockTranslator{
			name: "test-translator-no-path",
			transformRequestFunc: func(ctx context.Context, r *http.Request) (*translator.TransformedRequest, error) {
				return &translator.TransformedRequest{
					OpenAIRequest: map[string]interface{}{
						"model": "test-model",
						"messages": []interface{}{
							map[string]interface{}{
								"role":    "user",
								"content": "test",
							},
						},
					},
					ModelName:   "test-model",
					IsStreaming: false,
					TargetPath:  "", // No target path - should trigger warning
				}, nil
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

		reqBody := map[string]interface{}{"model": "test-model"}
		body, _ := json.Marshal(reqBody)
		req := httptest.NewRequest("POST", "/v1/messages", bytes.NewReader(body))
		req.Header.Set(constants.HeaderContentType, constants.ContentTypeJSON)

		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		// For this test, we're verifying the handler completes successfully despite missing TargetPath
	})

	t.Run("passthrough_without_target_path_no_warning", func(t *testing.T) {
		mockLogger := &mockStyledLogger{}

		trans := &mockTranslator{
			name: "passthrough",
			transformRequestFunc: func(ctx context.Context, r *http.Request) (*translator.TransformedRequest, error) {
				return &translator.TransformedRequest{
					OpenAIRequest: map[string]interface{}{
						"model": "test-model",
						"messages": []interface{}{
							map[string]interface{}{
								"role":    "user",
								"content": "test",
							},
						},
					},
					ModelName:   "test-model",
					IsStreaming: false,
					TargetPath:  "", // No target path but translator is "passthrough"
				}, nil
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

		reqBody := map[string]interface{}{"model": "test-model"}
		body, _ := json.Marshal(reqBody)
		req := httptest.NewRequest("POST", "/v1/messages", bytes.NewReader(body))
		req.Header.Set(constants.HeaderContentType, constants.ContentTypeJSON)

		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
	})
}

// Verifies that:
// 1. Correct paths without /olla prefix are unchanged
func TestTranslationHandler_TargetPathPrefixStripping(t *testing.T) {
	tests := []struct {
		name              string
		targetPath        string
		expectedFinalPath string
		shouldWarn        bool
	}{
		{
			name:              "correct_path_no_prefix",
			targetPath:        "/v1/chat/completions",
			expectedFinalPath: "/v1/chat/completions",
			shouldWarn:        false,
		},
		{
			name:              "incorrect_path_with_olla_prefix",
			targetPath:        constants.DefaultOllaProxyPathPrefix + "v1/chat/completions",
			expectedFinalPath: "/v1/chat/completions",
			shouldWarn:        true,
		},
		{
			name:              "path_with_only_prefix",
			targetPath:        constants.DefaultOllaProxyPathPrefix,
			expectedFinalPath: "/",
			shouldWarn:        true,
		},
		{
			name:              "empty_path",
			targetPath:        "",
			expectedFinalPath: "", // Should use original path
			shouldWarn:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockLogger := &mockStyledLogger{}

			// Track the actual path received by the proxy
			var receivedPath string
			mockTrans := &mockTranslator{
				name: "test-translator",
				transformRequestFunc: func(ctx context.Context, r *http.Request) (*translator.TransformedRequest, error) {
					return &translator.TransformedRequest{
						OpenAIRequest: map[string]interface{}{
							"model":    "test-model",
							"messages": []map[string]interface{}{},
						},
						TargetPath:  tt.targetPath,
						ModelName:   "test-model",
						IsStreaming: false,
					}, nil
				},
			}

			mockProxy := &mockProxyService{
				proxyFunc: func(ctx context.Context, w http.ResponseWriter, r *http.Request, endpoints []*domain.Endpoint, stats *ports.RequestStats, logger logger.StyledLogger) error {
					receivedPath = r.URL.Path

					response := map[string]interface{}{
						"id":      "test-id",
						"choices": []interface{}{},
					}
					w.Header().Set(constants.HeaderContentType, constants.ContentTypeJSON)
					w.WriteHeader(http.StatusOK)
					return json.NewEncoder(w).Encode(response)
				},
			}

			app := &Application{
				proxyService:     mockProxy,
				logger:           mockLogger,
				statsCollector:   &mockStatsCollector{},
				repository:       &mockEndpointRepository{},
				inspectorChain:   inspector.NewChain(mockLogger),
				profileFactory:   &mockProfileFactory{},
				discoveryService: &mockDiscoveryServiceForTranslation{},
				Config:           &config.Config{},
			}

			reqBody := []byte(`{"model": "test", "messages": []}`)
			req := httptest.NewRequest("POST", "/olla/test/v1/messages", bytes.NewReader(reqBody))
			req.Header.Set(constants.HeaderContentType, constants.ContentTypeJSON)

			w := httptest.NewRecorder()

			handler := app.translationHandler(mockTrans)
			handler.ServeHTTP(w, req)

			if tt.targetPath != "" {
				assert.Equal(t, tt.expectedFinalPath, receivedPath,
					"Path should be corrected to remove /olla prefix")
			}

			assert.Equal(t, http.StatusOK, w.Code)
		})
	}
}

// ensures utility function behaves as expected
func TestStripPrefixBehavior(t *testing.T) {
	// Import util package
	utilTests := []struct {
		name     string
		path     string
		prefix   string
		expected string
	}{
		{
			name:     "strip_olla_prefix",
			path:     "/olla/v1/chat/completions",
			prefix:   constants.DefaultOllaProxyPathPrefix,
			expected: "/v1/chat/completions",
		},
		{
			name:     "no_prefix_to_strip",
			path:     "/v1/chat/completions",
			prefix:   constants.DefaultOllaProxyPathPrefix,
			expected: "/v1/chat/completions",
		},
		{
			name:     "strip_ensures_leading_slash",
			path:     "/olla/",
			prefix:   constants.DefaultOllaProxyPathPrefix,
			expected: "/",
		},
		{
			name:     "strip_with_missing_slash",
			path:     constants.DefaultOllaProxyPathPrefix + "v1/chat/completions",
			prefix:   constants.DefaultOllaProxyPathPrefix,
			expected: "/v1/chat/completions",
		},
	}

	for _, tt := range utilTests {
		t.Run(tt.name, func(t *testing.T) {
			result := util.StripPrefix(tt.path, tt.prefix)
			assert.Equal(t, tt.expected, result, tt.name)
		})
	}
}

// BenchmarkStripPrefix measures the performance of util.StripPrefix
func BenchmarkStripPrefix(b *testing.B) {
	path := "/olla/v1/chat/completions"
	prefix := constants.DefaultOllaProxyPathPrefix

	b.ResetTimer()
	for range b.N {
		_ = util.StripPrefix(path, prefix)
	}
}

// TestStreamingResponseRecorder_EnsureHeadersReady_IdemPotent verifies that
// ensureHeadersReady can be called multiple times without panicking.
func TestStreamingResponseRecorder_EnsureHeadersReady_IdemPotent(t *testing.T) {
	t.Parallel()

	pr, pw := io.Pipe()
	defer pr.Close()
	defer pw.Close()

	rec := newStreamingResponseRecorder(pw)

	// Calling multiple times must not panic (sync.Once guards the close).
	rec.ensureHeadersReady()
	rec.ensureHeadersReady()
	rec.WriteHeader(http.StatusOK)
	rec.ensureHeadersReady()

	// Channel must already be closed.
	select {
	case <-rec.headersReady:
	default:
		t.Fatal("headersReady should be closed after ensureHeadersReady")
	}
}

// TestExecuteTranslatedStreamingRequest_ProxyErrorBeforeWrite verifies that when the proxy
// returns an error before ever writing headers, the handler does not deadlock and returns an
// error within a reasonable time.
func TestExecuteTranslatedStreamingRequest_ProxyErrorBeforeWrite(t *testing.T) {
	t.Parallel()

	mockLogger := &mockStyledLogger{}

	trans := &mockTranslator{
		name:                  "error-before-write-translator",
		implementsErrorWriter: true,
		writeErrorFunc: func(w http.ResponseWriter, err error, statusCode int) {
			w.Header().Set(constants.HeaderContentType, constants.ContentTypeJSON)
			w.WriteHeader(statusCode)
			json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
		},
		transformRequestFunc: func(ctx context.Context, r *http.Request) (*translator.TransformedRequest, error) {
			return &translator.TransformedRequest{
				OpenAIRequest: map[string]interface{}{
					"model":  "test-model",
					"stream": true,
					"messages": []interface{}{
						map[string]interface{}{"role": "user", "content": "test"},
					},
				},
				ModelName:   "test-model",
				IsStreaming: true,
			}, nil
		},
		// TransformStreamingResponse should never be reached in the error path, but
		// if it somehow is, copy the (empty) stream so the test can complete.
		transformStreamingFunc: func(ctx context.Context, openaiStream io.Reader, w http.ResponseWriter, original *http.Request) error {
			_, err := io.Copy(w, openaiStream)
			return err
		},
	}

	// Proxy that returns an error immediately without touching the ResponseWriter.
	proxyService := &mockProxyService{
		proxyFunc: func(ctx context.Context, w http.ResponseWriter, r *http.Request, endpoints []*domain.Endpoint, stats *ports.RequestStats, rlog logger.StyledLogger) error {
			return errors.New("connection refused")
		},
	}

	app := &Application{
		logger:           mockLogger,
		proxyService:     proxyService,
		statsCollector:   &mockStatsCollector{},
		repository:       &mockEndpointRepository{},
		inspectorChain:   inspector.NewChain(mockLogger),
		profileFactory:   &mockProfileFactory{},
		discoveryService: &mockDiscoveryServiceForTranslation{},
		Config:           &config.Config{},
	}

	handler := app.translationHandler(trans)

	reqBody := map[string]interface{}{
		"model":  "test-model",
		"stream": true,
		"messages": []interface{}{
			map[string]interface{}{"role": "user", "content": "Hello"},
		},
	}
	body, _ := json.Marshal(reqBody)

	// Use a context with a generous timeout so the test fails clearly rather than
	// hanging the suite if the deadlock resurfaces.
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "POST", "/test", bytes.NewReader(body))
	require.NoError(t, err)

	done := make(chan struct{})
	rec := httptest.NewRecorder()
	go func() {
		handler.ServeHTTP(rec, req)
		close(done)
	}()

	select {
	case <-done:
		// Handler returned - no deadlock. The response status must indicate an error.
		assert.GreaterOrEqual(t, rec.Code, http.StatusBadRequest,
			"expected an error status when proxy fails before writing headers")
	case <-ctx.Done():
		t.Fatal("handler deadlocked: did not return within timeout after proxy error-before-write")
	}
}

// TestExecuteTranslatedStreamingRequest_ContextCancellationUnblocks verifies that
// cancelling the request context unblocks the headersReady wait even when the proxy
// goroutine stalls indefinitely without writing.
func TestExecuteTranslatedStreamingRequest_ContextCancellationUnblocks(t *testing.T) {
	t.Parallel()

	mockLogger := &mockStyledLogger{}

	trans := &mockTranslator{
		name:                  "stalled-translator",
		implementsErrorWriter: true,
		writeErrorFunc: func(w http.ResponseWriter, err error, statusCode int) {
			w.Header().Set(constants.HeaderContentType, constants.ContentTypeJSON)
			w.WriteHeader(statusCode)
			json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
		},
		transformRequestFunc: func(ctx context.Context, r *http.Request) (*translator.TransformedRequest, error) {
			return &translator.TransformedRequest{
				OpenAIRequest: map[string]interface{}{
					"model":  "test-model",
					"stream": true,
					"messages": []interface{}{
						map[string]interface{}{"role": "user", "content": "test"},
					},
				},
				ModelName:   "test-model",
				IsStreaming: true,
			}, nil
		},
	}

	// Proxy that blocks until its context is cancelled without writing anything.
	proxyService := &mockProxyService{
		proxyFunc: func(ctx context.Context, w http.ResponseWriter, r *http.Request, endpoints []*domain.Endpoint, stats *ports.RequestStats, rlog logger.StyledLogger) error {
			<-ctx.Done()
			return ctx.Err()
		},
	}

	app := &Application{
		logger:           mockLogger,
		proxyService:     proxyService,
		statsCollector:   &mockStatsCollector{},
		repository:       &mockEndpointRepository{},
		inspectorChain:   inspector.NewChain(mockLogger),
		profileFactory:   &mockProfileFactory{},
		discoveryService: &mockDiscoveryServiceForTranslation{},
		Config:           &config.Config{},
	}

	handler := app.translationHandler(trans)

	reqBody := map[string]interface{}{
		"model":  "test-model",
		"stream": true,
		"messages": []interface{}{
			map[string]interface{}{"role": "user", "content": "Hello"},
		},
	}
	body, _ := json.Marshal(reqBody)

	// Cancel the context after a short delay to simulate client disconnect / server timeout.
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "POST", "/test", bytes.NewReader(body))
	require.NoError(t, err)

	done := make(chan struct{})
	rec := httptest.NewRecorder()
	go func() {
		handler.ServeHTTP(rec, req)
		close(done)
	}()

	// Give it a reasonable window beyond the context timeout; if context cancellation
	// correctly unblocks the select, the handler returns well before this deadline.
	select {
	case <-done:
		// Returned after cancellation - correct behaviour.
	case <-time.After(3 * time.Second):
		t.Fatal("handler did not unblock after context cancellation")
	}
}

// TestExecuteTranslatedStreamingRequest_SuccessfulFlow verifies that the happy path
// (proxy writes headers then streams data) still works correctly after the fix.
func TestExecuteTranslatedStreamingRequest_SuccessfulFlow(t *testing.T) {
	t.Parallel()

	mockLogger := &mockStyledLogger{}

	trans := &mockTranslator{
		name:                  "success-translator",
		implementsErrorWriter: true,
		writeErrorFunc: func(w http.ResponseWriter, err error, statusCode int) {
			w.Header().Set(constants.HeaderContentType, constants.ContentTypeJSON)
			w.WriteHeader(statusCode)
			json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
		},
		transformRequestFunc: func(ctx context.Context, r *http.Request) (*translator.TransformedRequest, error) {
			return &translator.TransformedRequest{
				OpenAIRequest: map[string]interface{}{
					"model":  "test-model",
					"stream": true,
					"messages": []interface{}{
						map[string]interface{}{"role": "user", "content": "test"},
					},
				},
				ModelName:   "test-model",
				IsStreaming: true,
			}, nil
		},
		transformStreamingFunc: func(ctx context.Context, openaiStream io.Reader, w http.ResponseWriter, original *http.Request) error {
			w.Header().Set(constants.HeaderContentType, "text/event-stream")
			_, err := io.Copy(w, openaiStream)
			return err
		},
	}

	// Proxy that successfully writes a header then streams a single SSE event.
	proxyService := &mockProxyService{
		proxyFunc: func(ctx context.Context, w http.ResponseWriter, r *http.Request, endpoints []*domain.Endpoint, stats *ports.RequestStats, rlog logger.StyledLogger) error {
			w.Header().Set(constants.HeaderContentType, "text/event-stream")
			w.Header().Set(constants.HeaderXOllaRequestID, "success-flow-id")
			w.WriteHeader(http.StatusOK)
			_, err := w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"Hello\"}}]}\n\n"))
			return err
		},
	}

	app := &Application{
		logger:           mockLogger,
		proxyService:     proxyService,
		statsCollector:   &mockStatsCollector{},
		repository:       &mockEndpointRepository{},
		inspectorChain:   inspector.NewChain(mockLogger),
		profileFactory:   &mockProfileFactory{},
		discoveryService: &mockDiscoveryServiceForTranslation{},
		Config:           &config.Config{},
	}

	handler := app.translationHandler(trans)

	reqBody := map[string]interface{}{
		"model":  "test-model",
		"stream": true,
		"messages": []interface{}{
			map[string]interface{}{"role": "user", "content": "Hello"},
		},
	}
	body, _ := json.Marshal(reqBody)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "POST", "/test", bytes.NewReader(body))
	require.NoError(t, err)

	done := make(chan struct{})
	rec := httptest.NewRecorder()
	go func() {
		handler.ServeHTTP(rec, req)
		close(done)
	}()

	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("handler deadlocked on the success path")
	}

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "text/event-stream", rec.Header().Get(constants.HeaderContentType))
	assert.Contains(t, rec.Body.String(), "Hello", "SSE payload should be forwarded")
	assert.NotEmpty(t, rec.Header().Get(constants.HeaderXOllaRequestID),
		"X-Olla-Request-ID should be copied to the client response")
}

// TestHandleStreamingPanic_Writes502 verifies that a panic inside the streaming
// path produces a 502 response to the client rather than re-panicking (which
// would close the connection without sending any error).
func TestHandleStreamingPanic_Writes502(t *testing.T) {
	t.Parallel()

	mockLogger := &mockStyledLogger{}
	app := &Application{logger: mockLogger}

	trans := &mockTranslator{
		name:                  "panicking-translator",
		implementsErrorWriter: true,
		writeErrorFunc: func(w http.ResponseWriter, err error, statusCode int) {
			w.WriteHeader(statusCode)
		},
	}

	// Set up a pipe and a buffered error channel as the real code does.
	pipeReader, pipeWriter := io.Pipe()
	proxyErrChan := make(chan error, 1)

	// Pre-fill the error channel so the drain in handleStreamingPanic does not block.
	proxyErrChan <- nil

	rec := httptest.NewRecorder()

	// committedResponseWriter with committed=false simulates a panic before any
	// byte was written to the real client - response is uncommitted.
	cw := newCommittedResponseWriter(rec)

	// Call handleStreamingPanic directly, simulating the deferred call after a panic.
	// We wrap in a function that sets up a panic so recover() fires.
	func() {
		defer app.handleStreamingPanic(cw, pipeReader, pipeWriter, proxyErrChan, cw, &proxyRequest{
			requestLogger: mockLogger,
		}, trans)
		panic("simulated streaming panic")
	}()

	// The panic must NOT propagate - if it did, the test would fail here.
	assert.Equal(t, http.StatusBadGateway, rec.Code,
		"streaming panic must produce 502 not a connection reset")
}

// TestHandleStreamingPanic_AfterStreamStarted_EmitsSSEError verifies that when a
// panic fires after the upstream has already written body bytes, the handler emits
// a well-formed Anthropic SSE error event rather than attempting a plain HTTP 502
// (which net/http silently ignores once the response is committed).
func TestHandleStreamingPanic_AfterStreamStarted_EmitsSSEError(t *testing.T) {
	t.Parallel()

	mockLogger := &mockStyledLogger{}
	app := &Application{logger: mockLogger}

	trans := &mockTranslator{
		name:                  "panicking-translator",
		implementsErrorWriter: true,
		writeErrorFunc: func(w http.ResponseWriter, err error, statusCode int) {
			w.WriteHeader(statusCode)
		},
	}

	pipeReader, pipeWriter := io.Pipe()
	proxyErrChan := make(chan error, 1)
	proxyErrChan <- nil

	rec := httptest.NewRecorder()

	// committedResponseWriter with committed=true simulates the state after at
	// least one byte has been written to the real client ResponseWriter.
	cw := newCommittedResponseWriter(rec)
	cw.committed.Store(true)

	func() {
		defer app.handleStreamingPanic(cw, pipeReader, pipeWriter, proxyErrChan, cw, &proxyRequest{
			requestLogger: mockLogger,
		}, trans)
		panic("simulated mid-stream panic")
	}()

	body := rec.Body.String()
	// Status must remain 200 - WriteHeader(502) is a no-op after the first Write.
	assert.Equal(t, http.StatusOK, rec.Code,
		"status must stay 200 when the panic fires after the stream has started")
	assert.Contains(t, body, "event: error",
		"a spec-valid SSE error line must be appended mid-stream")
	assert.Contains(t, body, "data: ",
		"error event must include a data line")
	assert.Contains(t, body, "api_error",
		"Anthropic error type must appear in the SSE data payload")
	assert.Contains(t, body, "internal error during stream transformation",
		"error message must appear in the SSE data payload")
	// Ensure no raw JSON is injected outside the SSE envelope.
	assert.NotContains(t, body, `{"error":`,
		"raw JSON error must not be injected outside the SSE envelope")
}

// TestStreamingPanic_BeforeWrite_Returns502 exercises the full translation handler
// end-to-end with a panic that fires before any SSE data is written to the client.
// The response must be HTTP 502 with an Anthropic-formatted JSON error body.
func TestStreamingPanic_BeforeWrite_Returns502(t *testing.T) {
	t.Parallel()

	mockLogger := &mockStyledLogger{}

	// The translator panics inside TransformStreamingResponse before writing
	// anything to w. Because no bytes reach the client, the response is
	// uncommitted and a 502 can be written.
	panicTrans := &mockTranslator{
		name:                  "early-panic-translator",
		implementsErrorWriter: true,
		writeErrorFunc: func(w http.ResponseWriter, err error, statusCode int) {
			w.Header().Set(constants.HeaderContentType, constants.ContentTypeJSON)
			w.WriteHeader(statusCode)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"type": "error",
				"error": map[string]interface{}{
					"type":    "api_error",
					"message": err.Error(),
				},
			})
		},
		transformRequestFunc: func(ctx context.Context, r *http.Request) (*translator.TransformedRequest, error) {
			return &translator.TransformedRequest{
				OpenAIRequest: map[string]interface{}{
					"model":  "test-model",
					"stream": true,
					"messages": []interface{}{
						map[string]interface{}{"role": "user", "content": "test"},
					},
				},
				ModelName:   "test-model",
				IsStreaming: true,
			}, nil
		},
		transformStreamingFunc: func(ctx context.Context, openaiStream io.Reader, w http.ResponseWriter, original *http.Request) error {
			// Panic before writing anything to w - stream is not started.
			panic("panic before any write")
		},
	}

	// Proxy does NOT write any body data (no Write call, only WriteHeader), so
	// streamRecorder.started remains false when the panic fires.
	proxyService := &mockProxyService{
		proxyFunc: func(ctx context.Context, w http.ResponseWriter, r *http.Request, endpoints []*domain.Endpoint, stats *ports.RequestStats, rlog logger.StyledLogger) error {
			w.Header().Set(constants.HeaderContentType, "text/event-stream")
			w.WriteHeader(http.StatusOK)
			// No Write call - started remains false.
			return nil
		},
	}

	app := &Application{
		logger:           mockLogger,
		proxyService:     proxyService,
		statsCollector:   &mockStatsCollector{},
		repository:       &mockEndpointRepository{},
		inspectorChain:   inspector.NewChain(mockLogger),
		profileFactory:   &mockProfileFactory{},
		discoveryService: &mockDiscoveryServiceForTranslation{},
		Config:           &config.Config{},
	}

	handler := app.translationHandler(panicTrans)

	reqBody, _ := json.Marshal(map[string]interface{}{
		"model":  "test-model",
		"stream": true,
		"messages": []interface{}{
			map[string]interface{}{"role": "user", "content": "test"},
		},
	})
	req := httptest.NewRequest("POST", "/test", bytes.NewReader(reqBody))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadGateway, rec.Code,
		"502 must be returned when the panic fires before any bytes are written")

	var errBody map[string]interface{}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &errBody),
		"error body must be valid JSON")
	assert.Equal(t, "error", errBody["type"],
		"Anthropic error envelope type must be 'error'")
	errObj, ok := errBody["error"].(map[string]interface{})
	require.True(t, ok, "error object must be present")
	assert.Equal(t, "api_error", errObj["type"])
}

// TestCommittedResponseWriter_FlushSetsCommitted verifies that Flush() marks the response
// as committed. Without this the translated SSE path calls http.NewResponseController(cw).Flush()
// which can't find http.Flusher on the wrapper, returns "feature not supported", and the
// streaming handler treats the flush failure as a transform error (502).
func TestCommittedResponseWriter_FlushSetsCommitted(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	cw := newCommittedResponseWriter(rec)

	if cw.committed.Load() {
		t.Fatal("committed must be false before any write")
	}

	rc := http.NewResponseController(cw)
	if err := rc.Flush(); err != nil {
		t.Fatalf("Flush via ResponseController must succeed, got: %v", err)
	}

	if !cw.committed.Load() {
		t.Fatal("committed must be true after Flush()")
	}
}

// TestCommittedResponseWriter_Unwrap verifies that the underlying ResponseWriter is
// reachable via Unwrap so ResponseController can discover optional interfaces
// (SetWriteDeadline, etc.) beyond what committedResponseWriter explicitly proxies.
func TestCommittedResponseWriter_Unwrap(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	cw := newCommittedResponseWriter(rec)

	if cw.Unwrap() != rec {
		t.Fatal("Unwrap must return the exact underlying ResponseWriter")
	}
}

// TestTokenCountHandler_SerialiseFailureReturns500 verifies that when a translator
// implements TokenCountSerializer but SerialiseCountTokens returns an error, the
// handler must NOT commit a 200 before the failure. Before the fix, WriteHeader(200)
// was called unconditionally before serialisation, so the client received a 200 with
// an empty or partial body.
func TestTokenCountHandler_SerialiseFailureReturns500(t *testing.T) {
	t.Parallel()

	mockLogger := &mockStyledLogger{}
	serErr := errors.New("json serialisation kaboom")

	trans := &mockTranslator{
		name:                      "anthropic",
		implementsErrorWriter:     true,
		implementsTokenCounter:    true,
		implementsTokenSerializer: true,
		writeErrorFunc: func(w http.ResponseWriter, err error, statusCode int) {
			w.Header().Set(constants.HeaderContentType, constants.ContentTypeJSON)
			w.WriteHeader(statusCode)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"type": "error",
				"error": map[string]interface{}{
					"type":    "api_error",
					"message": err.Error(),
				},
			})
		},
		countTokensFunc: func(_ context.Context, _ *http.Request) (*translator.TokenCountResponse, error) {
			return &translator.TokenCountResponse{InputTokens: 42}, nil
		},
		serialiseCountTokensFunc: func(_ *translator.TokenCountResponse) ([]byte, error) {
			return nil, serErr
		},
	}

	app := &Application{
		logger: mockLogger,
	}

	handler := app.tokenCountHandler(trans)

	reqBody, _ := json.Marshal(map[string]interface{}{
		"model":    "test-model",
		"messages": []interface{}{map[string]interface{}{"role": "user", "content": "hi"}},
	})
	req := httptest.NewRequest(http.MethodPost, "/olla/anthropic/v1/messages/count_tokens", bytes.NewReader(reqBody))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	// The status must not be 200 - serialisation failed before any bytes were committed.
	if rec.Code == http.StatusOK {
		t.Errorf("expected non-200 status when serialisation fails, got %d (body: %s)", rec.Code, rec.Body.String())
	}
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rec.Code)
	}
}

// TestTokenCountHandler_SerialiseSuccess verifies the happy path: successful
// serialisation writes the body with a 200 status.
func TestTokenCountHandler_SerialiseSuccess(t *testing.T) {
	t.Parallel()

	mockLogger := &mockStyledLogger{}

	trans := &mockTranslator{
		name:                      "anthropic",
		implementsTokenCounter:    true,
		implementsTokenSerializer: true,
		countTokensFunc: func(_ context.Context, _ *http.Request) (*translator.TokenCountResponse, error) {
			return &translator.TokenCountResponse{InputTokens: 17}, nil
		},
		serialiseCountTokensFunc: func(resp *translator.TokenCountResponse) ([]byte, error) {
			return json.Marshal(map[string]int{"input_tokens": resp.InputTokens})
		},
	}

	app := &Application{logger: mockLogger}
	handler := app.tokenCountHandler(trans)

	reqBody, _ := json.Marshal(map[string]interface{}{
		"model":    "test-model",
		"messages": []interface{}{map[string]interface{}{"role": "user", "content": "hi"}},
	})
	req := httptest.NewRequest(http.MethodPost, "/olla/anthropic/v1/messages/count_tokens", bytes.NewReader(reqBody))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var body map[string]int
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, 17, body["input_tokens"])
}

// TestCommittedResponseWriter_WriteAndWriteHeaderSetCommitted confirms the existing
// committed-flag behaviour for Write and WriteHeader is unaffected by the new methods.
func TestCommittedResponseWriter_WriteAndWriteHeaderSetCommitted(t *testing.T) {
	t.Parallel()

	t.Run("Write", func(t *testing.T) {
		t.Parallel()
		rec := httptest.NewRecorder()
		cw := newCommittedResponseWriter(rec)
		_, err := cw.Write([]byte("hello"))
		require.NoError(t, err)
		if !cw.committed.Load() {
			t.Fatal("committed must be true after Write()")
		}
	})

	t.Run("WriteHeader", func(t *testing.T) {
		t.Parallel()
		rec := httptest.NewRecorder()
		cw := newCommittedResponseWriter(rec)
		cw.WriteHeader(http.StatusOK)
		if !cw.committed.Load() {
			t.Fatal("committed must be true after WriteHeader()")
		}
	})
}
