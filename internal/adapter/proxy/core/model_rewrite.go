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

	// Parse the JSON to find and replace the model field
	rewritten := rewriteModelField(bodyBytes, actualModel)

	// Replace the request body with the rewritten content
	r.Body = io.NopCloser(bytes.NewReader(rewritten))
	r.ContentLength = int64(len(rewritten))
}

// rewriteModelField performs a targeted replacement of the top-level "model" field
// in a JSON body. Uses json.Decoder for correctness while keeping it simple.
func rewriteModelField(body []byte, newModel string) []byte {
	var parsed map[string]json.RawMessage
	if err := json.Unmarshal(body, &parsed); err != nil {
		// not valid JSON or not an object — return unchanged
		return body
	}

	if _, hasModel := parsed["model"]; !hasModel {
		// no model field to rewrite
		return body
	}

	// marshal the new model name as a JSON string value
	newModelJSON, err := json.Marshal(newModel)
	if err != nil {
		return body
	}

	parsed["model"] = json.RawMessage(newModelJSON)

	rewritten, err := json.Marshal(parsed)
	if err != nil {
		return body
	}

	return rewritten
}
