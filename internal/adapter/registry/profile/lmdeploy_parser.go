package profile

import (
	"fmt"
	"time"

	"github.com/thushan/olla/internal/core/domain"
)

type lmdeployParser struct{}

func (p *lmdeployParser) Parse(data []byte) ([]*domain.ModelInfo, error) {
	if len(data) == 0 {
		return make([]*domain.ModelInfo, 0), nil
	}

	var response LMDeployResponse
	if err := json.Unmarshal(data, &response); err != nil {
		return nil, fmt.Errorf("failed to parse LMDeploy response: %w", err)
	}

	models := make([]*domain.ModelInfo, 0, len(response.Data))
	now := time.Now()

	for _, model := range response.Data {
		if model.ID == "" {
			continue
		}

		modelInfo := &domain.ModelInfo{
			Name:     model.ID,
			Type:     "lmdeploy",
			LastSeen: now,
		}

		details := &domain.ModelDetails{}
		hasDetails := false

		if model.Created > 0 {
			createdTime := time.Unix(model.Created, 0)
			details.ModifiedAt = &createdTime
			hasDetails = true
		}

		// Skip the default owned_by value to avoid storing noise.
		if model.OwnedBy != "" && model.OwnedBy != "lmdeploy" {
			details.Publisher = &model.OwnedBy
			hasDetails = true
		}

		if model.Parent != nil {
			details.ParentModel = model.Parent
			hasDetails = true
		}

		if hasDetails {
			modelInfo.Details = details
		}

		models = append(models, modelInfo)
	}

	return models, nil
}
