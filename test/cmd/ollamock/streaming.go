package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const roleAssistant = "assistant"

// inferenceRequest is the minimal request shape shared across all protocols.
// Stream nil means "use protocol default" - true for Ollama, false for OpenAI.
type inferenceRequest struct {
	Stream *bool  `json:"stream"`
	Model  string `json:"model"`
}

func parseInferenceRequest(r *http.Request) (inferenceRequest, error) {
	var req inferenceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err != io.EOF {
		return inferenceRequest{}, err
	}
	return req, nil
}

// streamBool is true if stream==nil (use given default) or stream==value.
func resolveStream(s *bool, defaultStream bool) bool {
	if s == nil {
		return defaultStream
	}
	return *s
}

// --- Ollama chat / generate ---

func (srv *mockServer) handleOllamaChat(w http.ResponseWriter, r *http.Request) {
	var req inferenceRequest
	body := readBody(r)
	_ = json.Unmarshal(body, &req)

	model := req.Model
	if model == "" && len(srv.cfg.models) > 0 {
		model = srv.cfg.models[0]
	}

	stream := resolveStream(req.Stream, true) // Ollama streams by default

	if stream {
		srv.streamOllamaChat(w, r, model)
	} else {
		srv.nonStreamOllamaChat(w, model)
	}
}

func (srv *mockServer) handleOllamaGenerate(w http.ResponseWriter, r *http.Request) {
	var req inferenceRequest
	body := readBody(r)
	_ = json.Unmarshal(body, &req)

	model := req.Model
	if model == "" && len(srv.cfg.models) > 0 {
		model = srv.cfg.models[0]
	}

	stream := resolveStream(req.Stream, true)

	if stream {
		srv.streamOllamaGenerate(w, r, model)
	} else {
		srv.nonStreamOllamaGenerate(w, model)
	}
}

func (srv *mockServer) nonStreamOllamaChat(w http.ResponseWriter, model string) {
	now := time.Now().UTC().Format(time.RFC3339)
	resp := map[string]any{
		"model":      model,
		"created_at": now,
		"message": map[string]any{
			"role":    roleAssistant,
			"content": fmt.Sprintf("BACKEND:%s model:%s", srv.cfg.name, model),
		},
		"done":                 true,
		"prompt_eval_count":    10,
		"eval_count":           20,
		"total_duration":       1_000_000_000,
		"load_duration":        100_000_000,
		"prompt_eval_duration": 200_000_000,
		"eval_duration":        700_000_000,
	}
	writeJSON(w, http.StatusOK, resp)
}

func (srv *mockServer) nonStreamOllamaGenerate(w http.ResponseWriter, model string) {
	now := time.Now().UTC().Format(time.RFC3339)
	resp := map[string]any{
		"model":                model,
		"created_at":           now,
		"response":             fmt.Sprintf("BACKEND:%s model:%s", srv.cfg.name, model),
		"done":                 true,
		"prompt_eval_count":    10,
		"eval_count":           20,
		"total_duration":       1_000_000_000,
		"load_duration":        100_000_000,
		"prompt_eval_duration": 200_000_000,
		"eval_duration":        700_000_000,
	}
	writeJSON(w, http.StatusOK, resp)
}

func (srv *mockServer) streamOllamaChat(w http.ResponseWriter, r *http.Request, model string) {
	w.Header().Set("Content-Type", "application/x-ndjson")
	rc := http.NewResponseController(w)

	applyTTFT(r.Context(), srv.cfg.ttftMS)

	n := srv.cfg.streamChunks
	b := srv.bstate.get()
	dropAt := (n + 1) / 2 // ceil(n/2) for drop_mid_stream

	now := time.Now().UTC().Format(time.RFC3339)

	for i := range n {
		if b.DropMidStream && i == dropAt {
			// Truncate mid-stream - client gets EOF without a done:true final line.
			tryHijackClose(w)
			return
		}

		chunk := map[string]any{
			"model":      model,
			"created_at": now,
			"message": map[string]any{
				"role":    roleAssistant,
				"content": fmt.Sprintf("The answer is 42. BACKEND:%s model:%s chunk:%d", srv.cfg.name, model, i),
			},
			"done": false,
		}
		writeNDJSON(w, chunk)
		_ = rc.Flush()
		applyTPS(r.Context(), srv.cfg.tps)
	}

	// Final done:true line with metrics.
	final := map[string]any{
		"model":      model,
		"created_at": now,
		"message": map[string]any{
			"role":    roleAssistant,
			"content": "",
		},
		"done":                 true,
		"prompt_eval_count":    10,
		"eval_count":           20,
		"total_duration":       1_000_000_000,
		"load_duration":        100_000_000,
		"prompt_eval_duration": 200_000_000,
		"eval_duration":        700_000_000,
	}
	writeNDJSON(w, final)
	_ = rc.Flush()
}

