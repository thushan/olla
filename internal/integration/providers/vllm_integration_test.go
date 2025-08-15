//go:build integration
// +build integration

package providers

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thushan/olla/internal/adapter/discovery"
	"github.com/thushan/olla/internal/adapter/registry/profile"
	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/logger"
)

// getVLLMTestServer returns the vLLM test server URL from environment variable
func getVLLMTestServer(t *testing.T) string {
	vllmServer := os.Getenv("OLLA_TEST_SERVER_VLLM")
	if vllmServer == "" {
		t.Skip("OLLA_TEST_SERVER_VLLM environment variable not set. " +
			"Please set it to your vLLM server URL (e.g., http://192.168.0.1:8000) to run vLLM integration tests.")
	}

	// Ensure the URL has a scheme
	if !strings.HasPrefix(vllmServer, "http://") && !strings.HasPrefix(vllmServer, "https://") {
		vllmServer = "http://" + vllmServer
	}

	// Remove trailing slash if present
	vllmServer = strings.TrimSuffix(vllmServer, "/")

	return vllmServer
}

// checkVLLMServerAvailable verifies the vLLM server is reachable
func checkVLLMServerAvailable(t *testing.T, serverURL string) {
	healthURL := serverURL + "/health"

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(healthURL)
	if err != nil {
		t.Skipf("vLLM server not reachable at %s: %v\n"+
			"Please ensure your vLLM server is running and accessible.", serverURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Skipf("vLLM server health check failed with status %d at %s\n"+
			"Please ensure your vLLM server is healthy.", resp.StatusCode, serverURL)
	}
}

func TestVLLMDiscovery_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	vllmServer := getVLLMTestServer(t)
	checkVLLMServerAvailable(t, vllmServer)

	t.Logf("Running vLLM integration tests against: %s", vllmServer)

	ctx := context.Background()
	slogger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	testLogger := logger.NewPlainStyledLogger(slogger)
	profileFactory, err := profile.NewFactory("../../../config/profiles")
	require.NoError(t, err)

	t.Run("discovers vLLM models with auto-detection", func(t *testing.T) {
		client := discovery.NewHTTPModelDiscoveryClientWithDefaults(profileFactory, testLogger)

		endpoint := &domain.Endpoint{
			URLString: vllmServer,
			Type:      domain.ProfileAuto, // Test auto-detection
			Name:      "test-vllm",
		}

		models, err := client.DiscoverModels(ctx, endpoint)
		require.NoError(t, err)
		require.NotEmpty(t, models)

		// Check that at least one model is discovered
		// We don't assume a specific model since users may run different models
		t.Logf("Discovered %d models from vLLM server", len(models))

		// Verify basic model properties
		for _, model := range models {
			assert.NotEmpty(t, model.Name, "Model name should not be empty")
			assert.Equal(t, "vllm", model.Type, "Model type should be vllm")

			// Log discovered model for debugging
			t.Logf("Found model: %s", model.Name)

			// vLLM typically provides max context length
			if model.Details != nil && model.Details.MaxContextLength != nil {
				t.Logf("  - Max context length: %d", *model.Details.MaxContextLength)
			}
		}
	})

	t.Run("discovers vLLM models with explicit profile", func(t *testing.T) {
		client := discovery.NewHTTPModelDiscoveryClientWithDefaults(profileFactory, testLogger)

		endpoint := &domain.Endpoint{
			URLString: vllmServer,
			Type:      domain.ProfileVLLM, // Explicit vLLM profile
			Name:      "test-vllm-explicit",
		}

		models, err := client.DiscoverModels(ctx, endpoint)
		require.NoError(t, err)
		require.NotEmpty(t, models)

		// Verify the first model has expected properties
		model := models[0]
		assert.NotEmpty(t, model.Name)
		assert.Equal(t, "vllm", model.Type)

		t.Logf("Explicit vLLM profile discovered model: %s", model.Name)
	})

	t.Run("handles vLLM-specific response format", func(t *testing.T) {
		client := &http.Client{Timeout: 10 * time.Second}

		req, err := http.NewRequest("GET", vllmServer+"/v1/models", nil)
		require.NoError(t, err)

		resp, err := client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		// Parse response using vLLM profile
		vllmProfile, err := profileFactory.GetProfile(domain.ProfileVLLM)
		require.NoError(t, err)

		// Read response body properly
		buf := make([]byte, 10240)
		n, err := resp.Body.Read(buf)
		if err != nil && err.Error() != "EOF" && err.Error() != "unexpected EOF" {
			require.NoError(t, err)
		}

		models, err := vllmProfile.ParseModelsResponse(buf[:n])
		require.NoError(t, err)
		require.NotEmpty(t, models)

		// Verify vLLM-specific fields are captured
		model := models[0]
		assert.NotNil(t, model.Details)

		// Log what we found for debugging
		t.Logf("Parsed vLLM model: %s", model.Name)
		if model.Details != nil && model.Details.MaxContextLength != nil {
			t.Logf("  - Max context length: %d", *model.Details.MaxContextLength)
			assert.Greater(t, *model.Details.MaxContextLength, int64(0), "Max context length should be positive")
		}
	})
}

