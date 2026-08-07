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

func TestCombinedLoggingMiddleware_RequestIDAndContext(t *testing.T) {
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
	middleware := CombinedLoggingMiddleware(mockLogger)
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

func TestCombinedLoggingMiddleware_BasicPassthrough(t *testing.T) {
	// Create a mock styled logger
	mockLogger := &mockStyledLogger{}

	// Create a simple test handler
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("access log test"))
	})

	// Create the middleware
	middleware := CombinedLoggingMiddleware(mockLogger)
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

// TestCombinedLoggingMiddleware_UsesInjectedLogger is the regression test for
// the console and access records silently going through slog.Default()
// instead of the styledLogger passed in - a configured logger (level, theme,
// output target) would have no effect on request logging.
func TestCombinedLoggingMiddleware_UsesInjectedLogger(t *testing.T) {
	var buf strings.Builder
	injected := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	mockLogger := &mockStyledLogger{underlying: injected}

	handler := CombinedLoggingMiddleware(mockLogger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// A plain path outside /api/, /olla/ and /internal/ so neither quiet gate
	// fires and both records use their loud (non-abbreviated) message.
	req := httptest.NewRequest("GET", "/hello", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	out := buf.String()
	if !strings.Contains(out, "Request started") || !strings.Contains(out, "Request completed") {
		t.Errorf("expected both console lifecycle records on the injected logger, got: %q", out)
	}
	if !strings.Contains(out, "Access log") {
		t.Errorf("expected the access log record on the injected logger, got: %q", out)
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

func TestRedactQuery(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		wantHide []string // substrings that must NOT appear in output
		wantKeep []string // substrings that MUST appear in output
	}{
		{
			name:     "empty query",
			input:    "",
			wantKeep: []string{},
		},
		{
			name:     "api_key redacted",
			input:    "api_key=sk-1234",
			wantHide: []string{"sk-1234"},
			wantKeep: []string{"[REDACTED]"},
		},
		{
			name:     "safe param unchanged",
			input:    "safe_param=value",
			wantKeep: []string{"safe_param", "value"},
		},
		{
			name:     "mixed: token redacted, safe param kept",
			input:    "safe_param=value&token=secret",
			wantHide: []string{"secret"},
			wantKeep: []string{"[REDACTED]", "safe_param", "value"},
		},
		{
			name:     "case-insensitive TOKEN",
			input:    "TOKEN=foo",
			wantHide: []string{"foo"},
			wantKeep: []string{"[REDACTED]"},
		},
		{
			name:     "password redacted",
			input:    "user=alice&password=hunter2",
			wantHide: []string{"hunter2"},
			wantKeep: []string{"[REDACTED]", "alice"},
		},
		{
			name:     "access_token redacted",
			input:    "access_token=tok-xyz",
			wantHide: []string{"tok-xyz"},
			wantKeep: []string{"[REDACTED]"},
		},
		{
			name:     "secret redacted",
			input:    "secret=my-secret&page=2",
			wantHide: []string{"my-secret"},
			wantKeep: []string{"[REDACTED]", "page"},
		},
		{
			name:     "auth redacted",
			input:    "auth=bearer-token",
			wantHide: []string{"bearer-token"},
			wantKeep: []string{"[REDACTED]"},
		},
		{
			name:     "multiple sensitive keys all redacted",
			input:    "api_key=k1&token=t2&key=k3",
			wantHide: []string{"k1", "t2", "k3"},
			wantKeep: []string{"[REDACTED]"},
		},
		{
			// %70assword decodes to "password" and must still be redacted.
			name:     "percent-encoded key redacted",
			input:    "%70assword=secret",
			wantHide: []string{"secret"},
			wantKeep: []string{"[REDACTED]"},
		},
		{
			// api%5Fkey decodes to "api_key" (underscore encoded as %5F).
			name:     "encoded underscore in key redacted",
			input:    "api%5Fkey=foo",
			wantHide: []string{"foo"},
			wantKeep: []string{"[REDACTED]"},
		},
		{
			// %ZZ is not a valid percent-escape; must fall back to raw key comparison
			// without panicking. "zzz" is not sensitive so the value passes through.
			name:     "malformed escape falls back to raw key",
			input:    "%ZZname=value",
			wantKeep: []string{"value"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := redactQuery(tt.input)
			for _, hide := range tt.wantHide {
				if strings.Contains(result, hide) {
					t.Errorf("redactQuery(%q) = %q; should not contain %q", tt.input, result, hide)
				}
			}
			for _, keep := range tt.wantKeep {
				if keep != "" && !strings.Contains(result, keep) {
					t.Errorf("redactQuery(%q) = %q; should contain %q", tt.input, result, keep)
				}
			}
		})
	}
}

// TestSanitiseRequestID verifies that inbound X-Request-ID values containing
// log-injection characters or exceeding the length cap are rejected, while
// well-formed IDs pass through unchanged.
func TestSanitiseRequestID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		wantOut string // empty means a fresh ID should be generated (input rejected)
	}{
		{
			name:    "valid alphanumeric ID passes through",
			input:   "abc-def-1234",
			wantOut: "abc-def-1234",
		},
		{
			name:    "valid ID with printable ASCII passes through",
			input:   "req_!@#$%^&*()_+[]{}|;:,.<>?",
			wantOut: "req_!@#$%^&*()_+[]{}|;:,.<>?",
		},
		{
			name:    "CR injection rejected",
			input:   "valid\rmalicious",
			wantOut: "",
		},
		{
			name:    "LF injection rejected",
			input:   "valid\nmalicious",
			wantOut: "",
		},
		{
			name:    "CRLF injection rejected",
			input:   "valid\r\nX-Injected-Header: evil",
			wantOut: "",
		},
		{
			name:    "NUL byte rejected",
			input:   "valid\x00nul",
			wantOut: "",
		},
		{
			name:    "tab rejected",
			input:   "valid\tvalue",
			wantOut: "",
		},
		{
			name:    "space rejected",
			input:   "valid value",
			wantOut: "",
		},
		{
			name:    "DEL (0x7F) rejected",
			input:   "valid\x7Fvalue",
			wantOut: "",
		},
		{
			name:    "exactly maxRequestIDLength accepted",
			input:   strings.Repeat("a", maxRequestIDLength),
			wantOut: strings.Repeat("a", maxRequestIDLength),
		},
		{
			name:    "maxRequestIDLength+1 rejected",
			input:   strings.Repeat("a", maxRequestIDLength+1),
			wantOut: "",
		},
		{
			name:    "empty string returned as-is (caller generates fresh ID)",
			input:   "",
			wantOut: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := sanitiseRequestID(tt.input)
			if got != tt.wantOut {
				t.Errorf("sanitiseRequestID(%q) = %q, want %q", tt.input, got, tt.wantOut)
			}
		})
	}
}

