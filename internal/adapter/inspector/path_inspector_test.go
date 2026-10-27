package inspector

import (
	"context"
	"net/http"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/thushan/olla/internal/adapter/registry/profile"
	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/logger"
)

// TestPathInspector_Inspect_RealProfiles_EmbedPaths loads the real
// config/profiles to guard the Ollama vs TEI embedding paths: TEI's /embed is
// a suffix of Ollama's /api/embed, so the profile files themselves decide
// which backend an /api/embed request resolves to.
func TestPathInspector_Inspect_RealProfiles_EmbedPaths(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot determine test file location")
	}
	// Test files live under internal/adapter/inspector, three levels below the repo root
	profilesDir, err := filepath.Abs(filepath.Join(filepath.Dir(thisFile), "..", "..", "..", "config", "profiles"))
	if err != nil {
		t.Fatalf("cannot resolve profiles dir: %v", err)
	}

	factory, err := profile.NewFactory(profilesDir)
	if err != nil {
		t.Fatalf("failed to load real profiles from %s: %v", profilesDir, err)
	}
	inspector := NewPathInspector(factory, createTestLogger())
	ctx := context.Background()

	const teiProfile = "text-embeddings-inference"

	tests := []struct {
		name    string
		path    string
		want    string
		notWant string
	}{
		{
			name:    "ollama native embed resolves to ollama, not TEI",
			path:    "/api/embed",
			want:    domain.ProfileOllama,
			notWant: teiProfile,
		},
		{
			name:    "TEI native embed resolves to TEI, not ollama",
			path:    "/embed",
			want:    teiProfile,
			notWant: domain.ProfileOllama,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// reqProfile, not "profile": that name would shadow the imported
			// registry/profile package.
			reqProfile := domain.NewRequestProfile(tt.path)
			if err := inspector.Inspect(ctx, nil, reqProfile); err != nil {
				t.Fatalf("PathInspector.Inspect() error = %v", err)
			}

			if !contains(reqProfile.SupportedBy, tt.want) {
				t.Errorf("Inspect(%q) supported profiles = %v, want %q", tt.path, reqProfile.SupportedBy, tt.want)
			}
			if contains(reqProfile.SupportedBy, tt.notWant) {
				t.Errorf("Inspect(%q) supported profiles = %v, do not want %q", tt.path, reqProfile.SupportedBy, tt.notWant)
			}
		})
	}
}

func TestPathInspector_Name(t *testing.T) {
	inspector := createTestPathInspector(t)

	if got := inspector.Name(); got != "path" {
		t.Errorf("PathInspector.Name() = %v, want %v", got, "path")
	}
}

func TestPathInspector_Inspect_EmptyPath(t *testing.T) {
	inspector := createTestPathInspector(t)
	ctx := context.Background()
	req, _ := http.NewRequest("POST", "/", nil)
	profile := domain.NewRequestProfile("")

	err := inspector.Inspect(ctx, req, profile)

	if err != nil {
		t.Errorf("PathInspector.Inspect() with empty path should not error, got %v", err)
	}

	if len(profile.SupportedBy) != 0 {
		t.Errorf("PathInspector.Inspect() with empty path should not add supported profiles, got %v", profile.SupportedBy)
	}
}

func TestPathInspector_Inspect_ChatCompletions(t *testing.T) {
	inspector := createTestPathInspector(t)
	ctx := context.Background()
	req, _ := http.NewRequest("POST", "/v1/chat/completions", nil)
	profile := domain.NewRequestProfile("/v1/chat/completions")

	err := inspector.Inspect(ctx, req, profile)

	if err != nil {
		t.Errorf("PathInspector.Inspect() error = %v, want nil", err)
	}

	expectedProfiles := []string{domain.ProfileOllama, domain.ProfileLmStudio, domain.ProfileOpenAICompatible}
	if !containsAllProfiles(profile.SupportedBy, expectedProfiles) {
		t.Errorf("PathInspector.Inspect() supported profiles = %v, want all of %v", profile.SupportedBy, expectedProfiles)
	}

	meta, exists := profile.InspectionMeta.Load(domain.InspectionMetaPathSupport)
	if !exists {
		t.Error("PathInspector.Inspect() should set path support metadata")
	}

	if metaSlice, ok := meta.([]string); !ok || len(metaSlice) == 0 {
		t.Errorf("PathInspector.Inspect() metadata should be non-empty string slice, got %v", meta)
	}
}

