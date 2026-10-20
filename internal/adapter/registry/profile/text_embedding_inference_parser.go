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
	ModelType       map[string]any `json:"model_type,omitempty"`
	ModelID         string         `json:"model_id"`
	ServedModelName string         `json:"served_model_name"`
}

// teiModelTypes are the keys TEI uses in model_type's externally tagged
// object ({"embedding":{...}}, {"reranker":{...}}, {"classifier":{...}}).
// Anything else means a TEI version serving a kind we do not know about.
var teiModelTypes = map[string]struct{}{
	"embedding":  {},
	"reranker":   {},
	"classifier": {},
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
	// Olla has no rerank/classify capability yet, so this stays "embeddings" for
	// every TEI model; the real kind travels in Details.Type (surfaced as the
	// model_type metadata key) so the limitation is at least observable.
	modelInfo := &domain.ModelInfo{
		Name:     name,
		Type:     "embeddings",
		LastSeen: time.Now(),
	}

	if kind, ok := teiModelKind(response.ModelType); ok {
		modelInfo.Details = &domain.ModelDetails{Type: &kind}
	}

	return []*domain.ModelInfo{modelInfo}, nil
}

// teiModelKind extracts the tag key from model_type's single-key object.
// An absent, empty or multi-key object yields no kind: guessing among
// several tags would mislabel the model.
func teiModelKind(modelType map[string]any) (string, bool) {
	if len(modelType) != 1 {
		return "", false
	}
	for key := range modelType {
		if _, known := teiModelTypes[key]; known {
			return key, true
		}
	}
	return "", false
}
