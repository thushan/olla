package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"math"
	"net/http"
	"regexp"
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
	name         string
	models       []string
	ttftMS       int
	tps          int
	streamChunks int
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
	mux.HandleFunc("GET /api/v0/models", srv.wrap(srv.handleLMStudioModels))
	mux.HandleFunc("GET /api/v1/models", srv.wrap(srv.handleLemonadeModels))

	// Text Embedding Inference (TEI) protocol - single model per instance,
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
		Size       int64   `json:"size"`
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
	}
	writeJSON(w, http.StatusOK, response{Models: models})
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
		models[i] = model{
			ID:               m,
			Object:           "model",
			Type:             "llm",
			Publisher:        "ollamock",
			Arch:             "llama",
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

// --- Text Embedding Inference (TEI) protocol ---

// teiServedModel returns the model this instance reports as served. TEI runs
// exactly one embedding model per instance, so the first configured model is
// used regardless of how many names the instance was started with.
func (srv *mockServer) teiServedModel() string {
	if len(srv.cfg.models) == 0 {
		return ""
	}
	return srv.cfg.models[0]
}

func (srv *mockServer) handleTEIInfo(w http.ResponseWriter, _ *http.Request) {
	served := srv.teiServedModel()
	writeJSON(w, http.StatusOK, map[string]any{
		"model_id":          "/data/" + strings.ReplaceAll(served, "/", "-"),
		"served_model_name": served,
		"model_dtype":       "float16",
		"max_input_length":  8192,
		"model_type": map[string]any{
			"embedding": map[string]string{"pooling": "cls"},
		},
	})
}

func (srv *mockServer) handleTEIEmbeddings(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Model string          `json:"model"`
		Input json.RawMessage `json:"input"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"error": map[string]string{"message": "invalid JSON body", "type": "invalid_request_error"},
		})
		return
	}

	served := srv.teiServedModel()
	if req.Model == "" {
		req.Model = served
	} else if req.Model != served {
		// Real TEI rejects unknown model names; mirroring that lets routing
		// tests fail loudly when a request lands on the wrong instance.
		writeJSON(w, http.StatusNotFound, map[string]any{
			"error": map[string]string{"message": fmt.Sprintf("Model %q not found", req.Model), "type": "model_not_found"},
		})
		return
	}

	inputCount := 1
	if len(req.Input) > 0 {
		var input any
		if err := json.Unmarshal(req.Input, &input); err == nil {
			switch v := input.(type) {
			case []any:
				inputCount = len(v)
			case string:
				inputCount = 1
			}
		}
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
		"model":  req.Model,
		"data":   data,
		"usage": map[string]int{
			"prompt_tokens": inputCount * dims,
			"total_tokens":  inputCount * dims,
		},
	})
}

// teiInputCount reports how many inputs a native TEI request carries. Native
// endpoints take "inputs" as either a single value or an array; the mock only
// needs the count to size its deterministic responses.
func teiInputCount(r *http.Request) (int, error) {
	var req struct {
		Input json.RawMessage `json:"inputs"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return 0, err
	}
	if len(req.Input) == 0 {
		return 0, errors.New("missing inputs")
	}
	var input any
	if err := json.Unmarshal(req.Input, &input); err != nil {
		return 0, err
	}
	if v, ok := input.([]any); ok {
		return len(v), nil
	}
	return 1, nil
}

// teiWriteError mirrors TEI's native error shape ({"error", "error_type"}) for
// malformed or missing inputs, which TEI reports as a 400 with error_type
// "validation".
func teiWriteError(w http.ResponseWriter, message string) {
	writeJSON(w, http.StatusBadRequest, map[string]any{"error": message, "error_type": "validation"})
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
	count, err := teiInputCount(r)
	if err != nil {
		teiWriteError(w, "inputs is required")
		return
	}
	vectors := make([][]float64, count)
	for i := range vectors {
		vectors[i] = teiVector(i)
	}
	writeJSON(w, http.StatusOK, vectors)
}

func (srv *mockServer) handleTEIEmbedAll(w http.ResponseWriter, r *http.Request) {
	count, err := teiInputCount(r)
	if err != nil {
		teiWriteError(w, "inputs is required")
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
	count, err := teiInputCount(r)
	if err != nil {
		teiWriteError(w, "inputs is required")
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

func (srv *mockServer) handleTEITokenize(w http.ResponseWriter, r *http.Request) {
	count, err := teiInputCount(r)
	if err != nil {
		teiWriteError(w, "inputs is required")
		return
	}
	ids := make([][]int, count)
	for i := range ids {
		ids[i] = []int{i*100 + 1, i*100 + 2}
	}
	writeJSON(w, http.StatusOK, ids)
}

func (srv *mockServer) handleTEIDecode(w http.ResponseWriter, r *http.Request) {
	var req struct {
		IDs json.RawMessage `json:"ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || len(req.IDs) == 0 {
		teiWriteError(w, "ids is required")
		return
	}
	var ids any
	if err := json.Unmarshal(req.IDs, &ids); err != nil {
		teiWriteError(w, "ids must be an array")
		return
	}
	batch, ok := ids.([]any)
	if !ok {
		teiWriteError(w, "ids must be an array")
		return
	}
	// A nested first element means a batch of sequences; otherwise the whole
	// array is a single sequence.
	if len(batch) > 0 {
		if _, nested := batch[0].([]any); nested {
			out := make([]string, len(batch))
			for i := range out {
				out[i] = fmt.Sprintf("decoded-%d", i)
			}
			writeJSON(w, http.StatusOK, out)
			return
		}
	}
	writeJSON(w, http.StatusOK, []string{"decoded-0"})
}

func (srv *mockServer) handleTEIPredict(w http.ResponseWriter, r *http.Request) {
	count, err := teiInputCount(r)
	if err != nil {
		teiWriteError(w, "inputs is required")
		return
	}
	predictions := make([]map[string]any, count)
	for i := range predictions {
		predictions[i] = map[string]any{
			"label": "mock-label",
			"score": 0.5 + float64(i)*0.1,
		}
	}
	writeJSON(w, http.StatusOK, predictions)
}

func (srv *mockServer) handleTEIRerank(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Query string   `json:"query"`
		Texts []string `json:"texts"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || len(req.Texts) == 0 {
		teiWriteError(w, "texts is required")
		return
	}
	ranks := make([]map[string]any, len(req.Texts))
	for i := range req.Texts {
		ranks[i] = map[string]any{
			"index": i,
			"score": float64(len(req.Texts)-i) / float64(len(req.Texts)),
		}
	}
	writeJSON(w, http.StatusOK, ranks)
}

func (srv *mockServer) handleTESimilarity(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Inputs struct {
			SourceSentence string   `json:"source_sentence"`
			Sentences      []string `json:"sentences"`
		} `json:"inputs"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || len(req.Inputs.Sentences) == 0 {
		teiWriteError(w, "inputs.sentences is required")
		return
	}
	scores := make([]float64, len(req.Inputs.Sentences))
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
