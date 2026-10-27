package handlers

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thushan/olla/internal/adapter/security"
	"github.com/thushan/olla/internal/adapter/translator"
	"github.com/thushan/olla/internal/config"
	"github.com/thushan/olla/internal/core/constants"
	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/logger"
	"github.com/thushan/olla/internal/router"
	"github.com/thushan/olla/theme"
)

// plainTranslator hides everything but the base translator interface and a path, standing
// in for a translator added later that has no custom error format of its own.
type plainTranslator struct {
	translator.RequestTranslator
}

func (plainTranslator) GetAPIPath() string { return "/olla/plain/v1/messages" }

// translatorSecurity builds adapters that allow `burst` rapid requests per client before
// limiting, and cap bodies at maxBody bytes.
func translatorSecurity(t *testing.T, burst int, maxBody int64) *security.Adapters {
	t.Helper()

	cfg := &config.Config{Server: config.ServerConfig{
		RateLimits: config.ServerRateLimits{
			GlobalRequestsPerMinute: 100000,
			PerIPRequestsPerMinute:  60,
			BurstSize:               burst,
		},
		RequestLimits: config.ServerRequestLimits{MaxBodySize: maxBody},
	}}
	_, adapters := security.NewSecurityServices(cfg, &mockStatsCollector{}, &mockStyledLogger{})
	t.Cleanup(adapters.Stop)
	return adapters
}

// wireTranslatorRoutes registers the translator routes of a real engine-backed app and
// wires them through the production security chain.
func wireTranslatorRoutes(t *testing.T, engine string, adapters *security.Adapters) (*router.RouteRegistry, http.Handler) {
	t.Helper()

	ep := liveEndpoint(t, "ollama-host", "ollama", newPathBackend(t, rootChatPath).URL, 50)
	f := newTranslationPathApp(t, engine, false, false, []*domain.Endpoint{ep}, map[string][]*domain.Endpoint{"m": {ep}})

	reg := router.NewRouteRegistry(&mockStyledLogger{})
	f.app.routeRegistry = reg
	f.app.translatorRegistry = translator.NewRegistry(&mockStyledLogger{})
	f.app.translatorRegistry.Register("anthropic", f.trans)
	f.app.translatorRegistry.Register("plain", plainTranslator{f.trans})
	f.app.registerTranslatorRoutes()

	mux := http.NewServeMux()
	// The access-log middleware needs a real slog logger underneath.
	accessLog := logger.NewStyledLogger(slog.New(slog.NewTextHandler(io.Discard, nil)), &theme.Theme{}, false)
	reg.WireUpWithSecurityChain(mux, &SecurityAdapters{securityAdapters: adapters, logger: accessLog})
	return reg, mux
}

func translatorRequest(method, path, body string) *http.Request {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set(constants.HeaderContentType, constants.ContentTypeJSON)
	req.RemoteAddr = "203.0.113.9:5000"
	return req
}

const translatorBody = `{"model":"m","max_tokens":16,"messages":[{"role":"user","content":"hi"}]}`

// Every translator route is an inference entry point and has to sit behind the limiter.
// The loop reads the route table, so a translator registered later is covered without
// anyone remembering to add it here.
func TestTranslatorRoutes_AreRateLimited(t *testing.T) {
	t.Parallel()

	for _, engine := range translationEngines {
		t.Run(engine, func(t *testing.T) {
			t.Parallel()

			const burst = 3
			reg, _ := wireTranslatorRoutes(t, engine, translatorSecurity(t, burst, 1<<20))

			var translatorRoutes []string
			for path, info := range reg.GetRoutes() {
				if info.IsTranslator {
					translatorRoutes = append(translatorRoutes, path)
				}
			}
			require.GreaterOrEqual(t, len(translatorRoutes), 4, "anthropic messages, models, count_tokens and the plain translator")

			for _, path := range translatorRoutes {
				t.Run(path, func(t *testing.T) {
					// A fresh client and mux per route keeps the buckets independent.
					_, mux := wireTranslatorRoutes(t, engine, translatorSecurity(t, burst, 1<<20))
					method := reg.GetRoutes()[path].Method

					var limited *httptest.ResponseRecorder
					for i := 0; i < burst+2 && limited == nil; i++ {
						rec := httptest.NewRecorder()
						mux.ServeHTTP(rec, translatorRequest(method, path, translatorBody))
						if rec.Code == http.StatusTooManyRequests {
							limited = rec
						}
					}
					require.NotNil(t, limited, "%s was never rate limited", path)

					retry, err := strconv.Atoi(limited.Header().Get("Retry-After"))
					require.NoError(t, err, "Retry-After must be whole seconds")
					assert.Positive(t, retry)
				})
			}
		})
	}
}