func TestPathInspector_Inspect_Completions(t *testing.T) {
	inspector := createTestPathInspector(t)
	ctx := context.Background()
	req, _ := http.NewRequest("POST", "/v1/completions", nil)
	profile := domain.NewRequestProfile("/v1/completions")

	err := inspector.Inspect(ctx, req, profile)

	if err != nil {
		t.Errorf("PathInspector.Inspect() error = %v, want nil", err)
	}

	expectedProfiles := []string{domain.ProfileOllama, domain.ProfileLmStudio, domain.ProfileOpenAICompatible}
	if !containsAllProfiles(profile.SupportedBy, expectedProfiles) {
		t.Errorf("PathInspector.Inspect() supported profiles = %v, want all of %v", profile.SupportedBy, expectedProfiles)
	}
}

func TestPathInspector_Inspect_OllamaGenerate(t *testing.T) {
	inspector := createTestPathInspector(t)
	ctx := context.Background()
	req, _ := http.NewRequest("POST", "/api/generate", nil)
	profile := domain.NewRequestProfile("/api/generate")

	err := inspector.Inspect(ctx, req, profile)

	if err != nil {
		t.Errorf("PathInspector.Inspect() error = %v, want nil", err)
	}

	if !contains(profile.SupportedBy, domain.ProfileOllama) {
		t.Errorf("PathInspector.Inspect() should support Ollama for /api/generate path, got %v", profile.SupportedBy)
	}

	if contains(profile.SupportedBy, domain.ProfileLmStudio) {
		t.Errorf("PathInspector.Inspect() should not support LMStudio for /api/generate path, got %v", profile.SupportedBy)
	}
}

/*
	func TestPathInspector_Inspect_LMStudioModels(t *testing.T) {
		inspector := createTestPathInspector(t)
		ctx := context.Background()
		req, _ := http.NewRequest("GET", "/api/v0/models", nil)
		profile := domain.NewRequestProfile("/api/v0/models")

		err := inspector.Inspect(ctx, req, profile)

		if err != nil {
			t.Errorf("PathInspector.Inspect() error = %v, want nil", err)
		}

		if contains(profile.SupportedBy, domain.ProfileLmStudio) {
			t.Errorf("PathInspector.Inspect() LMStudio models path should not match parsing rules (models path != request path), got %v", profile.SupportedBy)
		}
	}
*/
func TestPathInspector_Inspect_UnknownPath(t *testing.T) {
	inspector := createTestPathInspector(t)
	ctx := context.Background()
	req, _ := http.NewRequest("POST", "/unknown/path", nil)
	profile := domain.NewRequestProfile("/unknown/path")

	err := inspector.Inspect(ctx, req, profile)

	if err != nil {
		t.Errorf("PathInspector.Inspect() error = %v, want nil", err)
	}

	if len(profile.SupportedBy) != 0 {
		t.Errorf("PathInspector.Inspect() unknown path should not match any profiles, got %v", profile.SupportedBy)
	}

	_, exists := profile.InspectionMeta.Load(domain.InspectionMetaPathSupport)
	if exists {
		t.Error("PathInspector.Inspect() should not set metadata for unknown paths")
	}
}

func TestPathInspector_Inspect_PrefixedPaths(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected []string
	}{
		{
			name:     "prefixed chat completions",
			path:     "/api/v1/chat/completions",
			expected: []string{domain.ProfileOllama, domain.ProfileLmStudio, domain.ProfileOpenAICompatible},
		},
		{
			name:     "deeply prefixed completions",
			path:     "/some/deep/prefix/v1/completions",
			expected: []string{domain.ProfileOllama, domain.ProfileLmStudio, domain.ProfileOpenAICompatible},
		},
		{
			name:     "prefixed ollama generate",
			path:     "/prefix/api/generate",
			expected: []string{domain.ProfileOllama},
		},
	}

	inspector := createTestPathInspector(t)
	ctx := context.Background()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, _ := http.NewRequest("POST", tt.path, nil)
			profile := domain.NewRequestProfile(tt.path)

			err := inspector.Inspect(ctx, req, profile)

			if err != nil {
				t.Errorf("PathInspector.Inspect() error = %v, want nil", err)
			}

			for _, expectedProfile := range tt.expected {
				if !contains(profile.SupportedBy, expectedProfile) {
					t.Errorf("PathInspector.Inspect() should support %v for path %v, got %v", expectedProfile, tt.path, profile.SupportedBy)
				}
			}
		})
	}
}

