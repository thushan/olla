package profile

import (
	"fmt"
	"time"

	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/util"
)

type OllamaResponse struct {
	Models []OllamaModel `json:"models"`
}

type OllamaModel struct {
	Size        *int64         `json:"size,omitempty"`
	Digest      *string        `json:"digest,omitempty"`
	ModifiedAt  *string        `json:"modified_at,omitempty"`
	Description *string        `json:"description,omitempty"`
	Details     *OllamaDetails `json:"details,omitempty"`
	Name        string         `json:"name"`
}

type OllamaDetails struct {
	ParameterSize     *string  `json:"parameter_size,omitempty"`
	QuantizationLevel *string  `json:"quantization_level,omitempty"`
	Family            *string  `json:"family,omitempty"`
	Format            *string  `json:"format,omitempty"`
	ParentModel       *string  `json:"parent_model,omitempty"`
	Families          []string `json:"families,omitempty"`
}

type OllamaProfile struct{}

const (
	OllamaProfileVersion           = "1.0"
	OllamaProfileHealthPathIndex   = 0
	OllamaCompletionsPathIndex     = 1
	OllamaChatCompletionsPathIndex = 2
	OllamaEmbeddingsPathIndex      = 3
	OllamaModelsPathIndex          = 4
)

var ollamaPaths []string

func init() {
	ollamaPaths = []string{
		"/", // ollama returns "Ollama is running" here
		"/api/generate",
		"/api/chat",
		"/api/embeddings",
		"/api/tags", // where ollama lists its models
		"/api/show", // detailed model info including modelfile

		// openai endpoints because everyone expects them
		"/v1/models",
		"/v1/chat/completions",
		"/v1/completions",
		"/v1/embeddings",

		/*
			// Not sure we want to support these yet, but they are
			// documented in the Ollama API docs.
				// Model management
				"/api/create", // Create model from Modelfile
				"/api/pull",   // Download model
				"/api/push",   // Upload model
				"/api/copy",   // Copy model
				"/api/delete", // Delete model

				// Blob management
				"/api/blobs/:digest", // Check blob exists
				"/api/blobs",         // Create blob

				// Process management
				"/api/ps", // List running models

		*/
		// skipping model management endpoints like /api/pull and /api/create
		// because we're a proxy, not ollama's package manager

	}
}

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
	return util.NormaliseBaseURL(baseURL) + ollamaPaths[OllamaModelsPathIndex]
}

func (p *OllamaProfile) GetHealthCheckPath() string {
	return ollamaPaths[OllamaProfileHealthPathIndex]
}

func (p *OllamaProfile) IsOpenAPICompatible() bool {
	return true
}

func (p *OllamaProfile) GetRequestParsingRules() domain.RequestParsingRules {
	return domain.RequestParsingRules{
		ChatCompletionsPath: ollamaPaths[OllamaChatCompletionsPathIndex],
		CompletionsPath:     ollamaPaths[OllamaCompletionsPathIndex],
		GeneratePath:        ollamaPaths[OllamaCompletionsPathIndex],
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
			ollamaPaths[OllamaProfileHealthPathIndex],
			ollamaPaths[OllamaModelsPathIndex],
		},
	}
}

func (p *OllamaProfile) GetPaths() []string {
	return ollamaPaths
}

func (p *OllamaProfile) GetPath(index int) string {
	if index < 0 || index >= len(ollamaPaths) {
		return ""
	}
	return ollamaPaths[index]
}
func (p *OllamaProfile) ParseModelsResponse(data []byte) ([]*domain.ModelInfo, error) {
	if len(data) == 0 {
		return make([]*domain.ModelInfo, 0), nil
	}

	var response OllamaResponse
	if err := json.Unmarshal(data, &response); err != nil {
		return nil, fmt.Errorf("failed to parse Ollama response: %w", err)
	}

	models := make([]*domain.ModelInfo, 0, len(response.Models))
	now := time.Now()

	for _, ollamaModel := range response.Models {
		if ollamaModel.Name == "" {
			continue
		}

		modelInfo := &domain.ModelInfo{
			Name:     ollamaModel.Name,
			LastSeen: now,
		}

		if ollamaModel.Size != nil {
			modelInfo.Size = *ollamaModel.Size
		}

		if ollamaModel.Description != nil {
			modelInfo.Description = *ollamaModel.Description
		}

		if ollamaModel.Details != nil || ollamaModel.Digest != nil || ollamaModel.ModifiedAt != nil {
			modelInfo.Details = createModelDetails(ollamaModel)
		}

		models = append(models, modelInfo)
	}

	return models, nil
}

func createModelDetails(ollamaModel OllamaModel) *domain.ModelDetails {
	details := &domain.ModelDetails{}

	if ollamaModel.Digest != nil {
		details.Digest = ollamaModel.Digest
	}

	if ollamaModel.ModifiedAt != nil {
		if parsedTime := util.ParseTime(*ollamaModel.ModifiedAt); parsedTime != nil {
			details.ModifiedAt = parsedTime
		}
	}

	if ollamaModel.Details != nil {
		if ollamaModel.Details.ParameterSize != nil {
			details.ParameterSize = ollamaModel.Details.ParameterSize
		}
		if ollamaModel.Details.QuantizationLevel != nil {
			details.QuantizationLevel = ollamaModel.Details.QuantizationLevel
		}
		if ollamaModel.Details.Family != nil {
			details.Family = ollamaModel.Details.Family
		}
		if ollamaModel.Details.Format != nil {
			details.Format = ollamaModel.Details.Format
		}
		if ollamaModel.Details.ParentModel != nil {
			details.ParentModel = ollamaModel.Details.ParentModel
		}
		if len(ollamaModel.Details.Families) > 0 {
			details.Families = ollamaModel.Details.Families
		}
	}
	return details
}
