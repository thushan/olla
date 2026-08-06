package profile

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/thushan/olla/internal/adapter/filter"
	"github.com/thushan/olla/internal/core/constants"
	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/core/ports"
	"gopkg.in/yaml.v3"
)

type ProfileLoader struct {
	filter        ports.Filter
	profiles      map[string]domain.InferenceProfile
	profileFilter *domain.FilterConfig
	profilesDir   string
	loadWarnings  []string
	mu            sync.RWMutex
}

func NewProfileLoader(profilesDir string) *ProfileLoader {
	return &ProfileLoader{
		profilesDir: profilesDir,
		profiles:    make(map[string]domain.InferenceProfile),
		filter:      filter.NewGlobFilter(),
	}
}

// NewProfileLoaderWithFilter creates a new ProfileLoader with a custom filter
func NewProfileLoaderWithFilter(profilesDir string, profileFilter *domain.FilterConfig, customFilter ports.Filter) *ProfileLoader {
	filterToUse := customFilter
	if filterToUse == nil {
		filterToUse = filter.NewGlobFilter()
	}

	return &ProfileLoader{
		profilesDir:   profilesDir,
		profiles:      make(map[string]domain.InferenceProfile),
		profileFilter: profileFilter,
		filter:        filterToUse,
	}
}

const DefaultModelKey = "model"
const DefaultModelsUri = "/v1/models"
const defaultNameFormat = "{{.Name}}"

func (l *ProfileLoader) LoadProfiles() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	// reset per-call so a reload doesn't accumulate warnings from a
	// previously-broken file that's since been fixed or removed
	l.loadWarnings = nil

	allProfiles := make(map[string]domain.InferenceProfile)

	// built-ins ensure it works out of the box, even without config files
	l.loadBuiltInProfilesInto(allProfiles)

	if _, err := os.Stat(l.profilesDir); os.IsNotExist(err) {
		// no config dir is fine - built-ins cover the common cases
		// Apply filtering before returning
		return l.applyProfileFilter(allProfiles)
	}

	// yaml files in config dir override built-ins
	err := filepath.WalkDir(l.profilesDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() || !strings.HasSuffix(path, ".yaml") {
			return nil
		}

		profile, err := l.loadProfile(path)
		if err != nil {
			// don't fail everything because of one bad yaml file - but don't
			// go silent either (issue #204's bug class): record it so
			// --validate-config and any caller with a real logger can
			// surface it, and warn now via slog so it shows up in ordinary
			// startup logs too.
			warning := fmt.Sprintf("%s: %v", path, err)
			l.loadWarnings = append(l.loadWarnings, warning)
			slog.Warn("profile failed to load, skipping", "path", path, "error", err)
			return nil
		}

		allProfiles[profile.GetName()] = profile
		return nil
	})

	if err != nil {
		return err
	}

	// Apply filtering to all loaded profiles
	return l.applyProfileFilter(allProfiles)
}

// LoadWarnings returns any profile files that were found but failed to load
// during the most recent LoadProfiles call - e.g. malformed YAML or a
// missing required field. Empty when every discovered profile loaded
// cleanly. Exposed so callers like --validate-config can surface these
// without depending on log capture.
func (l *ProfileLoader) LoadWarnings() []string {
	l.mu.RLock()
	defer l.mu.RUnlock()

	return append([]string(nil), l.loadWarnings...)
}

// applyProfileFilter applies the configured filter to the profiles
func (l *ProfileLoader) applyProfileFilter(allProfiles map[string]domain.InferenceProfile) error {
	// If no filter is configured, use all profiles
	if l.profileFilter == nil || l.profileFilter.IsEmpty() {
		l.profiles = allProfiles
		return nil
	}

	// Apply the filter
	ctx := context.Background()
	filteredProfiles, err := l.filter.ApplyToMap(ctx, l.profileFilter, convertProfilesToMap(allProfiles))
	if err != nil {
		return fmt.Errorf("failed to apply profile filter: %w", err)
	}

	// Convert back to typed map
	l.profiles = make(map[string]domain.InferenceProfile)
	for name, profile := range filteredProfiles {
		if p, ok := profile.(domain.InferenceProfile); ok {
			l.profiles[name] = p
		}
	}

	return nil
}

