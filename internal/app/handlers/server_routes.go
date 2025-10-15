package handlers

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/thushan/olla/internal/adapter/translator"
	"github.com/thushan/olla/internal/core/constants"
	"github.com/thushan/olla/internal/core/domain"
)

// staticRoute captures the minimal routing info needed when profile loading is unavailable
type staticRoute struct {
	path        string
	handler     http.HandlerFunc
	description string
	method      string
	isProxy     bool
}

// staticProvider bundles routes with their URL prefixes for test scenarios
type staticProvider struct {
	prefixes []string
	routes   []staticRoute
}

// registerRoutes sets up the complete HTTP routing table
func (a *Application) registerRoutes() {
	// Internal health and monitoring endpoints come first - they're critical
	// for operations and shouldn't depend on any provider configuration
	a.routeRegistry.RegisterWithMethod(constants.DefaultHealthCheckEndpoint, a.healthHandler, "Health check endpoint", "GET")
	a.routeRegistry.RegisterWithMethod("/internal/status", a.statusHandler, "Endpoint status", "GET")
	a.routeRegistry.RegisterWithMethod("/internal/status/endpoints", a.endpointsStatusHandler, "Endpoints status", "GET")
	a.routeRegistry.RegisterWithMethod("/internal/status/models", a.modelsStatusHandler, "Models status", "GET")
	a.routeRegistry.RegisterWithMethod("/internal/stats/models", a.modelStatsHandler, "Model statistics", "GET")
	a.routeRegistry.RegisterWithMethod("/internal/process", a.processStatsHandler, "Process status", "GET")
	a.routeRegistry.RegisterWithMethod("/version", a.versionHandler, "Olla version information", "GET")

	// Unified model views aggregate across all providers
	a.routeRegistry.RegisterWithMethod("/olla/models", a.unifiedModelsHandler, "Unified models listing with filtering", "GET")
	a.routeRegistry.RegisterWithMethod("/olla/models/", a.unifiedModelByAliasHandler, "Get unified model by ID or alias", "GET")

	// Legacy proxy paths for backward compatibility
	a.routeRegistry.RegisterProxyRoute("/olla/proxy/", a.proxyHandler, "Olla API proxy endpoint (sherpa)", "POST")
	a.routeRegistry.RegisterWithMethod("/olla/proxy/v1/models", a.openaiModelsHandler, "OpenAI-compatible models", "GET")

	// Dynamic translator route registration
	// Each translator that implements PathProvider gets its route automatically registered
	// This scales to unlimited translators (Gemini, Bedrock, etc.) without code changes
	a.registerTranslatorRoutes()

	// Provider routes are built from YAML configs when available
	a.registerProviderRoutes()
}

// registerTranslatorRoutes dynamically registers routes for all translators
// Translators that implement PathProvider interface provide their own API paths
// This enables adding new translators without modifying the routing code
func (a *Application) registerTranslatorRoutes() {
	if a.translatorRegistry == nil {
		a.logger.Warn("Translator registry not available, skipping translator routes")
		return
	}

	translators := a.translatorRegistry.GetAll()
	a.logger.Info("Registering translator routes", "count", len(translators))

	for name, trans := range translators {
		// Check if translator implements PathProvider for dynamic route registration
		// Translators without PathProvider must be registered manually
		if pathProvider, ok := trans.(translator.PathProvider); ok {
			path := pathProvider.GetAPIPath()
			handler := a.translationHandler(trans)

			a.routeRegistry.RegisterWithMethod(
				path,
				handler,
				name+" Messages API",
				"POST",
			)

			a.logger.Debug("Registered translator route",
				"translator", name,
				"path", path)
		} else {
			a.logger.Debug("Translator does not implement PathProvider, skipping route registration",
				"translator", name)
		}
	}
}