// Claude Code reads the Anthropic error envelope, so a 429 from the limiter has to use it
// for translators with an error writer, and fall back to plain text for ones without.
func TestTranslatorRoutes_RateLimitUsesTranslatorErrorFormat(t *testing.T) {
	t.Parallel()

	for _, engine := range translationEngines {
		t.Run(engine, func(t *testing.T) {
			t.Parallel()

			_, mux := wireTranslatorRoutes(t, engine, translatorSecurity(t, 1, 1<<20))

			for _, path := range []string{"/olla/anthropic/v1/messages", "/olla/anthropic/v1/messages/count_tokens"} {
				// Exhaust the single-token bucket, then expect the rejection.
				var rec *httptest.ResponseRecorder
				for range 3 {
					rec = httptest.NewRecorder()
					mux.ServeHTTP(rec, translatorRequest(http.MethodPost, path, translatorBody))
					if rec.Code == http.StatusTooManyRequests {
						break
					}
				}
				require.Equal(t, http.StatusTooManyRequests, rec.Code, path)
				assert.Equal(t, constants.ContentTypeJSON, rec.Header().Get(constants.HeaderContentType))
				assert.NotEmpty(t, rec.Header().Get("Retry-After"))

				var body struct {
					Type  string `json:"type"`
					Error struct {
						Type    string `json:"type"`
						Message string `json:"message"`
					} `json:"error"`
				}
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body), rec.Body.String())
				assert.Equal(t, "error", body.Type)
				assert.Equal(t, "rate_limit_error", body.Error.Type)
				assert.NotEmpty(t, body.Error.Message)
			}

			// A translator without an error writer still gets limited, in plain text.
			var rec *httptest.ResponseRecorder
			for range 3 {
				rec = httptest.NewRecorder()
				mux.ServeHTTP(rec, translatorRequest(http.MethodPost, "/olla/plain/v1/messages", translatorBody))
				if rec.Code == http.StatusTooManyRequests {
					break
				}
			}
			require.Equal(t, http.StatusTooManyRequests, rec.Code)
			assert.False(t, json.Valid(rec.Body.Bytes()), "plain translators keep the text rejection: %s", rec.Body.String())
		})
	}
}

// The size cap used to be skipped on /olla/anthropic/v1/messages for streamed bodies; it now
// applies as it does on the proxy routes, and answers in the Anthropic error schema.
func TestTranslatorRoutes_SizeLimitApplies(t *testing.T) {
	t.Parallel()

	for _, engine := range translationEngines {
		t.Run(engine, func(t *testing.T) {
			t.Parallel()

			_, mux := wireTranslatorRoutes(t, engine, translatorSecurity(t, 100, 64))

			big := `{"model":"m","max_tokens":16,"messages":[{"role":"user","content":"` + strings.Repeat("x", 512) + `"}]}`
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, translatorRequest(http.MethodPost, "/olla/anthropic/v1/messages", big))
			require.Equal(t, http.StatusRequestEntityTooLarge, rec.Code, rec.Body.String())

			var body map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body), rec.Body.String())
			assert.Equal(t, "request_too_large", body["error"].(map[string]any)["type"])

			// Chunked bodies carry no Content-Length, so the cap has to hold while streaming.
			chunked := translatorRequest(http.MethodPost, "/olla/anthropic/v1/messages", "")
			chunked.ContentLength = -1
			chunked.Body = readCloser{bytes.NewReader([]byte(big))}
			rec = httptest.NewRecorder()
			mux.ServeHTTP(rec, chunked)
			assert.NotEqual(t, http.StatusOK, rec.Code, "an oversized chunked body must not be served")
		})
	}
}

type readCloser struct{ *bytes.Reader }

func (readCloser) Close() error { return nil }

// A client inside the limits is served exactly as before: translation and passthrough
// both reach the backend.
func TestTranslatorRoutes_WithinLimitsStillServed(t *testing.T) {
	t.Parallel()

	for _, engine := range translationEngines {
		t.Run(engine, func(t *testing.T) {
			t.Parallel()

			_, mux := wireTranslatorRoutes(t, engine, translatorSecurity(t, 50, 1<<20))

			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, translatorRequest(http.MethodPost, "/olla/anthropic/v1/messages", translatorBody))
			require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
			assert.Contains(t, rec.Body.String(), `"type":"message"`)
		})
	}
}