// convertProfilesToMap converts typed profile map to generic map for filtering
func convertProfilesToMap(profiles map[string]domain.InferenceProfile) map[string]interface{} {
	result := make(map[string]interface{}, len(profiles))
	for name, profile := range profiles {
		result[name] = profile
	}
	return result
}

func (l *ProfileLoader) loadProfile(path string) (domain.InferenceProfile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read profile file: %w", err)
	}

	var config domain.ProfileConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse profile YAML: %w", err)
	}

	if config.Name == "" {
		return nil, errors.New("profile name is required")
	}
	if config.Version == "" {
		config.Version = "1.0"
	}

	return NewConfigurableProfile(&config), nil
}

// loadBuiltInProfilesInto loads built-in profiles into the provided map
func (l *ProfileLoader) loadBuiltInProfilesInto(profiles map[string]domain.InferenceProfile) {
	// hardcoded defaults for the common platforms everyone uses

	ollamaConfig := &domain.ProfileConfig{
		Name:        domain.ProfileOllama,
		Version:     "1.0",
		DisplayName: "Ollama",
		Description: "Local Ollama instance for running GGUF models",
	}
	ollamaConfig.Routing.Prefixes = []string{"ollama"}
	ollamaConfig.API.OpenAICompatible = true
	ollamaConfig.API.AnthropicSupport = &domain.AnthropicSupportConfig{
		Enabled:      true,
		MessagesPath: "/v1/messages",
		MinVersion:   "0.14.0",
		Limitations:  []string{"token_counting_404"},
	}
	ollamaConfig.API.Paths = []string{
		"/", // health check
		"/api/generate",
		"/api/chat",
		"/api/embeddings",
		"/api/tags", // models
		"/api/show",
		DefaultModelsUri,
		constants.PathV1ChatCompletions,
		constants.PathV1Completions,
		"/v1/embeddings",
	}
	ollamaConfig.API.ModelDiscoveryPath = "/api/tags"
	ollamaConfig.API.HealthCheckPath = "/"
	ollamaConfig.Characteristics.Timeout = 5 * time.Minute
	ollamaConfig.Characteristics.MaxConcurrentRequests = 10
	ollamaConfig.Characteristics.DefaultPriority = 100
	ollamaConfig.Characteristics.StreamingSupport = true
	ollamaConfig.Characteristics.Auth = domain.AuthHint{Types: []string{"bearer"}}
	ollamaConfig.Detection.UserAgentPatterns = []string{"ollama/"}
	ollamaConfig.Detection.Headers = []string{"X-ProfileOllama-Version"}
	ollamaConfig.Detection.PathIndicators = []string{"/", "/api/tags"}
	ollamaConfig.Detection.DefaultPorts = []int{11434}
	ollamaConfig.Models.NameFormat = defaultNameFormat
	ollamaConfig.Metrics.Extraction = domain.MetricsExtractionConfig{
		Enabled: true,
		Source:  "response_body",
		Format:  "json",
		Paths: map[string]string{ //nolint:gosec // JSONPath extraction map keys, not credentials
			"model":              "$.model",
			"is_complete":        "$.done",
			"finish_reason":      "$.finish_reason",
			"input_tokens":       "$.prompt_eval_count",
			"output_tokens":      "$.eval_count",
			"total_duration_ns":  "$.total_duration",
			"load_duration_ns":   "$.load_duration",
			"prompt_duration_ns": "$.prompt_eval_duration",
			"eval_duration_ns":   "$.eval_duration",
		},
		Calculations: map[string]string{ //nolint:gosec // derived-metric expressions, not credentials
			"tokens_per_second": "eval_duration_ns > 0 ? (output_tokens * 1000000000.0) / eval_duration_ns : 0",
			"ttft_ms":           "prompt_duration_ns / 1000000",
			"total_ms":          "total_duration_ns / 1000000",
			"model_load_ms":     "load_duration_ns / 1000000",
		},
	}
	ollamaConfig.Request.ResponseFormat = "ollama"
	ollamaConfig.Request.ModelFieldPaths = []string{DefaultModelKey}
	ollamaConfig.Request.ParsingRules.ChatCompletionsPath = "/api/chat"
	ollamaConfig.Request.ParsingRules.CompletionsPath = "/api/generate"
	ollamaConfig.Request.ParsingRules.GeneratePath = "/api/generate"
	ollamaConfig.Request.ParsingRules.ModelFieldName = DefaultModelKey
	ollamaConfig.Request.ParsingRules.SupportsStreaming = true
	ollamaConfig.PathIndices.Health = 0
	ollamaConfig.PathIndices.Models = 4
	ollamaConfig.PathIndices.Completions = 1
	ollamaConfig.PathIndices.ChatCompletions = 2
	ollamaConfig.PathIndices.Embeddings = 3

	// Resource patterns for built-in Ollama profile
	ollamaConfig.Resources.ModelSizes = []domain.ModelSizePattern{
		{Patterns: []string{"70b", "72b"}, MinMemoryGB: 40, RecommendedMemoryGB: 48, MinGPUMemoryGB: 40, EstimatedLoadTimeMS: 300000},
		{Patterns: []string{"65b"}, MinMemoryGB: 35, RecommendedMemoryGB: 40, MinGPUMemoryGB: 35, EstimatedLoadTimeMS: 240000},
		{Patterns: []string{"34b", "33b", "30b"}, MinMemoryGB: 20, RecommendedMemoryGB: 24, MinGPUMemoryGB: 20, EstimatedLoadTimeMS: 120000},
		{Patterns: []string{"13b", "14b"}, MinMemoryGB: 10, RecommendedMemoryGB: 16, MinGPUMemoryGB: 10, EstimatedLoadTimeMS: 60000},
		{Patterns: []string{"7b", "8b"}, MinMemoryGB: 6, RecommendedMemoryGB: 8, MinGPUMemoryGB: 6, EstimatedLoadTimeMS: 30000},
		{Patterns: []string{"3b"}, MinMemoryGB: 3, RecommendedMemoryGB: 4, MinGPUMemoryGB: 3, EstimatedLoadTimeMS: 15000},
		{Patterns: []string{"1b", "1.5b"}, MinMemoryGB: 2, RecommendedMemoryGB: 3, MinGPUMemoryGB: 2, EstimatedLoadTimeMS: 10000},
	}
	ollamaConfig.Resources.Quantization.Multipliers = map[string]float64{
		"q4": 0.5,
		"q5": 0.625,
		"q6": 0.75,
		"q8": 0.875,
	}
	ollamaConfig.Resources.Defaults = domain.ResourceRequirements{
		MinMemoryGB: 4, RecommendedMemoryGB: 8, MinGPUMemoryGB: 4, RequiresGPU: false, EstimatedLoadTimeMS: 5000,
	}

	// Model capability patterns
	ollamaConfig.Models.CapabilityPatterns = map[string][]string{
		"vision":     {"*llava*", "*vision*", "*bakllava*"},
		"embeddings": {"*embed*", "nomic-embed-text", "mxbai-embed-large"},
		"code":       {"*code*", "codellama*", "deepseek-coder*", "qwen*coder*"},
	}

	// Context window patterns
	ollamaConfig.Models.ContextPatterns = []domain.ContextPattern{
		{Pattern: "*-32k*", Context: 32768},
		{Pattern: "*-16k*", Context: 16384},
		{Pattern: "*-8k*", Context: 8192},
		{Pattern: "*:32k*", Context: 32768},
		{Pattern: "*:16k*", Context: 16384},
		{Pattern: "*:8k*", Context: 8192},
		{Pattern: "llama3*", Context: 8192},
		{Pattern: "llama-3*", Context: 8192},
	}

	// Concurrency limits based on model size
	ollamaConfig.Resources.ConcurrencyLimits = []domain.ConcurrencyLimitPattern{
		{MinMemoryGB: 30, MaxConcurrent: 1},
		{MinMemoryGB: 15, MaxConcurrent: 2},
		{MinMemoryGB: 8, MaxConcurrent: 4},
		{MinMemoryGB: 0, MaxConcurrent: 8},
	}

	// Timeout scaling
	ollamaConfig.Resources.TimeoutScaling = domain.TimeoutScaling{
		BaseTimeoutSeconds: 30,
		LoadTimeBuffer:     true,
	}

	profiles[domain.ProfileOllama] = NewConfigurableProfile(ollamaConfig)

	// LM Studio built-in profile
	lmStudioConfig := &domain.ProfileConfig{
		Name:        domain.ProfileLmStudio,
		Version:     "1.0",
		DisplayName: "LM Studio",
		Description: "LM Studio local inference server",
	}
	lmStudioConfig.Routing.Prefixes = []string{"lmstudio", "lm-studio", "lm_studio"}
	lmStudioConfig.API.OpenAICompatible = true
	lmStudioConfig.API.AnthropicSupport = &domain.AnthropicSupportConfig{
		Enabled:      true,
		MessagesPath: "/v1/messages",
		MinVersion:   "0.4.1",
	}
	lmStudioConfig.API.Paths = []string{
		DefaultModelsUri, // both health check and models
		constants.PathV1ChatCompletions,
		constants.PathV1Completions,
		"/v1/embeddings",
		"/api/v0/models",
	}
	lmStudioConfig.API.ModelDiscoveryPath = "/api/v0/models"
	lmStudioConfig.API.HealthCheckPath = DefaultModelsUri
	lmStudioConfig.Characteristics.Timeout = 3 * time.Minute
	lmStudioConfig.Characteristics.MaxConcurrentRequests = 1
	lmStudioConfig.Characteristics.DefaultPriority = 90
	lmStudioConfig.Characteristics.StreamingSupport = true
	lmStudioConfig.Detection.PathIndicators = []string{DefaultModelsUri, "/api/v0/models"}
	lmStudioConfig.Detection.DefaultPorts = []int{1234}
	lmStudioConfig.Models.NameFormat = defaultNameFormat
	lmStudioConfig.Models.CapabilityPatterns = map[string][]string{
		"chat": {"*"},
	}
	lmStudioConfig.Models.ContextPatterns = []domain.ContextPattern{
		{Pattern: "*-32k*", Context: 32768},
		{Pattern: "*-16k*", Context: 16384},
		{Pattern: "*-8k*", Context: 8192},
		{Pattern: "*:32k*", Context: 32768},
		{Pattern: "*:16k*", Context: 16384},
		{Pattern: "*:8k*", Context: 8192},
		{Pattern: "llama3*", Context: 8192},
		{Pattern: "llama-3*", Context: 8192},
	}
	lmStudioConfig.Metrics.Extraction = domain.MetricsExtractionConfig{
		Enabled: true,
		Source:  "response_body",
		Format:  "json",
		Paths: map[string]string{ //nolint:gosec // JSONPath extraction map keys, not credentials
			"model":         "$.model",
			"finish_reason": "$.choices[0].finish_reason",
			"input_tokens":  "$.usage.prompt_tokens",
			"output_tokens": "$.usage.completion_tokens",
			"total_tokens":  "$.usage.total_tokens",
		},
		Calculations: map[string]string{
			"is_complete": "len(finish_reason) > 0",
		},
	}
	lmStudioConfig.Request.ResponseFormat = "lmstudio"
	lmStudioConfig.Request.ModelFieldPaths = []string{DefaultModelKey}
	lmStudioConfig.Request.ParsingRules.ChatCompletionsPath = constants.PathV1ChatCompletions
	lmStudioConfig.Request.ParsingRules.CompletionsPath = constants.PathV1Completions
	lmStudioConfig.Request.ParsingRules.ModelFieldName = DefaultModelKey
	lmStudioConfig.Request.ParsingRules.SupportsStreaming = true
	lmStudioConfig.PathIndices.Health = 0
	lmStudioConfig.PathIndices.Models = 0
	lmStudioConfig.PathIndices.ChatCompletions = 1
	lmStudioConfig.PathIndices.Completions = 2
	lmStudioConfig.PathIndices.Embeddings = 3

	// Resource patterns for built-in LM Studio profile
	lmStudioConfig.Resources.ModelSizes = []domain.ModelSizePattern{
		{Patterns: []string{"70b", "72b"}, MinMemoryGB: 42, RecommendedMemoryGB: 52.5, MinGPUMemoryGB: 42, EstimatedLoadTimeMS: 1000},
		{Patterns: []string{"65b"}, MinMemoryGB: 39, RecommendedMemoryGB: 48.75, MinGPUMemoryGB: 39, EstimatedLoadTimeMS: 1000},
		{Patterns: []string{"34b", "33b"}, MinMemoryGB: 20.4, RecommendedMemoryGB: 25.5, MinGPUMemoryGB: 20.4, EstimatedLoadTimeMS: 1000},
		{Patterns: []string{"13b", "14b"}, MinMemoryGB: 8.4, RecommendedMemoryGB: 10.5, MinGPUMemoryGB: 8.4, EstimatedLoadTimeMS: 1000},
		{Patterns: []string{"7b", "8b"}, MinMemoryGB: 4.8, RecommendedMemoryGB: 6, MinGPUMemoryGB: 4.8, EstimatedLoadTimeMS: 1000},
		{Patterns: []string{"3b"}, MinMemoryGB: 1.8, RecommendedMemoryGB: 2.25, MinGPUMemoryGB: 1.8, EstimatedLoadTimeMS: 1000},
	}
	lmStudioConfig.Resources.Defaults = domain.ResourceRequirements{
		MinMemoryGB: 4.2, RecommendedMemoryGB: 5.25, MinGPUMemoryGB: 4.2, RequiresGPU: false, EstimatedLoadTimeMS: 1000,
	}
	lmStudioConfig.Resources.ConcurrencyLimits = []domain.ConcurrencyLimitPattern{
		{MinMemoryGB: 0, MaxConcurrent: 1},
	}
	lmStudioConfig.Resources.TimeoutScaling = domain.TimeoutScaling{
		BaseTimeoutSeconds: 180,
		LoadTimeBuffer:     false,
	}

	profiles[domain.ProfileLmStudio] = NewConfigurableProfile(lmStudioConfig)

	// OpenAI-compatible built-in profile
	openAIConfig := &domain.ProfileConfig{
		Name:        domain.ProfileOpenAICompatible,
		Version:     "1.0",
		DisplayName: "OpenAI Compatible",
		Description: "OpenAI-compatible API",
	}
	openAIConfig.Routing.Prefixes = []string{"openai", "openai-compatible"}
	openAIConfig.API.OpenAICompatible = true
	openAIConfig.API.AnthropicSupport = &domain.AnthropicSupportConfig{
		Enabled:      false,
		MessagesPath: "/v1/messages",
	}
	openAIConfig.API.Paths = []string{
		DefaultModelsUri,
		constants.PathV1ChatCompletions,
		constants.PathV1Completions,
		"/v1/embeddings",
	}
	openAIConfig.API.ModelDiscoveryPath = DefaultModelsUri
	openAIConfig.API.HealthCheckPath = DefaultModelsUri
	openAIConfig.Characteristics.Timeout = 2 * time.Minute
	openAIConfig.Characteristics.MaxConcurrentRequests = 20
	openAIConfig.Characteristics.DefaultPriority = 50
	openAIConfig.Characteristics.StreamingSupport = true
	openAIConfig.Characteristics.Auth = domain.AuthHint{Types: []string{"bearer", "api_key"}}
	openAIConfig.Detection.PathIndicators = []string{DefaultModelsUri}
	openAIConfig.Models.NameFormat = defaultNameFormat
	openAIConfig.Models.CapabilityPatterns = map[string][]string{
		"chat":       {"gpt-*", "*turbo*"},
		"embeddings": {"*embedding*", "text-embedding-*"},
		"vision":     {"*vision*", "gpt-4-turbo*"},
	}
	openAIConfig.Models.ContextPatterns = []domain.ContextPattern{
		{Pattern: "gpt-5-thinking*", Context: 400000},
		{Pattern: "gpt-5-main*", Context: 128000},
		{Pattern: "gpt-5*", Context: 32000},
		{Pattern: "gpt-4.1*", Context: 1000000},
		{Pattern: "gpt-4o*", Context: 128000},
		{Pattern: "gpt-4-turbo*", Context: 128000},
		{Pattern: "gpt-4*", Context: 8192},
		{Pattern: "gpt-3.5-turbo-16k*", Context: 16384},
		{Pattern: "gpt-3.5-turbo*", Context: 4096},
	}
	openAIConfig.Metrics.Extraction = domain.MetricsExtractionConfig{
		Enabled: true,
		Source:  "response_body",
		Format:  "json",
		Paths: map[string]string{ //nolint:gosec // JSONPath extraction map keys, not credentials
			"model":         "$.model",
			"finish_reason": "$.choices[0].finish_reason",
			"input_tokens":  "$.usage.prompt_tokens",
			"output_tokens": "$.usage.completion_tokens",
			"total_tokens":  "$.usage.total_tokens",
			"ttft_ms":       "$.metrics.time_to_first_token",
			"total_ms":      "$.metrics.total_time",
		},
		Calculations: map[string]string{ //nolint:gosec // derived-metric expressions, not credentials
			"is_complete":       "len(finish_reason) > 0",
			"tokens_per_second": "total_ms > 0 ? (output_tokens * 1000.0) / total_ms : 0",
		},
	}
	openAIConfig.Request.ResponseFormat = "openai"
	openAIConfig.Request.ModelFieldPaths = []string{DefaultModelKey}
	openAIConfig.Request.ParsingRules.ChatCompletionsPath = constants.PathV1ChatCompletions
	openAIConfig.Request.ParsingRules.CompletionsPath = constants.PathV1Completions
	openAIConfig.Request.ParsingRules.ModelFieldName = DefaultModelKey
	openAIConfig.Request.ParsingRules.SupportsStreaming = true
	openAIConfig.PathIndices.Health = 0
	openAIConfig.PathIndices.Models = 0
	openAIConfig.PathIndices.ChatCompletions = 1
	openAIConfig.PathIndices.Completions = 2
	openAIConfig.PathIndices.Embeddings = 3

	// Cloud-based models have no local resource requirements; Resources.Defaults
	// stays zero-valued to match.
	openAIConfig.Resources.ConcurrencyLimits = []domain.ConcurrencyLimitPattern{
		{MinMemoryGB: 0, MaxConcurrent: 20},
	}
	openAIConfig.Resources.TimeoutScaling = domain.TimeoutScaling{
		BaseTimeoutSeconds: 120,
		LoadTimeBuffer:     false,
	}

	profiles[domain.ProfileOpenAICompatible] = NewConfigurableProfile(openAIConfig)

	// Load llama.cpp built-in profile
	l.loadLlamaCppBuiltIn(profiles)
}

