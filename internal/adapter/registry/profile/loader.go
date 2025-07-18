package profile

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/thushan/olla/internal/core/domain"
	"gopkg.in/yaml.v3"
)

type ProfileLoader struct {
	profiles    map[string]domain.InferenceProfile
	profilesDir string
	mu          sync.RWMutex
}

func NewProfileLoader(profilesDir string) *ProfileLoader {
	return &ProfileLoader{
		profilesDir: profilesDir,
		profiles:    make(map[string]domain.InferenceProfile),
	}
}

const DefaultModelKey = "model"
const DefaultModelsUri = "/v1/models"

func (l *ProfileLoader) LoadProfiles() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.profiles = make(map[string]domain.InferenceProfile)

	// built-ins ensure it works out of the box, even without config files
	l.loadBuiltInProfiles()

	if _, err := os.Stat(l.profilesDir); os.IsNotExist(err) {
		// no config dir is fine - built-ins cover the common cases
		return nil
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

		l.profiles[profile.GetName()] = profile
		return nil
	})

	return err
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

// loadBuiltInProfiles ensures olla works out of the box without config files
func (l *ProfileLoader) loadBuiltInProfiles() {
	// hardcoded defaults for the common platforms everyone uses

	ollamaConfig := &domain.ProfileConfig{
		Name:        domain.ProfileOllama,
		Version:     "1.0",
		DisplayName: "Ollama",
		Description: "Local Ollama instance for running GGUF models",
	}
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

	l.profiles[domain.ProfileOllama] = NewConfigurableProfile(ollamaConfig)

	// LM Studio built-in profile
	lmStudioConfig := &domain.ProfileConfig{
		Name:        domain.ProfileLmStudio,
		Version:     "1.0",
		DisplayName: "LM Studio",
		Description: "LM Studio local inference server",
	}
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

	l.profiles[domain.ProfileLmStudio] = NewConfigurableProfile(lmStudioConfig)

	// OpenAI-compatible built-in profile
	openAIConfig := &domain.ProfileConfig{
		Name:        domain.ProfileOpenAICompatible,
		Version:     "1.0",
		DisplayName: "OpenAI Compatible",
		Description: "Generic OpenAI-compatible API",
	}
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

	l.profiles[domain.ProfileOpenAICompatible] = NewConfigurableProfile(openAIConfig)
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
