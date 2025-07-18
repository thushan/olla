package unifier

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadModelConfig(t *testing.T) {
	// Test loading default config when file doesn't exist
	config, err := LoadModelConfig()
	require.NoError(t, err)
	assert.NotNil(t, config)

	// Verify some default values
	assert.NotEmpty(t, config.ModelExtraction.FamilyPatterns)
	assert.NotEmpty(t, config.ModelExtraction.ArchitectureMappings)
	assert.NotEmpty(t, config.Quantization.Mappings)
	assert.Equal(t, "q4km", config.Quantization.Mappings["Q4_K_M"])
	assert.Equal(t, "meta", config.ModelExtraction.PublisherMappings["llama"])
}

func TestLoadModelConfigFromFile(t *testing.T) {
	// Create a temporary config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "models.yaml")

	configContent := `
model_extraction:
  family_patterns:
    - pattern: "^test-(\\w+)-(\\d+)"
      family_group: 1
      variant_group: 2
      description: "Test pattern"
  architecture_mappings:
    testarch: testfamily
  publisher_mappings:
    testfamily: testpublisher

quantization:
  mappings:
    TEST_QUANT: testq

capabilities:
  type_capabilities:
    test:
      - test-capability
`

	err := os.WriteFile(configPath, []byte(configContent), 0644)
	require.NoError(t, err)

	// Set environment variable to point to our test config
	oldConfigDir := os.Getenv("OLLA_CONFIG_DIR")
	os.Setenv("OLLA_CONFIG_DIR", tmpDir)
	defer os.Setenv("OLLA_CONFIG_DIR", oldConfigDir)

	// Clear the config cache to force reload
	configOnce = sync.Once{}
	configInstance = nil

	// Load config
	config, err := LoadModelConfig()
	require.NoError(t, err)
	assert.NotNil(t, config)

	// Verify custom values were loaded
	assert.Len(t, config.ModelExtraction.FamilyPatterns, 1)
	assert.Equal(t, "^test-(\\w+)-(\\d+)", config.ModelExtraction.FamilyPatterns[0].Pattern)
	assert.Equal(t, "testfamily", config.ModelExtraction.ArchitectureMappings["testarch"])
	assert.Equal(t, "testpublisher", config.ModelExtraction.PublisherMappings["testfamily"])
	assert.Equal(t, "testq", config.Quantization.Mappings["TEST_QUANT"])
	assert.Contains(t, config.Capabilities.TypeCapabilities["test"], "test-capability")
}

func TestPatternCompilation(t *testing.T) {
	config := getDefaultConfig()
	err := config.compilePatterns()
	require.NoError(t, err)

	// Verify patterns were compiled
	for _, pattern := range config.ModelExtraction.FamilyPatterns {
		assert.NotNil(t, pattern.regex, "Pattern %s should be compiled", pattern.Pattern)
	}

	for _, pattern := range config.Capabilities.NamePatterns {
		assert.NotNil(t, pattern.regex, "Capability pattern %s should be compiled", pattern.Pattern)
	}
}

func TestConfigWithInvalidRegex(t *testing.T) {
	config := &ModelUnificationConfig{}
	config.ModelExtraction.FamilyPatterns = []PatternConfig{
		{Pattern: "[invalid(regex"},
	}

	err := config.compilePatterns()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to compile family pattern")
}
