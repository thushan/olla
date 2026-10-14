package profile

import (
	"errors"
	"fmt"
	"time"

	"github.com/thushan/olla/internal/core/domain"
)

// TEIInfoResponse is the shape of the /info endpoint served by Hugging Face
// Text Embeddings Inference. Unlike OpenAI-style listing endpoints it describes
// the single model the instance serves rather than a list of models.
type TEIInfoResponse struct {
	ModelID         string `json:"model_id"`
	ServedModelName string `json:"served_model_name"`
}

type textEmbeddingInferenceParser struct{}

func (p *textEmbeddingInferenceParser) Parse(data []byte) ([]*domain.ModelInfo, error) {
	var response TEIInfoResponse
	if len(data) > 0 {
		if err := json.Unmarshal(data, &response); err != nil {
			return nil, fmt.Errorf("failed to parse Text Embeddings Inference response: %w", err)
		}
	}

	// served_model_name is the name the instance actually serves requests under;
	// model_id is the on-disk checkpoint path and only a fallback.
	name := response.ServedModelName
	if name == "" {
		name = response.ModelID
	}
	if name == "" {
		// Error rather than an empty list: an empty registration would silently
		// deregister the endpoint's model, while an error feeds the discovery
		// failure count and pause machinery.
		return nil, errors.New("tei /info response contains neither served_model_name nor model_id")
	}

	// "embeddings" rather than the provider type: capability inference keys off
	// this field to tag the model as an embedding model and drop text-generation.
	return []*domain.ModelInfo{
		{
			Name:     name,
			Type:     "embeddings",
			LastSeen: time.Now(),
		},
	}, nil
}
