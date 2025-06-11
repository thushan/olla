package discovery

import (
	"encoding/json"
	"fmt"
	"github.com/thushan/olla/internal/util"
	"time"

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

// ParseModelsResponse parses model discovery responses based on platform profile format
func (p *ResponseParser) ParseModelsResponse(responseBody []byte, format domain.ModelResponseFormat) ([]*domain.ModelInfo, error) {
	if len(responseBody) == 0 {
		return []*domain.ModelInfo{}, nil
	}

	switch format.ResponseType {
	case "object":
		return p.parseObjectResponse(responseBody, format)
	default:
		return nil, &ParseError{
			Data:   responseBody,
			Format: format.ResponseType,
			Err:    fmt.Errorf("unsupported response type: %s", format.ResponseType),
		}
	}
}

// parseObjectResponse handles JSON object responses with configurable field paths
func (p *ResponseParser) parseObjectResponse(data []byte, format domain.ModelResponseFormat) ([]*domain.ModelInfo, error) {
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
	now := time.Now()

	for _, modelData := range modelsArray {
		modelObj, ok := modelData.(map[string]interface{})
		if !ok {
			continue // Skip invalid entries
		}

		// doing this to make sure time is always the same at *this time*
		modelInfo := &domain.ModelInfo{
			LastSeen: now,
		}

		// assume they all call it Description for now,
		// TODO: make this a ModelTypeField
		modelInfo.Description = util.GetString(modelObj, "description")

		models = append(models, modelInfo)
	}

	return models, nil
}
