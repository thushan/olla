package discovery

import (
	"encoding/json"
	"fmt"

	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/logger"
)

// ResponseParser handles parsing of difefrent platform response formats
type ResponseParser struct {
	logger logger.StyledLogger
}

func NewResponseParser(logger logger.StyledLogger) *ResponseParser {
	return &ResponseParser{
		logger: logger,
	}
}

// ParseModelsResponse parses model discovery responses using platform-specific parsers
func (p *ResponseParser) ParseModelsResponse(responseBody []byte, format domain.ModelResponseFormat, profile domain.PlatformProfile) ([]*domain.ModelInfo, error) {
	if len(responseBody) == 0 {
		return []*domain.ModelInfo{}, nil
	}

	switch format.ResponseType {
	case "object":
		return p.parseObjectResponse(responseBody, format, profile)
	default:
		return nil, &ParseError{
			Data:   responseBody,
			Format: format.ResponseType,
			Err:    fmt.Errorf("unsupported response type: %s", format.ResponseType),
		}
	}
}

// parseObjectResponse handles JSON object responses using platform-specific model parsing
func (p *ResponseParser) parseObjectResponse(data []byte, format domain.ModelResponseFormat, profile domain.PlatformProfile) ([]*domain.ModelInfo, error) {
	var response map[string]interface{}
	if err := json.Unmarshal(data, &response); err != nil {
		return nil, &ParseError{
			Data:   data,
			Format: "json",
			Err:    fmt.Errorf("invalid JSON: %w", err),
		}
	}

	modelsData, exists := response[format.ModelsFieldPath]
	if !exists {
		return []*domain.ModelInfo{}, nil
	}

	modelsArray, ok := modelsData.([]interface{})
	if !ok {
		return nil, &ParseError{
			Data:   data,
			Format: "json",
			Err:    fmt.Errorf("models field '%s' is not an array", format.ModelsFieldPath),
		}
	}

	models := make([]*domain.ModelInfo, 0, len(modelsArray))

	for _, modelData := range modelsArray {
		modelObj, ok := modelData.(map[string]interface{})
		if !ok {
			continue // Skip invalid entries
		}

		// use the profile specific parsing
		modelInfo, err := profile.ParseModel(modelObj)
		if err != nil {
			// Log the error but continue processing other models
			p.logger.Warn("Failed to parse model", "platform", profile.GetName(), "error", err.Error())
			continue
		}

		if modelInfo != nil && modelInfo.Name != "" {
			// every model needs a minimum of a name, we may revisit this later
			models = append(models, modelInfo)
		}
	}

	return models, nil
}
