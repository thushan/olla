package domain_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/thushan/olla/internal/core/domain"
)

// TestEndpoint_AuthHeaderValue_NotSerialised ensures that AuthHeaderValue never
// appears in JSON output. The field carries live credentials; exposing it through
// status endpoints or logs would be a security issue.
func TestEndpoint_AuthHeaderValue_NotSerialised(t *testing.T) {
	t.Parallel()

	ep := &domain.Endpoint{
		Name:            "secure-ep",
		AuthHeaderName:  "Authorization",
		AuthHeaderValue: "Bearer super-secret-token",
	}

	data, err := json.Marshal(ep)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	if strings.Contains(string(data), "super-secret-token") {
		t.Errorf("AuthHeaderValue leaked into JSON output: %s", string(data))
	}
	if strings.Contains(string(data), "AuthHeaderValue") {
		t.Errorf("AuthHeaderValue field name present in JSON output: %s", string(data))
	}
}

func TestEndpoint_AuthFields_ZeroValueByDefault(t *testing.T) {
	t.Parallel()

	ep := &domain.Endpoint{Name: "plain"}

	if ep.AuthHeaderName != "" {
		t.Errorf("AuthHeaderName should be empty by default, got %q", ep.AuthHeaderName)
	}
	if ep.AuthHeaderValue != "" {
		t.Errorf("AuthHeaderValue should be empty by default, got %q", ep.AuthHeaderValue)
	}
	if ep.Headers != nil {
		t.Errorf("Headers should be nil by default, got %v", ep.Headers)
	}
}

func TestEndpoint_Headers_StoredVerbatim(t *testing.T) {
	t.Parallel()

	want := map[string]string{
		"X-Tenant": "acme",
		"X-Region": "us-east",
	}
	ep := &domain.Endpoint{
		Name:    "ep",
		Headers: want,
	}

	for k, v := range want {
		if ep.Headers[k] != v {
			t.Errorf("Headers[%q] = %q, want %q", k, ep.Headers[k], v)
		}
	}
}
