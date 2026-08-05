package unifier

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
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
	dir := filepath.Dir(shippedConfigPath)
	t.Setenv("OLLA_CONFIG_DIR", dir)

	resetConfigSingleton()
	t.Cleanup(resetConfigSingleton)

	var buf bytes.Buffer
	log := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	LogConfigStatus(logger.NewPlainStyledLogger(log))

	assert.Equal(t, filepath.Join(dir, "models.yaml"), ConfigSource())
	assert.Contains(t, buf.String(), "level=INFO")
	assert.Contains(t, buf.String(), "model unification config loaded")
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
	// into dir. OLLA_CONFIG_DIR is explicitly unset: filepath.Join("", "models.yaml")
	// collapses to "models.yaml", which is the same candidate as above - if the
	// loader didn't skip the env candidate when unset, this would read the same
	// broken file twice and double the warning count.
	t.Chdir(dir)
	t.Setenv("OLLA_CONFIG_DIR", "")

	config, source, warnings := loadConfigFromFile()

	require.NotNil(t, config, "must fall back to a usable config, never nil")
	assert.Equal(t, getDefaultConfig(), config, "must fall back to embedded defaults")
	assert.Empty(t, source, "no path should be reported as successfully loaded")

	require.Len(t, warnings, 1, "expected exactly one parse warning for the broken models.yaml, not a duplicate from the OLLA_CONFIG_DIR candidate")
	assert.Contains(t, warnings[0], "models.yaml")
	assert.Greater(t, len(warnings[0]), len("models.yaml"), "warning should carry the underlying parse error, not just the path")

	// the fallback config must itself still be usable - no panics downstream
	require.NoError(t, config.compilePatterns())
}

// TestLoadConfigFromFile_EmptyFileFallsBackWithWarning covers the other
// silent-failure shape: an empty (or comment-only) models.yaml is valid YAML
// and unmarshals without error, so without the isEffectivelyEmpty guard it
// would be treated as a successful load - configSource set, INFO logged -
// while every family pattern, mapping and rule is missing.
func TestLoadConfigFromFile_EmptyFileFallsBackWithWarning(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{"zero_byte_file", ""},
		{"comment_only_file", "# nothing to see here\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(dir, "models.yaml"), []byte(tt.content), 0644))

			t.Chdir(dir)
			t.Setenv("OLLA_CONFIG_DIR", "")

			config, source, warnings := loadConfigFromFile()

			require.NotNil(t, config, "must fall back to a usable config, never nil")
			assert.Equal(t, getDefaultConfig(), config, "must fall back to embedded defaults")
			assert.Empty(t, source, "no path should be reported as successfully loaded")

			require.Len(t, warnings, 1, "expected exactly one warning for the empty models.yaml")
			assert.Contains(t, warnings[0], "models.yaml")
			assert.Contains(t, warnings[0], "no configuration")

			require.NoError(t, config.compilePatterns())
		})
	}
}