func TestPathInspector_pathMatchesRules(t *testing.T) {
	inspector := createTestPathInspector(t)

	tests := []struct {
		name     string
		path     string
		rules    domain.RequestParsingRules
		expected bool
	}{
		{
			name: "exact chat completions match",
			path: "/v1/chat/completions",
			rules: domain.RequestParsingRules{
				ChatCompletionsPath: "/v1/chat/completions",
				CompletionsPath:     "/v1/completions",
			},
			expected: true,
		},
		{
			name: "suffix chat completions match",
			path: "/api/v1/chat/completions",
			rules: domain.RequestParsingRules{
				ChatCompletionsPath: "/v1/chat/completions",
			},
			expected: true,
		},
		{
			name: "completions path match",
			path: "/v1/completions",
			rules: domain.RequestParsingRules{
				CompletionsPath: "/v1/completions",
			},
			expected: true,
		},
		{
			name: "generate path match",
			path: "/api/generate",
			rules: domain.RequestParsingRules{
				GeneratePath: "/api/generate",
			},
			expected: true,
		},
		{
			name: "no match",
			path: "/unknown/path",
			rules: domain.RequestParsingRules{
				ChatCompletionsPath: "/v1/chat/completions",
				CompletionsPath:     "/v1/completions",
			},
			expected: false,
		},
		{
			name:     "empty rules",
			path:     "/v1/chat/completions",
			rules:    domain.RequestParsingRules{},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := inspector.pathMatchesRules(tt.path, tt.rules)
			if result != tt.expected {
				t.Errorf("pathMatchesRules() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func createTestPathInspector(t *testing.T) *PathInspector {
	t.Helper()

	profileFactory := createTestProfileFactory(t)
	logger := createTestLogger()

	return NewPathInspector(profileFactory, logger)
}

func createTestLogger() logger.StyledLogger {
	cfg := &logger.Config{
		Level:      "error",
		PrettyLogs: false,
		Theme:      "default",
	}

	baseLogger, styledLogger, _, err := logger.NewWithTheme(cfg)
	if err != nil {
		panic(err)
	}

	_ = baseLogger
	return styledLogger
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

func containsAllProfiles(got, expected []string) bool {
	for _, exp := range expected {
		if !contains(got, exp) {
			return false
		}
	}
	return true
}

// Native TEI bodies (/rerank, /predict, /embed) carry no model field, so the
// path alone must say which kind of model the request needs.
func TestPathInspector_RequiredCapabilitiesByPath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		path           string
		embeddings     bool
		rerank         bool
		classification bool
		none           bool
	}{
		{path: "/rerank", rerank: true},
		{path: "/v1/rerank", rerank: true},
		{path: "/v2/rerank", rerank: true},
		{path: "/predict", classification: true},
		{path: "/v1/embeddings", embeddings: true},
		{path: "/olla/proxy/v1/embeddings", embeddings: true},
		{path: "/embed", embeddings: true},
		{path: "/embed_all", embeddings: true},
		{path: "/embed_sparse", embeddings: true},
		{path: "/similarity", embeddings: true},
		{path: "/olla/tei/v1/rerank", rerank: true},
		{path: "/api/embeddings", none: true}, // Ollama's legacy embeddings; model and input are in the body
		{path: "/api/embed", none: true},      // Ollama's embed carries its model in the body
		{path: "/api/rerank", none: true},     // not a TEI path
		{path: "/prefix/predict", none: true}, // single-segment names match whole paths only
		{path: "/v1/chat/completions", none: true},
		{path: "/tokenize", none: true},
		{path: "/v1/models", none: true},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			t.Parallel()

			inspector := NewPathInspector(nil, createTestLogger())
			reqProfile := domain.NewRequestProfile(tt.path)
			if err := inspector.Inspect(context.Background(), nil, reqProfile); err != nil {
				t.Fatalf("Inspect() error = %v", err)
			}

			caps := reqProfile.ModelCapabilities
			if tt.none {
				if caps != nil {
					t.Errorf("Inspect(%q) set capabilities %+v, want none", tt.path, *caps)
				}
				return
			}
			if caps == nil {
				t.Fatalf("Inspect(%q) set no capabilities", tt.path)
			}
			if caps.Embeddings != tt.embeddings || caps.Rerank != tt.rerank || caps.Classification != tt.classification {
				t.Errorf("Inspect(%q) = embeddings:%v rerank:%v classification:%v, want %v/%v/%v",
					tt.path, caps.Embeddings, caps.Rerank, caps.Classification, tt.embeddings, tt.rerank, tt.classification)
			}
			if caps.ChatCompletion || caps.TextGeneration {
				t.Errorf("Inspect(%q) offered chat or text generation: %+v", tt.path, *caps)
			}
		})
	}
}

// shippedProfilePaths loads config/profiles and returns each profile's declared
// paths, deduplicated per profile.
func shippedProfilePaths(t *testing.T) (*profile.Factory, map[string][]string) {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot determine test file location")
	}
	profilesDir := filepath.Join(filepath.Dir(thisFile), "..", "..", "..", "config", "profiles")
	factory, err := profile.NewFactory(profilesDir)
	if err != nil {
		t.Fatalf("failed to load shipped profiles: %v", err)
	}
	out := make(map[string][]string)
	// the inspector registers openai-compatible whether or not the factory lists it
	names := append(factory.GetAvailableProfiles(), domain.ProfileOpenAICompatible)
	slices.Sort(names)
	for _, name := range slices.Compact(names) {
		p, err := factory.GetProfile(name)
		if err != nil {
			t.Fatalf("GetProfile(%q): %v", name, err)
		}
		seen := make(map[string]bool)
		for _, path := range p.GetPaths() {
			if path != "" && !seen[path] {
				seen[path] = true
				out[name] = append(out[name], path)
			}
		}
	}
	return factory, out
}

