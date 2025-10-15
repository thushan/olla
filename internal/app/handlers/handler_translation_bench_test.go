package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/thushan/olla/internal/adapter/inspector"
	"github.com/thushan/olla/internal/adapter/translator"
	"github.com/thushan/olla/internal/config"
	"github.com/thushan/olla/internal/core/constants"
	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/core/ports"
	"github.com/thushan/olla/internal/logger"
)

// BenchmarkTranslationHandler_NonStreamingOverhead measures the overhead
// introduced by the translation layer for non-streaming requests
// Target: <5ms overhead as per specification
func BenchmarkTranslationHandler_NonStreamingOverhead(b *testing.B) {
	mockLogger := &mockStyledLogger{}

	trans := &mockTranslator{
		name: "bench-translator",
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
			}, nil
		},
		transformResponseFunc: func(ctx context.Context, openaiResp interface{}, original *http.Request) (interface{}, error) {
			return map[string]interface{}{
				"id":      "bench-response",
				"content": "benchmark response",
			}, nil
		},
	}

	proxyService := &mockProxyService{
		proxyFunc: func(ctx context.Context, w http.ResponseWriter, r *http.Request, endpoints []*domain.Endpoint, stats *ports.RequestStats, logger logger.StyledLogger) error {
			response := map[string]interface{}{
				"id":      "chatcmpl-bench",
				"object":  "chat.completion",
				"created": 1677652288,
				"model":   "test-model",
				"choices": []interface{}{
					map[string]interface{}{
						"index": 0,
						"message": map[string]interface{}{
							"role":    "assistant",
							"content": "Hello!",
						},
						"finish_reason": "stop",
					},
				},
			}

			w.Header().Set(constants.HeaderContentType, constants.ContentTypeJSON)
			w.Header().Set(constants.HeaderXOllaRequestID, "bench-id")
			w.WriteHeader(http.StatusOK)
			return json.NewEncoder(w).Encode(response)
		},
	}

	app := &Application{
		logger:           mockLogger,
		proxyService:     proxyService,
		Config:           &config.Config{},
		inspectorChain:   inspector.NewChain(mockLogger),
		discoveryService: &mockDiscoveryService{},
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
	bodyBytes, _ := json.Marshal(reqBody)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest("POST", "/test", bytes.NewReader(bodyBytes))
		req.Header.Set(constants.HeaderContentType, constants.ContentTypeJSON)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			b.Fatalf("Expected status 200, got %d", rec.Code)
		}
	}
}

// BenchmarkTranslationHandler_StreamingSetup measures the overhead
// of setting up streaming translation (pipe creation, goroutine spawn)
// Target: <2ms setup time as per specification
func BenchmarkTranslationHandler_StreamingSetup(b *testing.B) {
	mockLogger := &mockStyledLogger{}

	trans := &mockTranslator{
		name: "streaming-bench-translator",
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
			w.Header().Set(constants.HeaderContentType, "text/event-stream")
			_, err := io.Copy(w, openaiStream)
			return err
		},
	}

	proxyService := &mockProxyService{
		proxyFunc: func(ctx context.Context, w http.ResponseWriter, r *http.Request, endpoints []*domain.Endpoint, stats *ports.RequestStats, logger logger.StyledLogger) error {
			w.Header().Set(constants.HeaderContentType, "text/event-stream")
			w.Header().Set(constants.HeaderXOllaRequestID, "streaming-bench-id")
			w.WriteHeader(http.StatusOK)
			// Simulate minimal SSE stream
			_, err := w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"Hi\"}}]}\n\n"))
			return err
		},
	}

	app := &Application{
		logger:           mockLogger,
		proxyService:     proxyService,
		Config:           &config.Config{},
		inspectorChain:   inspector.NewChain(mockLogger),
		discoveryService: &mockDiscoveryService{},
	}

	handler := app.translationHandler(trans)

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
	bodyBytes, _ := json.Marshal(reqBody)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest("POST", "/test", bytes.NewReader(bodyBytes))
		req.Header.Set(constants.HeaderContentType, constants.ContentTypeJSON)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			b.Fatalf("Expected status 200, got %d", rec.Code)
		}
	}
}

// Note: Additional benchmarks for request/response transformation and error handling
// can be added as needed for more granular performance analysis
