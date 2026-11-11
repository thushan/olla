package registry

import (
	"context"
	"strings"

	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/logger"
)

// AliasResolver resolves model aliases to actual model names and their endpoints.
// It sits between the handler and the model registry to transparently expand aliases
// into the concrete model names that backends recognise.
type AliasResolver struct {
	// aliases maps a virtual model name to a list of actual model names
	aliases   map[string][]string
	nameIndex map[string]string
	logger    logger.StyledLogger
}

// NewAliasResolver creates a new AliasResolver from the configured alias map.
// Returns nil if no aliases are configured, allowing callers to skip alias logic entirely.
func NewAliasResolver(aliases map[string][]string, logger logger.StyledLogger) *AliasResolver {
	if len(aliases) == 0 {
		return nil
	}

	return &AliasResolver{
		aliases: aliases,
		logger:  logger,
	}
}

// NewAliasResolverWithUnification also matches alias names without regard to case
// when model unification is enabled. Exact names retain their configured targets.
func NewAliasResolverWithUnification(aliases map[string][]string, logger logger.StyledLogger, enabled bool) *AliasResolver {
	r := NewAliasResolver(aliases, logger)
	if r == nil || !enabled {
		return r
	}
	r.nameIndex = make(map[string]string, len(aliases))
	for name := range aliases {
		key := strings.ToLower(name)
		if _, exists := r.nameIndex[key]; exists {
			// A non-exact spelling must not choose between different aliases.
			r.nameIndex[key] = ""
		} else {
			r.nameIndex[key] = name
		}
	}
	return r
}

// PreserveModelNameCase keeps the inspector from collapsing distinct configured
// aliases before lookup. Ambiguous spellings must reach lookup unchanged too.
func (r *AliasResolver) PreserveModelNameCase(name string) bool {
	_, exists := r.nameIndex[strings.ToLower(name)]
	return exists
}

// CanonicalAliasName returns the configured spelling of an unambiguous alias.
func (r *AliasResolver) CanonicalAliasName(name string) (string, bool) {
	if _, exists := r.aliases[name]; exists {
		return name, true
	}
	original := r.nameIndex[strings.ToLower(name)]
	return original, original != ""
}

func (r *AliasResolver) aliasModels(name string) ([]string, bool) {
	original, ok := r.CanonicalAliasName(name)
	if !ok {
		return nil, false
	}
	return r.aliases[original], true
}

// IsAlias returns true if the given model name resolves to a configured alias.
func (r *AliasResolver) IsAlias(modelName string) bool {
	_, ok := r.aliasModels(modelName)
	return ok
}

// GetActualModels returns the list of actual model names for an alias.
// Returns nil if the model name is not an alias.
func (r *AliasResolver) GetActualModels(aliasName string) []string {
	models, _ := r.aliasModels(aliasName)
	return models
}

// EndpointModelMapping maps an endpoint URL to the actual model name it should receive
type EndpointModelMapping struct {
	EndpointURL string
	ModelName   string
}

// ResolveEndpoints queries the registry for all endpoints that serve any of the aliased
// model names. It returns the combined endpoint list and a mapping from endpoint URL to
// the actual model name that endpoint knows, so the proxy can rewrite the request body.
func (r *AliasResolver) ResolveEndpoints(
	ctx context.Context,
	aliasName string,
	registry domain.ModelRegistry,
) (map[string]string, error) {
	actualModels := r.GetActualModels(aliasName)
	if len(actualModels) == 0 {
		return nil, nil
	}

	// endpointToModel maps endpoint URL → actual model name for that endpoint
	endpointToModel := make(map[string]string)

	for _, modelName := range actualModels {
		endpoints, err := registry.GetEndpointsForModel(ctx, modelName)
		if err != nil {
			r.logger.Debug("Failed to get endpoints for aliased model",
				"alias", aliasName,
				"model", modelName,
				"error", err)
			continue
		}

		for _, endpointURL := range endpoints {
			// First model found for an endpoint wins - this ensures deterministic
			// behaviour when multiple aliased names resolve to the same endpoint
			if _, exists := endpointToModel[endpointURL]; !exists {
				endpointToModel[endpointURL] = modelName
			}
		}
	}

	if len(endpointToModel) > 0 {
		r.logger.Debug("Resolved model alias to endpoints",
			"alias", aliasName,
			"actual_models", actualModels,
			"endpoint_count", len(endpointToModel))
	}

	return endpointToModel, nil
}
