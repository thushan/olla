package converter

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/core/ports"
)

func TestConverterFactory(t *testing.T) {
	factory := NewConverterFactory()

	t.Run("GetConverter returns correct converters", func(t *testing.T) {
		testCases := []struct {
			format       string
			expectedType string
		}{
			{"unified", "*converter.UnifiedConverter"},
			{"openai", "*converter.OpenAIConverter"},
			{"ollama", "*converter.OllamaConverter"},
			{"lmstudio", "*converter.LMStudioConverter"},
			{"", "*converter.UnifiedConverter"}, // Default
		}

		for _, tc := range testCases {
			t.Run(tc.format, func(t *testing.T) {
				converter, err := factory.GetConverter(tc.format)
				require.NoError(t, err)
				assert.NotNil(t, converter)
			})
		}
	})

	t.Run("GetConverter returns error for unsupported format", func(t *testing.T) {
		converter, err := factory.GetConverter("unsupported")
		assert.Nil(t, converter)
		require.Error(t, err)
		
		qpErr, ok := err.(*ports.QueryParameterError)
		require.True(t, ok)
		assert.Equal(t, "format", qpErr.Parameter)
		assert.Equal(t, "unsupported", qpErr.Value)
		assert.Contains(t, qpErr.Reason, "unsupported format")
		assert.Contains(t, qpErr.Reason, "unified")
		assert.Contains(t, qpErr.Reason, "openai")
		assert.Contains(t, qpErr.Reason, "ollama")
		assert.Contains(t, qpErr.Reason, "lmstudio")
	})

	t.Run("GetSupportedFormats returns all formats", func(t *testing.T) {
		formats := factory.GetSupportedFormats()
		assert.Len(t, formats, 4)
		
		// Check all expected formats are present
		formatMap := make(map[string]bool)
		for _, f := range formats {
			formatMap[f] = true
		}
		
		assert.True(t, formatMap["unified"])
		assert.True(t, formatMap["openai"])
		assert.True(t, formatMap["ollama"])
		assert.True(t, formatMap["lmstudio"])
	})

	t.Run("RegisterConverter adds new converter", func(t *testing.T) {
		// Create a mock converter
		mockConverter := &mockConverter{formatName: "custom"}
		
		factory.RegisterConverter(mockConverter)
		
		// Should be able to get it back
		converter, err := factory.GetConverter("custom")
		require.NoError(t, err)
		assert.Equal(t, mockConverter, converter)
		
		// Should appear in supported formats
		formats := factory.GetSupportedFormats()
		hasCustom := false
		for _, f := range formats {
			if f == "custom" {
				hasCustom = true
				break
			}
		}
		assert.True(t, hasCustom)
	})
}

// mockConverter for testing
type mockConverter struct {
	formatName string
}

func (m *mockConverter) ConvertToFormat(models []*domain.UnifiedModel, filters ports.ModelFilters) (interface{}, error) {
	return nil, nil
}

func (m *mockConverter) GetFormatName() string {
	return m.formatName
}