// TestCombinedLoggingMiddleware_InvalidRequestIDReplacedWithGenerated verifies
// that a request carrying an X-Request-ID containing CRLF does not propagate the
// injected value into the response or context; instead a fresh ID is generated.
func TestCombinedLoggingMiddleware_InvalidRequestIDReplacedWithGenerated(t *testing.T) {
	t.Parallel()

	mockLogger := &mockStyledLogger{}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The context request ID must not contain the injected header value.
		ctxID := GetRequestID(r.Context())
		if strings.Contains(ctxID, "injected") {
			t.Errorf("log-injection payload leaked into context request ID: %q", ctxID)
		}
		if strings.ContainsAny(ctxID, "\r\n") {
			t.Errorf("context request ID contains CRLF: %q", ctxID)
		}
		w.WriteHeader(http.StatusOK)
	})

	mw := CombinedLoggingMiddleware(mockLogger)(handler)

	req := httptest.NewRequest(http.MethodGet, "/internal/health", nil)
	req.Header.Set("X-Request-ID", "ok\r\nX-Injected: evil")

	rr := httptest.NewRecorder()
	mw.ServeHTTP(rr, req)

	responseID := rr.Header().Get("X-Olla-Request-ID")
	if strings.ContainsAny(responseID, "\r\n") {
		t.Errorf("response X-Olla-Request-ID contains CRLF: %q", responseID)
	}
	if responseID == "" {
		t.Error("response X-Olla-Request-ID must be non-empty even when the inbound ID is rejected")
	}
}

// TestCombinedLoggingMiddleware_ValidRequestIDPreserved verifies that a clean
// inbound X-Request-ID is echoed in the response header unchanged.
func TestCombinedLoggingMiddleware_ValidRequestIDPreserved(t *testing.T) {
	t.Parallel()

	mockLogger := &mockStyledLogger{}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	mw := CombinedLoggingMiddleware(mockLogger)(handler)

	const cleanID = "my-clean-request-id-abc123"
	req := httptest.NewRequest(http.MethodGet, "/internal/health", nil)
	req.Header.Set("X-Request-ID", cleanID)

	rr := httptest.NewRecorder()
	mw.ServeHTTP(rr, req)

	if got := rr.Header().Get("X-Olla-Request-ID"); got != cleanID {
		t.Errorf("X-Olla-Request-ID = %q, want %q", got, cleanID)
	}
}

