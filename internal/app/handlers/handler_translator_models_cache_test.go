package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/thushan/olla/internal/adapter/translator"
	"github.com/thushan/olla/internal/adapter/translator/anthropic"
	"github.com/thushan/olla/internal/config"
	"github.com/thushan/olla/internal/core/domain"
)

// TestAnthropicModelsCache_HitReturnsSameContent verifies that a cache hit
// returns byte-identical content to the first (miss) response.
func TestAnthropicModelsCache_HitReturnsSameContent(t *testing.T) {
	t.Parallel()

	models := []*domain.UnifiedModel{
		{
			ID:              "claude/3:opus",
			Aliases:         []domain.AliasEntry{{Name: "claude-3-opus", Source: "test"}},
			SourceEndpoints: []domain.SourceEndpoint{{EndpointURL: "http://localhost:8080"}},
		},
		{
			ID:              "claude/3:sonnet",
			Aliases:         []domain.AliasEntry{{Name: "claude-3-sonnet", Source: "test"}},
			SourceEndpoints: []domain.SourceEndpoint{{EndpointURL: "http://localhost:8080"}},
		},
	}

	app := &Application{
		logger:           &mockStyledLogger{},
		modelRegistry:    &mockTranslatorRegistry{models: models},
		discoveryService: &mockDiscoveryService{},
		repository:       &mockEndpointRepository{},
	}

	trans := mustNewAnthropicTranslator(t)
	handler := app.translatorModelsHandler(trans)

	body1 := doGetModels(t, handler)
	body2 := doGetModels(t, handler)

	if string(body1) != string(body2) {
		t.Errorf("cache hit returned different content:\n  miss: %s\n  hit:  %s", body1, body2)
	}

	// Both responses must be valid JSON with the expected envelope.
	validateModelsEnvelope(t, body1)
}

// TestAnthropicModelsCache_InvalidatesOnModelSetChange verifies that when the
// set of healthy models changes, the cache is invalidated and the response is
// rebuilt with the updated data.
func TestAnthropicModelsCache_InvalidatesOnModelSetChange(t *testing.T) {
	t.Parallel()

	initialModels := []*domain.UnifiedModel{
		{
			ID:              "model/a",
			Aliases:         []domain.AliasEntry{{Name: "model-a", Source: "test"}},
			SourceEndpoints: []domain.SourceEndpoint{{EndpointURL: "http://localhost:8080"}},
		},
	}

	reg := &mutableTranslatorRegistry{models: initialModels}
	app := &Application{
		logger:           &mockStyledLogger{},
		modelRegistry:    reg,
		discoveryService: &mockDiscoveryService{},
		repository:       &mockEndpointRepository{},
	}

	trans := mustNewAnthropicTranslator(t)
	handler := app.translatorModelsHandler(trans)

	// First call: miss, builds cache.
	body1 := doGetModels(t, handler)
	resp1 := decodeModelsData(t, body1)
	if len(resp1) != 1 {
		t.Fatalf("expected 1 model, got %d", len(resp1))
	}

	// Simulate a discovery cycle adding a new model.
	reg.mu.Lock()
	reg.models = append(reg.models, &domain.UnifiedModel{
		ID:              "model/b",
		Aliases:         []domain.AliasEntry{{Name: "model-b", Source: "test"}},
		SourceEndpoints: []domain.SourceEndpoint{{EndpointURL: "http://localhost:8080"}},
	})
	reg.mu.Unlock()

	// Second call: fingerprint changed, must rebuild.
	body2 := doGetModels(t, handler)
	resp2 := decodeModelsData(t, body2)
	if len(resp2) != 2 {
		t.Fatalf("expected 2 models after registry update, got %d", len(resp2))
	}
}

