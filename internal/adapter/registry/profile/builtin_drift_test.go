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

// TestBuiltinProfilePathIndicesAlignWithPaths guards against a class of bug
// TestBuiltinProfilesMatchShippedYAML cannot catch: PathIndices drifting out
// of step with API.Paths itself. The drift test above only proves the Go
// fallback matches the shipped YAML - if both sides carry the same wrong
// indices (as happened when API.Paths grew new entries and PathIndices was
// never updated to match), that test keeps passing while every index is
// still wrong. This test checks the indices against the paths list they
// actually index into: out of range, and - for Completions/ChatCompletions,
// which have an unambiguous expected value in ParsingRules - pointing at the
// wrong path entirely.
func TestBuiltinProfilePathIndicesAlignWithPaths(t *testing.T) {
	loader := &ProfileLoader{}
	builtins := make(map[string]domain.InferenceProfile)
	loader.loadBuiltInProfilesInto(builtins)

	for name := range builtinShippedPairs {
		t.Run(name, func(t *testing.T) {
			profile, ok := builtins[name]
			require.True(t, ok)
			configurable, ok := profile.(*ConfigurableProfile)
			require.True(t, ok)
			cfg := configurable.GetConfig()

			indices := map[string]int{
				"health":           cfg.PathIndices.Health,
				"models":           cfg.PathIndices.Models,
				"completions":      cfg.PathIndices.Completions,
				"chat_completions": cfg.PathIndices.ChatCompletions,
				"embeddings":       cfg.PathIndices.Embeddings,
			}
			for field, idx := range indices {
				// -1 marks the function as deliberately not addressable in
				// API.Paths (e.g. llama.cpp's disabled /health). Anything
				// else must be a real slot.
				if idx == -1 {
					continue
				}
				require.GreaterOrEqualf(t, idx, 0, "%s.PathIndices.%s is negative but not -1", name, field)
				require.Lessf(t, idx, len(cfg.API.Paths), "%s.PathIndices.%s = %d is out of range for API.Paths (len %d)", name, field, idx, len(cfg.API.Paths))
			}

			if cfg.Request.ParsingRules.CompletionsPath != "" && cfg.PathIndices.Completions >= 0 {
				assert.Equal(t, cfg.Request.ParsingRules.CompletionsPath, cfg.API.Paths[cfg.PathIndices.Completions],
					"%s.PathIndices.Completions points at the wrong entry in API.Paths", name)
			}
			if cfg.Request.ParsingRules.ChatCompletionsPath != "" && cfg.PathIndices.ChatCompletions >= 0 {
				assert.Equal(t, cfg.Request.ParsingRules.ChatCompletionsPath, cfg.API.Paths[cfg.PathIndices.ChatCompletions],
					"%s.PathIndices.ChatCompletions points at the wrong entry in API.Paths", name)
			}
		})
	}
}