// Mock styled logger for testing
type mockStyledLogger struct {
	underlying *slog.Logger
}

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
func (m *mockStyledLogger) GetUnderlying() *slog.Logger {
	if m.underlying != nil {
		return m.underlying
	}
	return slog.Default()
}
func (m *mockStyledLogger) WithRequestID(requestID string) logger.StyledLogger                  { return m }
func (m *mockStyledLogger) InfoConfigChange(oldName, newName string)                            {}
func (m *mockStyledLogger) WithAttrs(attrs ...slog.Attr) logger.StyledLogger                    { return m }
func (m *mockStyledLogger) With(args ...any) logger.StyledLogger                                { return m }
func (m *mockStyledLogger) InfoWithContext(msg string, endpoint string, ctx logger.LogContext)  {}
func (m *mockStyledLogger) WarnWithContext(msg string, endpoint string, ctx logger.LogContext)  {}
func (m *mockStyledLogger) ErrorWithContext(msg string, endpoint string, ctx logger.LogContext) {}

// TestAccessLogLevel pins the level the access log resolves to per outcome.
// The access log is the operator's audit record, so it must NOT quiet proxy
// traffic: a proxy success now logs at Info (the regression this test guards
// against is the proxy-success-quieting that previously demoted it to Debug),
// and a proxy 4xx/5xx stays at Info. Only successful /internal/ polling is
// demoted to Debug. See accessLogLevel / isQuietAccessOutcome in logging.go.
//
// Note on 5xx mapping: accessLogLevel is intentionally binary (Debug for
// routine traffic, Info otherwise). Promoting 5xx to Warn/Error is a
// behaviour change beyond this fix's scope; the operator-visible requirement
// is "loud at default verbosity", which Info already satisfies.
func TestAccessLogLevel(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		method string
		path   string
		status int
		want   slog.Level
	}{
		// Proxy traffic: never quietened in the access log. Success is the
		// audit-worthy outcome the security-practices doc relies on.
		{"proxy success", http.MethodPost, "/olla/proxy/v1/chat/completions", http.StatusOK, slog.LevelInfo},
		{"proxy not modified", http.MethodGet, "/olla/proxy/v1/models", http.StatusNotModified, slog.LevelInfo},
		{"proxy server error", http.MethodPost, "/olla/proxy/v1/chat/completions", http.StatusInternalServerError, slog.LevelInfo},
		{"proxy client error", http.MethodPost, "/olla/proxy/v1/chat/completions", http.StatusBadRequest, slog.LevelInfo},
		{"/api/ proxy success", http.MethodPost, "/api/chat", http.StatusOK, slog.LevelInfo},

		// /internal/ polling: still demoted when routine.
		{"internal poll success", http.MethodGet, "/internal/health", http.StatusOK, slog.LevelDebug},
		{"internal poll not modified", http.MethodGet, "/internal/health", http.StatusNotModified, slog.LevelDebug},
		{"internal poll not found", http.MethodGet, "/internal/health", http.StatusNotFound, slog.LevelInfo},
		{"internal poll server error", http.MethodGet, "/internal/status", http.StatusInternalServerError, slog.LevelInfo},
		{"internal poll forbidden", http.MethodGet, "/internal/ui/", http.StatusForbidden, slog.LevelInfo},
		// Non-GET/HEAD under /internal/ is never routine polling.
		{"internal POST success", http.MethodPost, "/internal/status", http.StatusOK, slog.LevelInfo},

		// Everything else logs at Info.
		{"non-proxy non-internal success", http.MethodGet, "/version", http.StatusOK, slog.LevelInfo},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := accessLogLevel(tc.method, tc.path, tc.status)
			if got != tc.want {
				t.Errorf("accessLogLevel(%q, %q, %d) = %v, want %v",
					tc.method, tc.path, tc.status, got, tc.want)
			}
		})
	}
}

// TestConsolePollOutcomeStillQuietsProxy pins the invariant that the console
// log path (isQuietPollOutcome) is SEPARATE from the access log and still
// quietens proxy traffic regardless of status. The console has dedicated
// proxy-handler logging of its own; do not let access-log fixes leak across.
func TestConsolePollOutcomeStillQuietsProxy(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		method string
		path   string
		status int
		want   bool
	}{
		{"proxy success quieted on console", http.MethodPost, "/olla/proxy/v1/chat/completions", http.StatusOK, true},
		{"proxy 500 still quieted on console", http.MethodPost, "/olla/proxy/v1/chat/completions", http.StatusInternalServerError, true},
		{"internal poll success quieted", http.MethodGet, "/internal/health", http.StatusOK, true},
		{"internal poll 500 not quieted", http.MethodGet, "/internal/status", http.StatusInternalServerError, false},
		{"non-proxy non-internal not quieted", http.MethodGet, "/version", http.StatusOK, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := isQuietPollOutcome(tc.method, tc.path, tc.status)
			if got != tc.want {
				t.Errorf("isQuietPollOutcome(%q, %q, %d) = %v, want %v",
					tc.method, tc.path, tc.status, got, tc.want)
			}
		})
	}
}