func segments(path string) int {
	return len(strings.FieldsFunc(path, func(r rune) bool { return r == '/' }))
}

// Every path a shipped profile declares must resolve to exactly the profiles
// that declare it, so a path shared by name (/health, /v1/embeddings) lists all
// its owners and nothing else.
func TestPathInspector_ShippedProfiles_DeclaredPathsResolveToTheirOwners(t *testing.T) {
	factory, byProfile := shippedProfilePaths(t)
	inspector := NewPathInspector(factory, createTestLogger())

	owners := make(map[string][]string)
	for name, paths := range byProfile {
		for _, path := range paths {
			owners[path] = append(owners[path], name)
		}
	}

	for path, want := range owners {
		t.Run(path, func(t *testing.T) {
			reqProfile := domain.NewRequestProfile(path)
			if err := inspector.Inspect(context.Background(), nil, reqProfile); err != nil {
				t.Fatalf("Inspect(%q) error = %v", path, err)
			}
			got := slices.Clone(reqProfile.SupportedBy)
			slices.Sort(got)
			got = slices.Compact(got)
			slices.Sort(want)
			if !slices.Equal(got, want) {
				t.Errorf("Inspect(%q) supported by %v, want exactly %v", path, got, want)
			}
		})
	}
}

// A profile path only matches a longer request path when it has two or more
// segments. A one-segment name such as /embed, /version or /health says nothing
// about a deeper path, which belongs to whichever backend declares it.
func TestPathInspector_ShippedProfiles_SingleSegmentPathsDoNotCaptureDeeperPaths(t *testing.T) {
	factory, byProfile := shippedProfilePaths(t)
	inspector := NewPathInspector(factory, createTestLogger())

	single := make(map[string]bool)
	multi := make(map[string][]string)
	for name, paths := range byProfile {
		for _, path := range paths {
			if segments(path) == 1 {
				single[path] = true
			} else if segments(path) > 1 {
				multi[path] = append(multi[path], name)
			}
		}
	}

	for path := range single {
		deeper := "/api/unlisted" + path
		t.Run(deeper, func(t *testing.T) {
			reqProfile := domain.NewRequestProfile(deeper)
			if err := inspector.Inspect(context.Background(), nil, reqProfile); err != nil {
				t.Fatalf("Inspect(%q) error = %v", deeper, err)
			}
			if len(reqProfile.SupportedBy) != 0 {
				t.Errorf("Inspect(%q) was captured by %v via the one-segment path %q", deeper, reqProfile.SupportedBy, path)
			}
		})
	}

	// the other side of the rule: prefixed multi-segment paths still resolve
	for path, owners := range multi {
		prefixed := "/mount" + path
		t.Run(prefixed, func(t *testing.T) {
			reqProfile := domain.NewRequestProfile(prefixed)
			if err := inspector.Inspect(context.Background(), nil, reqProfile); err != nil {
				t.Fatalf("Inspect(%q) error = %v", prefixed, err)
			}
			for _, owner := range owners {
				if !contains(reqProfile.SupportedBy, owner) {
					t.Errorf("Inspect(%q) supported by %v, want %q included", prefixed, reqProfile.SupportedBy, owner)
				}
			}
		})
	}
}

