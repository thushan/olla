package app

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/thushan/olla/internal/config"
	"github.com/thushan/olla/internal/logger"
	"github.com/thushan/olla/theme"
)

func createTestSizeLimitLogger() *logger.StyledLogger {
	loggerCfg := &logger.Config{Level: "error", Theme: "default"}
	log, _, _ := logger.New(loggerCfg)
	return logger.NewStyledLogger(log, theme.Default())
}

func TestNewRequestSizeLimiter(t *testing.T) {
	limits := config.ServerRequestLimits{
		MaxBodySize:   1024,
		MaxHeaderSize: 512,
	}

	rsl := NewRequestSizeLimiter(limits, createTestSizeLimitLogger())

	if rsl.maxBodySize != 1024 {
		t.Errorf("Expected max body size 1024, got %d", rsl.maxBodySize)
	}
	if rsl.maxHeaderSize != 512 {
		t.Errorf("Expected max header size 512, got %d", rsl.maxHeaderSize)
	}
	if rsl.logger == nil {
		t.Error("Expected logger to be set")
	}
}

func TestRequestSizeLimiter_SmallRequest(t *testing.T) {
	limits := config.ServerRequestLimits{
		MaxBodySize:   1024, // 1KB body limit
		MaxHeaderSize: 512,  // 512B header limit
	}

	rsl := NewRequestSizeLimiter(limits, createTestSizeLimitLogger())
	middleware := rsl.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("success"))
	}))

	// Small request that should pass
	body := `{"prompt": "Hello world"}`
	req := httptest.NewRequest("POST", "/test", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.ContentLength = int64(len(body))

	w := httptest.NewRecorder()
	middleware.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}
	if w.Body.String() != "success" {
		t.Errorf("Expected 'success', got %s", w.Body.String())
	}
}

func TestRequestSizeLimiter_BodyTooLarge_ContentLength(t *testing.T) {
	limits := config.ServerRequestLimits{
		MaxBodySize:   100, // Very small limit for testing
		MaxHeaderSize: 1024,
	}

	rsl := NewRequestSizeLimiter(limits, createTestSizeLimitLogger())
	middleware := rsl.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Large body that exceeds limit
	body := strings.Repeat("x", 200) // 200 bytes > 100 byte limit
	req := httptest.NewRequest("POST", "/test", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.ContentLength = int64(len(body))

	w := httptest.NewRecorder()
	middleware.ServeHTTP(w, req)

	if w.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("Expected status 413, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "Request body too large") {
		t.Errorf("Expected 'Request body too large' in response, got %s", w.Body.String())
	}
}

func TestRequestSizeLimiter_BodyTooLarge_MaxBytesReader(t *testing.T) {
	limits := config.ServerRequestLimits{
		MaxBodySize:   50, // Very small limit
		MaxHeaderSize: 1024,
	}

	rsl := NewRequestSizeLimiter(limits, createTestSizeLimitLogger())
	middleware := rsl.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Try to read body to trigger MaxBytesReader
		buf := make([]byte, 100)
		_, err := r.Body.Read(buf)
		if err != nil {
			http.Error(w, "Body read error", http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))

	// Large body with no Content-Length (MaxBytesReader will catch it)
	body := strings.Repeat("y", 100) // 100 bytes > 50 byte limit
	req := httptest.NewRequest("POST", "/test", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	// Don't set Content-Length to test MaxBytesReader path

	w := httptest.NewRecorder()
	middleware.ServeHTTP(w, req)

	// Should either be caught by Content-Length check or by reading the body
	if w.Code != http.StatusRequestEntityTooLarge && w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 413 or 400, got %d", w.Code)
	}
}

func TestRequestSizeLimiter_HeadersTooLarge(t *testing.T) {
	limits := config.ServerRequestLimits{
		MaxBodySize:   1024,
		MaxHeaderSize: 100, // Very small header limit for testing
	}

	rsl := NewRequestSizeLimiter(limits, createTestSizeLimitLogger())
	middleware := rsl.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("POST", "/test", strings.NewReader("small body"))
	req.Header.Set("Content-Type", "application/json")
	// Add a large header that will exceed the 100 byte limit
	req.Header.Set("X-Large-Header", strings.Repeat("z", 200))

	w := httptest.NewRecorder()
	middleware.ServeHTTP(w, req)

	if w.Code != http.StatusRequestHeaderFieldsTooLarge {
		t.Errorf("Expected status 431, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "Request headers too large") {
		t.Errorf("Expected 'Request headers too large' in response, got %s", w.Body.String())
	}
}

func TestRequestSizeLimiter_MultipleHeaders(t *testing.T) {
	limits := config.ServerRequestLimits{
		MaxBodySize:   1024,
		MaxHeaderSize: 200, // Small but reasonable limit
	}

	rsl := NewRequestSizeLimiter(limits, createTestSizeLimitLogger())
	middleware := rsl.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("POST", "/test", strings.NewReader("body"))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+strings.Repeat("x", 50))
	req.Header.Set("X-Custom-Header", strings.Repeat("y", 50))
	req.Header.Set("User-Agent", "TestAgent/1.0")

	w := httptest.NewRecorder()
	middleware.ServeHTTP(w, req)

	if w.Code != http.StatusRequestHeaderFieldsTooLarge {
		t.Errorf("Expected status 431, got %d", w.Code)
	}
}

func TestRequestSizeLimiter_ZeroLimits_Disabled(t *testing.T) {
	limits := config.ServerRequestLimits{
		MaxBodySize:   0, // 0 = disabled
		MaxHeaderSize: 0, // 0 = disabled
	}

	rsl := NewRequestSizeLimiter(limits, createTestSizeLimitLogger())
	middleware := rsl.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("success"))
	}))

	// Very large request that would normally be rejected
	body := strings.Repeat("x", 10000) // 10KB body
	req := httptest.NewRequest("POST", "/test", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Huge-Header", strings.Repeat("z", 5000)) // 5KB header
	req.ContentLength = int64(len(body))

	w := httptest.NewRecorder()
	middleware.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200 (limits disabled), got %d", w.Code)
	}
	if w.Body.String() != "success" {
		t.Errorf("Expected 'success', got %s", w.Body.String())
	}
}

