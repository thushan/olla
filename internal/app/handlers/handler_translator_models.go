package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/thushan/olla/internal/adapter/translator"
	"github.com/thushan/olla/internal/core/constants"
	"github.com/thushan/olla/internal/core/domain"
)

// processStartTime is captured once at startup so that created_at in the
// Anthropic models response is stable across calls. There is no genuine
// creation timestamp on UnifiedModel (LastSeen is a discovery heartbeat that
// shifts on every health poll), so process start is the best stable proxy.
var processStartTime = time.Now().UTC().Format(time.RFC3339)

// anthropicModelsCache is a one-entry cache for the Anthropic GET /v1/models
// response. The underlying model set changes only on the 30-second discovery
// cycle, so rebuilding it on every HTTP request is pure waste under load.
//
// Invalidation key: the sorted, comma-joined set of healthy model IDs. A change
// in any model ID (add, remove, rename) produces a different fingerprint and
// triggers a rebuild. Endpoint health changes are captured because we filter
// by healthy endpoints before building the fingerprint.
type anthropicModelsCache struct {
	fingerprint string
	body        []byte // pre-encoded JSON ready to write
	mu          sync.RWMutex
}

// get returns the cached body if the fingerprint matches, otherwise nil.
func (c *anthropicModelsCache) get(fingerprint string) []byte {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.fingerprint == fingerprint {
		return c.body
	}
	return nil
}

// set stores a new fingerprint and pre-encoded body.
func (c *anthropicModelsCache) set(fingerprint string, body []byte) {
	c.mu.Lock()
	c.fingerprint = fingerprint
	c.body = body
	c.mu.Unlock()
}

// modelSetFingerprint builds a stable, cheap change-detection key from the
// set of models as they will be emitted to clients.
//
// The response uses the first alias as the emitted id, not UnifiedModel.ID, so
// the fingerprint must include the emitted value. Without this, an alias change
// (e.g. a version suffix update) while the underlying IDs are stable would leave
// stale alias strings in the cache until the next model-set change. Each entry is
// Each entry is "id\x00emittedID" (null-byte separator) so that changes to
// either field invalidate the cache. A colon would collide with IDs that contain
// colons (e.g. "a:b" + alias "c" vs "a" + alias "b:c" both produce "a:b:c").
// Sorting ensures the key is order-independent.
func modelSetFingerprint(models []*domain.UnifiedModel) string {
	if len(models) == 0 {
		return ""
	}
	ids := make([]string, 0, len(models))
	for _, m := range models {
		emitted := m.ID
		if len(m.Aliases) > 0 {
			emitted = m.Aliases[0].Name
		}
		ids = append(ids, m.ID+"\x00"+emitted)
	}
	sort.Strings(ids)
	return strings.Join(ids, ",")
}

// list models in translator format (eg /olla/anthropic/v1/models)
func (a *Application) translatorModelsHandler(trans translator.RequestTranslator) http.HandlerFunc {
	// One cache per translator registration; lives for the lifetime of the handler.
	cache := &anthropicModelsCache{}

	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		// fetch available models from registry
		var unifiedModels []*domain.UnifiedModel
		var err error

		// check if registry supports getting unified models
		type unifiedModelsGetter interface {
			GetUnifiedModels(ctx context.Context) ([]*domain.UnifiedModel, error)
		}

		if getter, ok := a.modelRegistry.(unifiedModelsGetter); ok {
			unifiedModels, err = getter.GetUnifiedModels(ctx)
			if err != nil {
				a.writeTranslatorModelsError(w, trans, "failed to get unified models")
				return
			}
		} else {
			a.writeTranslatorModelsError(w, trans, "unified models not supported")
			return
		}

		// skip unhealthy models for endpoint response
		healthyModels, err := a.filterModelsByHealth(ctx, unifiedModels)
		if err != nil {
			a.writeTranslatorModelsError(w, trans, "failed to filter models by health")
			return
		}

		// Cache hit: the model set has not changed since the last build.
		// GetUnifiedModels + filterModelsByHealth above are still called on every
		// request so we always have fresh health data for the fingerprint, but the
		// expensive JSON encoding and map allocation are skipped on a hit.
		fp := modelSetFingerprint(healthyModels)
		if body := cache.get(fp); body != nil {
			w.Header().Set(constants.HeaderContentType, constants.ContentTypeJSON)
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(body)
			return
		}

		// Cache miss: build the Anthropic-format response and store it.
		response := a.convertModelsToAnthropicFormat(healthyModels)
		body, err := json.Marshal(response)
		if err != nil {
			a.writeTranslatorModelsError(w, trans, "failed to encode models response")
			return
		}
		cache.set(fp, body)

		w.Header().Set(constants.HeaderContentType, constants.ContentTypeJSON)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	}
}

// convertModelsToAnthropicFormat emits the Anthropic GET /v1/models wire format.
// The SDK uses "type" as a deserialise discriminator so it must be "model", not "chat".
// "display_name" and "created_at" (RFC3339) match the published Anthropic spec.
func (a *Application) convertModelsToAnthropicFormat(models []*domain.UnifiedModel) map[string]any {
	data := make([]map[string]any, 0, len(models))

	for _, model := range models {
		// prefer the first alias as the canonical id clients send back to us
		modelID := model.ID
		if len(model.Aliases) > 0 {
			modelID = model.Aliases[0].Name
		}

		entry := map[string]any{
			"type":         "model",
			"id":           modelID,
			"display_name": modelID,
			"created_at":   processStartTime,
		}

		data = append(data, entry)
	}

	// first_id / last_id are null when the list is empty
	var firstID, lastID any
	if len(data) > 0 {
		firstID = data[0]["id"]
		lastID = data[len(data)-1]["id"]
	}

	return map[string]any{
		"data":     data,
		"has_more": false,
		"first_id": firstID,
		"last_id":  lastID,
	}
}

// send error response for models endpoint. All failure paths here are 500s —
// either the registry is unavailable or JSON encoding failed, both of which are
// internal failures rather than client errors.
func (a *Application) writeTranslatorModelsError(w http.ResponseWriter, trans translator.RequestTranslator, message string) {
	a.logger.Error("Translator models request failed",
		"translator", trans.Name(),
		"error", message)

	// use custom error format or fallback to generic
	if errorWriter, ok := trans.(translator.ErrorWriter); ok {
		errorWriter.WriteError(w, err{message: message}, http.StatusInternalServerError)
		return
	}

	errorResp := map[string]any{
		"error": map[string]any{
			"message": message,
			"type":    "models_error",
		},
	}

	w.Header().Set(constants.HeaderContentType, constants.ContentTypeJSON)
	w.WriteHeader(http.StatusInternalServerError)
	json.NewEncoder(w).Encode(errorResp)
}

// minimal error type for response handling
type err struct {
	message string
}

func (e err) Error() string {
	return e.message
}
