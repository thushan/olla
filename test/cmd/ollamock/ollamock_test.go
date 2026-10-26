package main

import (
	"bufio"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// testServer creates an isolated httptest.Server for each test, so tests never
// share mutable state. The server is closed automatically via t.Cleanup.
func testServer(t *testing.T, models ...string) *httptest.Server {
	t.Helper()
	if len(models) == 0 {
		models = []string{"test-model"}
	}
	cfg := serverConfig{
		name:         "test",
		models:       models,
		ttftMS:       0,
		tps:          0,
		streamChunks: 3,
	}
	srv := newServer(cfg)
	ts := httptest.NewServer(srv.handler())
	t.Cleanup(ts.Close)
	return ts
}

// postJSON sends a POST with a JSON body and returns the response.
func postJSON(t *testing.T, url, body string) *http.Response {
	t.Helper()
	resp, err := http.Post(url, "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}
	return resp
}

// getURL sends a GET and returns the response.
func getURL(t *testing.T, url string) *http.Response {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	return resp
}

// decodeJSON decodes the response body into v, closing the body.
func decodeJSON(t *testing.T, resp *http.Response, v any) {
	t.Helper()
	defer resp.Body.Close()
	if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
		t.Fatalf("decode JSON: %v", err)
	}
}

// --- Tests ---

// TestModelMetadataCaseInsensitiveParamSize covers the regression: the param
// size regex used to be case-sensitive, so a model named with an uppercase
// unit suffix (as real Ollama library names commonly are, e.g. "Llama-3-8B")
// matched nothing, leaving paramDisplay empty and sizeBytes 0.
func TestModelMetadataCaseInsensitiveParamSize(t *testing.T) {
	t.Parallel()

	_, paramDisplay, billions, _ := modelMetadata("Llama-3-8B")
	if paramDisplay != "8B" {
		t.Errorf("paramDisplay = %q, want 8B", paramDisplay)
	}
	if billions != 8 {
		t.Errorf("billions = %v, want 8", billions)
	}
}

// TestModelMetadataNonFiniteGuard covers the non-finite ParseFloat guard: a
// name whose digits parse to Inf must not propagate into billions/paramDisplay,
// which would otherwise make sizeBytes compute an Inf/garbage size.
func TestModelMetadataNonFiniteGuard(t *testing.T) {
	t.Parallel()

	// A run of digits long enough that ParseFloat overflows to +Inf.
	huge := strings.Repeat("9", 400) + "b"
	_, paramDisplay, billions, _ := modelMetadata(huge)
	if paramDisplay != "" {
		t.Errorf("paramDisplay = %q, want empty on non-finite parse", paramDisplay)
	}
	if billions != 0 {
		t.Errorf("billions = %v, want 0 on non-finite parse", billions)
	}
}

func TestOllamaTagsFormat(t *testing.T) {
	t.Parallel()
	ts := testServer(t, "llama3.2", "phi4")

	resp := getURL(t, ts.URL+"/api/tags")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}

	var body struct {
		Models []struct {
			Name    string `json:"name"`
			Model   string `json:"model"`
			Digest  string `json:"digest"`
			Details struct {
				Family string `json:"family"`
			} `json:"details"`
		} `json:"models"`
	}
	decodeJSON(t, resp, &body)

	if len(body.Models) != 2 {
		t.Fatalf("want 2 models, got %d", len(body.Models))
	}
	m := body.Models[0]
	if m.Name == "" {
		t.Error("name field is empty")
	}
	if m.Model == "" {
		t.Error("model field is empty")
	}
	if m.Digest == "" {
		t.Error("digest field is empty")
	}
	if m.Details.Family == "" {
		t.Error("details.family field is empty")
	}
}

func TestOpenAIModelsList(t *testing.T) {
	t.Parallel()
	ts := testServer(t, "gpt-mock")

	resp := getURL(t, ts.URL+"/v1/models")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}

	var body struct {
		Object string `json:"object"`
		Data   []struct {
			ID     string `json:"id"`
			Object string `json:"object"`
		} `json:"data"`
	}
	decodeJSON(t, resp, &body)

	if body.Object != "list" {
		t.Errorf("want object=list, got %q", body.Object)
	}
	if len(body.Data) != 1 {
		t.Fatalf("want 1 model, got %d", len(body.Data))
	}
	if body.Data[0].ID != "gpt-mock" {
		t.Errorf("want id=gpt-mock, got %q", body.Data[0].ID)
	}
	if body.Data[0].Object != "model" {
		t.Errorf("want object=model, got %q", body.Data[0].Object)
	}
}

