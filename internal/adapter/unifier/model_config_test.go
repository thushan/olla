package unifier

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/thushan/olla/internal/logger"
)

// shippedConfigPath is config/models.yaml, relative to this package
// (internal/adapter/unifier -> repo root is three levels up).
const shippedConfigPath = "../../../config/models.yaml"

// TestShippedConfigParses guards against issue #204: yaml.v3's double-quoted
// scalars only accept a small fixed escape set, so a stray `\d` inside "..."
// breaks the shipped file. loadConfigFromFile() swallows unmarshal errors and
// silently falls back to embedded defaults, so a broken file was never caught
// until now - this test fails loudly instead.
func TestShippedConfigParses(t *testing.T) {
	data, err := os.ReadFile(shippedConfigPath)
	require.NoError(t, err, "config/models.yaml must exist")

	var fromFile ModelUnificationConfig
	require.NoError(t, yaml.Unmarshal(data, &fromFile), "config/models.yaml must be valid YAML")
	require.NoError(t, fromFile.compilePatterns(), "config/models.yaml patterns must compile")
}

// TestShippedConfigMatchesDefaults is an equivalence guard between
// config/models.yaml and getDefaultConfig(). Because loadConfigFromFile()
// silently fell back to defaults on any parse error, the shipped file and
// the embedded fallback drifted apart for years without anyone noticing
// (issue #204). If this fails, reconcile the drift in both directions -
// don't just update one side to make the test pass.
func TestShippedConfigMatchesDefaults(t *testing.T) {
	data, err := os.ReadFile(shippedConfigPath)
	require.NoError(t, err)

	var fromFile ModelUnificationConfig
	require.NoError(t, yaml.Unmarshal(data, &fromFile))

	defaults := getDefaultConfig()

	assert.Equal(t, defaults.ModelExtraction.FamilyPatterns, fromFile.ModelExtraction.FamilyPatterns, "family_patterns drifted")
	assert.Equal(t, defaults.ModelExtraction.ArchitectureMappings, fromFile.ModelExtraction.ArchitectureMappings, "architecture_mappings drifted")
	assert.Equal(t, defaults.ModelExtraction.FamilyAliases, fromFile.ModelExtraction.FamilyAliases, "family_aliases drifted")
	assert.Equal(t, defaults.ModelExtraction.PublisherMappings, fromFile.ModelExtraction.PublisherMappings, "publisher_mappings drifted")
	assert.Equal(t, defaults.Quantization.Mappings, fromFile.Quantization.Mappings, "quantization mappings drifted")
	assert.Equal(t, defaults.Capabilities.TypeCapabilities, fromFile.Capabilities.TypeCapabilities, "type_capabilities drifted")
	assert.Equal(t, defaults.Capabilities.NamePatterns, fromFile.Capabilities.NamePatterns, "capability name_patterns drifted")
	assert.Equal(t, defaults.Capabilities.ContextThresholds, fromFile.Capabilities.ContextThresholds, "context_thresholds drifted")
	assert.ElementsMatch(t, defaults.SpecialRules.PreserveFamily, fromFile.SpecialRules.PreserveFamily, "preserve_family drifted")
	assert.ElementsMatch(t, defaults.SpecialRules.GenericNames, fromFile.SpecialRules.GenericNames, "generic_names drifted")
}

// resetConfigSingleton clears the LoadModelConfig sync.Once cache so a test
// can force a fresh load. Registered via t.Cleanup by every test that
// touches the singleton, so a broken/mutated state from one test can't leak
// into the next (LoadModelConfig's whole point is process-lifetime caching,
// which fights individual test isolation otherwise).
func resetConfigSingleton() {
	configOnce = sync.Once{}
	configInstance, errConfig, configSource, configParseWarnings = nil, nil, "", nil
}

// TestLogConfigStatus_ReportsShippedFileSource verifies LogConfigStatus surfaces
// the real path when config/models.yaml loads cleanly, giving operators a
// startup-log trail rather than the previous total silence.
func TestLogConfigStatus_ReportsShippedFileSource(t *testing.T) {
	oldConfigDir := os.Getenv("OLLA_CONFIG_DIR")
	dir := filepath.Dir(shippedConfigPath)
	require.NoError(t, os.Setenv("OLLA_CONFIG_DIR", dir))
	defer os.Setenv("OLLA_CONFIG_DIR", oldConfigDir)

	resetConfigSingleton()
	t.Cleanup(resetConfigSingleton)

	log, _, _ := logger.New(&logger.Config{Level: "debug", Theme: "default"})
	LogConfigStatus(logger.NewPlainStyledLogger(log))

	assert.Equal(t, filepath.Join(dir, "models.yaml"), ConfigSource())
}

// brokenModelsYAML reproduces the exact issue #204 construct: a `\d` inside
// a double-quoted YAML scalar. yaml.v3's double-quoted scalars only accept a
// small fixed escape set, so this is invalid YAML, not just an unwanted regex.
const brokenModelsYAML = `
model_extraction:
  family_patterns:
    - pattern: "^(llama|gemma|phi|qwen)[-_]?(\d+(?:\.\d+)?)"
      family_group: 1
      variant_group: 2
      description: "broken escape, matches issue #204"
`

// TestLoadConfigFromFile_BrokenYAMLFallsBackWithWarning is the failure-path
// counterpart to TestShippedConfigParses. loadConfigFromFile() is called
// directly (bypassing the LoadModelConfig sync.Once singleton) so the
// behaviour under test is the loader itself, not caching. A directory whose
// models.yaml has the issue #204 escape bug must still yield a usable
// config - falling back to getDefaultConfig() without panicking - while
// recording a parse warning that names the broken path.
func TestLoadConfigFromFile_BrokenYAMLFallsBackWithWarning(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "models.yaml"), []byte(brokenModelsYAML), 0644))

	// t.Chdir makes the loader's first candidate path ("models.yaml") resolve
	// into dir; OLLA_CONFIG_DIR is pointed at a second, empty temp dir so the
	// env-derived candidate doesn't re-read the same file and double the
	// warning count.
	t.Chdir(dir)
	t.Setenv("OLLA_CONFIG_DIR", t.TempDir())

	config, source, warnings := loadConfigFromFile()

	require.NotNil(t, config, "must fall back to a usable config, never nil")
	assert.Equal(t, getDefaultConfig(), config, "must fall back to embedded defaults")
	assert.Empty(t, source, "no path should be reported as successfully loaded")

	require.Len(t, warnings, 1, "expected exactly one parse warning for the broken models.yaml")
	assert.Contains(t, warnings[0], "models.yaml")
	assert.Contains(t, warnings[0], "unknown escape character")

	// the fallback config must itself still be usable - no panics downstream
	require.NoError(t, config.compilePatterns())
}

// TestLogConfigStatus_WarnsOnBrokenShippedFile is the LogConfigStatus
// counterpart: a models.yaml that exists but fails to parse must produce a
// warning-level log line naming the file, not the same silence as the
// "no file found" case.
func TestLogConfigStatus_WarnsOnBrokenShippedFile(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "models.yaml"), []byte(brokenModelsYAML), 0644))

	t.Chdir(dir)
	t.Setenv("OLLA_CONFIG_DIR", t.TempDir())

	resetConfigSingleton()
	t.Cleanup(resetConfigSingleton)

	var buf bytes.Buffer
	log := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	LogConfigStatus(logger.NewPlainStyledLogger(log))

	out := buf.String()
	assert.Contains(t, out, "level=WARN")
	assert.Contains(t, out, "could not parse")
	assert.Contains(t, out, "models.yaml")
}

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
