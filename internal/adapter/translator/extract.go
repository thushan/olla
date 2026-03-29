package translator

import (
	"fmt"

	"github.com/tidwall/gjson"
)

// ExtractModelName performs a lightweight extraction of the top-level "model"
// field from a JSON request body. This exists to avoid a full unmarshal on the
// hot path -- the handler needs the model name for endpoint filtering and
// routing decisions before it knows whether passthrough or translation will be
// used. A full TransformRequest parse is deferred to the translation path only.
//
// Uses gjson.GetBytes which scans forward to the first matching key without
// allocating an intermediate map, making it significantly cheaper than
// encoding/json.Unmarshal for this single-field lookup.
func ExtractModelName(body []byte) (string, error) {
	if len(body) == 0 {
		return "", fmt.Errorf("empty request body")
	}

	result := gjson.GetBytes(body, "model")

	if !result.Exists() {
		return "", fmt.Errorf("model field is required (body may not be valid JSON)")
	}

	// gjson coerces non-string types via .String() (numbers become "123",
	// arrays become their raw JSON). We only accept actual JSON strings.
	if result.Type != gjson.String {
		return "", fmt.Errorf("model field must be a string, got %s", result.Type)
	}

	model := result.Str
	if model == "" {
		return "", fmt.Errorf("model field must not be empty")
	}

	return model, nil
}
