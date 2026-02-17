package core

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"regexp"

	"github.com/thushan/olla/internal/core/constants"
	"github.com/thushan/olla/internal/core/domain"
)

// modelFieldPattern matches the top-level "model" key and its string value in JSON.
// It captures everything up to and including the "model" key, then the quoted string value,
// so we can replace only the value while preserving all surrounding formatting, whitespace,
// and key ordering.
var modelFieldPattern = regexp.MustCompile(`("model"\s*:\s*)"((?:[^"\\]|\\.)*)"`)

// RewriteModelForAlias checks whether the request uses a model alias and, if the
// selected endpoint has an alias mapping, rewrites the "model" field in the JSON
// request body to the actual model name the backend expects. The original request
// body is replaced with the rewritten content.
//
// The replacement is done via targeted string substitution to preserve the original
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
// string value in a JSON body. Uses regex to find and replace only the value portion,
// preserving all original formatting, key ordering, and whitespace so the result is
// byte-identical to the input except for the model name itself.
func rewriteModelField(body []byte, newModel string) []byte {
	// First, verify this is valid JSON with a top-level "model" field
	// (quick structural check to avoid false positives from nested content)
	var parsed map[string]json.RawMessage
	if err := json.Unmarshal(body, &parsed); err != nil {
		// not valid JSON or not an object — return unchanged
		return body
	}

	if _, hasModel := parsed["model"]; !hasModel {
		// no model field to rewrite
		return body
	}

	// Escape the new model name for safe insertion into a JSON string value
	escapedModel := jsonEscapeString(newModel)

	// Replace only the first occurrence of the "model" field value using targeted substitution.
	// The regex captures the key + colon portion and the quoted value separately,
	// so we only change the value while preserving all surrounding structure.
	replaced := false
	result := modelFieldPattern.ReplaceAllFunc(body, func(match []byte) []byte {
		if replaced {
			// Only replace the first (top-level) occurrence
			return match
		}
		replaced = true

		// Find the submatch to reconstruct with new value
		submatches := modelFieldPattern.FindSubmatch(match)
		if len(submatches) < 3 {
			return match
		}

		// submatches[1] is the key part: "model" :
		// submatches[2] is the old value without quotes
		// Reconstruct: key part + "new-value"
		var buf bytes.Buffer
		buf.Write(submatches[1])
		buf.WriteByte('"')
		buf.WriteString(escapedModel)
		buf.WriteByte('"')
		return buf.Bytes()
	})

	return result
}

// jsonEscapeString escapes a string for safe inclusion as a JSON string value
// (without the surrounding quotes). Handles special characters per RFC 8259.
func jsonEscapeString(s string) string {
	// Use json.Marshal to get proper escaping, then strip the surrounding quotes
	b, err := json.Marshal(s)
	if err != nil {
		return s
	}
	// json.Marshal wraps in quotes: "value" — strip them
	return string(b[1 : len(b)-1])
}