func TestVLLMHealthCheck_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	vllmServer := getVLLMTestServer(t)

	// Test vLLM health endpoint specifically
	healthURL := vllmServer + "/health"
	client := &http.Client{Timeout: 5 * time.Second}

	resp, err := client.Get(healthURL)
	if err != nil {
		t.Fatalf("Failed to reach vLLM health endpoint at %s: %v", healthURL, err)
	}
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode, "vLLM health endpoint should return 200 OK")
	t.Logf("vLLM health check successful at %s", healthURL)
}

// TestVLLMChatCompletion_Integration tests the chat completion endpoint if available
func TestVLLMChatCompletion_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	vllmServer := getVLLMTestServer(t)
	checkVLLMServerAvailable(t, vllmServer)

	// First, get available models
	modelsURL := vllmServer + "/v1/models"
	client := &http.Client{Timeout: 10 * time.Second}

	resp, err := client.Get(modelsURL)
	if err != nil {
		t.Fatalf("Failed to get models: %v", err)
	}
	defer resp.Body.Close()

	// Parse the response to get model name
	var modelsResp struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&modelsResp); err != nil {
		t.Fatalf("Failed to parse models response: %v", err)
	}

	if len(modelsResp.Data) == 0 {
		t.Skip("No models available on vLLM server")
	}

	modelName := modelsResp.Data[0].ID
	t.Logf("Testing chat completion with model: %s", modelName)

	// Test chat completion
	chatURL := vllmServer + "/v1/chat/completions"
	chatRequest := fmt.Sprintf(`{
		"model": "%s",
		"messages": [{"role": "user", "content": "Say hello in one word"}],
		"max_tokens": 10,
		"temperature": 0.7
	}`, modelName)

	req, err := http.NewRequest("POST", chatURL, strings.NewReader(chatRequest))
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp2, err := client.Do(req)
	if err != nil {
		t.Fatalf("Failed to send chat completion request: %v", err)
	}
	defer resp2.Body.Close()

	assert.Equal(t, http.StatusOK, resp2.StatusCode, "Chat completion should return 200 OK")

	// Parse response to verify it's valid
	var chatResp struct {
		ID      string `json:"id"`
		Object  string `json:"object"`
		Model   string `json:"model"`
		Choices []struct {
			Message struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}

	if err := json.NewDecoder(resp2.Body).Decode(&chatResp); err != nil {
		t.Fatalf("Failed to parse chat response: %v", err)
	}

	assert.NotEmpty(t, chatResp.ID, "Response should have an ID")
	assert.Equal(t, "chat.completion", chatResp.Object, "Response object should be chat.completion")
	assert.Equal(t, modelName, chatResp.Model, "Response should echo the model name")
	assert.NotEmpty(t, chatResp.Choices, "Response should have choices")

	if len(chatResp.Choices) > 0 {
		t.Logf("vLLM response: %s", chatResp.Choices[0].Message.Content)
	}
}