// TestLoadConfigFromFile_PartialFileWithOnlyFamilyAliasesLoads guards against
// a regression in isEffectivelyEmpty: a file that only sets one section (here
// family_aliases) is a legitimate partial override, not an empty file, and
// must load successfully - no warning, configSource pointing at it, and the
// aliases present in the returned config.
func TestLoadConfigFromFile_PartialFileWithOnlyFamilyAliasesLoads(t *testing.T) {
	dir := t.TempDir()
	partialYAML := `
model_extraction:
  family_aliases:
    llama3: llama
    devstral: mistral
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "models.yaml"), []byte(partialYAML), 0644))

	t.Chdir(dir)
	t.Setenv("OLLA_CONFIG_DIR", "")

	config, source, warnings := loadConfigFromFile()

	require.NotNil(t, config)
	assert.Equal(t, filepath.Join(dir, "models.yaml"), mustAbs(t, source), "partial file with only family_aliases must be treated as usable, not empty")
	assert.Empty(t, warnings)

	assert.Equal(t, "llama", config.ModelExtraction.FamilyAliases["llama3"])
	assert.Equal(t, "mistral", config.ModelExtraction.FamilyAliases["devstral"])
}

// mustAbs resolves path against the current (t.Chdir'd) working directory so
// it can be compared against a path built from t.TempDir() (already absolute).
func mustAbs(t *testing.T, path string) string {
	t.Helper()
	abs, err := filepath.Abs(path)
	require.NoError(t, err)
	return abs
}

// TestLoadConfigFromFile_UnreadableCandidateRecordsWarning covers a candidate
// path that exists but can't be read as a file - here, a directory named
// models.yaml. That's not fs.ErrNotExist, so it must be recorded as a
// warning rather than silently skipped like a genuinely absent candidate.
// The exact OS error text isn't asserted since it differs by platform.
func TestLoadConfigFromFile_UnreadableCandidateRecordsWarning(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(dir, "models.yaml"), 0755))

	t.Chdir(dir)
	t.Setenv("OLLA_CONFIG_DIR", "")

	config, source, warnings := loadConfigFromFile()

	require.NotNil(t, config, "must fall back to a usable config, never crash")
	assert.Equal(t, getDefaultConfig(), config)
	assert.Empty(t, source)

	require.Len(t, warnings, 1, "a directory named models.yaml is not fs.ErrNotExist and must be recorded")
	assert.Contains(t, warnings[0], "models.yaml")

	require.NoError(t, config.compilePatterns())
}

// TestLoadModelConfig_InvalidRegexPatternFallsBackToCompiledDefaults covers a
// candidate that parses as valid YAML but contains a pattern that isn't a
// valid regex. Before this fix, LoadModelConfig would hand back the broken
// (uncompiled) config with errConfig set, and getConfig() in
// metadata_extractor.go would swap to getDefaultConfig() - which was never
// compiled either, so every regex-driven lookup silently no-opped. The fix
// must produce a *compiled*, *functional* fallback.
func TestLoadModelConfig_InvalidRegexPatternFallsBackToCompiledDefaults(t *testing.T) {
	dir := t.TempDir()
	badRegexYAML := `
model_extraction:
  family_patterns:
    - pattern: '(['
      family_group: 1
      variant_group: 2
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "models.yaml"), []byte(badRegexYAML), 0644))

	t.Chdir(dir)
	t.Setenv("OLLA_CONFIG_DIR", "")

	resetConfigSingleton()
	t.Cleanup(resetConfigSingleton)

	config, err := LoadModelConfig()
	require.NoError(t, err, "must recover onto the embedded defaults, not surface the bad-regex error")
	require.NotNil(t, config)

	assert.Empty(t, ConfigSource(), "defaults are in use, not the broken candidate")

	requireWarningNaming(t, configParseWarnings, "models.yaml")

	// functional, not just present: a default pattern must be compiled and
	// actually match, proving the fallback config went through compilePatterns.
	require.Len(t, config.ModelExtraction.FamilyPatterns, len(getDefaultConfig().ModelExtraction.FamilyPatterns))
	llamaPattern := config.ModelExtraction.FamilyPatterns[1]
	require.NotNil(t, llamaPattern.regex, "fallback defaults must be compiled, not just assigned")
	assert.True(t, llamaPattern.regex.MatchString("llama-3"), "compiled default pattern should match a real model name")
}

// requireWarningNaming fails the test unless one of the warnings mentions
// substr (typically a candidate path).
func requireWarningNaming(t *testing.T, warnings []string, substr string) {
	t.Helper()
	for _, w := range warnings {
		if strings.Contains(w, substr) {
			return
		}
	}
	t.Fatalf("expected a warning naming %q, got %v", substr, warnings)
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
	assert.Contains(t, out, "unusable models.yaml candidate")
	assert.Contains(t, out, "falling back to embedded defaults")
	assert.Contains(t, out, "models.yaml")
}

// TestLogConfigStatus_ShadowedBrokenCandidateReportsRealSource is the
// scenario issue #204's original complaint was about: a broken candidate
// earlier in the search order (here "models.yaml" in the cwd) sitting next
// to a working one later in the order (here the OLLA_CONFIG_DIR candidate).
// The broken one must still be warned about - the earlier "shadowed" bug
// dropped that warning entirely once a later candidate loaded - but the
// message must not claim defaults are active when a real file loaded.
func TestLogConfigStatus_ShadowedBrokenCandidateReportsRealSource(t *testing.T) {
	cwd := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(cwd, "models.yaml"), []byte(brokenModelsYAML), 0644))
	t.Chdir(cwd)

	configDir := t.TempDir()
	workingYAML := `
model_extraction:
  family_aliases:
    llama3: llama
`
	require.NoError(t, os.WriteFile(filepath.Join(configDir, "models.yaml"), []byte(workingYAML), 0644))
	t.Setenv("OLLA_CONFIG_DIR", configDir)

	resetConfigSingleton()
	t.Cleanup(resetConfigSingleton)

	var buf bytes.Buffer
	log := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	LogConfigStatus(logger.NewPlainStyledLogger(log))

	out := buf.String()
	assert.Contains(t, out, "level=WARN", "the broken cwd candidate must still be reported, not shadowed into silence")
	assert.Contains(t, out, "unusable models.yaml candidate")
	assert.Contains(t, out, "using ", "must say which source is active instead of claiming defaults")
	assert.Contains(t, out, "instead")
	assert.NotContains(t, out, "falling back to embedded defaults", "a real file loaded - defaults are not what's active")

	assert.Contains(t, out, "level=INFO")
	assert.Equal(t, filepath.Join(configDir, "models.yaml"), ConfigSource())
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
	t.Setenv("OLLA_CONFIG_DIR", tmpDir)

	// Clear the config cache to force reload, and restore it afterwards -
	// otherwise configSource is left pointing at this test's (now-deleted)
	// tmpDir for whatever test runs next.
	resetConfigSingleton()
	t.Cleanup(resetConfigSingleton)

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
