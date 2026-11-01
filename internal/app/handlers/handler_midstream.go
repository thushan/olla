package handlers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

// streamErrorFormat is the provider wire format of an in-flight streaming body.
// A mid-stream failure has to be reported in the format the client is already
// parsing; a status code can no longer be changed once bytes are committed.
type streamErrorFormat int

const (
	// streamFormatNone covers bodies with no in-band error channel (plain JSON,
	// binary). The connection abort alone signals the failure.
	streamFormatNone streamErrorFormat = iota
	streamFormatOpenAISSE
	streamFormatAnthropicSSE
	streamFormatNDJSON
)

// Fixed message: the backend error can carry internal addresses, so it is logged
// rather than sent to the client.
const midStreamErrorMessage = "upstream connection lost during streaming response"

// streamFormatForContentType picks the error format from the response
// Content-Type the proxy engine copied from the backend. anthropic selects the
// Anthropic event shape for SSE bodies on Anthropic routes.
func streamFormatForContentType(contentType string, anthropic bool) streamErrorFormat {
	ct := strings.ToLower(contentType)
	switch {
	case strings.Contains(ct, "text/event-stream"):
		if anthropic {
			return streamFormatAnthropicSSE
		}
		return streamFormatOpenAISSE
	case strings.Contains(ct, "ndjson"), strings.Contains(ct, "x-json-stream"):
		return streamFormatNDJSON
	default:
		return streamFormatNone
	}
}

// writeStreamError emits one in-band error event in the given format and flushes
// it. It is the single writer for every mid-stream error path. The leading
// newline(s) close a record that may have been cut off part-way through.
func (a *Application) writeStreamError(w http.ResponseWriter, format streamErrorFormat, message string) {
	if w == nil {
		return
	}
	var event string
	switch format {
	case streamFormatAnthropicSSE:
		event = fmt.Sprintf("\n\nevent: error\ndata: {\"type\":\"error\",\"error\":{\"type\":\"api_error\",\"message\":%q}}\n\n", message)
	case streamFormatOpenAISSE:
		event = fmt.Sprintf("\n\ndata: {\"error\":{\"message\":%q,\"type\":\"api_error\",\"code\":\"upstream_stream_interrupted\"}}\n\n", message)
	case streamFormatNDJSON:
		event = fmt.Sprintf("\n{\"error\":%q}\n", message)
	default:
		return
	}
	_, _ = fmt.Fprint(w, event)
	_ = http.NewResponseController(w).Flush()
}

// failMidStream handles a backend failure after response bytes reached the client.
// It writes the provider-format error event and reports whether the caller must
// abort the connection (panic(http.ErrAbortHandler)) once its own logging and
// metrics are done. Aborting is what stops net/http finishing the chunked body
// cleanly, which would present a truncated stream to the client as complete.
//
// A cancelled request context means the client left, so there is nobody to tell
// and nothing to abort.
func (a *Application) failMidStream(w http.ResponseWriter, r *http.Request, pr *proxyRequest, err error, anthropic bool) bool {
	if err == nil || errors.Is(err, context.Canceled) || r.Context().Err() != nil {
		return false
	}
	pr.hadError = true
	format := streamFormatForContentType(w.Header().Get("Content-Type"), anthropic)
	pr.requestLogger.Warn("Backend failed after response started, aborting client connection",
		"error", err, "format", int(format))
	a.writeStreamError(w, format, midStreamErrorMessage)
	return true
}