// registerProviderRoutes builds HTTP paths from provider YAML configurations.
// Falls back to static registration in test environments without profile loading.
func (a *Application) registerProviderRoutes() {
	if a.profileFactory == nil {
		a.logger.Warn("Profile factory not available, using static route registration")
		a.registerStaticProviderRoutes()
		return
	}

	// Need all profiles for complete route coverage
	profiles := a.profileFactory.GetAvailableProfiles()

	// openai-compatible is special - it's a routing target but not listed as a provider
	profiles = append(profiles, "openai-compatible")

	a.logger.Info("Registering provider routes from profiles", "count", len(profiles))

	for _, profileName := range profiles {
		profile, err := a.profileFactory.GetProfile(profileName)
		if err != nil {
			// openai-compatible might not exist in minimal test setups
			if profileName != "openai-compatible" {
				a.logger.Warn("Failed to get profile", "profile", profileName, "error", err)
			}
			continue
		}

		config := profile.GetConfig()
		if config == nil || len(config.Routing.Prefixes) == 0 {
			a.logger.Debug("Profile has no routing prefixes", "profile", profileName)
			continue
		}

		// Each prefix becomes a distinct URL namespace (e.g., /olla/lmstudio/, /olla/lm-studio/)
		a.logger.Debug("Profile has routing prefixes", "profile", profileName, "prefixes", config.Routing.Prefixes)
		for _, prefix := range config.Routing.Prefixes {
			a.registerProviderPrefixRoutes(prefix, profileName, config)
		}
	}
}

// registerProviderPrefixRoutes creates the full routing table for one provider prefix.
// This handles both native endpoints (e.g., Ollama's /api/tags) and OpenAI compatibility.
func (a *Application) registerProviderPrefixRoutes(prefix, profileName string, config *domain.ProfileConfig) {
	basePath := constants.DefaultOllaProxyPathPrefix + prefix + constants.DefaultPathPrefix

	a.logger.Debug("Registering routes for provider", "prefix", prefix, "profile", profileName)

	// Model discovery varies by provider - some use /api/tags, others /v1/models
	if config.API.ModelDiscoveryPath != "" {
		// Provider's native API format
		nativePath := basePath + strings.TrimPrefix(config.API.ModelDiscoveryPath, constants.DefaultPathPrefix)

		// Ollama has unique endpoints beyond standard model discovery
		if profileName == constants.ProviderTypeOllama {
			a.routeRegistry.RegisterWithMethod(nativePath,
				a.ollamaModelsHandler,
				prefix+" models listing", "GET")

			// Ollama-specific model inspection
			a.routeRegistry.RegisterWithMethod(basePath+"api/list",
				a.ollamaRunningModelsHandler,
				prefix+" running models", "GET")
			a.routeRegistry.RegisterWithMethod(basePath+"api/show",
				a.ollamaModelShowHandler,
				prefix+" model details", "POST")

			// Model management would require persistent storage - explicitly unsupported
			a.routeRegistry.RegisterWithMethod(basePath+"api/pull", a.unsupportedModelManagementHandler, "Model pull (unsupported)", "POST")
			a.routeRegistry.RegisterWithMethod(basePath+"api/push", a.unsupportedModelManagementHandler, "Model push (unsupported)", "POST")
			a.routeRegistry.RegisterWithMethod(basePath+"api/create", a.unsupportedModelManagementHandler, "Model create (unsupported)", "POST")
			a.routeRegistry.RegisterWithMethod(basePath+"api/copy", a.unsupportedModelManagementHandler, "Model copy (unsupported)", "POST")
			a.routeRegistry.RegisterWithMethod(basePath+"api/delete", a.unsupportedModelManagementHandler, "Model delete (unsupported)", "DELETE")
		} else {
			// Generic providers only need their discovery endpoint
			a.routeRegistry.RegisterWithMethod(nativePath,
				a.genericProviderModelsHandler(prefix, "native"),
				prefix+" models", "GET")
		}
	}

	// OpenAI compatibility enables cross-provider client support
	if config.API.OpenAICompatible {
		openAIPath := basePath + "v1/models"

		// Provider-specific handlers optimise for their unique response formats
		switch profileName {
		case constants.ProviderTypeOllama:
			a.routeRegistry.RegisterWithMethod(openAIPath,
				a.ollamaOpenAIModelsHandler,
				prefix+" models (OpenAI format)", "GET")
		case constants.ProviderTypeLMStudio:
			a.routeRegistry.RegisterWithMethod(openAIPath,
				a.lmstudioOpenAIModelsHandler,
				prefix+" models (OpenAI format)", "GET")
			// LM Studio uses multiple OpenAI paths for different client versions
			a.routeRegistry.RegisterWithMethod(basePath+"api/v1/models",
				a.lmstudioOpenAIModelsHandler,
				prefix+" models (OpenAI format alt path)", "GET")
			// v0 provides richer model metadata than standard OpenAI
			a.routeRegistry.RegisterWithMethod(basePath+"api/v0/models",
				a.lmstudioEnhancedModelsHandler,
				prefix+" enhanced models", "GET")
		case constants.ProviderTypeOpenAICompat:
			// Pure OpenAI routes get the full compatibility handler
			if prefix == constants.ProviderTypeOpenAI {
				a.routeRegistry.RegisterWithMethod(openAIPath,
					a.openaiModelsHandler,
					"OpenAI-compatible models", "GET")
			} else {
				a.routeRegistry.RegisterWithMethod(openAIPath,
					a.genericProviderModelsHandler(prefix, constants.ProviderTypeOpenAI),
					prefix+" models (OpenAI format)", "GET")
			}
		default:
			// Unknown providers still get OpenAI compatibility
			a.routeRegistry.RegisterWithMethod(openAIPath,
				a.genericProviderModelsHandler(prefix, constants.ProviderTypeOpenAI),
				prefix+" models (OpenAI format)", "GET")
		}
	}

	// Catch-all must come last - forwards everything else to the backend
	a.routeRegistry.RegisterProxyRoute(basePath, a.providerProxyHandler, prefix+" proxy", "")
}

