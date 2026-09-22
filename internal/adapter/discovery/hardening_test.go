package discovery

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/thushan/olla/internal/core/domain"
)

func TestLlamaCppMetadataDriftRegistersModels(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"object":"list","data":[{"id":"Qwen3.5-35B-A3B","object":"model","owned_by":"llamacpp","meta":{"vocab_type":true,"n_vocab":248320,"n_ctx_train":262144,"n_embd":2048,"n_params":35000000000,"size":21000000000,"ftype":"Q4_K_M"}}]}`))
	}))
	defer server.Close()
	client := NewHTTPModelDiscoveryClientWithDefaults(createTestProfileFactory(t), createTestLogger())
	defer client.Close()
	endpoint := createTestEndpoint(server.URL, domain.ProfileLlamaCpp)
	registry := &mockModelRegistry{}
	service := NewModelDiscoveryService(client, nil, registry, DiscoveryConfig{Timeout: time.Second}, createTestLogger())
	if err := service.DiscoverEndpoint(context.Background(), endpoint); err != nil {
		t.Fatal(err)
	}
	if registry.registerCallCount != 1 || len(registry.registeredModels) != 1 || registry.registeredModels[0].Name != "Qwen3.5-35B-A3B" {
		t.Fatalf("model not registered: %+v", registry)
	}
	if service.getFailureCount(endpoint.URLString) != 0 || client.GetMetrics().SuccessfulRequests != 1 {
		t.Fatal("valid discovery recorded as failure")
	}
}
