package profile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/thushan/olla/internal/core/domain"
)

// The SGLang and LMDeploy integration pages tell operators to copy the shipped
// profile to zz-<name>.yaml next to the original and set anthropic_support.enabled
// to false. The loader reads files in name order and the last profile with a given
// name wins, so the copy has to sort after the original. Olla ignores unknown YAML
// keys, so a typo'd override would silently do nothing; these tests load the exact
// override the docs describe.

func TestShippedProfiles_NativeAnthropicDefaults(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		file        string
		minVersion  string
		limitations []string
	}{
		{domain.ProfileSGLang, "sglang.yaml", "0.5.9", nil},
		{domain.ProfileLMDeploy, "lmdeploy.yaml", "0.13.0", []string{domain.AnthropicLimitationNoToolUse}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			loader := NewProfileLoader(shippedProfilesDir)
			require.NoError(t, loader.LoadProfiles())
			p, ok := loader.GetProfile(tc.name)
			require.True(t, ok)

			support := p.GetConfig().API.AnthropicSupport
			require.NotNil(t, support)
			assert.True(t, support.Enabled)
			assert.Equal(t, "/v1/messages", support.MessagesPath)
			assert.Equal(t, tc.minVersion, support.MinVersion)
			assert.Equal(t, tc.limitations, support.Limitations)
		})
	}
}

func TestShippedProfiles_NativeAnthropicLocalOverrideDisables(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		file string
	}{
		{domain.ProfileSGLang, "sglang.yaml"},
		{domain.ProfileLMDeploy, "lmdeploy.yaml"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			shipped, err := os.ReadFile(shippedProfilesDir + tc.file)
			require.NoError(t, err)

			const enabled = "anthropic_support:\n    enabled: true"
			require.Contains(t, string(shipped), enabled, "the docs quote this block")
			override := strings.Replace(string(shipped), enabled, "anthropic_support:\n    enabled: false", 1)

			dir := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(dir, tc.file), shipped, 0o600))
			require.NoError(t, os.WriteFile(filepath.Join(dir, "zz-"+tc.file), []byte(override), 0o600))

			loader := NewProfileLoader(dir)
			require.NoError(t, loader.LoadProfiles())
			assert.Empty(t, loader.LoadWarnings())

			p, ok := loader.GetProfile(tc.name)
			require.True(t, ok)
			support := p.GetConfig().API.AnthropicSupport
			require.NotNil(t, support)
			assert.False(t, support.Enabled, "the zz- copy must replace the original")
			assert.Equal(t, "/v1/messages", support.MessagesPath, "the rest of the profile is kept")
		})
	}
}
