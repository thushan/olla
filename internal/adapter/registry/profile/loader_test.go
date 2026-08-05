package profile

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestLoadProfiles_MalformedYAMLSurfacesWarning is the regression test for
// the same silent-drop bug class as issue #204: a profile file that exists
// but fails to parse used to be reported with a bare fmt.Printf and then
// vanish - LoadProfiles() itself returned no error, so NewFactory never knew
// anything went wrong. It must now show up in LoadWarnings(), and must not
// stop the rest of the directory (valid custom profiles, and the built-ins)
// from loading.
func TestLoadProfiles_MalformedYAMLSurfacesWarning(t *testing.T) {
	dir := t.TempDir()

	// invalid YAML: unbalanced flow mapping
	broken := "name: broken-profile\nrouting: {prefixes: [broken\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "broken.yaml"), []byte(broken), 0644))

	valid := "name: custom-valid\nversion: \"1.0\"\nrouting:\n  prefixes:\n    - custom-valid\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "valid.yaml"), []byte(valid), 0644))

	loader := NewProfileLoader(dir)
	require.NoError(t, loader.LoadProfiles(), "LoadProfiles must not fail outright for one bad file")

	warnings := loader.LoadWarnings()
	require.Len(t, warnings, 1, "expected exactly one load warning")
	assert.Contains(t, warnings[0], "broken.yaml", "warning should name the broken file")

	// the valid custom profile must still have loaded
	_, ok := loader.GetProfile("custom-valid")
	assert.True(t, ok, "valid profile in the same directory should still load")

	// built-ins must still be present - a bad custom file must not wipe them out
	_, ok = loader.GetProfile("ollama")
	assert.True(t, ok, "built-in ollama profile should still be present")
}

// TestLoadProfiles_CleanDirectoryHasNoWarnings guards against false
// positives: a directory with only valid profiles must report zero warnings.
func TestLoadProfiles_CleanDirectoryHasNoWarnings(t *testing.T) {
	dir := t.TempDir()
	valid := "name: custom-clean\nversion: \"1.0\"\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "clean.yaml"), []byte(valid), 0644))

	loader := NewProfileLoader(dir)
	require.NoError(t, loader.LoadProfiles())

	assert.Empty(t, loader.LoadWarnings())
}

// TestLoadProfiles_ReloadClearsStaleWarnings ensures a fixed-then-reloaded
// profile doesn't leave a stale warning behind.
func TestLoadProfiles_ReloadClearsStaleWarnings(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "flaky.yaml")
	require.NoError(t, os.WriteFile(path, []byte("name: flaky\nrouting: {prefixes: [broken\n"), 0644))

	loader := NewProfileLoader(dir)
	require.NoError(t, loader.LoadProfiles())
	require.Len(t, loader.LoadWarnings(), 1, "expected one warning before the fix")

	require.NoError(t, os.WriteFile(path, []byte("name: flaky\nversion: \"1.0\"\n"), 0644))
	require.NoError(t, loader.LoadProfiles())

	assert.Empty(t, loader.LoadWarnings(), "reload after fixing the file should clear the warning")
	_, ok := loader.GetProfile("flaky")
	assert.True(t, ok, "fixed profile should now be loaded")
}

// TestFactory_LoadWarnings_SurfacesFromNewFactory confirms the Factory-level
// accessor NewFactory/--validate-config will actually use.
func TestFactory_LoadWarnings_SurfacesFromNewFactory(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "broken.yaml"), []byte("name: [unterminated\n"), 0644))

	factory, err := NewFactory(dir)
	require.NoError(t, err, "NewFactory must not hard-fail for a single bad profile")

	assert.Len(t, factory.LoadWarnings(), 1)
}
