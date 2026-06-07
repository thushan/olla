package domain_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/thushan/olla/internal/core/domain"
)

// TestEndpointModelsURLNotSerialised asserts that EndpointModels.EndpointURL
// is never emitted in JSON output. The field is used as an internal map key
// and must not appear in API responses; it may carry auth credentials or
// internal network addresses.
func TestEndpointModelsURLNotSerialised(t *testing.T) {
	t.Parallel()

	em := domain.EndpointModels{
		LastUpdated: time.Now(),
		EndpointURL: "http://user:pass@192.168.1.100:8000",
		Models:      []*domain.ModelInfo{{Name: "llama3"}},
	}

	data, err := json.Marshal(em)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	if strings.Contains(string(data), "192.168.1.100") {
		t.Errorf("EndpointModels JSON contains endpoint URL: %s", data)
	}
	if strings.Contains(string(data), "endpoint_url") {
		t.Errorf("EndpointModels JSON contains 'endpoint_url' key: %s", data)
	}
	// Sanity check: the model data is still present.
	if !strings.Contains(string(data), "llama3") {
		t.Errorf("EndpointModels JSON missing model data: %s", data)
	}
}

// TestSourceEndpointURLNotSerialised asserts that SourceEndpoint.EndpointURL
// is not emitted in JSON output. The field holds the backend URL used for
// internal routing and must not surface in unified model API responses.
func TestSourceEndpointURLNotSerialised(t *testing.T) {
	t.Parallel()

	se := domain.SourceEndpoint{
		EndpointURL:  "http://admin:secret@gpu-host:8000",
		EndpointName: "gpu-vllm",
		NativeName:   "meta-llama/Llama-3.1-8B",
	}

	data, err := json.Marshal(se)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	if strings.Contains(string(data), "gpu-host") {
		t.Errorf("SourceEndpoint JSON contains backend hostname: %s", data)
	}
	if strings.Contains(string(data), "admin") {
		t.Errorf("SourceEndpoint JSON contains auth info: %s", data)
	}
	// Sanity: public fields are still serialised.
	if !strings.Contains(string(data), "gpu-vllm") {
		t.Errorf("SourceEndpoint JSON missing endpoint_name: %s", data)
	}
}

// TestEndpointAuthFieldsNotSerialised asserts that Endpoint.AuthHeaderValue
// is not included in JSON output. Leaking a resolved credential through any
// status endpoint would be a serious security issue.
func TestEndpointAuthFieldsNotSerialised(t *testing.T) {
	t.Parallel()

	ep := domain.Endpoint{
		Name:            "vllm-gpu",
		Type:            "vllm",
		AuthHeaderName:  "Authorization",
		AuthHeaderValue: "Bearer sk-super-secret-token",
	}

	data, err := json.Marshal(ep)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	if strings.Contains(string(data), "sk-super-secret-token") {
		t.Errorf("Endpoint JSON contains auth header value: %s", data)
	}
}

// TestEndpointHeadersNotSerialised asserts that Endpoint.Headers is not included
// in JSON output. The map may contain API keys or other custom auth values set
// via the headers: config block; exposing them through status endpoints would
// leak operator secrets.
func TestEndpointHeadersNotSerialised(t *testing.T) {
	t.Parallel()

	ep := domain.Endpoint{
		Name: "guarded-backend",
		Type: "ollama",
		Headers: map[string]string{
			"X-Custom-Key": "do-not-leak",
		},
	}

	data, err := json.Marshal(ep)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	if strings.Contains(string(data), "do-not-leak") {
		t.Errorf("Endpoint JSON contains Headers map value: %s", data)
	}
	if strings.Contains(string(data), "X-Custom-Key") {
		t.Errorf("Endpoint JSON contains Headers map key: %s", data)
	}
	// Sanity: other fields still serialise.
	if !strings.Contains(string(data), "guarded-backend") {
		t.Errorf("Endpoint JSON missing name field: %s", data)
	}
}
