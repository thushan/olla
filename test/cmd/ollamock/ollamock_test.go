package main

import (
	"bufio"
	"encoding/json"
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

	// Missing inputs stays a 400.
	resp4 := postJSON(t, ts.URL+"/tokenize", `{}`)
	resp4.Body.Close()
	if resp4.StatusCode != http.StatusBadRequest {
		t.Errorf("missing inputs want 400, got %d", resp4.StatusCode)
	}
}