func TestOpenAINonStreamChat(t *testing.T) {
	t.Parallel()
	ts := testServer(t)

	resp := postJSON(t, ts.URL+"/v1/chat/completions",
		`{"model":"test-model","stream":false,"messages":[{"role":"user","content":"hi"}]}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}

	var body struct {
		Object  string `json:"object"`
		Choices []struct {
			Message struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
			TotalTokens      int `json:"total_tokens"`
		} `json:"usage"`
	}
	decodeJSON(t, resp, &body)

	if body.Object != "chat.completion" {
		t.Errorf("want object=chat.completion, got %q", body.Object)
	}
	if len(body.Choices) == 0 {
		t.Fatal("no choices in response")
	}
	if body.Choices[0].FinishReason != "stop" {
		t.Errorf("want finish_reason=stop, got %q", body.Choices[0].FinishReason)
	}
	if body.Usage.TotalTokens == 0 {
		t.Error("usage.total_tokens is 0")
	}
}

func TestOpenAIStreamChat(t *testing.T) {
	t.Parallel()
	ts := testServer(t)

	resp := postJSON(t, ts.URL+"/v1/chat/completions",
		`{"model":"test-model","stream":true,"messages":[{"role":"user","content":"hi"}]}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
	defer resp.Body.Close()

	var chunks []string
	var sawDone bool
	var sawContent bool

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		if line == "data: [DONE]" {
			sawDone = true
			break
		}
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		chunks = append(chunks, data)

		var chunk struct {
			Choices []struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
			} `json:"choices"`
		}
		if err := json.Unmarshal([]byte(data), &chunk); err == nil {
			if len(chunk.Choices) > 0 && chunk.Choices[0].Delta.Content != "" {
				sawContent = true
			}
		}
	}

	if !sawDone {
		t.Error("stream did not end with data: [DONE]")
	}
	if !sawContent {
		t.Error("no chunk with delta content found")
	}
	if len(chunks) == 0 {
		t.Error("no SSE chunks received")
	}
}

func TestOllamaChatStreamDefault(t *testing.T) {
	t.Parallel()
	ts := testServer(t)

	// Ollama defaults to streaming when stream field is absent.
	resp := postJSON(t, ts.URL+"/api/chat",
		`{"model":"test-model","messages":[{"role":"user","content":"hi"}]}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
	defer resp.Body.Close()

	var lastLine map[string]any
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var obj map[string]any
		if err := json.Unmarshal([]byte(line), &obj); err != nil {
			t.Fatalf("invalid NDJSON line: %q: %v", line, err)
		}
		lastLine = obj
	}

	if lastLine == nil {
		t.Fatal("no NDJSON lines received")
	}

	done, ok := lastLine["done"].(bool)
	if !ok || !done {
		t.Errorf("final line has done != true: %v", lastLine["done"])
	}
}

func TestOllamaChatNonStream(t *testing.T) {
	t.Parallel()
	ts := testServer(t)

	resp := postJSON(t, ts.URL+"/api/chat",
		`{"model":"test-model","stream":false,"messages":[{"role":"user","content":"hi"}]}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}

	var body map[string]any
	decodeJSON(t, resp, &body)

	done, ok := body["done"].(bool)
	if !ok || !done {
		t.Errorf("want done=true, got %v", body["done"])
	}
	if _, ok := body["message"]; !ok {
		t.Error("missing message field in non-stream chat response")
	}
}

