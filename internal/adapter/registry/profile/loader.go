package profile

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/thushan/olla/internal/adapter/filter"
	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/core/ports"
	"gopkg.in/yaml.v3"
)

type ProfileLoader struct {
	filter        ports.Filter
	profiles      map[string]domain.InferenceProfile
	profileFilter *domain.FilterConfig
	profilesDir   string
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

func (l *ProfileLoader) LoadProfiles() error {
	l.mu.Lock()
	defer l.mu.Unlock()

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
			// don't fail everything because of one bad yaml file
			fmt.Printf("failed to load profile %s: %v\n", path, err)
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
		return nil, fmt.Errorf("profile name is required")
	}
	if config.Version == "" {
		config.Version = "1.0"
	}

	// hook for future custom parsers if yaml isn't enough
	if needsCustomParser(config.Name) {
		return l.createCustomProfile(&config)
	}

	return NewConfigurableProfile(&config), nil
}

func needsCustomParser(name string) bool {
	// everything works with yaml config for now
	return false
}

func (l *ProfileLoader) createCustomProfile(config *domain.ProfileConfig) (domain.InferenceProfile, error) {
	// placeholder for when yaml isn't enough
	return NewConfigurableProfile(config), nil
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
	ollamaConfig.API.Paths = []string{
		"/", // health check
		"/api/generate",
		"/api/chat",
		"/api/embeddings",
		"/api/tags", // models
		"/api/show",
		DefaultModelsUri,
		"/v1/chat/completions",
		"/v1/completions",
		"/v1/embeddings",
	}
	ollamaConfig.API.ModelDiscoveryPath = "/api/tags"
	ollamaConfig.API.HealthCheckPath = "/"
	ollamaConfig.Characteristics.Timeout = 5 * time.Minute
	ollamaConfig.Characteristics.MaxConcurrentRequests = 10
	ollamaConfig.Characteristics.DefaultPriority = 100
	ollamaConfig.Characteristics.StreamingSupport = true
	ollamaConfig.Detection.UserAgentPatterns = []string{"ollama/"}
	ollamaConfig.Detection.Headers = []string{"X-ProfileOllama-Version"}
	ollamaConfig.Detection.PathIndicators = []string{"/", "/api/tags"}
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
	lmStudioConfig.API.Paths = []string{
		DefaultModelsUri, // both health check and models
		"/v1/chat/completions",
		"/v1/completions",
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
	lmStudioConfig.Request.ResponseFormat = "lmstudio"
	lmStudioConfig.Request.ModelFieldPaths = []string{DefaultModelKey}
	lmStudioConfig.Request.ParsingRules.ChatCompletionsPath = "/v1/chat/completions"
	lmStudioConfig.Request.ParsingRules.CompletionsPath = "/v1/completions"
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

	profiles[domain.ProfileLmStudio] = NewConfigurableProfile(lmStudioConfig)

	// OpenAI-compatible built-in profile
	openAIConfig := &domain.ProfileConfig{
		Name:        domain.ProfileOpenAICompatible,
		Version:     "1.0",
		DisplayName: "OpenAI Compatible",
		Description: "Generic OpenAI-compatible API",
	}
	openAIConfig.Routing.Prefixes = []string{"openai", "openai-compatible"}
	openAIConfig.API.OpenAICompatible = true
	openAIConfig.API.Paths = []string{
		DefaultModelsUri,
		"/v1/chat/completions",
		"/v1/completions",
		"/v1/embeddings",
	}
	openAIConfig.API.ModelDiscoveryPath = DefaultModelsUri
	openAIConfig.API.HealthCheckPath = DefaultModelsUri
	openAIConfig.Characteristics.Timeout = 2 * time.Minute
	openAIConfig.Characteristics.MaxConcurrentRequests = 20
	openAIConfig.Characteristics.DefaultPriority = 50
	openAIConfig.Characteristics.StreamingSupport = true
	openAIConfig.Detection.PathIndicators = []string{DefaultModelsUri}
	openAIConfig.Request.ResponseFormat = "openai"
	openAIConfig.Request.ModelFieldPaths = []string{DefaultModelKey}
	openAIConfig.Request.ParsingRules.ChatCompletionsPath = "/v1/chat/completions"
	openAIConfig.Request.ParsingRules.CompletionsPath = "/v1/completions"
	openAIConfig.Request.ParsingRules.ModelFieldName = DefaultModelKey
	openAIConfig.Request.ParsingRules.SupportsStreaming = true
	openAIConfig.PathIndices.Health = 0
	openAIConfig.PathIndices.Models = 0
	openAIConfig.PathIndices.ChatCompletions = 1
	openAIConfig.PathIndices.Completions = 2
	openAIConfig.PathIndices.Embeddings = 3

	profiles[domain.ProfileOpenAICompatible] = NewConfigurableProfile(openAIConfig)
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