func TestRequestSizeLimiter_NegativeLimits_Disabled(t *testing.T) {
	limits := config.ServerRequestLimits{
		MaxBodySize:   -1,   // Negative = disabled
		MaxHeaderSize: -100, // Negative = disabled
	}

	rsl := NewRequestSizeLimiter(limits, createTestSizeLimitLogger())
	middleware := rsl.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("success"))
	}))

	// Large request that should pass when limits are disabled
	body := strings.Repeat("x", 5000)
	req := httptest.NewRequest("POST", "/test", strings.NewReader(body))
	req.Header.Set("X-Large-Header", strings.Repeat("z", 2000))
	req.ContentLength = int64(len(body))

	w := httptest.NewRecorder()
	middleware.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200 (negative limits = disabled), got %d", w.Code)
	}
}

func TestRequestSizeLimiter_EdgeCase_ExactLimit(t *testing.T) {
	limits := config.ServerRequestLimits{
		MaxBodySize:   100, // Exactly 100 bytes
		MaxHeaderSize: 200, // Exactly 200 bytes
	}

	rsl := NewRequestSizeLimiter(limits, createTestSizeLimitLogger())
	middleware := rsl.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("success"))
	}))

	// Body exactly at limit
	body := strings.Repeat("x", 100) // Exactly 100 bytes
	req := httptest.NewRequest("POST", "/test", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.ContentLength = int64(len(body))

	w := httptest.NewRecorder()
	middleware.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200 for request at exact limit, got %d", w.Code)
	}
}

func TestRequestSizeLimiter_EdgeCase_OneByteTooLarge(t *testing.T) {
	limits := config.ServerRequestLimits{
		MaxBodySize:   100,
		MaxHeaderSize: 200,
	}

	rsl := NewRequestSizeLimiter(limits, createTestSizeLimitLogger())
	middleware := rsl.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Body one byte over limit
	body := strings.Repeat("x", 101) // 101 bytes > 100 limit
	req := httptest.NewRequest("POST", "/test", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.ContentLength = int64(len(body))

	w := httptest.NewRecorder()
	middleware.ServeHTTP(w, req)

	if w.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("Expected status 413 for request one byte over limit, got %d", w.Code)
	}
}

func TestRequestSizeLimiter_EmptyBody(t *testing.T) {
	limits := config.ServerRequestLimits{
		MaxBodySize:   100,
		MaxHeaderSize: 200,
	}

	rsl := NewRequestSizeLimiter(limits, createTestSizeLimitLogger())
	middleware := rsl.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("success"))
	}))

	// Empty body should always pass
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	middleware.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200 for empty body, got %d", w.Code)
	}
}

