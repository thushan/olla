package unifier

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thushan/olla/internal/core/domain"
)

// Helper functions
func ptrString(s string) *string {
	return &s
}

func createTestEndpoint(url, name string) *domain.Endpoint {
	return &domain.Endpoint{
		URLString: url,
		Name:      name,
	}
}

func TestDefaultUnifier_UnifyModels(t *testing.T) {
	tests := []struct {
		name           string
		inputModels    [][]*domain.ModelInfo // models from different endpoints
		endpoints      []*domain.Endpoint
		expectedCount  int
		expectedDigest int
		expectedName   int
	}{
		{
			name: "deduplicate by digest",
			inputModels: [][]*domain.ModelInfo{
				{
					{
						Name: "llama3:8b",
						Size: 8000000000,
						Details: &domain.ModelDetails{
							Digest: ptrString("sha256:abc123"),
						},
					},
				},
				{
					{
						Name: "llama3-8b",
						Size: 8000000000,
						Details: &domain.ModelDetails{
							Digest: ptrString("sha256:abc123"),
						},
					},
				},
			},
			endpoints: []*domain.Endpoint{
				createTestEndpoint("http://localhost:11434", "Ollama Server"),
				createTestEndpoint("http://localhost:1234", "LM Studio"),
			},
			expectedCount:  1,
			expectedDigest: 1,
			expectedName:   0,
		},
		{
			name: "deduplicate by exact name",
			inputModels: [][]*domain.ModelInfo{
				{
					{
						Name: "phi3:mini",
						Size: 3800000000,
					},
				},
				{
					{
						Name: "phi3:mini",
						Size: 3800000000,
					},
				},
			},
			endpoints: []*domain.Endpoint{
				createTestEndpoint("http://localhost:11434", "Ollama Server"),
				createTestEndpoint("http://localhost:1234", "LM Studio"),
			},
			expectedCount:  1,
			expectedDigest: 0,
			expectedName:   1,
		},
		{
			name: "no deduplication for different models",
			inputModels: [][]*domain.ModelInfo{
				{
					{
						Name:    "llama3:8b",
						Details: &domain.ModelDetails{Digest: ptrString("sha256:abc123")},
					},
					{
						Name:    "phi3:mini",
						Details: &domain.ModelDetails{Digest: ptrString("sha256:def456")},
					},
				},
				{
					{
						Name:    "mistral:7b",
						Details: &domain.ModelDetails{Digest: ptrString("sha256:ghi789")},
					},
				},
			},
			endpoints: []*domain.Endpoint{
				createTestEndpoint("http://localhost:11434", "Ollama Server"),
				createTestEndpoint("http://localhost:1234", "LM Studio"),
			},
			expectedCount:  3,
			expectedDigest: 0,
			expectedName:   0,
		},
		{
			name: "case insensitive name matching",
			inputModels: [][]*domain.ModelInfo{
				{
					{
						Name: "Llama3:8B",
					},
				},
				{
					{
						Name: "llama3:8b",
					},
				},
			},
			endpoints: []*domain.Endpoint{
				createTestEndpoint("http://localhost:11434", "Ollama Server"),
				createTestEndpoint("http://localhost:1234", "LM Studio"),
			},
			expectedCount:  1,
			expectedDigest: 0,
			expectedName:   1,
		},
		{
			name: "conflicting digests prevent name-based merge",
			inputModels: [][]*domain.ModelInfo{
				{
					{
						Name:    "llama3:8b",
						Details: &domain.ModelDetails{Digest: ptrString("sha256:abc123")},
					},
				},
				{
					{
						Name:    "llama3:8b",
						Details: &domain.ModelDetails{Digest: ptrString("sha256:def456")},
					},
				},
			},
			endpoints: []*domain.Endpoint{
				createTestEndpoint("http://localhost:11434", "Ollama Server"),
				createTestEndpoint("http://localhost:1234", "LM Studio"),
			},
			expectedCount:  2,
			expectedDigest: 0,
			expectedName:   0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			unifier := NewDefaultUnifier()

			var allModels []*domain.UnifiedModel
			for i, models := range tt.inputModels {
				unified, err := unifier.UnifyModels(ctx, models, tt.endpoints[i])
				require.NoError(t, err)
				allModels = append(allModels, unified...)
			}

			// Check unified model count by casting to implementation
			impl := unifier.(*DefaultUnifier)
			assert.Equal(t, tt.expectedCount, len(impl.catalog))

			// Check stats
			stats := unifier.(*DefaultUnifier).GetDeduplicationStats()
			assert.Equal(t, tt.expectedDigest, stats.DigestMatches)
			assert.Equal(t, tt.expectedName, stats.NameMatches)
		})
	}
}

