package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"io"
	"log/slog"
	"math"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

// errUnknownMode is returned when a PATCH body sets an unrecognised mode.
var errUnknownMode = errors.New("unknown mode")

// Precompiled once at package init rather than per call inside modelMetadata -
// these run on every /api/tags response and the quantisation ones were
// previously compiled twice each (once for MatchString, again for FindString).
var (
	paramSizeRe = regexp.MustCompile(`(?i)(\d+(?:\.\d+)?)\s*([bm])`)
	quantKLMSRe = regexp.MustCompile(`(?i)q\d+_k_[lms]`)
	quantNumRe  = regexp.MustCompile(`(?i)q\d+_\d+`)
	quantBareRe = regexp.MustCompile(`(?i)\bq\d+\b`)
	quantF16Re  = regexp.MustCompile(`(?i)fp?16`)
	quantF8Re   = regexp.MustCompile(`(?i)fp?8`)
)

// serverConfig holds static configuration set at startup.
// Field order largest-to-smallest for alignment.
type serverConfig struct {
	name string
	// teiKind is the model_type /info reports and the TEI routes enforce:
	// teiKindEmbedding (also the zero value), teiKindReranker or teiKindClassifier.
	teiKind string
	models  []string
	// loaded names the models /api/ps reports as resident. Empty means none.
	loaded       []string
	ttftMS       int
	tps          int
	streamChunks int
	// tagsCapabilities makes /api/tags carry per-model capabilities, as Ollama 0.30+
	// does. Off (the default) keeps the pre-existing listing for the main fleet.
	tagsCapabilities bool
}

// mockServer is the central state container wired into the HTTP mux.
type mockServer struct {
	bstate *behaviourState
	stats  statsStore
	cfg    serverConfig
}

func newServer(cfg serverConfig) *mockServer {
	// Seed from FNV-1a hash of the instance name so behaviour is deterministic
	// per-instance but distinct across instances in a multi-node test fleet.
	h := fnv.New64a()
	_, _ = h.Write([]byte(cfg.name))
	seed := int64(h.Sum64())

	return &mockServer{
		cfg:    cfg,
		bstate: newBehaviourState(seed),
	}
}

// handler builds and returns the ServeMux for this server instance.
func (srv *mockServer) handler() http.Handler {
	mux := http.NewServeMux()

	// Control plane - always healthy, immune to behaviour.
	srv.controlHandlers(mux)

	// LLM protocol routes.
	mux.HandleFunc("GET /health", srv.wrap(srv.handleHealth))
	mux.HandleFunc("GET /", srv.wrap(srv.handleRoot))

	// Model listing endpoints - one per protocol.
	mux.HandleFunc("GET /v1/models", srv.wrap(srv.handleOpenAIModels))
	mux.HandleFunc("GET /api/tags", srv.wrap(srv.handleOllamaTags))
	mux.HandleFunc("GET /api/version", srv.wrap(srv.handleOllamaVersion))
	mux.HandleFunc("GET /api/ps", srv.wrap(srv.handleOllamaPS))
	mux.HandleFunc("GET /api/v0/models", srv.wrap(srv.handleLMStudioModels))
	mux.HandleFunc("GET /api/v1/models", srv.wrap(srv.handleLemonadeModels))

	// Text Embeddings Inference (TEI) protocol - single model per instance,
	// discovered via /info rather than a model list. Native request bodies
	// carry no model field; response shapes mirror the OpenAPI spec closely
	// enough for forwarding tests.
	mux.HandleFunc("GET /info", srv.wrap(srv.handleTEIInfo))
	mux.HandleFunc("POST /v1/embeddings", srv.wrap(srv.handleTEIEmbeddings))
	mux.HandleFunc("POST /embed", srv.wrap(srv.handleTEIEmbed))
	mux.HandleFunc("POST /embed_all", srv.wrap(srv.handleTEIEmbedAll))
	mux.HandleFunc("POST /embed_sparse", srv.wrap(srv.handleTEIEmbedSparse))
	mux.HandleFunc("POST /tokenize", srv.wrap(srv.handleTEITokenize))
	mux.HandleFunc("POST /decode", srv.wrap(srv.handleTEIDecode))
	mux.HandleFunc("POST /predict", srv.wrap(srv.handleTEIPredict))
	mux.HandleFunc("POST /rerank", srv.wrap(srv.handleTEIRerank))
	mux.HandleFunc("POST /similarity", srv.wrap(srv.handleTESimilarity))
	mux.HandleFunc("GET /metrics", srv.wrap(srv.handleTEIMetrics))

	// Inference endpoints - streaming handled in streaming.go.
	mux.HandleFunc("POST /api/chat", srv.wrap(srv.handleOllamaChat))
	mux.HandleFunc("POST /api/generate", srv.wrap(srv.handleOllamaGenerate))
	mux.HandleFunc("POST /api/embed", srv.wrap(srv.handleOllamaEmbed))
	mux.HandleFunc("POST /v1/chat/completions", srv.wrap(srv.handleOpenAIChat))
	mux.HandleFunc("POST /v1/completions", srv.wrap(srv.handleOpenAICompletion))
	mux.HandleFunc("POST /api/v1/chat/completions", srv.wrap(srv.handleOpenAIChat))
	mux.HandleFunc("POST /v1/messages", srv.wrap(srv.handleAnthropicMessages))

	return mux
}

// wrap returns a handler that records stats, sets the instance header, and
// applies the current behaviour before delegating to the inner handler.
func (srv *mockServer) wrap(inner http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		srv.stats.inc(r.URL.Path)
		w.Header().Set("X-Ollamock-Instance", srv.cfg.name)
		if !srv.applyBehaviour(w, r) {
			return
		}
		inner(w, r)
	}
}