func TestRequestSizeLimiter_HeaderSizeCalculation(t *testing.T) {
	limits := config.ServerRequestLimits{
		MaxBodySize:   1024,
		MaxHeaderSize: 100,
	}

	rsl := NewRequestSizeLimiter(limits, createTestSizeLimitLogger())

	// Test precise header size calculation
	req := httptest.NewRequest("POST", "/test", nil)
	req.Header.Set("Test", "Value") // "Test" (4) + "Value" (5) + ": \r\n" (4) = 13 bytes

	err := rsl.validateHeaderSize(req)
	if err != nil {
		t.Errorf("Small header should not exceed limit, got error: %v", err)
	}

	// Add headers until we exceed the limit
	req.Header.Set("Long-Header-Name", strings.Repeat("x", 50))

	err = rsl.validateHeaderSize(req)
	if err == nil {
		t.Error("Large headers should exceed limit")
	}
}

func TestRequestSizeLimiter_ContentLengthMismatch(t *testing.T) {
	limits := config.ServerRequestLimits{
		MaxBodySize:   50,
		MaxHeaderSize: 200,
	}

	rsl := NewRequestSizeLimiter(limits, createTestSizeLimitLogger())
	middleware := rsl.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Try to read more than Content-Length to test MaxBytesReader
		buf := make([]byte, 100)
		_, err := r.Body.Read(buf)
		if err != nil {
			http.Error(w, "Read error", http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))

	// Small Content-Length but large actual body
	actualBody := strings.Repeat("x", 100) // 100 bytes actual
	req := httptest.NewRequest("POST", "/test", strings.NewReader(actualBody))
	req.ContentLength = 30 // Claim only 30 bytes
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	middleware.ServeHTTP(w, req)

	// Should pass Content-Length check but fail when actually reading body
	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400 when MaxBytesReader catches oversized body, got %d", w.Code)
	}
}

func TestRequestSizeLimiter_ConcurrentRequests(t *testing.T) {
	limits := config.ServerRequestLimits{
		MaxBodySize:   1000,
		MaxHeaderSize: 500,
	}

	rsl := NewRequestSizeLimiter(limits, createTestSizeLimitLogger())
	middleware := rsl.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Test concurrent requests to ensure no race conditions
	var wg sync.WaitGroup
	successCount := int32(0)
	rejectedCount := int32(0)

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			var body string
			if id%2 == 0 {
				body = strings.Repeat("x", 500) // Within limit
			} else {
				body = strings.Repeat("x", 1500) // Exceeds limit
			}

			req := httptest.NewRequest("POST", "/test", strings.NewReader(body))
			req.ContentLength = int64(len(body))
			w := httptest.NewRecorder()

			middleware.ServeHTTP(w, req)

			if w.Code == http.StatusOK {
				atomic.AddInt32(&successCount, 1)
			} else if w.Code == http.StatusRequestEntityTooLarge {
				atomic.AddInt32(&rejectedCount, 1)
			}
		}(i)
	}

	wg.Wait()

	if atomic.LoadInt32(&successCount) != 5 {
		t.Errorf("Expected 5 successful requests, got %d", atomic.LoadInt32(&successCount))
	}
	if atomic.LoadInt32(&rejectedCount) != 5 {
		t.Errorf("Expected 5 rejected requests, got %d", atomic.LoadInt32(&rejectedCount))
	}
}

func TestRequestSizeLimiter_MultiValueHeaders(t *testing.T) {
	limits := config.ServerRequestLimits{
		MaxBodySize:   1024,
		MaxHeaderSize: 150,
	}

	rsl := NewRequestSizeLimiter(limits, createTestSizeLimitLogger())

	req := httptest.NewRequest("POST", "/test", nil)
	// Add multiple values for the same header
	req.Header.Add("Accept", "application/json")
	req.Header.Add("Accept", "text/html")
	req.Header.Add("Accept", "application/xml")

	err := rsl.validateHeaderSize(req)
	if err != nil {
		t.Errorf("Multi-value headers should be calculated correctly, got error: %v", err)
	}

	// Add more headers to exceed limit
	req.Header.Set("Large-Header", strings.Repeat("z", 100))

	err = rsl.validateHeaderSize(req)
	if err == nil {
		t.Error("Large multi-value headers should exceed limit")
	}
}