func (srv *mockServer) streamOllamaGenerate(w http.ResponseWriter, r *http.Request, model string) {
	w.Header().Set("Content-Type", "application/x-ndjson")
	rc := http.NewResponseController(w)

	applyTTFT(r.Context(), srv.cfg.ttftMS)

	n := srv.cfg.streamChunks
	b := srv.bstate.get()
	dropAt := (n + 1) / 2
	now := time.Now().UTC().Format(time.RFC3339)

	for i := range n {
		if b.DropMidStream && i == dropAt {
			tryHijackClose(w)
			return
		}

		chunk := map[string]any{
			"model":      model,
			"created_at": now,
			"response":   fmt.Sprintf("The answer is 42. BACKEND:%s model:%s chunk:%d", srv.cfg.name, model, i),
			"done":       false,
		}
		writeNDJSON(w, chunk)
		_ = rc.Flush()
		applyTPS(r.Context(), srv.cfg.tps)
	}

	final := map[string]any{
		"model":                model,
		"created_at":           now,
		"response":             "",
		"done":                 true,
		"prompt_eval_count":    10,
		"eval_count":           20,
		"total_duration":       1_000_000_000,
		"load_duration":        100_000_000,
		"prompt_eval_duration": 200_000_000,
		"eval_duration":        700_000_000,
	}
	writeNDJSON(w, final)
	_ = rc.Flush()
}

// --- OpenAI chat completions ---

func (srv *mockServer) handleOpenAIChat(w http.ResponseWriter, r *http.Request) {
	var req inferenceRequest
	body := readBody(r)
	_ = json.Unmarshal(body, &req)

	model := req.Model
	if model == "" && len(srv.cfg.models) > 0 {
		model = srv.cfg.models[0]
	}

	stream := resolveStream(req.Stream, false) // OpenAI defaults to non-streaming

	if stream {
		srv.streamOpenAIChat(w, r, model)
	} else {
		srv.nonStreamOpenAIChat(w, model)
	}
}

func (srv *mockServer) nonStreamOpenAIChat(w http.ResponseWriter, model string) {
	now := time.Now().Unix()
	resp := map[string]any{
		"id":      fmt.Sprintf("chatcmpl-mock-%s", srv.cfg.name),
		"object":  "chat.completion",
		"created": now,
		"model":   model,
		"choices": []map[string]any{
			{
				"index": 0,
				"message": map[string]any{
					"role":    roleAssistant,
					"content": fmt.Sprintf("BACKEND:%s reply", srv.cfg.name),
				},
				"finish_reason": "stop",
			},
		},
		"usage": map[string]any{
			"prompt_tokens":     10,
			"completion_tokens": 20,
			"total_tokens":      30,
		},
	}
	writeJSON(w, http.StatusOK, resp)
}

func (srv *mockServer) streamOpenAIChat(w http.ResponseWriter, r *http.Request, model string) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	rc := http.NewResponseController(w)

	applyTTFT(r.Context(), srv.cfg.ttftMS)

	id := fmt.Sprintf("chatcmpl-mock-%s", srv.cfg.name)
	now := time.Now().Unix()
	n := srv.cfg.streamChunks
	b := srv.bstate.get()
	dropAt := (n + 1) / 2

	for i := range n {
		if b.DropMidStream && i == dropAt {
			tryHijackClose(w)
			return
		}

		chunk := map[string]any{
			"id":      id,
			"object":  "chat.completion.chunk",
			"created": now,
			"model":   model,
			"choices": []map[string]any{
				{
					"index": 0,
					"delta": map[string]any{
						"role":    roleAssistant,
						"content": fmt.Sprintf("BACKEND:%s reply: chunk %d", srv.cfg.name, i),
					},
					"finish_reason": nil,
				},
			},
		}
		data, _ := json.Marshal(chunk)
		writeSSEData(w, data)
		_ = rc.Flush()
		applyTPS(r.Context(), srv.cfg.tps)
	}

	// Final chunk signals stop.
	final := map[string]any{
		"id":      id,
		"object":  "chat.completion.chunk",
		"created": now,
		"model":   model,
		"choices": []map[string]any{
			{
				"index":         0,
				"delta":         map[string]any{},
				"finish_reason": "stop",
			},
		},
	}
	data, _ := json.Marshal(final)
	writeSSEData(w, data)
	_ = rc.Flush()

	// Terminal usage chunk in the include_usage shape: choices:[] with populated usage.
	// Matches the non-stream path token numbers so streaming and non-streaming agree.
	// This exercises the choices:[] usage-capture path in the translation layer and
	// gives the translator a real output_tokens value instead of falling back to the
	// chars/4 estimate.
	usageChunk := map[string]any{
		"id":      id,
		"object":  "chat.completion.chunk",
		"created": now,
		"model":   model,
		"choices": []map[string]any{},
		"usage": map[string]any{
			"prompt_tokens":     10,
			"completion_tokens": 20,
			"total_tokens":      30,
		},
	}
	usageData, _ := json.Marshal(usageChunk)
	writeSSEData(w, usageData)
	_ = rc.Flush()

	_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
	_ = rc.Flush()
}