// TestAnthropicModelsCache_ConcurrentSafety exercises concurrent reads and a
// simultaneous registry update. The -race flag enforces there are no data races.
func TestAnthropicModelsCache_ConcurrentSafety(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping concurrency test in short mode")
	}
	t.Parallel()

	initialModels := []*domain.UnifiedModel{
		{
			ID:              "concurrent/model",
			SourceEndpoints: []domain.SourceEndpoint{{EndpointURL: "http://localhost:8080"}},
		},
	}

	reg := &mutableTranslatorRegistry{models: initialModels}
	app := &Application{
		logger:           &mockStyledLogger{},
		modelRegistry:    reg,
		discoveryService: &mockDiscoveryService{},
		repository:       &mockEndpointRepository{},
	}

	trans := mustNewAnthropicTranslator(t)
	handler := app.translatorModelsHandler(trans)

	const readers = 20
	var wg sync.WaitGroup

	for i := range readers {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			for range 50 {
				doGetModels(t, handler)
			}
			_ = n
		}(i)
	}

	// Simulate discovery updates while readers are running.
	wg.Go(func() {
		for i := range 5 {
			reg.mu.Lock()
			reg.models = append(reg.models, &domain.UnifiedModel{
				ID:              "extra/model-" + string(rune('a'+i)),
				SourceEndpoints: []domain.SourceEndpoint{{EndpointURL: "http://localhost:8080"}},
			})
			reg.mu.Unlock()
		}
	})

	wg.Wait()
}

// TestModelSetFingerprint verifies the fingerprint is order-independent and
// changes when models are added or removed.
func TestModelSetFingerprint(t *testing.T) {
	t.Parallel()

	a := []*domain.UnifiedModel{{ID: "a"}, {ID: "b"}, {ID: "c"}}
	b := []*domain.UnifiedModel{{ID: "c"}, {ID: "a"}, {ID: "b"}} // different order
	c := []*domain.UnifiedModel{{ID: "a"}, {ID: "b"}}            // one fewer

	if modelSetFingerprint(a) != modelSetFingerprint(b) {
		t.Error("fingerprint must be order-independent")
	}
	if modelSetFingerprint(a) == modelSetFingerprint(c) {
		t.Error("fingerprint must differ when model set changes")
	}
	if modelSetFingerprint(nil) != "" {
		t.Error("empty model set must produce empty fingerprint")
	}
}

// TestModelSetFingerprint_AliasChange verifies that the fingerprint changes when an
// alias changes even if the underlying model IDs are stable. Without this, the cache
// would serve stale emitted IDs (first alias) for the lifetime of the unchanged ID set.
func TestModelSetFingerprint_AliasChange(t *testing.T) {
	t.Parallel()

	before := []*domain.UnifiedModel{
		{
			ID:      "model/internal",
			Aliases: []domain.AliasEntry{{Name: "my-model-v1", Source: "test"}},
		},
	}
	after := []*domain.UnifiedModel{
		{
			ID:      "model/internal",                                           // same underlying ID
			Aliases: []domain.AliasEntry{{Name: "my-model-v2", Source: "test"}}, // alias changed
		},
	}

	if modelSetFingerprint(before) == modelSetFingerprint(after) {
		t.Error("fingerprint must differ when the first alias (emitted id) changes")
	}
}