// applyBehaviour enforces the current Mode, returning false when the request
// should be short-circuited (response already written).
func (srv *mockServer) applyBehaviour(w http.ResponseWriter, r *http.Request) bool {
	b := srv.bstate.get()

	isHealthPath := r.URL.Path == "/health" || r.URL.Path == "/"

	// fail_health only gates the health endpoints - all other routes are unaffected.
	if b.FailHealth && isHealthPath {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = fmt.Fprintf(w, `{"status":"unhealthy"}`)
		return false
	}

	switch b.Mode {
	case ModeOK:
		// No-op - proceed to handler.

	case ModeHang:
		// Block until the client disconnects or context is cancelled.
		select {
		case <-r.Context().Done():
		case <-time.After(10 * time.Minute):
		}
		return false

	case ModeError:
		writeErrorResponse(w, b.ErrorStatus)
		return false

	case ModeFlaky:
		if srv.bstate.float64() < b.ErrorRate {
			writeErrorResponse(w, b.ErrorStatus)
			return false
		}

	case ModeSlow:
		if b.LatencyMS > 0 {
			select {
			case <-r.Context().Done():
				return false
			case <-time.After(time.Duration(b.LatencyMS) * time.Millisecond):
			}
		}
	}

	if b.MalformedJSON {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, `{"broken":`)
		return false
	}

	return true
}

func writeErrorResponse(w http.ResponseWriter, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = fmt.Fprintf(w, `{"error":{"message":"mock error","type":"mock_error"}}`)
}

// writeJSON serialises v as JSON and writes it with the given status.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// --- Health / liveness ---

func (srv *mockServer) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (srv *mockServer) handleRoot(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)
	_, _ = fmt.Fprint(w, "Ollama is running")
}

// --- Ollama protocol ---

func (srv *mockServer) handleOllamaTags(w http.ResponseWriter, _ *http.Request) {
	type details struct {
		Family            string `json:"family"`
		ParameterSize     string `json:"parameter_size"`
		QuantizationLevel string `json:"quantization_level"`
	}
	type model struct {
		Details    details `json:"details"`
		ModifiedAt string  `json:"modified_at"`
		Digest     string  `json:"digest"`
		Name       string  `json:"name"`
		Model      string  `json:"model"`
		// omitted entirely when empty, which is what older Ollama sends
		Capabilities []string `json:"capabilities,omitempty"`
		Size         int64    `json:"size"`
	}
	type response struct {
		Models []model `json:"models"`
	}

	now := time.Now().UTC().Format(time.RFC3339)
	models := make([]model, len(srv.cfg.models))
	for i, m := range srv.cfg.models {
		fam, pdisp, billions, q := modelMetadata(m)
		models[i] = model{
			Name:       m,
			Model:      m,
			ModifiedAt: now,
			Size:       sizeBytes(billions, q),
			Digest:     fmt.Sprintf("sha256:ollamock%s%s", srv.cfg.name, m),
			Details: details{
				Family:            fam,
				ParameterSize:     pdisp,
				QuantizationLevel: q,
			},
		}
		if srv.cfg.tagsCapabilities {
			models[i].Capabilities = ollamaCapabilities(m)
		}
	}
	writeJSON(w, http.StatusOK, response{Models: models})
}

