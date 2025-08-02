package middleware

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/logger"
)

func TestEnhancedLoggingMiddleware(t *testing.T) {
	// Create a mock styled logger
	mockLogger := &mockStyledLogger{}

	// Create a test handler that uses the context logger
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Test that we can get the logger from context
		ctxLogger := GetLogger(r.Context())
		if ctxLogger == nil {
			t.Error("Expected context logger to be available")
			return
		}

		// Test that we can get the request ID from context
		requestID := GetRequestID(r.Context())
		if requestID == "" {
			t.Error("Expected request ID to be available")
			return
		}

		// Log something with the context logger
		ctxLogger.Info("Test handler executed", "request_id", requestID)

		w.WriteHeader(http.StatusOK)
		w.Write([]byte("test response"))
	})

	// Create the middleware
	middleware := EnhancedLoggingMiddleware(mockLogger)
	handler := middleware(testHandler)

	// Create a test request
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-Request-ID", "test-request-123")

	// Create a response recorder
	rr := httptest.NewRecorder()

	// Execute the request
	handler.ServeHTTP(rr, req)

	// Verify response
	if rr.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rr.Code)
	}

	// Verify that the request ID header was set
	responseRequestID := rr.Header().Get("X-Olla-Request-ID")
	if responseRequestID != "test-request-123" {
		t.Errorf("Expected X-Olla-Request-ID header to be 'test-request-123', got '%s'", responseRequestID)
	}

	// Verify response body
	expectedBody := "test response"
	if rr.Body.String() != expectedBody {
		t.Errorf("Expected body %q, got %q", expectedBody, rr.Body.String())
	}
}

func TestAccessLoggingMiddleware(t *testing.T) {
	// Create a mock styled logger
	mockLogger := &mockStyledLogger{}

	// Create a simple test handler
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("access log test"))
	})

	// Create the middleware
	middleware := AccessLoggingMiddleware(mockLogger)
	handler := middleware(testHandler)

	// Create a test request
	req := httptest.NewRequest("POST", "/api/test?param=value", strings.NewReader("test body"))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "test-agent")
	req.ContentLength = 9 // length of "test body"

	// Create a response recorder
	rr := httptest.NewRecorder()

	// Execute the request
	handler.ServeHTTP(rr, req)

	// Verify response
	if rr.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rr.Code)
	}

	expectedBody := "access log test"
	if rr.Body.String() != expectedBody {
		t.Errorf("Expected body %q, got %q", expectedBody, rr.Body.String())
	}
}

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		input    int64
		expected string
	}{
		{0, "0B"},
		{500, "500B"},
		{1024, "1.0KB"},
		{1536, "1.5KB"},
		{1048576, "1.0MB"},
		{1073741824, "1.0GB"},
		{1099511627776, "1.0TB"},
	}

	for _, test := range tests {
		result := FormatBytes(test.input)
		if result != test.expected {
			t.Errorf("FormatBytes(%d) = %s, want %s", test.input, result, test.expected)
		}
	}
}

func TestGetLoggerWithoutContext(t *testing.T) {
	ctx := context.Background()
	logger := GetLogger(ctx)

	// Should return the default logger when no logger is in context
	if logger == nil {
		t.Error("Expected default logger when no logger in context")
	}
}

func TestGetRequestIDWithoutContext(t *testing.T) {
	ctx := context.Background()
	requestID := GetRequestID(ctx)

	// Should return empty string when no request ID in context
	if requestID != "" {
		t.Errorf("Expected empty request ID when not in context, got %s", requestID)
	}
}

// Mock styled logger for testing
type mockStyledLogger struct{}

func (m *mockStyledLogger) Debug(msg string, args ...any)                                {}
func (m *mockStyledLogger) Info(msg string, args ...any)                                 {}
func (m *mockStyledLogger) Warn(msg string, args ...any)                                 {}
func (m *mockStyledLogger) Error(msg string, args ...any)                                {}
func (m *mockStyledLogger) ResetLine()                                                   {}
func (m *mockStyledLogger) InfoWithStatus(msg string, status string, args ...any)        {}
func (m *mockStyledLogger) InfoWithCount(msg string, count int, args ...any)             {}
func (m *mockStyledLogger) InfoWithEndpoint(msg string, endpoint string, args ...any)    {}
func (m *mockStyledLogger) InfoWithHealthCheck(msg string, endpoint string, args ...any) {}
func (m *mockStyledLogger) InfoWithNumbers(msg string, numbers ...int64)                 {}
func (m *mockStyledLogger) WarnWithEndpoint(msg string, endpoint string, args ...any)    {}
func (m *mockStyledLogger) ErrorWithEndpoint(msg string, endpoint string, args ...any)   {}
func (m *mockStyledLogger) InfoHealthy(msg string, endpoint string, args ...any)         {}
func (m *mockStyledLogger) InfoHealthStatus(msg string, name string, status domain.EndpointStatus, args ...any) {
}
func (m *mockStyledLogger) GetUnderlying() *slog.Logger                                         { return slog.Default() }
func (m *mockStyledLogger) WithRequestID(requestID string) logger.StyledLogger                  { return m }
func (m *mockStyledLogger) InfoConfigChange(oldName, newName string)                            {}
func (m *mockStyledLogger) WithAttrs(attrs ...slog.Attr) logger.StyledLogger                    { return m }
func (m *mockStyledLogger) With(args ...any) logger.StyledLogger                                { return m }
func (m *mockStyledLogger) InfoWithContext(msg string, endpoint string, ctx logger.LogContext)  {}
func (m *mockStyledLogger) WarnWithContext(msg string, endpoint string, ctx logger.LogContext)  {}
func (m *mockStyledLogger) ErrorWithContext(msg string, endpoint string, ctx logger.LogContext) {}