func TestDefaultUnifier_ResolveModel(t *testing.T) {
	ctx := context.Background()
	unifier := NewDefaultUnifier()

	// Add some models
	models := []*domain.ModelInfo{
		{
			Name:    "llama3:8b",
			Details: &domain.ModelDetails{Digest: ptrString("sha256:abc123")},
		},
		{
			Name:    "Phi3:Mini",
			Details: &domain.ModelDetails{Digest: ptrString("sha256:def456")},
		},
	}

	endpoint := createTestEndpoint("http://localhost:11434", "Ollama Server")
	_, err := unifier.UnifyModels(ctx, models, endpoint)
	require.NoError(t, err)

	// Test resolution by exact ID
	model, err := unifier.ResolveAlias(ctx, "llama3:8b")
	require.NoError(t, err)
	assert.Equal(t, "llama3:8b", model.ID)

	// Test resolution by case-insensitive name
	model, err = unifier.ResolveAlias(ctx, "phi3:mini")
	require.NoError(t, err)
	assert.Equal(t, "Phi3:Mini", model.ID)

	// Test resolution not found
	_, err = unifier.ResolveAlias(ctx, "nonexistent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "model not found")
}

func TestDefaultUnifier_ModelMerging(t *testing.T) {
	ctx := context.Background()
	unifier := NewDefaultUnifier()

	// Helper functions
	ptrInt64 := func(i int64) *int64 {
		return &i
	}
	ptrString := func(s string) *string {
		return &s
	}

	// Add model from first endpoint
	models1 := []*domain.ModelInfo{
		{
			Name: "llama3:8b",
			Details: &domain.ModelDetails{
				Digest:           ptrString("sha256:abc123"),
				MaxContextLength: ptrInt64(8192),
				State:            ptrString("loaded"),
			},
		},
	}
	endpoint1 := createTestEndpoint("http://localhost:11434", "Ollama Server")
	_, err := unifier.UnifyModels(ctx, models1, endpoint1)
	require.NoError(t, err)

	// Add same model from second endpoint
	models2 := []*domain.ModelInfo{
		{
			Name: "llama3:8b",
			Details: &domain.ModelDetails{
				Digest:           ptrString("sha256:abc123"),
				MaxContextLength: ptrInt64(8192),
				State:            ptrString("not-loaded"),
			},
		},
	}
	endpoint2 := createTestEndpoint("http://localhost:1234", "LM Studio")
	_, err = unifier.UnifyModels(ctx, models2, endpoint2)
	require.NoError(t, err)

	// Check merged model
	model, err := unifier.ResolveAlias(ctx, "llama3:8b")
	require.NoError(t, err)
	assert.Equal(t, 2, len(model.SourceEndpoints))
	assert.Contains(t, model.Capabilities, "context:8192")
	assert.Contains(t, model.Capabilities, "text-generation")

	// Verify both endpoints are present
	endpoints := make(map[string]bool)
	for _, source := range model.SourceEndpoints {
		endpoints[source.EndpointURL] = true
	}
	assert.True(t, endpoints["http://localhost:11434"])
	assert.True(t, endpoints["http://localhost:1234"])
}

func TestDefaultUnifier_EndpointCleanup(t *testing.T) {
	ctx := context.Background()
	unifier := NewDefaultUnifier()

	// Add models from endpoint
	models := []*domain.ModelInfo{
		{
			Name:    "llama3:8b",
			Details: &domain.ModelDetails{Digest: ptrString("sha256:abc123")},
		},
		{
			Name:    "phi3:mini",
			Details: &domain.ModelDetails{Digest: ptrString("sha256:def456")},
		},
	}
	endpoint := createTestEndpoint("http://localhost:11434", "Ollama Server")
	_, err := unifier.UnifyModels(ctx, models, endpoint)
	require.NoError(t, err)

	// Update endpoint with only one model
	newModels := []*domain.ModelInfo{
		{
			Name:    "llama3:8b",
			Details: &domain.ModelDetails{Digest: ptrString("sha256:abc123")},
		},
	}
	_, err = unifier.UnifyModels(ctx, newModels, endpoint)
	require.NoError(t, err)

	// Verify phi3 was removed
	_, err = unifier.ResolveAlias(ctx, "phi3:mini")
	assert.Error(t, err)

	// Verify llama3 still exists
	model, err := unifier.ResolveAlias(ctx, "llama3:8b")
	require.NoError(t, err)
	assert.Equal(t, "llama3:8b", model.ID)
}

func TestDefaultUnifier_Clear(t *testing.T) {
	ctx := context.Background()
	unifier := NewDefaultUnifier()

	// Add some models
	models := []*domain.ModelInfo{
		{
			Name:    "llama3:8b",
			Details: &domain.ModelDetails{Digest: ptrString("sha256:abc123")},
		},
	}
	endpoint := createTestEndpoint("http://localhost:11434", "Ollama Server")
	_, err := unifier.UnifyModels(ctx, models, endpoint)
	require.NoError(t, err)

	// Clear
	err = unifier.Clear(ctx)
	require.NoError(t, err)

	// Verify all data is cleared
	impl := unifier.(*DefaultUnifier)
	assert.Empty(t, impl.catalog)

	stats := unifier.(*DefaultUnifier).GetDeduplicationStats()
	assert.Equal(t, 0, stats.TotalModels)
	assert.Equal(t, 0, stats.DigestMatches)
	assert.Equal(t, 0, stats.NameMatches)
}

func TestDefaultUnifier_StaleModelCleanup(t *testing.T) {
	ctx := context.Background()
	unifier := NewDefaultUnifier().(*DefaultUnifier)

	// Set short cleanup interval for testing
	unifier.cleanupInterval = 100 * time.Millisecond

	// Add a model
	models := []*domain.ModelInfo{
		{
			Name:    "llama3:8b",
			Details: &domain.ModelDetails{Digest: ptrString("sha256:abc123")},
		},
	}
	endpoint := createTestEndpoint("http://localhost:11434", "Ollama Server")
	_, err := unifier.UnifyModels(ctx, models, endpoint)
	require.NoError(t, err)

	// Manually set LastSeen to old time
	unifier.mu.Lock()
	for _, model := range unifier.catalog {
		model.LastSeen = time.Now().Add(-25 * time.Hour)
	}
	unifier.mu.Unlock()

	// Wait for cleanup interval
	time.Sleep(150 * time.Millisecond)

	// Add new model to trigger cleanup
	newModels := []*domain.ModelInfo{
		{
			Name:    "phi3:mini",
			Details: &domain.ModelDetails{Digest: ptrString("sha256:def456")},
		},
	}
	endpoint2 := createTestEndpoint("http://localhost:1234", "LM Studio")
	_, err = unifier.UnifyModels(ctx, newModels, endpoint2)
	require.NoError(t, err)

	// Verify old model was cleaned up
	_, err = unifier.ResolveAlias(ctx, "llama3:8b")
	assert.Error(t, err)

	// Verify new model exists
	model, err := unifier.ResolveAlias(ctx, "phi3:mini")
	require.NoError(t, err)
	assert.Equal(t, "phi3:mini", model.ID)
}

func TestDefaultUnifier_NilAndEmptyInputs(t *testing.T) {
	ctx := context.Background()
	unifier := NewDefaultUnifier()

	// Test nil models
	endpoint := createTestEndpoint("http://localhost:11434", "Ollama Server")
	result, err := unifier.UnifyModels(ctx, nil, endpoint)
	require.NoError(t, err)
	assert.Nil(t, result)

	// Test empty models
	result, err = unifier.UnifyModels(ctx, []*domain.ModelInfo{}, endpoint)
	require.NoError(t, err)
	assert.Nil(t, result)

	// Test with nil model in slice
	models := []*domain.ModelInfo{
		{
			Name: "llama3:8b",
		},
		nil,
		{
			Name: "phi3:mini",
		},
	}
	result, err = unifier.UnifyModels(ctx, models, endpoint)
	require.NoError(t, err)
	assert.Len(t, result, 2)
}
