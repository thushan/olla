package profile

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/thushan/olla/internal/core/domain"
)

// shippedProfilesDir is config/profiles, relative to this package
// (internal/adapter/registry/profile -> repo root is four levels up).
const shippedProfilesDir = "../../../../config/profiles/"

// builtinShippedPairs maps each hardcoded Go fallback in loadBuiltInProfilesInto
// to the shipped YAML file it must match.
var builtinShippedPairs = map[string]string{
	domain.ProfileOllama:           "ollama.yaml",
	domain.ProfileLmStudio:         "lmstudio.yaml",
	domain.ProfileOpenAICompatible: "openai-compatible.yaml",
	domain.ProfileLlamaCpp:         "llamacpp.yaml",
}

// TestBuiltinProfilesMatchShippedYAML is a drift guard between the hardcoded
// Go fallbacks in loadBuiltInProfilesInto (used when config/profiles/ is
// missing or a specific file fails to load) and the shipped YAML in
// config/profiles/. These drifted apart in practice - the Ollama fallback was
// missing anthropic_support, characteristics.auth and metrics.extraction
// entirely, so an operator running without the profiles directory silently
// got a materially weaker profile than the shipped default. If this fails,
// reconcile the drift in both directions - don't just update one side.
func TestBuiltinProfilesMatchShippedYAML(t *testing.T) {
	loader := &ProfileLoader{}
	builtins := make(map[string]domain.InferenceProfile)
	loader.loadBuiltInProfilesInto(builtins)

	// Completeness guard: if a new builtin fallback is ever added to
	// loadBuiltInProfilesInto without a matching entry here, this whole test
	// would keep passing while silently not drift-checking it.
	require.Len(t, builtins, len(builtinShippedPairs), "loadBuiltInProfilesInto produced a different number of builtins than builtinShippedPairs knows about - update builtinShippedPairs")

	for name, filename := range builtinShippedPairs {
		t.Run(name, func(t *testing.T) {
			data, err := os.ReadFile(shippedProfilesDir + filename)
			require.NoError(t, err, "shipped profile file must exist")

			var fromFile domain.ProfileConfig
			require.NoError(t, yaml.Unmarshal(data, &fromFile), "shipped profile must be valid YAML")

			profile, ok := builtins[name]
			require.True(t, ok, "loadBuiltInProfilesInto must produce a %q profile", name)
			configurable, ok := profile.(*ConfigurableProfile)
			require.True(t, ok, "builtin profile must be a *ConfigurableProfile")

			assert.Equal(t, &fromFile, configurable.GetConfig(), "builtin %q drifted from %s%s", name, shippedProfilesDir, filename)
		})
	}
}