// knownServers is a curated record of which shipped profiles serve each
// standard OpenAI/Anthropic path, taken from each project's own docs or source.
// A profile that serves a path but does not declare it is evicted by stage 1
// whenever another profile in the fleet does declare it, so the declarations
// have to agree with reality both ways. Add a row here when verifying a new
// backend; do not widen a row on a guess.
//
// Deliberate absences: lmdeploy returns 400 on /v1/embeddings; Ollama documents
// /v1/messages/count_tokens as unsupported; dmr and lemonade serve the OpenAI
// surface under their own prefixes, and dmr's Anthropic path is
// /anthropic/v1/messages (declared, but not a standard path so absent from this
// table); TEI only has the bare /rerank and /embed style routes.
//
// openai-compatible is the catch-all for servers without a profile (a remote vLLM
// or SGLang typed "openai"), so it is permissive by design: it is listed for every
// path another profile declares, including /v1/messages, /v1/messages/count_tokens
// and /v1/rerank. Left off, stage 1 would drop those hosts from the path whenever
// any other profile in the fleet declares it. Declaring a path is not passthrough
// (its anthropic_support stays disabled), and a request naming a model is still
// routed by that model. lemonade serves /v1/responses at
// /api/v1/responses and /v1/responses (routes/openai/text/responses.cpp); dmr
// registers it under /v1 and /engines/v1 (pkg/responses/handler.go).
var knownServers = map[string][]string{
	"/v1/chat/completions":      {"llamacpp", "lm-studio", "lmdeploy", "litellm", "ollama", "omlx", "openai-compatible", "sglang", "vllm", "vllm-mlx"},
	"/v1/completions":           {"llamacpp", "lm-studio", "lmdeploy", "litellm", "ollama", "omlx", "openai-compatible", "sglang", "vllm", "vllm-mlx"},
	"/v1/models":                {"llamacpp", "lm-studio", "lmdeploy", "litellm", "ollama", "omlx", "openai-compatible", "sglang", "vllm", "vllm-mlx"},
	"/v1/embeddings":            {"llamacpp", "lm-studio", "litellm", "ollama", "omlx", "openai-compatible", "sglang", "text-embeddings-inference", "vllm", "vllm-mlx"},
	"/v1/responses":             {"docker-model-runner", "lemonade", "llamacpp", "lm-studio", "litellm", "lmdeploy", "ollama", "omlx", "openai-compatible", "sglang", "vllm", "vllm-mlx"},
	"/v1/messages":              {"lemonade", "llamacpp", "litellm", "lm-studio", "lmdeploy", "ollama", "omlx", "openai-compatible", "sglang", "vllm", "vllm-mlx"},
	"/v1/messages/count_tokens": {"llamacpp", "litellm", "lmdeploy", "omlx", "sglang", "vllm", "vllm-mlx"},
	"/v1/rerank":                {"llamacpp", "omlx", "sglang", "vllm", "vllm-mlx"},
}

func TestShippedProfiles_StandardPathDeclarationsMatchKnownServers(t *testing.T) {
	_, byProfile := shippedProfilePaths(t)

	for path, want := range knownServers {
		t.Run(path, func(t *testing.T) {
			var got []string
			for name, paths := range byProfile {
				if slices.Contains(paths, path) {
					got = append(got, name)
				}
			}
			slices.Sort(got)
			want := slices.Clone(want)
			slices.Sort(want)
			if !slices.Equal(got, want) {
				t.Errorf("%s declared by %v, known servers are %v", path, got, want)
			}
		})
	}
}