// handleOllamaPS reports the models named by --loaded as resident, in Ollama's
// /api/ps shape. Loaded state is per instance in real Ollama too, which is what
// makes a merge across instances worth testing.
func (srv *mockServer) handleOllamaPS(w http.ResponseWriter, _ *http.Request) {
	type model struct {
		Details       map[string]string `json:"details"`
		ExpiresAt     string            `json:"expires_at"`
		Name          string            `json:"name"`
		Model         string            `json:"model"`
		Digest        string            `json:"digest"`
		Runner        string            `json:"runner"`
		Size          int64             `json:"size"`
		SizeVRAM      int64             `json:"size_vram"`
		ContextLength int               `json:"context_length"`
	}

	expires := time.Now().Add(5 * time.Minute).UTC().Format(time.RFC3339)
	models := make([]model, 0, len(srv.cfg.loaded))
	for _, m := range srv.cfg.loaded {
		fam, pdisp, billions, q := modelMetadata(m)
		size := sizeBytes(billions, q)
		models = append(models, model{
			Name:      m,
			Model:     m,
			Size:      size,
			SizeVRAM:  size,
			ExpiresAt: expires,
			Digest:    fmt.Sprintf("sha256:ollamock%s%s", srv.cfg.name, m),
			Details:   map[string]string{"family": fam, "parameter_size": pdisp, "quantization_level": q},
			// Ollama reports the loaded context window and the runner serving it
			// (api.ProcessModelResponse); fixed plausible values, as for size.
			ContextLength: 4096,
			Runner:        "ggml",
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"models": models})
}

// handleOllamaEmbed answers Ollama's native /api/embed: "input" is a string or an
// array of strings and the reply carries one vector per input. It reuses the TEI
// deterministic vectors so tests can tell which instance answered by header, not
// by content. Like Ollama's EmbedHandler, an unknown model is a 404 and empty or
// absent input is a 200 with an empty embeddings array, not an error. A request
// with no model at all is still served: the harness sends model-less bodies to
// check routing.
func (srv *mockServer) handleOllamaEmbed(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Model string          `json:"model"`
		Input json.RawMessage `json:"input"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if req.Model != "" && len(srv.cfg.models) > 0 && !srv.hasOllamaModel(req.Model) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": fmt.Sprintf("model '%s' not found", req.Model)})
		return
	}
	count, ok := ollamaEmbedInputCount(req.Input)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid input type"})
		return
	}
	embeddings := make([][]float64, count)
	for i := range embeddings {
		embeddings[i] = teiVector(i)
	}
	resp := map[string]any{"model": req.Model, "embeddings": embeddings}
	if count > 0 {
		resp["total_duration"] = 1000000
		resp["load_duration"] = 500000
		resp["prompt_eval_count"] = count
	}
	writeJSON(w, http.StatusOK, resp)
}

// ollamaEmbedInputCount counts the inputs of an /api/embed body. Absent, null and
// "" all mean none; ok is false for any other type.
func ollamaEmbedInputCount(raw json.RawMessage) (count int, ok bool) {
	if !teiPresent(raw) {
		return 0, true
	}
	var in any
	if err := json.Unmarshal(raw, &in); err != nil {
		return 0, false
	}
	switch v := in.(type) {
	case string:
		if v == "" {
			return 0, true
		}
		return 1, true
	case []any:
		for _, item := range v {
			if _, isStr := item.(string); !isStr {
				return 0, false
			}
		}
		return len(v), true
	}
	return 0, false
}

// hasOllamaModel reports whether the instance serves name, treating a bare name
// as :latest the way Ollama does.
func (srv *mockServer) hasOllamaModel(name string) bool {
	withTag := func(m string) string {
		if !strings.Contains(m, ":") {
			return m + ":latest"
		}
		return m
	}
	return slices.ContainsFunc(srv.cfg.models, func(m string) bool { return withTag(m) == withTag(name) })
}

// embedderNameHints are substrings that mark a model name as an embedder. The
// mock has no model files to inspect, so the name stands in for what Ollama
// reads from the model's architecture.
var embedderNameHints = []string{"embed", "minilm", "bge-", "mxbai", "gte-", "e5-", "stella", "msmarco", "multi-qa"}

// Name hints for the capabilities beyond embedding. As with embedders, the name
// stands in for what Ollama reads from the model's template and projector.
var (
	visionNameHints = []string{"-vl", "llava", "vision", "gemma3", "minicpm-v"}
	toolsNameHints  = []string{"llama3.1", "llama3.2", "llama3.3", "qwen3", "qwen2.5", "mistral", "-vl"}
)

// ollamaCapabilities returns the capabilities Ollama 0.30+ lists for a model:
// "embedding" alone for embedders, otherwise "completion" plus "vision" and
// "tools" where the name suggests them. Order follows Ollama's: completion first.
func ollamaCapabilities(name string) []string {
	lower := strings.ToLower(name)
	if containsAny(lower, embedderNameHints) {
		return []string{"embedding"}
	}
	caps := []string{"completion"}
	if containsAny(lower, visionNameHints) {
		caps = append(caps, "vision")
	}
	if containsAny(lower, toolsNameHints) {
		caps = append(caps, "tools")
	}
	return caps
}

func containsAny(s string, hints []string) bool {
	return slices.ContainsFunc(hints, func(h string) bool { return strings.Contains(s, h) })
}

// modelMetadata derives a plausible family, parameter size and quantisation
// from a model name so the Ollama tags listing reflects the actual model
// rather than one hardcoded 7B/Q4_0 value for everything. Lets the mock back
// a realistic-looking fleet for screenshots and demos.
func modelMetadata(name string) (family, paramDisplay string, billions float64, quant string) {
	lower := strings.ToLower(name)
	if idx := strings.LastIndex(lower, "/"); idx != -1 {
		lower = lower[idx+1:]
	}
	family = "llama"
	for _, f := range []string{"deepseek", "llama", "gemma", "qwen", "mistral", "mixtral", "phi", "nomic", "yi", "falcon", "kimi", "granite", "glm", "starcoder", "codellama"} {
		if strings.HasPrefix(lower, f) {
			family = f
			break
		}
	}

	// Parameter size: first "<digits>[.<digits>]<b|m>" token in the name, case
	// insensitive so names like "Llama-3-8B" match (upstream naming isn't
	// consistent about casing here).
	if ms := paramSizeRe.FindStringSubmatch(name); ms != nil {
		num, err := strconv.ParseFloat(ms[1], 64)
		// Config-sourced input only (never request-controlled), but guard
		// against Inf/NaN anyway - sizeBytes would otherwise propagate them
		// into the response.
		if err == nil && !math.IsInf(num, 0) && !math.IsNaN(num) {
			if strings.EqualFold(ms[2], "m") {
				paramDisplay = fmt.Sprintf("%.0fM", num)
				billions = num / 1000
			} else {
				paramDisplay = fmt.Sprintf("%gB", num)
				billions = num
			}
		}
	}

	// Quantisation from the name when present, else the common Ollama pull default.
	quant = "Q4_K_M"
	switch {
	case quantKLMSRe.MatchString(name):
		quant = strings.ToUpper(quantKLMSRe.FindString(name))
	case quantNumRe.MatchString(name):
		quant = strings.ToUpper(quantNumRe.FindString(name))
	case quantBareRe.MatchString(name):
		quant = strings.ToUpper(quantBareRe.FindString(name))
	case quantF16Re.MatchString(name):
		quant = "F16"
	case quantF8Re.MatchString(name):
		quant = "F8"
	}
	return family, paramDisplay, billions, quant
}

// sizeBytes approximates on-disk size from a parameter count and quantisation.
func sizeBytes(billions float64, quant string) int64 {
	if billions <= 0 {
		return 0
	}
	bpp := 0.56 // bytes per parameter at Q4_K_M
	switch {
	case strings.HasPrefix(quant, "Q2"):
		bpp = 0.30
	case strings.HasPrefix(quant, "Q3"):
		bpp = 0.42
	case strings.HasPrefix(quant, "Q4"):
		bpp = 0.56
	case strings.HasPrefix(quant, "Q5"):
		bpp = 0.70
	case strings.HasPrefix(quant, "Q6"):
		bpp = 0.84
	case strings.HasPrefix(quant, "Q8"):
		bpp = 1.00
	case strings.HasPrefix(quant, "F16"), strings.HasPrefix(quant, "FP16"):
		bpp = 2.00
	case strings.HasPrefix(quant, "F32"), strings.HasPrefix(quant, "FP32"):
		bpp = 4.00
	}
	return int64(billions * bpp * float64(1_000_000_000))
}

func (srv *mockServer) handleOllamaVersion(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"version": "0.6.0-ollamock"})
}

// --- LM Studio protocol ---

func (srv *mockServer) handleLMStudioModels(w http.ResponseWriter, _ *http.Request) {
	type model struct {
		Type             string `json:"type"`
		Publisher        string `json:"publisher"`
		Arch             string `json:"arch"`
		State            string `json:"state"`
		ID               string `json:"id"`
		Object           string `json:"object"`
		MaxContextLength int64  `json:"max_context_length"`
	}
	type response struct {
		Object string  `json:"object"`
		Data   []model `json:"data"`
	}

	models := make([]model, len(srv.cfg.models))
	for i, m := range srv.cfg.models {
		family, _, _, _ := modelMetadata(m)
		models[i] = model{
			ID:               m,
			Object:           "model",
			Type:             "llm",
			Publisher:        "ollamock",
			Arch:             family,
			State:            "loaded",
			MaxContextLength: 4096,
		}
	}
	writeJSON(w, http.StatusOK, response{Object: "list", Data: models})
}

// --- OpenAI-compatible protocol ---

func (srv *mockServer) handleOpenAIModels(w http.ResponseWriter, _ *http.Request) {
	type model struct {
		OwnedBy string `json:"owned_by"`
		ID      string `json:"id"`
		Object  string `json:"object"`
		Created int64  `json:"created"`
	}
	type response struct {
		Object string  `json:"object"`
		Data   []model `json:"data"`
	}

	now := time.Now().Unix()
	models := make([]model, len(srv.cfg.models))
	for i, m := range srv.cfg.models {
		models[i] = model{
			ID:      m,
			Object:  "model",
			Created: now,
			OwnedBy: "ollamock",
		}
	}
	writeJSON(w, http.StatusOK, response{Object: "list", Data: models})
}

// --- Lemonade protocol ---

func (srv *mockServer) handleLemonadeModels(w http.ResponseWriter, _ *http.Request) {
	type model struct {
		ID         string `json:"id"`
		Object     string `json:"object"`
		OwnedBy    string `json:"owned_by"`
		Checkpoint string `json:"checkpoint"`
		Recipe     string `json:"recipe"`
		Created    int64  `json:"created"`
		Downloaded bool   `json:"downloaded"`
	}
	type response struct {
		Object string  `json:"object"`
		Data   []model `json:"data"`
	}

	now := time.Now().Unix()
	models := make([]model, len(srv.cfg.models))
	for i, m := range srv.cfg.models {
		models[i] = model{
			ID:         m,
			Object:     "model",
			OwnedBy:    "lemonade",
			Checkpoint: fmt.Sprintf("amd/%s", m),
			Recipe:     "oga-cpu",
			Created:    now,
			Downloaded: true,
		}
	}
	writeJSON(w, http.StatusOK, response{Object: "list", Data: models})
}

// --- Text Embeddings Inference (TEI) protocol ---

// teiServedModel returns the model this instance reports as served. TEI runs
// exactly one embedding model per instance, so the first configured model is
// used regardless of how many names the instance was started with.
func (srv *mockServer) teiServedModel() string {
	if len(srv.cfg.models) == 0 {
		return ""
	}
	return srv.cfg.models[0]
}

// TEI model kinds, as spelled in /info's model_type tag.
const (
	teiKindEmbedding  = "embedding"
	teiKindReranker   = "reranker"
	teiKindClassifier = "classifier"
)

// teiKindOrDefault is the kind this instance serves. TEI runs one model per
// instance and the zero value means an embedding model.
func (srv *mockServer) teiKindOrDefault() string {
	if srv.cfg.teiKind == "" {
		return teiKindEmbedding
	}
	return srv.cfg.teiKind
}

// handleTEIInfo reports model_type as TEI does: an externally tagged object whose
// single key is the kind. Classifiers and rerankers carry their label maps.
func (srv *mockServer) handleTEIInfo(w http.ResponseWriter, _ *http.Request) {
	served := srv.teiServedModel()
	var modelType map[string]any
	switch srv.teiKindOrDefault() {
	case teiKindReranker:
		modelType = map[string]any{teiKindReranker: map[string]any{
			"id2label": map[string]string{"0": "LABEL_0"},
			"label2id": map[string]int{"LABEL_0": 0},
		}}
	case teiKindClassifier:
		modelType = map[string]any{teiKindClassifier: map[string]any{
			"id2label": map[string]string{"0": "negative", "1": "positive"},
			"label2id": map[string]int{"negative": 0, "positive": 1},
		}}
	default:
		modelType = map[string]any{teiKindEmbedding: map[string]string{"pooling": "cls"}}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"model_id":          "/data/" + strings.ReplaceAll(served, "/", "-"),
		"served_model_name": served,
		"model_dtype":       "float16",
		"max_input_length":  8192,
		"model_type":        modelType,
	})
}

// teiError is a TEI error response. TEI serialises ErrorType by variant name and
// maps it to a status: Empty is 400, Validation 422, Backend 424 (router/src/http/
// server.rs, router/src/lib.rs).
type teiError struct {
	errType string
	msg     string
	status  int
	// plain marks an axum extractor rejection: a text/plain body rather than the
	// JSON error envelope, because the request never reached TEI's own handler.
	plain bool
}

func teiEmptyErr(msg string) *teiError {
	return &teiError{status: http.StatusBadRequest, errType: "Empty", msg: msg}
}

// teiValidationErr is a body that parses as JSON but does not fit the request type
// (a missing field, a wrong JSON type). axum 0.7's Json extractor rejects it with a
// plain-text 422 before the handler runs, so there is no error_type to report.
func teiValidationErr(msg string) *teiError {
	return &teiError{status: http.StatusUnprocessableEntity, plain: true,
		msg: "Failed to deserialize the JSON body into the target type: " + msg}
}

// teiSyntaxErr is a body that is not JSON at all: axum answers a plain-text 400.
func teiSyntaxErr(msg string) *teiError {
	return &teiError{status: http.StatusBadRequest, plain: true,
		msg: "Failed to parse the request body as JSON: " + msg}
}

// teiWrongKindErr is what TEI answers when a route needs a different kind of
// model than the one it loaded (an embed call to a reranker, and so on).
func teiWrongKindErr(msg string) *teiError {
	return &teiError{status: http.StatusFailedDependency, errType: "Backend", msg: msg}
}

// writePlain emits an axum extractor rejection.
func (e *teiError) writePlain(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(e.status)
	// text/plain with nosniff is never rendered as HTML; this is a test mock.
	_, _ = io.WriteString(w, e.msg) //nolint:gosec // G705: plain-text body, not HTML
}

// write emits the native {"error","error_type"} shape.
func (e *teiError) write(w http.ResponseWriter) {
	if e.plain {
		e.writePlain(w)
		return
	}
	writeJSON(w, e.status, map[string]any{"error": e.msg, "error_type": e.errType})
}

// writeOpenAI emits the OpenAI-compatible shape /v1/embeddings uses. Its key is
// "type": OpenAICompatErrorResponse renames error_type on serialisation.
func (e *teiError) writeOpenAI(w http.ResponseWriter) {
	if e.plain {
		e.writePlain(w)
		return
	}
	writeJSON(w, e.status, map[string]any{"message": e.msg, "code": e.status, "type": e.errType})
}

// requireTEIKind answers 424 unless this instance serves one of the allowed kinds.
func (srv *mockServer) requireTEIKind(msg string, allowed ...string) *teiError {
	if slices.Contains(allowed, srv.teiKindOrDefault()) {
		return nil
	}
	return teiWrongKindErr(msg)
}

// The wrong-kind messages are upstream's, behind the "Backend error: " prefix of
// TextEmbeddingsError::Backend. core/src/infer.rs capitalises its two; the
// re-ranker one is written in router/src/http/server.rs and is not.
const (
	teiNotEmbeddingMsg  = "Backend error: Model is not an embedding model"
	teiNotRerankerMsg   = "Backend error: model is not a re-ranker model"
	teiNotClassifierMsg = "Backend error: Model is not a classifier model"
)

// teiDecode reads a JSON object body as axum 0.7's Json extractor does: text that
// is not JSON is a 400, JSON of the wrong shape a 422, both plain text.
func teiDecode(r *http.Request, v any) *teiError {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return teiSyntaxErr(err.Error())
	}
	if !json.Valid(body) {
		return teiSyntaxErr("invalid JSON")
	}
	if err := json.Unmarshal(body, v); err != nil {
		return teiValidationErr(err.Error())
	}
	return nil
}

// teiPresent reports whether a raw JSON field was sent and not null.
func teiPresent(raw json.RawMessage) bool {
	return len(raw) > 0 && string(raw) != "null"
}

// teiInputCount validates a native "inputs" (or OpenAI "input") value for the
// embedding routes and returns how many inputs it carries. TEI takes a string, or
// an array of strings, an array of token ids (one sequence) or an array of token id
// arrays (a batch); anything else is a
// shape error. An empty array is Empty (400) straight away, as the handler checks
// it. An empty string, or empty id array, is only found when the tokeniser runs,
// which is after the model-kind check, so it comes back as blank for the caller to
// report once the kind is known. Whitespace-only text is not empty to TEI.
func teiInputCount(raw json.RawMessage, field string) (count int, blank, err *teiError) {
	if !teiPresent(raw) {
		return 0, nil, teiValidationErr("missing field `" + field + "`")
	}
	var input any
	if err := json.Unmarshal(raw, &input); err != nil {
		return 0, nil, teiValidationErr("invalid `" + field + "`: " + err.Error())
	}
	emptyInput := teiEmptyErr("`" + field + "` cannot be empty")
	switch v := input.(type) {
	case string:
		if v == "" {
			blank = emptyInput
		}
		return 1, blank, nil
	case []any:
		if len(v) == 0 {
			return 0, nil, emptyInput
		}
		// A flat array of numbers is one sequence of token ids (TEI's InputIds),
		// not a batch of items.
		if teiAllNumbers(v) {
			if !teiValidTokenIDs(v) {
				return 0, nil, teiValidationErr(teiTokenIDMsg(field))
			}
			return 1, nil, nil
		}
		for _, item := range v {
			itemBlank, err := teiBatchItem(item, field, emptyInput)
			if err != nil {
				return 0, nil, err
			}
			if itemBlank != nil {
				blank = itemBlank
			}
		}
		return len(v), blank, nil
	default:
		return 0, nil, teiValidationErr("`" + field + "` must be a string or an array of strings")
	}
}

// teiBatchItem validates one element of a batch: a string or an array of token
// ids. It returns emptyInput as blank when the element is empty.
func teiBatchItem(item any, field string, emptyInput *teiError) (blank, err *teiError) {
	switch it := item.(type) {
	case string:
		if it == "" {
			return emptyInput, nil
		}
	case []any:
		for _, id := range it {
			if _, ok := id.(float64); !ok {
				return nil, teiValidationErr("`" + field + "` token ids must be numbers")
			}
		}
		if !teiValidTokenIDs(it) {
			return nil, teiValidationErr(teiTokenIDMsg(field))
		}
		if len(it) == 0 {
			return emptyInput, nil
		}
	default:
		return nil, teiValidationErr("`" + field + "` must be a string or an array of strings")
	}
	return nil, nil
}

// teiAllNumbers reports whether every element is a JSON number. Callers have
// already rejected the empty array.
func teiAllNumbers(v []any) bool {
	for _, item := range v {
		if _, ok := item.(float64); !ok {
			return false
		}
	}
	return true
}

// teiMaxTokenID is the largest value TEI accepts as a token id: it types them as
// u32 (InputType::Ids(Vec<u32>)), so fractional, negative or larger values fail
// deserialisation.
const teiMaxTokenID = 4294967295

func teiTokenIDMsg(field string) string {
	return "invalid `" + field + "`: token ids must be unsigned 32-bit integers"
}

// teiValidTokenIDs reports whether every element is a number that fits a u32.
// Callers have already checked each element is a float64.
func teiValidTokenIDs(v []any) bool {
	for _, item := range v {
		f, _ := item.(float64)
		if f < 0 || f > teiMaxTokenID || f != math.Trunc(f) {
			return false
		}
	}
	return true
}

// teiKindThenBlank applies the model-kind check, then reports blank input: TEI
// only discovers an empty string once it tokenises, after the kind check.
func (srv *mockServer) teiKindThenBlank(blank *teiError, msg string, allowed ...string) *teiError {
	if err := srv.requireTEIKind(msg, allowed...); err != nil {
		return err
	}
	return blank
}

// teiEmbeddingInputs is the shared prologue of the native embedding routes: parse
// the body, validate "inputs", then check the loaded model can embed.
func (srv *mockServer) teiEmbeddingInputs(r *http.Request) (int, *teiError) {
	var req struct {
		Inputs json.RawMessage `json:"inputs"`
	}
	if err := teiDecode(r, &req); err != nil {
		return 0, err
	}
	count, blank, err := teiInputCount(req.Inputs, "inputs")
	if err != nil {
		return 0, err
	}
	return count, srv.teiKindThenBlank(blank, teiNotEmbeddingMsg, teiKindEmbedding)
}

func (srv *mockServer) handleTEIEmbeddings(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Model string          `json:"model"`
		Input json.RawMessage `json:"input"`
	}
	if err := teiDecode(r, &req); err != nil {
		err.writeOpenAI(w)
		return
	}
	inputCount, blank, err := teiInputCount(req.Input, "input")
	if err == nil {
		err = srv.teiKindThenBlank(blank, teiNotEmbeddingMsg, teiKindEmbedding)
	}
	if err != nil {
		err.writeOpenAI(w)
		return
	}

	served := srv.teiServedModel()
	if req.Model != "" && req.Model != served {
		// Real TEI accepts any model name, logs a warning, and reports the model
		// it actually serves; mirror that so routing tests observe the served
		// identity rather than the requested one.
		slog.Warn("tei: requested model differs from served model", "requested", req.Model, "served", served)
	}

	dims := 8
	data := make([]map[string]any, inputCount)
	for i := range data {
		vec := make([]float64, dims)
		for d := range vec {
			vec[d] = float64(i*dims+d+1) / 100
		}
		data[i] = map[string]any{
			"object":    "embedding",
			"embedding": vec,
			"index":     i,
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"object": "list",
		"model":  served,
		"data":   data,
		"usage": map[string]int{
			"prompt_tokens": inputCount * dims,
			"total_tokens":  inputCount * dims,
		},
	})
}

// teiVector returns the deterministic 8-dim vector used across the native
// embedding endpoints so tests can assert on exact values.
func teiVector(i int) []float64 {
	const dims = 8
	vec := make([]float64, dims)
	for d := range vec {
		vec[d] = float64(i*dims+d+1) / 100
	}
	return vec
}

func (srv *mockServer) handleTEIEmbed(w http.ResponseWriter, r *http.Request) {
	count, err := srv.teiEmbeddingInputs(r)
	if err != nil {
		err.write(w)
		return
	}
	vectors := make([][]float64, count)
	for i := range vectors {
		vectors[i] = teiVector(i)
	}
	writeJSON(w, http.StatusOK, vectors)
}
func (srv *mockServer) handleTEIEmbedAll(w http.ResponseWriter, r *http.Request) {
	count, err := srv.teiEmbeddingInputs(r)
	if err != nil {
		err.write(w)
		return
	}
	// Per-token embeddings without pooling: [batch][tokens][dims]. The mock
	// emits a single token per input.
	all := make([][][]float64, count)
	for i := range all {
		all[i] = [][]float64{teiVector(i)}
	}
	writeJSON(w, http.StatusOK, all)
}
func (srv *mockServer) handleTEIEmbedSparse(w http.ResponseWriter, r *http.Request) {
	count, err := srv.teiEmbeddingInputs(r)
	if err != nil {
		err.write(w)
		return
	}
	sparse := make([][]map[string]any, count)
	for i := range sparse {
		sparse[i] = []map[string]any{
			{"index": 0, "value": 1.0},
			{"index": i + 1, "value": 0.5},
		}
	}
	writeJSON(w, http.StatusOK, sparse)
}

// teiTokenCLSID and teiTokenSEPID follow BERT's special token IDs, which TEI's
// WordPiece tokenisers also use.
const (
	teiTokenCLSID = 101
	teiTokenSEPID = 102
)

func (srv *mockServer) handleTEITokenize(w http.ResponseWriter, r *http.Request) {
	inputs, err := teiStringInputs(r)
	if err != nil {
		err.write(w)
		return
	}
	// TEI returns one array of token objects (id, text, special, start, stop)
	// per input, not bare ID arrays. The mock tokenises by splitting on
	// whitespace, deterministically, so tests can assert on exact values.
	tokens := make([][]map[string]any, len(inputs))
	for i, input := range inputs {
		tokens[i] = teiTokenise(input)
	}
	writeJSON(w, http.StatusOK, tokens)
}

// teiStringInputs extracts "inputs" as a single string or an array of strings,
// the two shapes TEI's native tokenize endpoint accepts. Blank input is Empty;
// anything else of the wrong shape is Validation.
func teiStringInputs(r *http.Request) ([]string, *teiError) {
	var req struct {
		Input json.RawMessage `json:"inputs"`
	}
	if err := teiDecode(r, &req); err != nil {
		return nil, err
	}
	if !teiPresent(req.Input) {
		return nil, teiValidationErr("missing field `inputs`")
	}
	var input any
	if err := json.Unmarshal(req.Input, &input); err != nil {
		return nil, teiValidationErr("invalid `inputs`: " + err.Error())
	}
	switch v := input.(type) {
	case string:
		if v == "" {
			return nil, teiEmptyErr("`inputs` cannot be empty")
		}
		return []string{v}, nil
	case []any:
		if len(v) == 0 {
			return nil, teiEmptyErr("`inputs` cannot be empty")
		}
		inputs := make([]string, 0, len(v))
		for _, item := range v {
			s, ok := item.(string)
			if !ok {
				return nil, teiValidationErr("`inputs` must be a string or an array of strings")
			}
			if s == "" {
				return nil, teiEmptyErr("`inputs` cannot contain an empty input")
			}
			inputs = append(inputs, s)
		}
		return inputs, nil
	default:
		return nil, teiValidationErr("`inputs` must be a string or an array of strings")
	}
}

// teiTokenise returns [CLS], one token per whitespace-separated word, then
// [SEP]. Word start/stop are byte offsets into the input.
func teiTokenise(input string) []map[string]any {
	tokens := []map[string]any{
		{"id": teiTokenCLSID, "text": "[CLS]", "special": true, "start": nil, "stop": nil},
	}
	// strings.Fields would lose the offsets, so scan bytes instead: UTF-8
	// continuation bytes are never mistaken for ASCII whitespace.
	start := -1
	for i := 0; i <= len(input); i++ {
		isSpace := i == len(input) || isASCIIWhitespace(input[i])
		if !isSpace && start < 0 {
			start = i
		}
		if isSpace && start >= 0 {
			tokens = append(tokens, map[string]any{
				"id":      teiWordTokenID(input[start:i]),
				"text":    input[start:i],
				"special": false,
				"start":   start,
				"stop":    i,
			})
			start = -1
		}
	}
	return append(tokens, map[string]any{
		"id": teiTokenSEPID, "text": "[SEP]", "special": true, "start": nil, "stop": nil,
	})
}

func isASCIIWhitespace(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n' || b == '\r'
}

// teiWordTokenID maps a word to a deterministic pseudo token ID via FNV-1a,
// kept clear of the special IDs 101 and 102.
func teiWordTokenID(word string) int {
	h := fnv.New64a()
	h.Write([]byte(word))
	return teiTokenSEPID + 1 + int(h.Sum64()%10000)
}

// handleTEIDecode takes "ids" as one sequence of token ids or a batch of them.
// Upstream deserialises `[]` as a single, empty sequence (InputIds::Single is
// tried first), which decodes to an empty string, so only the shape is checked.
func (srv *mockServer) handleTEIDecode(w http.ResponseWriter, r *http.Request) {
	var req struct {
		IDs json.RawMessage `json:"ids"`
	}
	if err := teiDecode(r, &req); err != nil {
		err.write(w)
		return
	}
	if !teiPresent(req.IDs) {
		teiValidationErr("missing field `ids`").write(w)
		return
	}
	var ids any
	if err := json.Unmarshal(req.IDs, &ids); err != nil {
		teiValidationErr("invalid `ids`: " + err.Error()).write(w)
		return
	}
	batch, ok := ids.([]any)
	if !ok {
		teiValidationErr("`ids` must be an array of token ids or an array of arrays").write(w)
		return
	}
	allNumbers := func(items []any) bool {
		return !slices.ContainsFunc(items, func(v any) bool { _, isNum := v.(float64); return !isNum })
	}
	// A nested first element means a batch of sequences; otherwise the whole
	// array is a single sequence.
	if len(batch) > 0 {
		if _, nested := batch[0].([]any); nested {
			for _, seq := range batch {
				if items, isArr := seq.([]any); !isArr || !allNumbers(items) {
					teiValidationErr("`ids` batch elements must be arrays of token ids").write(w)
					return
				}
			}
			out := make([]string, len(batch))
			for i := range out {
				out[i] = fmt.Sprintf("decoded-%d", i)
			}
			writeJSON(w, http.StatusOK, out)
			return
		}
	}
	if !allNumbers(batch) {
		teiValidationErr("`ids` must contain only token ids").write(w)
		return
	}
	writeJSON(w, http.StatusOK, []string{"decoded-0"})
}

// teiPredictInputs follows TEI's hand-written PredictInput deserialiser
// (router/src/http/types.rs). A string, ["a"] or ["a","b"] is ONE sequence (the
// last being a sentence pair); only an array whose first element is itself an
// array is a batch, with each element a 1 or 2 string sequence. It returns the
// sequence count and whether the request was a batch. An empty array is Empty
// (400) at once; blank text is only found by the tokeniser, after the kind check,
// so it comes back as blank. Every other malformed shape is Validation (422).
func teiPredictInputs(r *http.Request) (count int, batch bool, blank, err *teiError) {
	var req struct {
		Inputs json.RawMessage `json:"inputs"`
	}
	if err := teiDecode(r, &req); err != nil {
		return 0, false, nil, err
	}
	if !teiPresent(req.Inputs) {
		return 0, false, nil, teiValidationErr("missing field `inputs`")
	}
	var in any
	if err := json.Unmarshal(req.Inputs, &in); err != nil {
		return 0, false, nil, teiValidationErr("invalid `inputs`: " + err.Error())
	}
	emptyInput := teiEmptyErr("`inputs` cannot be empty")
	if str, ok := in.(string); ok {
		if str == "" {
			blank = emptyInput
		}
		return 1, false, blank, nil
	}
	items, ok := in.([]any)
	if !ok {
		return 0, false, nil, teiValidationErr("inputs must be a string, a string pair or a batch")
	}
	if len(items) == 0 {
		return 0, false, nil, emptyInput
	}
	if _, nested := items[0].([]any); !nested {
		if len(items) > 2 {
			return 0, false, nil, teiValidationErr("a sequence is one string or a pair")
		}
		seqBlank, seqErr := teiSequenceTexts(items, emptyInput)
		if seqErr != nil {
			return 0, false, nil, seqErr
		}
		return 1, false, seqBlank, nil
	}
	for _, it := range items {
		seq, isArr := it.([]any)
		if !isArr || len(seq) < 1 || len(seq) > 2 {
			return 0, false, nil, teiValidationErr("batch elements must be 1 or 2 strings")
		}
		b, err := teiSequenceTexts(seq, emptyInput)
		if err != nil {
			return 0, false, nil, err
		}
		if b != nil {
			blank = b
		}
	}
	return len(items), true, blank, nil
}

// teiSequenceTexts checks one predict sequence is made of strings. TEI calls a
// single string empty when it is "", and a pair empty only when both halves are,
// so ["a",""] is a valid pair.
func teiSequenceTexts(seq []any, emptyInput *teiError) (blank, err *teiError) {
	empty := true
	for _, e := range seq {
		str, isStr := e.(string)
		if !isStr {
			return nil, teiValidationErr("sequence elements must be strings")
		}
		if str != "" {
			empty = false
		}
	}
	if empty {
		return emptyInput, nil
	}
	return nil, nil
}

// handleTEIPredict answers with a flat [{score,label}] for a single sequence
// and one such array per sequence for a batch, each sorted by score descending
// as upstream does. Scores vary by position so batches are distinguishable.
func (srv *mockServer) handleTEIPredict(w http.ResponseWriter, r *http.Request) {
	count, batch, blank, err := teiPredictInputs(r)
	if err == nil {
		// a reranker is a sequence classifier too, so it answers /predict
		err = srv.teiKindThenBlank(blank, teiNotClassifierMsg, teiKindClassifier, teiKindReranker)
	}
	if err != nil {
		err.write(w)
		return
	}
	one := func(i int) []map[string]any {
		pos := 0.6 + 0.05*float64(i%8)
		return []map[string]any{
			{"score": pos, "label": "positive"},
			{"score": 1 - pos, "label": "negative"},
		}
	}
	if !batch {
		writeJSON(w, http.StatusOK, one(0))
		return
	}
	out := make([][]map[string]any, count)
	for i := range out {
		out[i] = one(i)
	}
	writeJSON(w, http.StatusOK, out)
}

// handleTEIRerank validates in upstream's order: body shape (422), empty texts
// (400), whether the loaded model is a re-ranker (424), then the pairs. A blank
// query or text is fine; only a pair with both halves empty fails (400).
func (srv *mockServer) handleTEIRerank(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Query json.RawMessage `json:"query"`
		Texts json.RawMessage `json:"texts"`
	}
	if err := teiDecode(r, &req); err != nil {
		err.write(w)
		return
	}
	var query string
	var texts []string
	if !teiPresent(req.Query) || json.Unmarshal(req.Query, &query) != nil {
		teiValidationErr("`query` must be a string").write(w)
		return
	}
	if !teiPresent(req.Texts) || json.Unmarshal(req.Texts, &texts) != nil {
		teiValidationErr("`texts` must be an array of strings").write(w)
		return
	}
	if len(texts) == 0 {
		teiEmptyErr("`texts` cannot be empty").write(w)
		return
	}
	if err := srv.requireTEIKind(teiNotRerankerMsg, teiKindReranker); err != nil {
		err.write(w)
		return
	}
	if query == "" && slices.Contains(texts, "") {
		teiEmptyErr("`inputs` cannot be empty").write(w)
		return
	}
	ranks := make([]map[string]any, len(texts))
	for i := range texts {
		ranks[i] = map[string]any{
			"index": i,
			"score": float64(len(texts)-i) / float64(len(texts)),
		}
	}
	writeJSON(w, http.StatusOK, ranks)
}
func (srv *mockServer) handleTESimilarity(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Inputs *struct {
			SourceSentence *string  `json:"source_sentence"`
			Sentences      []string `json:"sentences"`
		} `json:"inputs"`
	}
	if err := teiDecode(r, &req); err != nil {
		err.write(w)
		return
	}
	if req.Inputs == nil || req.Inputs.SourceSentence == nil || req.Inputs.Sentences == nil {
		teiValidationErr("`inputs` needs `source_sentence` and `sentences`").write(w)
		return
	}
	sentences := req.Inputs.Sentences
	if len(sentences) == 0 {
		teiEmptyErr("`inputs.sentences` cannot be empty").write(w)
		return
	}
	if err := srv.requireTEIKind(teiNotEmbeddingMsg, teiKindEmbedding); err != nil {
		err.write(w)
		return
	}
	if *req.Inputs.SourceSentence == "" || slices.Contains(sentences, "") {
		teiEmptyErr("`inputs` cannot be empty").write(w)
		return
	}
	scores := make([]float64, len(sentences))
	for i := range scores {
		scores[i] = 0.9 - float64(i)*0.1
	}
	writeJSON(w, http.StatusOK, scores)
}
func (srv *mockServer) handleTEIMetrics(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	fmt.Fprint(w, "# HELP tei_mock_requests_total Mock request counter\n"+
		"# TYPE tei_mock_requests_total counter\n"+
		"tei_mock_requests_total 0\n")
}

// contextDone returns a closed channel when ctx is already done, otherwise the
// ctx.Done() channel. Used in select statements that must compile on Go 1.24.
func contextDone(ctx context.Context) <-chan struct{} {
	return ctx.Done()
}