func TestAnthropicStreamEventOrder(t *testing.T) {
	t.Parallel()
	ts := testServer(t)

	resp := postJSON(t, ts.URL+"/v1/messages",
		`{"model":"test-model","stream":true,"messages":[{"role":"user","content":"hi"}],"max_tokens":100}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
	defer resp.Body.Close()

	// Collect event types in order.
	var eventOrder []string
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "event: ") {
			eventOrder = append(eventOrder, strings.TrimPrefix(line, "event: "))
		}
	}

	// Validate required sequence.
	want := []string{
		"message_start",
		"content_block_start",
		"content_block_stop",
		"message_delta",
		"message_stop",
	}
	// Find each required event in order (content_block_delta(s) sit between
	// content_block_start and content_block_stop).
	pos := 0
	for _, w := range want {
		found := false
		for pos < len(eventOrder) {
			if eventOrder[pos] == w {
				pos++
				found = true
				break
			}
			pos++
		}
		if !found {
			t.Errorf("missing event %q in stream; got sequence: %v", w, eventOrder)
		}
	}

	// Confirm at least one content_block_delta was emitted.
	hasDelta := false
	for _, e := range eventOrder {
		if e == "content_block_delta" {
			hasDelta = true
			break
		}
	}
	if !hasDelta {
		t.Error("no content_block_delta events in stream")
	}
}

func TestBehaviourModeError(t *testing.T) {
	t.Parallel()
	ts := testServer(t)

	// Switch to error mode with status 503.
	patchResp := postJSON(t, ts.URL+"/_mock/behaviour",
		`{"mode":"error","error_status":503}`)
	if patchResp.StatusCode != http.StatusOK {
		patchResp.Body.Close()
		t.Fatalf("PATCH behaviour want 200, got %d", patchResp.StatusCode)
	}
	patchResp.Body.Close()

	// Any subsequent request should get 503.
	resp := getURL(t, ts.URL+"/v1/models")
	resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("want 503 under error mode, got %d", resp.StatusCode)
	}
}

func TestMockStats(t *testing.T) {
	t.Parallel()
	ts := testServer(t)

	path := "/v1/chat/completions"
	const requestCount = 3

	for range requestCount {
		resp := postJSON(t, ts.URL+path,
			`{"model":"test-model","stream":false,"messages":[{"role":"user","content":"hi"}]}`)
		resp.Body.Close()
	}

	statsResp := getURL(t, ts.URL+"/_mock/stats")
	if statsResp.StatusCode != http.StatusOK {
		statsResp.Body.Close()
		t.Fatalf("/_mock/stats want 200, got %d", statsResp.StatusCode)
	}

	var stats struct {
		Total  int64            `json:"total"`
		ByPath map[string]int64 `json:"by_path"`
	}
	decodeJSON(t, statsResp, &stats)

	if stats.Total < requestCount {
		t.Errorf("want total >= %d, got %d", requestCount, stats.Total)
	}
	if stats.ByPath[path] < requestCount {
		t.Errorf("want by_path[%q] >= %d, got %d", path, requestCount, stats.ByPath[path])
	}
}

func TestFailHealth(t *testing.T) {
	t.Parallel()
	ts := testServer(t)

	// Enable fail_health.
	patchResp := postJSON(t, ts.URL+"/_mock/behaviour", `{"fail_health":true}`)
	if patchResp.StatusCode != http.StatusOK {
		patchResp.Body.Close()
		t.Fatalf("PATCH behaviour want 200, got %d", patchResp.StatusCode)
	}
	patchResp.Body.Close()

	// Health endpoint should now return 503.
	healthResp := getURL(t, ts.URL+"/health")
	healthResp.Body.Close()
	if healthResp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("want 503 on /health with fail_health, got %d", healthResp.StatusCode)
	}

	// Non-health routes must be unaffected - fail_health is health-path only.
	modelsResp := getURL(t, ts.URL+"/v1/models")
	modelsResp.Body.Close()
	if modelsResp.StatusCode != http.StatusOK {
		t.Errorf("want 200 on /v1/models with fail_health, got %d", modelsResp.StatusCode)
	}
}

// TestTEITokenizeShape pins the TEI /tokenize response: arrays of token
// objects per input with real word text and byte offsets, plus determinism
// across calls so tests can assert on exact IDs.
func TestTEITokenizeShape(t *testing.T) {
	t.Parallel()
	ts := testServer(t)

	var body [][]struct {
		ID      int    `json:"id"`
		Text    string `json:"text"`
		Special bool   `json:"special"`
		Start   *int   `json:"start"`
		Stop    *int   `json:"stop"`
	}
	resp := postJSON(t, ts.URL+"/tokenize", `{"inputs":["hello world","Olla routes TEI"]}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
	decodeJSON(t, resp, &body)

	if len(body) != 2 {
		t.Fatalf("want 2 input arrays, got %d", len(body))
	}

	// "hello world": [CLS], hello (0..5), world (6..11), [SEP].
	if len(body[0]) != 4 {
		t.Fatalf("want 4 tokens for %q, got %d", "hello world", len(body[0]))
	}
	first := body[0]
	if first[0].ID != 101 || first[0].Text != "[CLS]" || !first[0].Special || first[0].Start != nil || first[0].Stop != nil {
		t.Errorf("[CLS] token wrong: %+v", first[0])
	}
	if first[1].ID == 101 || first[1].ID == 102 || first[1].Text != "hello" || first[1].Special {
		t.Errorf("word token 1 wrong: %+v", first[1])
	}
	if first[1].Start == nil || *first[1].Start != 0 || first[1].Stop == nil || *first[1].Stop != 5 {
		t.Errorf("%q offsets wrong: start=%v stop=%v", "hello", first[1].Start, first[1].Stop)
	}
	if first[2].Text != "world" || first[2].Start == nil || *first[2].Start != 6 || first[2].Stop == nil || *first[2].Stop != 11 {
		t.Errorf("%q token or offsets wrong: %+v", "world", first[2])
	}
	if first[3].ID != 102 || first[3].Text != "[SEP]" || !first[3].Special || first[3].Start != nil || first[3].Stop != nil {
		t.Errorf("[SEP] token wrong: %+v", first[3])
	}

	// "Olla routes TEI": three words plus specials.
	if len(body[1]) != 5 {
		t.Fatalf("want 5 tokens for %q, got %d", "Olla routes TEI", len(body[1]))
	}

	// Determinism: same request, same IDs, byte for byte.
	resp2 := postJSON(t, ts.URL+"/tokenize", `{"inputs":["hello world","Olla routes TEI"]}`)
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("second call want 200, got %d", resp2.StatusCode)
	}
	var body2 [][]map[string]any
	decodeJSON(t, resp2, &body2)
	first2 := body2[0]
	if first2[1]["id"] != float64(first[1].ID) {
		t.Errorf("token ID not deterministic: %v then %v", first[1].ID, first2[1]["id"])
	}

	// Single string input is accepted too.
	resp3 := postJSON(t, ts.URL+"/tokenize", `{"inputs":"hello world"}`)
	if resp3.StatusCode != http.StatusOK {
		t.Fatalf("single-string call want 200, got %d", resp3.StatusCode)
	}
	resp3.Body.Close()

	// Missing inputs is axum's plain-text 422, as upstream TEI reports it.
	status, ctype, raw := teiRaw(t, ts, "/tokenize", `{}`)
	if status != http.StatusUnprocessableEntity {
		t.Errorf("missing inputs want 422, got %d", status)
	}
	if !strings.HasPrefix(ctype, "text/plain") || !strings.HasPrefix(raw, "Failed to deserialize the JSON body") {
		t.Errorf("want a plain-text axum rejection, got %q %q", ctype, raw)
	}
}