// A profile that enables Anthropic passthrough on /v1/messages is, by its own
// declaration, a server of that path; the same goes for token counting.
func TestShippedProfiles_AnthropicSupportImpliesDeclaredPaths(t *testing.T) {
	factory, byProfile := shippedProfilePaths(t)

	for name, paths := range byProfile {
		support := factory.GetAnthropicSupport(name)
		if support == nil || !support.Enabled {
			continue
		}
		t.Run(name, func(t *testing.T) {
			// dmr serves Anthropic under /anthropic/v1/messages, outside its /engines routes,
			// so the declared path follows messages_path rather than being fixed
			if !slices.Contains(paths, support.MessagesPath) {
				t.Errorf("%s enables anthropic_support on %s but does not declare the path", name, support.MessagesPath)
			}
			if countPath := support.MessagesPath + "/count_tokens"; support.TokenCount && !slices.Contains(paths, countPath) {
				t.Errorf("%s sets anthropic_support.token_count but does not declare %s", name, countPath)
			}
		})
	}
}

// Translators rewrite every request to one OpenAI path, but backends mount it in
// different places. The path a profile resolves to must be one it declares, and
// profiles with no chat surface (TEI) must resolve to nothing.
func TestPathInspector_ResolveTargetPath_ShippedProfiles(t *testing.T) {
	t.Parallel()
	factory, byProfile := shippedProfilePaths(t)
	inspector := NewPathInspector(factory, createTestLogger())

	const target = "/v1/chat/completions"
	want := map[string]string{
		"docker-model-runner": "/engines/v1/chat/completions",
		"lemonade":            "/api/v1/chat/completions",
		"ollama":              target, // native /api/chat exists, but the compat path is declared
		"vllm":                target,
		"llamacpp":            target,
		"lm-studio":           target,
		"openai-compatible":   target,
	}
	for profile, path := range want {
		got, ok := inspector.ResolveTargetPath(profile, target)
		if !ok || got != path {
			t.Errorf("ResolveTargetPath(%q) = %q, %v; want %q", profile, got, ok, path)
		}
	}

	// anything we resolve must be a path the profile declares
	for profile, declared := range byProfile {
		got, ok := inspector.ResolveTargetPath(profile, target)
		if ok && !slices.Contains(declared, got) {
			t.Errorf("ResolveTargetPath(%q) = %q, not in its declared paths %v", profile, got, declared)
		}
	}

	if got, ok := inspector.ResolveTargetPath("text-embeddings-inference", target); ok {
		t.Errorf("TEI has no chat surface, resolved %q", got)
	}
	// unknown provider types are treated as OpenAI-compatible, as the profile factory does
	if got, ok := inspector.ResolveTargetPath("some-new-engine", target); !ok || got != target {
		t.Errorf("unknown type resolved %q, %v; want %q", got, ok, target)
	}
	if _, ok := inspector.ResolveTargetPath("vllm", ""); ok {
		t.Error("empty target must not resolve")
	}
	// a single segment would suffix-match unrelated paths
	if _, ok := inspector.ResolveTargetPath("docker-model-runner", "/completions"); ok {
		t.Error("single-segment target must only match exactly")
	}
}

func TestPathInspector_TranslationProfiles(t *testing.T) {
	t.Parallel()
	factory, _ := shippedProfilePaths(t)
	inspector := NewPathInspector(factory, createTestLogger())

	got := inspector.TranslationProfiles("/v1/chat/completions")
	for _, name := range []string{"docker-model-runner", "lemonade", "ollama", "vllm", "llamacpp", "lm-studio"} {
		if !slices.Contains(got, name) {
			t.Errorf("TranslationProfiles missing %q: %v", name, got)
		}
	}
	if slices.Contains(got, "text-embeddings-inference") {
		t.Errorf("TEI serves no chat completions: %v", got)
	}
	if len(inspector.TranslationProfiles("")) != 0 {
		t.Error("empty target must yield nothing")
	}
}
