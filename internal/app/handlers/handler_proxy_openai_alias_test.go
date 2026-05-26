package handlers

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/thushan/olla/internal/core/domain"
)

// TestFilterEndpointsByProfile_OpenAITypeAlias guards the alias fix in domain.IsCompatibleWith.
// An endpoint configured with type "openai" must be accepted when the request profile's
// SupportedBy slice contains only "openai-compatible". Without the alias, both endpoints
// would be dropped and the fallback warning fires — the original symptom of issue #148.
func TestFilterEndpointsByProfile_OpenAITypeAlias(t *testing.T) {
	t.Parallel()

	styledLog := &mockStyledLogger{}

	openaiEndpoint := &domain.Endpoint{
		Name:      "openai-backend",
		URLString: "http://openai-backend:8080",
		Type:      "openai",
		Status:    domain.StatusHealthy,
	}
	compatibleEndpoint := &domain.Endpoint{
		Name:      "openai-compatible-backend",
		URLString: "http://compat-backend:8080",
		Type:      "openai-compatible",
		Status:    domain.StatusHealthy,
	}

	app := &Application{
		logger: styledLog,
		// no modelRegistry: we only want stage-1 (platform compatibility) to run
	}

	profile := &domain.RequestProfile{
		Path:        "/v1/chat/completions",
		SupportedBy: []string{domain.ProfileOpenAICompatible},
	}

	result := app.filterEndpointsByProfile([]*domain.Endpoint{openaiEndpoint, compatibleEndpoint}, profile, styledLog)

	// Both endpoints speak the OpenAI-compatible protocol; neither should be dropped.
	// If the alias in IsCompatibleWith is removed, "openai" no longer matches
	// "openai-compatible" and result shrinks to 1, causing this assertion to fail.
	assert.Len(t, result, 2, "both openai and openai-compatible endpoints must be accepted when SupportedBy contains openai-compatible")
	assert.Contains(t, result, openaiEndpoint, "type=openai endpoint must be accepted as openai-compatible alias")
	assert.Contains(t, result, compatibleEndpoint, "type=openai-compatible endpoint must always be accepted")
}
