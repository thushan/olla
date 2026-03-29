package core

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"

	"github.com/thushan/olla/internal/core/constants"
	"github.com/thushan/olla/internal/core/domain"
)

// RewriteModelForAlias checks whether the request uses a model alias and, if the
// selected endpoint has an alias mapping, rewrites the "model" field in the JSON
// request body to the actual model name the backend expects. The original request
// body is replaced with the rewritten content.
//
// The replacement is done via targeted byte-splice to preserve the original
// JSON formatting, key ordering, and whitespace — making the result byte-identical
// to the input except for the model name change.
//
// This is a no-op when:
//   - there is no alias map in the context
//   - the selected endpoint is not in the alias map
//   - the request has no body or is not JSON
//   - the body does not contain a top-level "model" field
func RewriteModelForAlias(ctx context.Context, r *http.Request, endpoint *domain.Endpoint) {
	aliasMap, ok := ctx.Value(constants.ContextModelAliasMapKey).(map[string]string)
	if !ok || len(aliasMap) == 0 {
		return
	}

	actualModel, ok := aliasMap[endpoint.GetURLString()]
	if !ok {
		return
	}

	if r.Body == nil || r.ContentLength == 0 {
		return
	}

	// Read the current body
	bodyBytes, err := io.ReadAll(r.Body)
	r.Body.Close()
	if err != nil || len(bodyBytes) == 0 {
		// restore original body on error
		r.Body = io.NopCloser(bytes.NewReader(bodyBytes))
		return
	}

	// Perform targeted replacement of the model field value
	rewritten := rewriteModelField(bodyBytes, actualModel)

	// Replace the request body with the rewritten content
	r.Body = io.NopCloser(bytes.NewReader(rewritten))
	r.ContentLength = int64(len(rewritten))
}

// rewriteModelField performs a targeted replacement of the top-level "model" field's
// string value in a JSON body. Uses a JSON token scanner to find the precise byte
// offset of the top-level value, then splices in the replacement — preserving all
// original formatting, key ordering, and whitespace so the result is byte-identical
// to the input except for the model name itself.
//
// This avoids false positives from nested content (e.g. a message whose text
// contains the literal word "model") that a naïve regex scan would wrongly match.
func rewriteModelField(body []byte, newModel string) []byte {
	// Verify this is a valid JSON object with a top-level "model" string field.
	var parsed map[string]json.RawMessage
	if err := json.Unmarshal(body, &parsed); err != nil {
		return body
	}

	oldRaw, hasModel := parsed["model"]
	if !hasModel {
		return body
	}

	// Only rewrite string values — leave numbers, null, etc. untouched.
	var oldStr string
	if err := json.Unmarshal(oldRaw, &oldStr); err != nil {
		return body
	}

	// Encode the replacement model name as a JSON string (with surrounding quotes
	// and proper escaping for characters like \, ", etc.).
	newRaw, err := json.Marshal(newModel)
	if err != nil {
		return body
	}

	// Locate the exact byte range of the top-level "model" value using a token
	// scanner, so we can splice without disturbing any surrounding bytes.
	start, end, ok := findTopLevelValueOffset(body, "model")
	if !ok {
		return body
	}

	// Splice: bytes before value + new JSON value + bytes after value.
	result := make([]byte, 0, len(body)-len(oldRaw)+len(newRaw))
	result = append(result, body[:start]...)
	result = append(result, newRaw...)
	result = append(result, body[end:]...)
	return result
}

// findTopLevelValueOffset uses a streaming JSON token scanner to find the byte
// offsets [start, end) of the value for the given key at the top level of a JSON
// object.  It intentionally operates at depth 1 only, so occurrences of the same
// key name inside nested objects or string values are ignored.
func findTopLevelValueOffset(body []byte, key string) (start, end int64, ok bool) {
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()

	// The body must be a JSON object.
	tok, err := dec.Token()
	if err != nil {
		return 0, 0, false
	}
	if delim, isDelim := tok.(json.Delim); !isDelim || delim != '{' {
		return 0, 0, false
	}

	for dec.More() {
		// Read the key token.
		tok, err = dec.Token()
		if err != nil {
			return 0, 0, false
		}
		k, isString := tok.(string)
		if !isString {
			return 0, 0, false
		}

		// InputOffset is now just past the key's closing quote.
		// body[beforeValue:afterValue] will be ": <value>" (colon + optional
		// whitespace + the value bytes), so we scan forward to skip the separator.
		beforeValue := dec.InputOffset()

		var rawVal json.RawMessage
		if err = dec.Decode(&rawVal); err != nil {
			return 0, 0, false
		}
		afterValue := dec.InputOffset()

		if k == key {
			// Find the first byte of rawVal within the segment between key and end
			// of value — the segment only contains `:`, whitespace, and the value
			// itself, so the first non-separator character is always the value start.
			segment := body[beforeValue:afterValue]
			idx := bytes.Index(segment, rawVal)
			if idx < 0 {
				return 0, 0, false
			}
			actualStart := beforeValue + int64(idx)
			actualEnd := actualStart + int64(len(rawVal))
			return actualStart, actualEnd, true
		}
	}

	return 0, 0, false
}