// --- OpenAI text completions (legacy) ---

func (srv *mockServer) handleOpenAICompletion(w http.ResponseWriter, r *http.Request) {
	var req inferenceRequest
	body := readBody(r)
	_ = json.Unmarshal(body, &req)

	model := req.Model
	if model == "" && len(srv.cfg.models) > 0 {
		model = srv.cfg.models[0]
	}

	stream := resolveStream(req.Stream, false)

	if stream {
		// Stream legacy completions using SSE - same shape as chat but with
		// text field instead of delta.content.
		srv.streamOpenAICompletion(w, r, model)
		return
	}

	now := time.Now().Unix()
	resp := map[string]any{
		"id":      fmt.Sprintf("cmpl-mock-%s", srv.cfg.name),
		"object":  "text_completion",
		"created": now,
		"model":   model,
		"choices": []map[string]any{
			{
				"text":          fmt.Sprintf("BACKEND:%s reply", srv.cfg.name),
				"index":         0,
				"finish_reason": "stop",
			},
		},
		"usage": map[string]any{
			"prompt_tokens":     10,
			"completion_tokens": 20,
			"total_tokens":      30,
		},
	}
	writeJSON(w, http.StatusOK, resp)
}

func (srv *mockServer) streamOpenAICompletion(w http.ResponseWriter, r *http.Request, model string) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	rc := http.NewResponseController(w)

	applyTTFT(r.Context(), srv.cfg.ttftMS)

	id := fmt.Sprintf("cmpl-mock-%s", srv.cfg.name)
	now := time.Now().Unix()
	n := srv.cfg.streamChunks

	for i := range n {
		chunk := map[string]any{
			"id":      id,
			"object":  "text_completion",
			"created": now,
			"model":   model,
			"choices": []map[string]any{
				{
					"text":          fmt.Sprintf("BACKEND:%s chunk %d", srv.cfg.name, i),
					"index":         0,
					"finish_reason": nil,
				},
			},
		}
		data, _ := json.Marshal(chunk)
		writeSSEData(w, data)
		_ = rc.Flush()
		applyTPS(r.Context(), srv.cfg.tps)
	}

	_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
	_ = rc.Flush()
}

// --- Anthropic Messages API ---

func (srv *mockServer) handleAnthropicMessages(w http.ResponseWriter, r *http.Request) {
	var req inferenceRequest
	body := readBody(r)
	_ = json.Unmarshal(body, &req)

	model := req.Model
	if model == "" && len(srv.cfg.models) > 0 {
		model = srv.cfg.models[0]
	}

	stream := resolveStream(req.Stream, false)

	if stream {
		srv.streamAnthropic(w, r, model)
	} else {
		srv.nonStreamAnthropic(w, model)
	}
}

func (srv *mockServer) nonStreamAnthropic(w http.ResponseWriter, model string) {
	resp := map[string]any{
		"id":    "msg_mock_001",
		"type":  "message",
		"role":  roleAssistant,
		"model": model,
		"content": []map[string]any{
			{
				"type": "text",
				"text": fmt.Sprintf("BACKEND:%s reply", srv.cfg.name),
			},
		},
		"stop_reason": "end_turn",
		"usage": map[string]any{
			"input_tokens":  10,
			"output_tokens": 20,
		},
	}
	writeJSON(w, http.StatusOK, resp)
}