// TestAnthropicModelsCache_InvalidatesOnAliasChange verifies that when only an alias
// changes (underlying ID stable), the cache is invalidated and the new alias is served.
func TestAnthropicModelsCache_InvalidatesOnAliasChange(t *testing.T) {
	t.Parallel()

	reg := &mutableTranslatorRegistry{
		models: []*domain.UnifiedModel{
			{
				ID:              "internal/model",
				Aliases:         []domain.AliasEntry{{Name: "model-v1", Source: "test"}},
				SourceEndpoints: []domain.SourceEndpoint{{EndpointURL: "http://localhost:8080"}},
			},
		},
	}

	app := &Application{
		logger:           &mockStyledLogger{},
		modelRegistry:    reg,
		discoveryService: &mockDiscoveryService{},
		repository:       &mockEndpointRepository{},
	}

	trans := mustNewAnthropicTranslator(t)
	handler := app.translatorModelsHandler(trans)

	// First call: cache miss, builds with model-v1.
	body1 := doGetModels(t, handler)
	data1 := decodeModelsData(t, body1)
	if len(data1) != 1 {
		t.Fatalf("expected 1 model, got %d", len(data1))
	}
	firstID1, ok := data1[0].(map[string]any)["id"].(string)
	if !ok {
		t.Fatal("expected id to be a string")
	}
	if firstID1 != "model-v1" {
		t.Errorf("expected emitted id %q, got %q", "model-v1", firstID1)
	}

	// Simulate a discovery cycle that updates only the alias (same underlying ID).
	reg.mu.Lock()
	reg.models[0] = &domain.UnifiedModel{
		ID:              "internal/model", // unchanged
		Aliases:         []domain.AliasEntry{{Name: "model-v2", Source: "test"}},
		SourceEndpoints: []domain.SourceEndpoint{{EndpointURL: "http://localhost:8080"}},
	}
	reg.mu.Unlock()

	// Second call: fingerprint changed due to alias change, must rebuild.
	body2 := doGetModels(t, handler)
	data2 := decodeModelsData(t, body2)
	if len(data2) != 1 {
		t.Fatalf("expected 1 model after alias update, got %d", len(data2))
	}
	firstID2, ok := data2[0].(map[string]any)["id"].(string)
	if !ok {
		t.Fatal("expected id to be a string")
	}
	if firstID2 != "model-v2" {
		t.Errorf("cache not invalidated on alias change: expected %q, got %q", "model-v2", firstID2)
	}
}

// mustNewAnthropicTranslator creates an Anthropic translator for test use.
func mustNewAnthropicTranslator(t *testing.T) translator.RequestTranslator {
	t.Helper()
	cfg := config.AnthropicTranslatorConfig{Enabled: true, MaxMessageSize: 10 << 20}
	trans, err := anthropic.NewTranslator(&mockStyledLogger{}, cfg)
	if err != nil {
		t.Fatalf("failed to create anthropic translator: %v", err)
	}
	return trans
}

func doGetModels(t *testing.T, handler http.HandlerFunc) []byte {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/olla/anthropic/v1/models", nil)
	rr := httptest.NewRecorder()
	handler(rr, req)
	if rr.Code != http.StatusOK {
		// t.Errorf rather than t.Fatalf: this helper is called from goroutines
		// in the concurrency test, and t.Fatalf calls runtime.Goexit which only
		// exits the calling goroutine, silently leaving the parent test passing.
		t.Errorf("expected 200, got %d: %s", rr.Code, rr.Body.String())
		return nil
	}
	// Return a copy so the recorder's buffer can be reused.
	out := make([]byte, rr.Body.Len())
	copy(out, rr.Body.Bytes())
	return out
}

func decodeModelsData(t *testing.T, body []byte) []any {
	t.Helper()
	var resp map[string]any
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	data, ok := resp["data"].([]any)
	if !ok {
		t.Fatalf("expected data to be an array, got %T", resp["data"])
	}
	return data
}

func validateModelsEnvelope(t *testing.T, body []byte) {
	t.Helper()
	var resp map[string]any
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	for _, key := range []string{"data", "has_more", "first_id", "last_id"} {
		if _, ok := resp[key]; !ok {
			t.Errorf("missing envelope field %q", key)
		}
	}
}

// mutableTranslatorRegistry allows tests to swap the model list mid-test to
// simulate a discovery cycle update.
type mutableTranslatorRegistry struct {
	baseMockRegistry
	mu     sync.RWMutex
	models []*domain.UnifiedModel
}

func (m *mutableTranslatorRegistry) GetUnifiedModels(_ context.Context) ([]*domain.UnifiedModel, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]*domain.UnifiedModel, len(m.models))
	copy(out, m.models)
	return out, nil
}
