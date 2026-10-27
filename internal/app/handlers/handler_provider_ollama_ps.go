package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/thushan/olla/internal/adapter/factory"
	"github.com/thushan/olla/internal/adapter/proxy/common"
	"github.com/thushan/olla/internal/core/constants"
	"github.com/thushan/olla/internal/core/domain"
)

const (
	// ollamaPSTimeout bounds the whole fan-out. /api/ps is answered from memory by
	// Ollama, so a healthy instance replies in milliseconds; anything slower is
	// better left out than allowed to hold the client.
	ollamaPSTimeout = 5 * time.Second

	// ollamaPSMaxBody caps what one instance can make us buffer. A real response is
	// a few hundred bytes per loaded model.
	ollamaPSMaxBody = 4 << 20

	// headerOllaPartial marks a merged answer that is missing instances.
	headerOllaPartial = constants.HeaderXOllaPartial

	// ollamaPSEndpointField names the instance a merged entry came from. Ollama's
	// own clients ignore unknown fields.
	ollamaPSEndpointField = "olla_endpoint"
)

var (
	errOllamaPSStatus = errors.New("unexpected status from /api/ps")
	errOllamaPSNoURL  = errors.New("endpoint has no parsed URL")
)

// ollamaRunningModelsHandler merges /api/ps across every healthy Ollama endpoint.
// endpoint: GET /olla/ollama/api/ps
//
// What is loaded is state held by each instance (not something Olla's registry
// tracks), so it has to be asked for live. Entries are kept per instance rather
// than de-duplicated: a model resident on two hosts occupies memory on both, and
// collapsing them would hide that. Each entry gains olla_endpoint naming its host.
// An instance that fails or times out is skipped and flagged with X-Olla-Partial;
// only when every instance fails is the whole request a 502.
func (a *Application) ollamaRunningModelsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), ollamaPSTimeout)
	defer cancel()

	healthy, err := a.repository.GetHealthy(ctx)
	if err != nil {
		http.Error(w, "failed to list endpoints", http.StatusInternalServerError)
		return
	}

	var targets []*domain.Endpoint
	for _, ep := range healthy {
		if NormaliseProviderType(ep.Type) == constants.ProviderTypeOllama {
			targets = append(targets, ep)
		}
	}

	models := make([]json.RawMessage, 0)
	failed := 0
	if len(targets) > 0 {
		results := make([]ollamaPSResult, len(targets))
		var wg sync.WaitGroup
		for i, ep := range targets {
			wg.Add(1)
			go func() {
				defer wg.Done()
				// a panic here would take the process down, and a bad peer
				// response must not be able to do that
				defer func() {
					if rec := recover(); rec != nil {
						results[i].err = fmt.Errorf("panic reading /api/ps: %v", rec)
					}
				}()
				results[i].models, results[i].err = fetchOllamaPS(ctx, ep)
			}()
		}
		wg.Wait()

		for i, res := range results {
			if res.err != nil {
				failed++
				a.logger.Debug("Ollama /api/ps failed", "endpoint", targets[i].Name, "error", res.err)
				if a.rejectLog.allowPSFailure(targets[i].Name) {
					a.logger.Warn("Ollama /api/ps failed", "endpoint", targets[i].Name, "error", res.err)
				}
				continue
			}
			models = append(models, res.models...)
		}

		if failed == len(targets) {
			http.Error(w, "no Ollama endpoint answered /api/ps", http.StatusBadGateway)
			return
		}
	}

	body, err := json.Marshal(map[string]any{"models": models})
	if err != nil {
		http.Error(w, "failed to encode response", http.StatusInternalServerError)
		return
	}

	w.Header().Set(constants.HeaderContentType, constants.ContentTypeJSON)
	if failed > 0 {
		w.Header().Set(headerOllaPartial, fmt.Sprintf("%d of %d endpoints failed", failed, len(targets)))
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

type ollamaPSResult struct {
	err    error
	models []json.RawMessage
}

// ollamaPSClient is shared for the life of the process so repeated fan-outs reuse
// pooled keep-alive connections to each instance. It comes from the shared backend
// constructor because the endpoint's credentials ride on these requests: no
// environment proxy, and no redirect to a host the operator did not configure.
// ctx bounds the whole fan-out; ResponseHeaderTimeout is the backstop for an
// instance that accepts the connection and then never sends headers.
var ollamaPSClient = factory.NewBackendHTTPClient(factory.BackendClientOptions{
	MaxIdleConns:          10,
	MaxIdleConnsPerHost:   2,
	IdleConnTimeout:       30 * time.Second,
	ResponseHeaderTimeout: ollamaPSTimeout,
})

func fetchOllamaPS(ctx context.Context, ep *domain.Endpoint) ([]json.RawMessage, error) {
	// The proxy's own builder, so an endpoint URL with a base path honours
	// preserve_path exactly as proxied requests do.
	if ep.URL == nil {
		return nil, errOllamaPSNoURL
	}
	target := common.BuildTargetURL(&http.Request{URL: &url.URL{Path: "/api/ps"}}, ep, "")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Accept", constants.ContentTypeJSON)
	for name, value := range ep.Headers {
		req.Header.Set(name, value)
	}
	if ep.AuthHeaderName != "" {
		req.Header.Set(ep.AuthHeaderName, ep.AuthHeaderValue)
	}

	resp, err := ollamaPSClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: %d", errOllamaPSStatus, resp.StatusCode)
	}

	raw, err := io.ReadAll(io.LimitReader(resp.Body, ollamaPSMaxBody+1))
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}
	if len(raw) > ollamaPSMaxBody {
		return nil, fmt.Errorf("response exceeds %d bytes", ollamaPSMaxBody)
	}

	var parsed struct {
		Models []map[string]json.RawMessage `json:"models"`
	}
	if err = json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}

	name, err := json.Marshal(ep.Name)
	if err != nil {
		return nil, fmt.Errorf("encode endpoint name: %w", err)
	}
	out := make([]json.RawMessage, 0, len(parsed.Models))
	for _, m := range parsed.Models {
		m[ollamaPSEndpointField] = name
		entry, merr := json.Marshal(m)
		if merr != nil {
			return nil, fmt.Errorf("encode entry: %w", merr)
		}
		out = append(out, entry)
	}
	return out, nil
}