// getStaticProviders defines minimal routing for tests without YAML configs.
// This duplicates some YAML content but ensures tests can run independently.
func getStaticProviders(a *Application) map[string]staticProvider {
	return map[string]staticProvider{
		constants.ProviderTypeLemonade: {
			prefixes: []string{constants.ProviderTypeLemonade},
			routes: []staticRoute{
				{path: "api/v1/models", handler: a.genericProviderModelsHandler(constants.ProviderTypeLemonade, constants.ProviderTypeOpenAI), description: "Lemonade models (OpenAI format)", method: "GET"},
				{path: "v1/models", handler: a.genericProviderModelsHandler(constants.ProviderTypeLemonade, constants.ProviderTypeOpenAI), description: "Lemonade models (OpenAI format alt path)", method: "GET"},
				{path: "", handler: a.providerProxyHandler, description: "Lemonade proxy", isProxy: true},
			},
		},
		constants.ProviderTypeOllama: {
			prefixes: []string{constants.ProviderTypeOllama},
			routes: []staticRoute{
				{path: "api/tags", handler: a.ollamaModelsHandler, description: "models listing", method: "GET"},
				{path: "v1/models", handler: a.ollamaOpenAIModelsHandler, description: "models (OpenAI format)", method: "GET"},
				{path: "api/list", handler: a.ollamaRunningModelsHandler, description: "running models", method: "GET"},
				{path: "api/show", handler: a.ollamaModelShowHandler, description: "model details", method: "POST"},
				{path: "api/pull", handler: a.unsupportedModelManagementHandler, description: "Model pull (unsupported)", method: "POST"},
				{path: "api/push", handler: a.unsupportedModelManagementHandler, description: "Model push (unsupported)", method: "POST"},
				{path: "api/create", handler: a.unsupportedModelManagementHandler, description: "Model create (unsupported)", method: "POST"},
				{path: "api/copy", handler: a.unsupportedModelManagementHandler, description: "Model copy (unsupported)", method: "POST"},
				{path: "api/delete", handler: a.unsupportedModelManagementHandler, description: "Model delete (unsupported)", method: "DELETE"},
				{path: "", handler: a.providerProxyHandler, description: "proxy", isProxy: true},
			},
		},
		constants.ProviderPrefixLMStudio2: {
			// Mirror the prefixes from lmstudio.yaml for consistency
			prefixes: []string{constants.ProviderPrefixLMStudio1, constants.ProviderPrefixLMStudio2, constants.ProviderPrefixLMStudio3},
			routes: []staticRoute{
				{path: "v1/models", handler: a.lmstudioOpenAIModelsHandler, description: "models (OpenAI format)", method: "GET"},
				{path: "api/v1/models", handler: a.lmstudioOpenAIModelsHandler, description: "models (OpenAI format alt path)", method: "GET"},
				{path: "api/v0/models", handler: a.lmstudioEnhancedModelsHandler, description: "enhanced models", method: "GET"},
				{path: "", handler: a.providerProxyHandler, description: "proxy", isProxy: true},
			},
		},
		constants.ProviderTypeOpenAI: {
			prefixes: []string{constants.ProviderTypeOpenAI},
			routes: []staticRoute{
				{path: "v1/models", handler: a.openaiModelsHandler, description: "OpenAI-compatible models", method: "GET"},
				{path: "", handler: a.providerProxyHandler, description: "OpenAI-compatible proxy", isProxy: true},
			},
		},
		constants.ProviderTypeSGLang: {
			prefixes: []string{constants.ProviderTypeSGLang},
			routes: []staticRoute{
				{path: "v1/models", handler: a.genericProviderModelsHandler(constants.ProviderTypeSGLang, constants.ProviderTypeOpenAI), description: "SGLang models (OpenAI format)", method: "GET"},
				{path: "", handler: a.providerProxyHandler, description: "SGLang proxy", isProxy: true},
			},
		},
		constants.ProviderTypeVLLM: {
			prefixes: []string{constants.ProviderTypeVLLM},
			routes: []staticRoute{
				{path: "v1/models", handler: a.genericProviderModelsHandler(constants.ProviderTypeVLLM, constants.ProviderTypeOpenAI), description: "vLLM models (OpenAI format)", method: "GET"},
				{path: "", handler: a.providerProxyHandler, description: "vLLM proxy", isProxy: true},
			},
		},
	}
}