// TestTEIPredictShapes pins TEI's PredictInput semantics: a string, ["a"] and
// the pair ["a","b"] are one sequence (flat response); only an array of arrays
// is a batch (one prediction array per sequence).
func TestTEIPredictShapes(t *testing.T) {
	t.Parallel()
	ts := testServerWith(t, serverConfig{teiKind: teiKindClassifier})

	sorted := func(p []struct {
		Score float64 `json:"score"`
		Label string  `json:"label"`
	}) bool {
		for i := 1; i < len(p); i++ {
			if p[i].Score > p[i-1].Score {
				return false
			}
		}
		return len(p) > 0
	}

	flat := []string{`"a"`, `["a"]`, `["a","b"]`}
	for _, in := range flat {
		resp := postJSON(t, ts.URL+"/predict", `{"inputs":`+in+`}`)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("%s: want 200, got %d", in, resp.StatusCode)
		}
		var got []struct {
			Score float64 `json:"score"`
			Label string  `json:"label"`
		}
		decodeJSON(t, resp, &got)
		if !sorted(got) {
			t.Errorf("%s: want flat score-desc array, got %+v", in, got)
		}
	}

	batches := map[string]int{
		`[["a"],["b"]]`:           2,
		`[["a","b"],["c","d"]]`:   2,
		`[["a"],["b","c"],["d"]]`: 3,
	}
	for in, want := range batches {
		resp := postJSON(t, ts.URL+"/predict", `{"inputs":`+in+`}`)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("%s: want 200, got %d", in, resp.StatusCode)
		}
		var got [][]struct {
			Score float64 `json:"score"`
			Label string  `json:"label"`
		}
		decodeJSON(t, resp, &got)
		if len(got) != want {
			t.Fatalf("%s: want %d prediction arrays, got %d", in, want, len(got))
		}
		for _, p := range got {
			if !sorted(p) {
				t.Errorf("%s: predictions not score-desc: %+v", in, p)
			}
		}
	}

	for _, in := range []string{`["a","b","c"]`, `["a",["b"]]`, `[["a","b","c"]]`, `[["a"],"b"]`, `5`} {
		resp := postJSON(t, ts.URL+"/predict", `{"inputs":`+in+`}`)
		resp.Body.Close()
		if resp.StatusCode != http.StatusUnprocessableEntity {
			t.Errorf("%s: want 422, got %d", in, resp.StatusCode)
		}
	}
}

