package unifier_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thushan/olla/internal/adapter/unifier"
	"github.com/thushan/olla/internal/core/domain"
)

func TestAliasDuplication(t *testing.T) {
	ctx := context.Background()
	unifierInstance := unifier.NewDefaultUnifier(createTestLogger())

	t.Run("no duplicate aliases in unified model", func(t *testing.T) {
		model := &domain.ModelInfo{
			Name: "deepseek-coder-v2:latest",
			Details: &domain.ModelDetails{
				Family:            strPtr("deepseek"),
				ParameterSize:     strPtr("15.7B"),
				QuantizationLevel: strPtr("Q4_0"),
			},
		}

		unified, err := unifierInstance.UnifyModel(ctx, model, "http://localhost:11434")
		require.NoError(t, err)
		require.NotNil(t, unified)

		// Check for duplicates
		aliasMap := make(map[string]int)
		for _, alias := range unified.Aliases {
			aliasMap[alias.Name]++
		}

		// Assert no duplicates
		for alias, count := range aliasMap {
			assert.Equal(t, 1, count, "Alias %s appears %d times", alias, count)
		}
	})

	t.Run("no duplicate aliases after merging", func(t *testing.T) {
		// Create two models that will be merged
		model1 := &domain.UnifiedModel{
			ID:             "phi/4:14.7b-q4km",
			Family:         "phi",
			Variant:        "4",
			ParameterSize:  "14.7b",
			ParameterCount: 14700000000,
			Quantization:   "q4km",
			Aliases:        []domain.AliasEntry{
				{Name: "phi4:latest", Source: "ollama"},
				{Name: "phi4:14.7b", Source: "generated"},
				{Name: "phi4:14.7b-q4km", Source: "generated"},
			},
			SourceEndpoints: []domain.SourceEndpoint{
				{
					EndpointURL: "http://localhost:11434",
					NativeName:  "phi4:latest",
				},
			},
		}

		model2 := &domain.UnifiedModel{
			ID:             "phi/4:14.7b-q4km",
			Family:         "phi",
			Variant:        "4",
			ParameterSize:  "14.7b",
			ParameterCount: 14700000000,
			Quantization:   "q4km",
			Aliases:        []domain.AliasEntry{
				{Name: "microsoft/phi-4-mini-reasoning", Source: "lmstudio"},
				{Name: "phi4:14.7b", Source: "generated"},
				{Name: "phi4:14.7b-q4km", Source: "generated"},
			},
			SourceEndpoints: []domain.SourceEndpoint{
				{
					EndpointURL: "http://localhost:1234",
					NativeName:  "microsoft/phi-4-mini-reasoning",
				},
			},
		}

		merged, err := unifierInstance.MergeUnifiedModels(ctx, []*domain.UnifiedModel{model1, model2})
		require.NoError(t, err)
		require.NotNil(t, merged)

		// Check for duplicates
		aliasMap := make(map[string]int)
		for _, alias := range merged.Aliases {
			aliasMap[alias.Name]++
		}

		// Assert no duplicates
		for alias, count := range aliasMap {
			assert.Equal(t, 1, count, "Alias %s appears %d times", alias, count)
		}

		// Should have all unique aliases
		aliasStrings := merged.GetAliasStrings()
		assert.Contains(t, aliasStrings, "phi4:latest")
		assert.Contains(t, aliasStrings, "phi4:14.7b")
		assert.Contains(t, aliasStrings, "phi4:14.7b-q4km")
		assert.Contains(t, aliasStrings, "microsoft/phi-4-mini-reasoning")
	})

	t.Run("alias generation does not create duplicates", func(t *testing.T) {
		normalizer := unifier.NewModelNormalizer()
		
		unified := &domain.UnifiedModel{
			Family:        "llama",
			Variant:       "3",
			ParameterSize: "8b",
			Quantization:  "q4",
		}

		// Generate aliases for the same model multiple times
		aliases1 := normalizer.GenerateAliases(unified, "ollama", "llama3:latest")
		aliases2 := normalizer.GenerateAliases(unified, "ollama", "llama3:latest")

		// Both should be identical
		assert.Equal(t, aliases1, aliases2)

		// Check no duplicates within the generated aliases
		aliasMap := make(map[string]int)
		for _, alias := range aliases1 {
			aliasMap[alias.Name]++
		}

		for alias, count := range aliasMap {
			assert.Equal(t, 1, count, "Alias %s appears %d times", alias, count)
		}
	})

	t.Run("fuzzy alias resolution with separator variations", func(t *testing.T) {
		model := &domain.ModelInfo{
			Name: "deepseek-coder-v2:latest",
			Details: &domain.ModelDetails{
				Family:            strPtr("deepseek"),
				ParameterSize:     strPtr("15.7B"),
				QuantizationLevel: strPtr("Q4_0"),
			},
		}

		unified, err := unifierInstance.UnifyModel(ctx, model, "http://localhost:11434")
		require.NoError(t, err)
		require.NotNil(t, unified)

		// Print actual aliases for debugging
		t.Logf("Unified ID: %s", unified.ID)
		t.Logf("Actual aliases: %v", unified.GetAliasStrings())

		// Test resolving with both separator styles
		testCases := []struct {
			alias       string
			shouldFind  bool
		}{
			// Direct aliases that should exist
			{"deepseek-coder-v2:latest", true},
			{"deepseekcoder:15.7b", true},
			{"deepseek:15.7b", true},
			
			// Fuzzy matches with different separators
			{"deepseekcoder-15.7b", true},      // Should match deepseekcoder:15.7b
			{"deepseekcoder:15.7b-q4", true},   // Should match existing alias
			{"deepseekcoder-15.7b-q4", true},   // Should match deepseekcoder:15.7b-q4
			{"deepseek-15.7b", true},           // Should match deepseek:15.7b
			
			// Should not match
			{"deepseek3:15.7b", false},
			{"llama:15.7b", false},
		}

		for _, tc := range testCases {
			t.Run(tc.alias, func(t *testing.T) {
				resolved, err := unifierInstance.ResolveAlias(ctx, tc.alias)
				if tc.shouldFind {
					assert.NoError(t, err, "Should find alias %s", tc.alias)
					assert.NotNil(t, resolved)
					assert.Equal(t, unified.ID, resolved.ID)
				} else {
					assert.Error(t, err, "Should not find alias %s", tc.alias)
				}
			})
		}
	})
}