// registerStaticProviderRoutes provides hardcoded routing for test isolation.
// Production always uses profile-based routing from YAML.
func (a *Application) registerStaticProviderRoutes() {
	providers := getStaticProviders(a)

	// Build routes for all static providers
	for providerName, provider := range providers {
		for _, prefix := range provider.prefixes {
			basePath := fmt.Sprintf("%s%s/", constants.DefaultOllaProxyPathPrefix, prefix)

			// Human-readable names for logging and debugging
			displayName := providerName
			if providerName == constants.ProviderTypeLMStudio {
				displayName = constants.ProviderDisplayLMStudio
				if prefix != constants.ProviderPrefixLMStudio1 {
					displayName += fmt.Sprintf(" (%s)", prefix)
				}
			} else if providerName == constants.ProviderTypeOllama {
				displayName = constants.ProviderDisplayOllama
			}

			// Add all routes for this prefix
			for _, route := range provider.routes {
				fullPath := basePath + route.path
				desc := fmt.Sprintf("%s %s", displayName, route.description)

				if route.isProxy {
					a.routeRegistry.RegisterProxyRoute(basePath, route.handler, desc, "")
				} else {
					a.routeRegistry.RegisterWithMethod(fullPath, route.handler, desc, route.method)
				}
			}
		}
	}
}