// testServerWith builds an isolated server from an explicit config for tests
// that need flags the default helper leaves off.
func testServerWith(t *testing.T, cfg serverConfig) *httptest.Server {
	t.Helper()
	if cfg.name == "" {
		cfg.name = "test"
	}
	if len(cfg.models) == 0 {
		cfg.models = []string{"test-model"}
	}
	if cfg.streamChunks == 0 {
		cfg.streamChunks = 3
	}
	ts := httptest.NewServer(newServer(cfg).handler())
	t.Cleanup(ts.Close)
	return ts
}

// teiCall posts body to path and returns the status and decoded JSON body.
func teiCall(t *testing.T, ts *httptest.Server, path, body string) (int, map[string]any) {
	t.Helper()
	resp := postJSON(t, ts.URL+path, body)
	defer resp.Body.Close()
	var out map[string]any
	// success bodies are arrays for most routes, so a decode miss is not an error here
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

// TEI accepts embedding input as a string, strings, one sequence of token ids or
// a batch of token id arrays; the mock must too, on every route that takes it.
func TestTEIEmbedAcceptsTokenIDInput(t *testing.T) {
	t.Parallel()
	ts := testServerWith(t, serverConfig{teiKind: teiKindEmbedding})

	cases := []struct {
		name, path, body string
		wantRows         int
	}{
		{"embed single sequence", "/embed", `{"inputs":[101,42,102]}`, 1},
		{"embed batch of sequences", "/embed", `{"inputs":[[101,42],[7,8,9]]}`, 2},
		{"embed_all single sequence", "/embed_all", `{"inputs":[101,42,102]}`, 1},
		{"embed_sparse single sequence", "/embed_sparse", `{"inputs":[101,42,102]}`, 1},
		{"openai embeddings single sequence", "/v1/embeddings", `{"input":[101,42,102]}`, 1},
		{"openai embeddings batch", "/v1/embeddings", `{"input":[[101,42],[7]]}`, 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			resp := postJSON(t, ts.URL+tc.path, tc.body)
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				b, _ := io.ReadAll(resp.Body)
				t.Fatalf("status = %d, want 200 (body %s)", resp.StatusCode, b)
			}
			var raw json.RawMessage
			if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
				t.Fatal(err)
			}
			var rows []json.RawMessage
			if err := json.Unmarshal(raw, &rows); err != nil {
				var obj struct {
					Data []json.RawMessage `json:"data"`
				}
				if err := json.Unmarshal(raw, &obj); err != nil {
					t.Fatal(err)
				}
				rows = obj.Data
			}
			if len(rows) != tc.wantRows {
				t.Fatalf("got %d rows, want %d (%s)", len(rows), tc.wantRows, raw)
			}
		})
	}

	// mixed element types are still a shape error
	status, _, _ := teiRaw(t, ts, "/embed", `{"inputs":[101,"a"]}`)
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("mixed ids and strings: status = %d, want 422", status)
	}
}

// Real TEI types token ids as u32, so anything outside that range fails
// deserialisation with a 422, in both the flat and the batch form.
func TestTEIEmbedTokenIDRange(t *testing.T) {
	t.Parallel()
	ts := testServerWith(t, serverConfig{teiKind: teiKindEmbedding})

	cases := []struct {
		name, body string
		want       int
	}{
		{"zero single", `{"inputs":[0]}`, http.StatusOK},
		{"max u32 single", `{"inputs":[4294967295]}`, http.StatusOK},
		{"zero and max batch", `{"inputs":[[0],[4294967295]]}`, http.StatusOK},
		{"fractional single", `{"inputs":[101,1.5]}`, http.StatusUnprocessableEntity},
		{"negative single", `{"inputs":[101,-1]}`, http.StatusUnprocessableEntity},
		{"overflow single", `{"inputs":[101,4294967296]}`, http.StatusUnprocessableEntity},
		{"fractional batch", `{"inputs":[[101],[1.5]]}`, http.StatusUnprocessableEntity},
		{"negative batch", `{"inputs":[[101],[-1]]}`, http.StatusUnprocessableEntity},
		{"overflow batch", `{"inputs":[[101],[4294967296]]}`, http.StatusUnprocessableEntity},
		{"openai overflow single", `{"input":[4294967296]}`, http.StatusUnprocessableEntity},
		{"openai negative batch", `{"input":[[-1]]}`, http.StatusUnprocessableEntity},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			path := "/embed"
			if strings.HasPrefix(tc.body, `{"input":`) {
				path = "/v1/embeddings"
			}
			status, _, raw := teiRaw(t, ts, path, tc.body)
			if status != tc.want {
				t.Fatalf("status = %d, want %d (body %s)", status, tc.want, raw)
			}
		})
	}
}