// Built-in llama.cpp profile added for Phase 5 integration
func (l *ProfileLoader) loadLlamaCppBuiltIn(profiles map[string]domain.InferenceProfile) {
	llamaCppConfig := &domain.ProfileConfig{
		Name:        domain.ProfileLlamaCpp,
		Version:     "1.0",
		DisplayName: "llama.cpp",
		Description: "llama.cpp high-performance C++ inference server for GGUF models",
	}
	llamaCppConfig.Routing.Prefixes = []string{"llamacpp"}
	llamaCppConfig.API.OpenAICompatible = true
	llamaCppConfig.API.AnthropicSupport = &domain.AnthropicSupportConfig{
		Enabled:      true,
		MessagesPath: "/v1/messages",
		MinVersion:   "b4847",
		TokenCount:   true,
	}
	llamaCppConfig.API.Paths = []string{
		DefaultModelsUri, // model management
		"/completion",
		constants.PathV1Completions,
		constants.PathV1ChatCompletions,
		"/embedding",
		"/v1/embeddings",
		"/tokenize",
		"/detokenize",
		"/infill",
	}
	llamaCppConfig.API.ModelDiscoveryPath = DefaultModelsUri
	llamaCppConfig.API.HealthCheckPath = "/health"
	llamaCppConfig.Characteristics.Timeout = 5 * time.Minute
	llamaCppConfig.Characteristics.MaxConcurrentRequests = 4
	llamaCppConfig.Characteristics.DefaultPriority = 95 // High priority: native GGUF inference
	llamaCppConfig.Characteristics.StreamingSupport = true
	llamaCppConfig.Characteristics.Auth = domain.AuthHint{Types: []string{"bearer"}}
	llamaCppConfig.Detection.PathIndicators = []string{DefaultModelsUri, "/health", "/slots", "/props"}
	llamaCppConfig.Detection.DefaultPorts = []int{8080, 8001}
	llamaCppConfig.Models.NameFormat = defaultNameFormat
	llamaCppConfig.Models.CapabilityPatterns = map[string][]string{
		"chat":       {"*-chat-*", "*-instruct*", "*-Chat*", "*-Instruct*", "*chat*", "*instruct*"},
		"embeddings": {"*embed*", "*-embed-*", "*embedding*"},
		"vision":     {"*vision*", "*llava*", "*-vision*", "*bakllava*", "*minicpm*"},
		"code":       {"*code*", "*-code-*", "*coder*", "*deepseek-coder*", "*codellama*", "*starcoder*"},
	}
	llamaCppConfig.Models.ContextPatterns = []domain.ContextPattern{
		{Pattern: "*-32k*", Context: 32768},
		{Pattern: "*-16k*", Context: 16384},
		{Pattern: "*-8k*", Context: 8192},
		{Pattern: "*:32k*", Context: 32768},
		{Pattern: "*:16k*", Context: 16384},
		{Pattern: "*:8k*", Context: 8192},
		{Pattern: "*llama-3.1*", Context: 131072},
		{Pattern: "*llama-3.2*", Context: 131072},
		{Pattern: "*llama-3*", Context: 8192},
		{Pattern: "*mistral*", Context: 32768},
		{Pattern: "*mixtral*", Context: 32768},
		{Pattern: "*phi*", Context: 4096},
		{Pattern: "*qwen*", Context: 32768},
		{Pattern: "*gemma*", Context: 8192},
	}
	llamaCppConfig.Metrics.Extraction = domain.MetricsExtractionConfig{
		Enabled: true,
		Source:  "response_body",
		Format:  "json",
		Paths: map[string]string{ //nolint:gosec // JSONPath extraction map keys, not credentials
			"model":                    "$.model",
			"finish_reason":            "$.choices[0].finish_reason",
			"input_tokens":             "$.usage.prompt_tokens",
			"output_tokens":            "$.usage.completion_tokens",
			"total_tokens":             "$.usage.total_tokens",
			"processing_time_ms":       "$.timings.predicted_ms",
			"prompt_processing_ms":     "$.timings.prompt_ms",
			"total_time_ms":            "$.timings.total_ms",
			"predicted_per_second":     "$.timings.predicted_per_second",
			"prompt_tokens_per_second": "$.timings.prompt_per_second",
			"predicted_n":              "$.timings.predicted_n",
			"predicted_ms":             "$.timings.predicted_ms",
		},
		Calculations: map[string]string{
			"is_complete":       "len(finish_reason) > 0",
			"tokens_per_second": "predicted_per_second > 0 ? predicted_per_second : (predicted_ms > 0 ? (predicted_n * 1000.0) / predicted_ms : 0)",
			"ttft_ms":           "prompt_processing_ms",
		},
	}
	llamaCppConfig.Request.ResponseFormat = constants.ProviderTypeLlamaCpp
	llamaCppConfig.Request.ModelFieldPaths = []string{DefaultModelKey}
	llamaCppConfig.Request.ParsingRules.ChatCompletionsPath = constants.PathV1ChatCompletions
	llamaCppConfig.Request.ParsingRules.CompletionsPath = constants.PathV1Completions
	llamaCppConfig.Request.ParsingRules.ModelFieldName = DefaultModelKey
	llamaCppConfig.Request.ParsingRules.SupportsStreaming = true
	llamaCppConfig.PathIndices.Health = 0
	llamaCppConfig.PathIndices.Models = 4
	llamaCppConfig.PathIndices.ChatCompletions = 7
	llamaCppConfig.PathIndices.Completions = 6
	llamaCppConfig.PathIndices.Embeddings = 9

	// Resource patterns for built-in llama.cpp profile
	llamaCppConfig.Resources.ModelSizes = []domain.ModelSizePattern{
		{Patterns: []string{"*70b*", "*72b*"}, MinMemoryGB: 40, RecommendedMemoryGB: 48, MinGPUMemoryGB: 40, EstimatedLoadTimeMS: 300000},
		{Patterns: []string{"*34b*", "*33b*", "*30b*"}, MinMemoryGB: 20, RecommendedMemoryGB: 24, MinGPUMemoryGB: 20, EstimatedLoadTimeMS: 120000},
		{Patterns: []string{"*13b*", "*14b*"}, MinMemoryGB: 10, RecommendedMemoryGB: 16, MinGPUMemoryGB: 10, EstimatedLoadTimeMS: 60000},
		{Patterns: []string{"*7b*", "*8b*"}, MinMemoryGB: 6, RecommendedMemoryGB: 8, MinGPUMemoryGB: 6, EstimatedLoadTimeMS: 30000},
		{Patterns: []string{"*3b*"}, MinMemoryGB: 3, RecommendedMemoryGB: 4, MinGPUMemoryGB: 3, EstimatedLoadTimeMS: 15000},
		{Patterns: []string{"*1b*", "*1.5b*"}, MinMemoryGB: 2, RecommendedMemoryGB: 3, MinGPUMemoryGB: 2, EstimatedLoadTimeMS: 10000},
	}
	llamaCppConfig.Resources.Quantization.Multipliers = map[string]float64{
		"q2":  0.35,
		"q3":  0.45,
		"q4":  0.50,
		"q5":  0.625,
		"q6":  0.75,
		"q8":  0.875,
		"f16": 1.0,
		"f32": 2.0,
	}
	llamaCppConfig.Resources.Defaults = domain.ResourceRequirements{
		MinMemoryGB: 4, RecommendedMemoryGB: 8, MinGPUMemoryGB: 4, RequiresGPU: false, EstimatedLoadTimeMS: 5000,
	}
	llamaCppConfig.Resources.ConcurrencyLimits = []domain.ConcurrencyLimitPattern{
		{MinMemoryGB: 30, MaxConcurrent: 1},
		{MinMemoryGB: 15, MaxConcurrent: 2},
		{MinMemoryGB: 8, MaxConcurrent: 4},
		{MinMemoryGB: 0, MaxConcurrent: 8},
	}
	llamaCppConfig.Resources.TimeoutScaling = domain.TimeoutScaling{
		BaseTimeoutSeconds: 30,
		LoadTimeBuffer:     true,
	}

	profiles[domain.ProfileLlamaCpp] = NewConfigurableProfile(llamaCppConfig)
}

// GetProfile returns a profile by name
func (l *ProfileLoader) GetProfile(name string) (domain.InferenceProfile, bool) {
	l.mu.RLock()
	defer l.mu.RUnlock()

	profile, ok := l.profiles[name]
	return profile, ok
}

// GetAllProfiles returns all loaded profiles
func (l *ProfileLoader) GetAllProfiles() map[string]domain.InferenceProfile {
	l.mu.RLock()
	defer l.mu.RUnlock()

	// return a copy to prevent external modifications
	profiles := make(map[string]domain.InferenceProfile, len(l.profiles))
	for k, v := range l.profiles {
		profiles[k] = v
	}
	return profiles
}

// SetFilter sets the profile filter configuration
func (l *ProfileLoader) SetFilter(filter *domain.FilterConfig) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.profileFilter = filter
}

// SetFilterAdapter sets the filter implementation
func (l *ProfileLoader) SetFilterAdapter(filterAdapter ports.Filter) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.filter = filterAdapter
}