func (srv *mockServer) streamAnthropic(w http.ResponseWriter, r *http.Request, model string) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	rc := http.NewResponseController(w)

	applyTTFT(r.Context(), srv.cfg.ttftMS)

	n := srv.cfg.streamChunks
	b := srv.bstate.get()
	dropAt := (n + 1) / 2

	// 1. message_start
	writeSSEEvent(w, "message_start", map[string]any{
		"type": "message_start",
		"message": map[string]any{
			"id":    "msg_mock_001",
			"type":  "message",
			"role":  roleAssistant,
			"model": model,
			"usage": map[string]any{
				"input_tokens":  10,
				"output_tokens": 0,
			},
		},
	})
	_ = rc.Flush()

	// 2. content_block_start
	writeSSEEvent(w, "content_block_start", map[string]any{
		"type":  "content_block_start",
		"index": 0,
		"content_block": map[string]any{
			"type": "text",
			"text": "",
		},
	})
	_ = rc.Flush()

	if b.DropMidStream {
		// Stop after content_block_start + ceil(n/2) deltas - no closing events.
		for i := range dropAt {
			chunk := fmt.Sprintf("The answer is 42. BACKEND:%s model:%s chunk:%d", srv.cfg.name, model, i)
			writeSSEEvent(w, "content_block_delta", map[string]any{
				"type":  "content_block_delta",
				"index": 0,
				"delta": map[string]any{
					"type": "text_delta",
					"text": chunk,
				},
			})
			_ = rc.Flush()
			applyTPS(r.Context(), srv.cfg.tps)
		}
		tryHijackClose(w)
		return
	}

	// 3. N× content_block_delta
	for i := range n {
		chunk := fmt.Sprintf("The answer is 42. BACKEND:%s model:%s chunk:%d", srv.cfg.name, model, i)
		writeSSEEvent(w, "content_block_delta", map[string]any{
			"type":  "content_block_delta",
			"index": 0,
			"delta": map[string]any{
				"type": "text_delta",
				"text": chunk,
			},
		})
		_ = rc.Flush()
		applyTPS(r.Context(), srv.cfg.tps)
	}

	// 4. content_block_stop
	writeSSEEvent(w, "content_block_stop", map[string]any{
		"type":  "content_block_stop",
		"index": 0,
	})
	_ = rc.Flush()

	// 5. message_delta
	writeSSEEvent(w, "message_delta", map[string]any{
		"type": "message_delta",
		"delta": map[string]any{
			"stop_reason":   "end_turn",
			"stop_sequence": nil,
		},
		"usage": map[string]any{
			"output_tokens": 20,
		},
	})
	_ = rc.Flush()

	// 6. message_stop
	writeSSEEvent(w, "message_stop", map[string]any{
		"type": "message_stop",
	})
	_ = rc.Flush()
}

// --- Shared streaming helpers ---

// writeSSEEvent writes an Anthropic-style named SSE event with JSON data.
func writeSSEEvent(w http.ResponseWriter, eventType string, data any) {
	payload, _ := json.Marshal(data)
	_, _ = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", eventType, payload)
}

// writeSSEData writes an OpenAI-style SSE data line (no event: prefix).
func writeSSEData(w http.ResponseWriter, data []byte) {
	_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
}

// writeNDJSON writes a single NDJSON line (newline-delimited JSON).
func writeNDJSON(w http.ResponseWriter, v any) {
	data, _ := json.Marshal(v)
	_, _ = fmt.Fprintf(w, "%s\n", data)
}

// applyTTFT sleeps for ttftMS milliseconds before the first token.
// The sleep is context-aware so a cancelled request doesn't block.
func applyTTFT(ctx context.Context, ttftMS int) {
	if ttftMS <= 0 {
		return
	}
	select {
	case <-ctx.Done():
	case <-time.After(time.Duration(ttftMS) * time.Millisecond):
	}
}

// applyTPS paces token emission. When tps > 0 it sleeps for 1000/tps ms
// between chunks to simulate realistic streaming throughput.
func applyTPS(ctx context.Context, tps int) {
	if tps <= 0 {
		return
	}
	delay := time.Duration(1000/tps) * time.Millisecond
	select {
	case <-ctx.Done():
	case <-time.After(delay):
	}
}

// tryHijackClose attempts to abruptly close the TCP connection to simulate a
// mid-stream backend failure. When hijack is unavailable (e.g. httptest with
// HTTP/2), it silently falls through - the truncated write still achieves the
// test goal of presenting an incomplete response.
func tryHijackClose(w http.ResponseWriter) {
	hj, ok := w.(http.Hijacker)
	if !ok {
		return
	}
	conn, _, err := hj.Hijack()
	if err != nil {
		return
	}
	_ = conn.Close()
}

// readBody reads the entire request body, returning an empty slice on error.
// Errors are intentionally swallowed because missing bodies are treated as
// empty requests in the mock - we still want to return a valid response.
func readBody(r *http.Request) []byte {
	if r.Body == nil {
		return nil
	}
	buf := make([]byte, 0, 512)
	tmp := make([]byte, 512)
	for {
		n, err := r.Body.Read(tmp)
		if n > 0 {
			buf = append(buf, tmp[:n]...)
		}
		if err != nil {
			break
		}
	}
	_ = r.Body.Close()
	return buf
}