// teiRaw posts body and returns the status, Content-Type and raw body, for the
// answers that are not JSON.
func teiRaw(t *testing.T, ts *httptest.Server, path, body string) (status int, contentType, raw string) {
	t.Helper()
	resp := postJSON(t, ts.URL+path, body)
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return resp.StatusCode, resp.Header.Get("Content-Type"), string(b)
}

// Real TEI answers empty inputs with 400 error_type "Empty". A body axum cannot
// fit to the request type never reaches the handler: JSON of the wrong shape is a
// plain-text 422 and text that is not JSON a plain-text 400. The model-kind check
// (424) comes before the tokeniser, so an empty string on the wrong kind is 424
// while an empty array, which the handler checks itself, stays 400. Every handler
// has to agree, whatever its shape.
func TestTEIErrorContract(t *testing.T) {
	t.Parallel()
	embedding := testServerWith(t, serverConfig{teiKind: teiKindEmbedding})
	reranker := testServerWith(t, serverConfig{teiKind: teiKindReranker})
	classifier := testServerWith(t, serverConfig{teiKind: teiKindClassifier})

	// plainRejection marks an axum extractor rejection, which has no error_type.
	const plainRejection = ""
	cases := []struct {
		srv        *httptest.Server
		path, body string
		status     int
		errType    string
	}{
		{embedding, "/embed", `{"inputs":[]}`, 400, "Empty"},
		{embedding, "/embed", `{"inputs":""}`, 400, "Empty"},
		{embedding, "/embed", `{"inputs":["a",""]}`, 400, "Empty"},
		{embedding, "/embed", `{"inputs":5}`, 422, plainRejection},
		{embedding, "/embed", `{"inputs":["a",5]}`, 422, plainRejection},
		{embedding, "/embed", `{}`, 422, plainRejection},
		{embedding, "/embed", `{"inputs":`, 400, plainRejection},
		{embedding, "/embed_all", `{"inputs":[]}`, 400, "Empty"},
		{embedding, "/embed_all", `{"inputs":{"a":1}}`, 422, plainRejection},
		{embedding, "/embed_sparse", `{"inputs":""}`, 400, "Empty"},
		{embedding, "/embed_sparse", `{"inputs":true}`, 422, plainRejection},
		{embedding, "/tokenize", `{"inputs":[]}`, 400, "Empty"},
		{embedding, "/tokenize", `{"inputs":""}`, 400, "Empty"},
		{embedding, "/tokenize", `{"inputs":[1]}`, 422, plainRejection},
		{embedding, "/decode", `{"ids":"x"}`, 422, plainRejection},
		{embedding, "/decode", `{"ids":["a"]}`, 422, plainRejection},
		{embedding, "/decode", `{"ids":[[1],"b"]}`, 422, plainRejection},
		{embedding, "/decode", `{}`, 422, plainRejection},
		{embedding, "/similarity", `{"inputs":{"source_sentence":"a","sentences":[]}}`, 400, "Empty"},
		{embedding, "/similarity", `{"inputs":{"source_sentence":"","sentences":["a"]}}`, 400, "Empty"},
		{embedding, "/similarity", `{"inputs":"x"}`, 422, plainRejection},
		{embedding, "/similarity", `{"inputs":{"sentences":["a"]}}`, 422, plainRejection},
		{reranker, "/rerank", `{"query":"q","texts":[]}`, 400, "Empty"},
		{reranker, "/rerank", `{"query":"","texts":[""]}`, 400, "Empty"},
		{reranker, "/embed", `{"inputs":""}`, 424, "Backend"},
		{reranker, "/embed", `{"inputs":[]}`, 400, "Empty"},
		{embedding, "/predict", `{"inputs":""}`, 424, "Backend"},
		{embedding, "/rerank", `{"query":"","texts":[""]}`, 424, "Backend"},
		{reranker, "/similarity", `{"inputs":{"source_sentence":"","sentences":["a"]}}`, 424, "Backend"},
		{reranker, "/rerank", `{"query":"q"}`, 422, plainRejection},
		{reranker, "/rerank", `{"query":"q","texts":"a"}`, 422, plainRejection},
		{reranker, "/rerank", `{"query":5,"texts":["a"]}`, 422, plainRejection},
		{classifier, "/predict", `{"inputs":""}`, 400, "Empty"},
		{classifier, "/predict", `{"inputs":["",""]}`, 400, "Empty"},
		{classifier, "/predict", `{"inputs":[]}`, 400, "Empty"},
		{classifier, "/predict", `{"inputs":[""]}`, 400, "Empty"},
		{classifier, "/predict", `{"inputs":[["a"],[""]]}`, 400, "Empty"},
		{classifier, "/predict", `{"inputs":5}`, 422, plainRejection},
		{classifier, "/predict", `{}`, 422, plainRejection},
		// a model of the wrong kind answers 424 even to a well-formed request
		{embedding, "/rerank", `{"query":"q","texts":["a"]}`, 424, "Backend"},
		{embedding, "/predict", `{"inputs":"a"}`, 424, "Backend"},
		{reranker, "/embed", `{"inputs":"a"}`, 424, "Backend"},
		{reranker, "/embed_all", `{"inputs":"a"}`, 424, "Backend"},
		{reranker, "/embed_sparse", `{"inputs":"a"}`, 424, "Backend"},
		{reranker, "/similarity", `{"inputs":{"source_sentence":"a","sentences":["b"]}}`, 424, "Backend"},
		{classifier, "/embed", `{"inputs":"a"}`, 424, "Backend"},
		{classifier, "/rerank", `{"query":"q","texts":["a"]}`, 424, "Backend"},
		// an empty texts array is checked by the handler, ahead of the kind
		{embedding, "/rerank", `{"query":"q","texts":[]}`, 400, "Empty"},
	}
	for _, tc := range cases {
		t.Run(tc.path+" "+tc.body, func(t *testing.T) {
			t.Parallel()
			if tc.errType == plainRejection {
				status, ctype, raw := teiRaw(t, tc.srv, tc.path, tc.body)
				if status != tc.status || !strings.HasPrefix(ctype, "text/plain") || !strings.HasPrefix(raw, "Failed to ") {
					t.Fatalf("got %d %q %q, want %d plain-text axum rejection", status, ctype, raw, tc.status)
				}
				return
			}
			status, body := teiCall(t, tc.srv, tc.path, tc.body)
			if status != tc.status {
				t.Fatalf("status = %d, want %d (body %v)", status, tc.status, body)
			}
			if body["error_type"] != tc.errType {
				t.Errorf("error_type = %v, want %s", body["error_type"], tc.errType)
			}
			if msg, _ := body["error"].(string); msg == "" {
				t.Errorf("error message empty: %v", body)
			}
		})
	}

	t.Run("well-formed requests succeed on the right kind", func(t *testing.T) {
		t.Parallel()
		for _, tc := range []struct {
			srv        *httptest.Server
			path, body string
		}{
			{embedding, "/embed", `{"inputs":["a","b"]}`},
			{embedding, "/embed", `{"inputs":[[1,2,3]]}`},
			// whitespace-only text is not empty to TEI
			{embedding, "/embed", `{"inputs":"   "}`},
			{embedding, "/tokenize", `{"inputs":" "}`},
			{reranker, "/rerank", `{"query":"","texts":["a"]}`},
			{reranker, "/rerank", `{"query":"q","texts":["a",""]}`},
			{reranker, "/rerank", `{"query":"  ","texts":[" "]}`},
			{classifier, "/predict", `{"inputs":["a",""]}`},
			{embedding, "/similarity", `{"inputs":{"source_sentence":" ","sentences":[" "]}}`},
			{embedding, "/tokenize", `{"inputs":"a b"}`},
			{embedding, "/decode", `{"ids":[1,2]}`},
			{embedding, "/decode", `{"ids":[]}`},
			{embedding, "/similarity", `{"inputs":{"source_sentence":"a","sentences":["b"]}}`},
			{reranker, "/rerank", `{"query":"q","texts":["a","b"]}`},
			{reranker, "/predict", `{"inputs":"a"}`},
			{classifier, "/predict", `{"inputs":"a"}`},
		} {
			resp := postJSON(t, tc.srv.URL+tc.path, tc.body)
			resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				t.Errorf("%s %s: status %d, want 200", tc.path, tc.body, resp.StatusCode)
			}
		}
	})

	t.Run("v1/embeddings uses the OpenAI-compatible error shape", func(t *testing.T) {
		t.Parallel()
		for body, want := range map[string]struct {
			status  int
			errType string
		}{
			`{"input":[]}`: {400, "Empty"},
			`{"input":""}`: {400, "Empty"},
		} {
			status, got := teiCall(t, embedding, "/v1/embeddings", body)
			if status != want.status || got["type"] != want.errType || got["code"] != float64(want.status) || got["message"] == "" {
				t.Errorf("%s: status %d body %v, want %d %s with message and code", body, status, got, want.status, want.errType)
			}
			if _, has := got["error_type"]; has {
				t.Errorf("%s: OpenAI shape must use \"type\", not \"error_type\": %v", body, got)
			}
		}
		for body, want := range map[string]int{`{"input":5}`: 422, `{"model":"x"}`: 422, `{"input":"a" `: 400} {
			if status, ctype, raw := teiRaw(t, embedding, "/v1/embeddings", body); status != want || !strings.HasPrefix(ctype, "text/plain") || raw == "" {
				t.Errorf("%s: got %d %q %q, want %d plain text", body, status, ctype, raw, want)
			}
		}
		status, got := teiCall(t, reranker, "/v1/embeddings", `{"input":""}`)
		if status != http.StatusFailedDependency || got["type"] != "Backend" || got["message"] != "Backend error: Model is not an embedding model" {
			t.Errorf("empty string on a reranker: %d %v, want 424 Backend with upstream's message", status, got)
		}
		if status, _ := teiCall(t, reranker, "/v1/embeddings", `{"input":"a"}`); status != http.StatusFailedDependency {
			t.Errorf("embeddings on a reranker: status %d, want 424", status)
		}
		if status, _ := teiCall(t, embedding, "/v1/embeddings", `{"input":["a","b"]}`); status != http.StatusOK {
			t.Errorf("valid embeddings: status %d, want 200", status)
		}
	})
}

