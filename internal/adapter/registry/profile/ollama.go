package profile

import (
	"fmt"
	"time"

	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/util"
)

const (
	OllamaProfileVersion             = "1.0"
	OllamaProfileHealthPath          = "/"
	OllamaProfileGeneratePath        = "/api/generate"
	OllamaProfileChatCompletionsPath = "/v1/chat/completions"
	OllamaProfileCompletionsPath     = "/v1/completions"
	OllamaProfileModelModelsPath     = "/api/tags"
)

type OllamaProfile struct{}

func NewOllamaProfile() *OllamaProfile {
	return &OllamaProfile{}
}

func (p *OllamaProfile) GetName() string {
	return domain.ProfileOllama
}

func (p *OllamaProfile) GetVersion() string {
	return OllamaProfileVersion
}

func (p *OllamaProfile) GetModelDiscoveryURL(baseURL string) string {
	return util.NormaliseBaseURL(baseURL) + OllamaProfileModelModelsPath
}

func (p *OllamaProfile) GetHealthCheckPath() string {
	return OllamaProfileHealthPath
}

func (p *OllamaProfile) IsOpenAPICompatible() bool {
	return true
}

func (p *OllamaProfile) GetRequestParsingRules() domain.RequestParsingRules {
	return domain.RequestParsingRules{
		ChatCompletionsPath: "/v1/chat/completions",
		CompletionsPath:     "/v1/completions",
		GeneratePath:        "/api/generate",
		ModelFieldName:      "model",
		SupportsStreaming:   true,
	}
}

func (p *OllamaProfile) GetModelResponseFormat() domain.ModelResponseFormat {
	return domain.ModelResponseFormat{
		ResponseType:    "object",
		ModelsFieldPath: "models",
	}
}

func (p *OllamaProfile) GetDetectionHints() domain.DetectionHints {
	return domain.DetectionHints{
		UserAgentPatterns: []string{"ollama/"},
		ResponseHeaders:   []string{"X-ProfileOllama-Version"},
		PathIndicators: []string{
			OllamaProfileModelModelsPath,
			OllamaProfileGeneratePath,
		},
	}
}

func (p *OllamaProfile) ParseModel(modelData map[string]interface{}) (*domain.ModelInfo, error) {
	modelInfo := &domain.ModelInfo{
		LastSeen: time.Now(),
	}

	if name := util.GetString(modelData, "name"); name != "" {
		modelInfo.Name = name
	} else {
		return nil, fmt.Errorf("model name is required")
	}

	if size, ok := util.GetFloat64(modelData, "size"); ok {
		modelInfo.Size = size
	}

	modelInfo.Description = util.GetString(modelData, "description")

	modelInfo.Details = &domain.ModelDetails{}
	hasDetails := false

	if digest := util.GetString(modelData, "digest"); digest != "" {
		modelInfo.Details.Digest = &digest
		hasDetails = true
	}

	if modifiedAt := util.ParseTime(modelData, "modified_at"); modifiedAt != nil {
		modelInfo.Details.ModifiedAt = modifiedAt
		hasDetails = true
	}

	// extra juicy details, seems like _sometimes_ we don't get a "details" object
	if detailsData, exists := modelData["details"]; exists {
		if detailsObj, ok := detailsData.(map[string]interface{}); ok {
			if paramSize := util.GetString(detailsObj, "parameter_size"); paramSize != "" {
				modelInfo.Details.ParameterSize = &paramSize
				hasDetails = true
			}

			if quantLevel := util.GetString(detailsObj, "quantization_level"); quantLevel != "" {
				modelInfo.Details.QuantizationLevel = &quantLevel
				hasDetails = true
			}

			if family := util.GetString(detailsObj, "family"); family != "" {
				modelInfo.Details.Family = &family
				hasDetails = true
			}

			if format := util.GetString(detailsObj, "format"); format != "" {
				modelInfo.Details.Format = &format
				hasDetails = true
			}

			if parentModel := util.GetString(detailsObj, "parent_model"); parentModel != "" {
				modelInfo.Details.ParentModel = &parentModel
				hasDetails = true
			}

			if families := util.GetStringArray(detailsObj, "families"); len(families) > 0 {
				modelInfo.Details.Families = families
				hasDetails = true
			}
		}
	}

	if !hasDetails {
		modelInfo.Details = nil
	}

	return modelInfo, nil
}