func TestTEIInfoModelType(t *testing.T) {
	t.Parallel()
	for kind, labels := range map[string]bool{teiKindEmbedding: false, teiKindReranker: true, teiKindClassifier: true, "": false} {
		t.Run("kind="+kind, func(t *testing.T) {
			t.Parallel()
			ts := testServerWith(t, serverConfig{teiKind: kind})
			resp := getURL(t, ts.URL+"/info")
			defer resp.Body.Close()
			var info struct {
				ModelType map[string]map[string]any `json:"model_type"`
			}
			decodeJSON(t, resp, &info)

			want := kind
			if want == "" {
				want = teiKindEmbedding
			}
			if len(info.ModelType) != 1 {
				t.Fatalf("model_type should be a single-key tagged object, got %v", info.ModelType)
			}
			body, ok := info.ModelType[want]
			if !ok {
				t.Fatalf("model_type = %v, want key %q", info.ModelType, want)
			}
			if _, has := body["id2label"]; has != labels {
				t.Errorf("id2label present = %v, want %v", has, labels)
			}
		})
	}
}

func TestTEIWrongKindMessages(t *testing.T) {
	t.Parallel()
	embedding := testServerWith(t, serverConfig{teiKind: teiKindEmbedding})
	reranker := testServerWith(t, serverConfig{teiKind: teiKindReranker})
	for _, tc := range []struct {
		srv        *httptest.Server
		path, body string
		want       string
	}{
		{reranker, "/embed", `{"inputs":"a"}`, "Backend error: Model is not an embedding model"},
		{embedding, "/predict", `{"inputs":"a"}`, "Backend error: Model is not a classifier model"},
		{embedding, "/rerank", `{"query":"q","texts":["a"]}`, "Backend error: model is not a re-ranker model"},
	} {
		if _, body := teiCall(t, tc.srv, tc.path, tc.body); body["error"] != tc.want {
			t.Errorf("%s: error = %v, want %q", tc.path, body["error"], tc.want)
		}
	}
}
