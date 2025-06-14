package inspector

import (
	"context"
	"net/http"

	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/core/ports"
	"github.com/thushan/olla/internal/logger"
)

type Chain struct {
	logger     logger.StyledLogger
	inspectors []ports.RequestInspector
}

func NewChain(logger logger.StyledLogger) *Chain {
	return &Chain{
		inspectors: make([]ports.RequestInspector, 0, 4),
		logger:     logger,
	}
}

func (c *Chain) AddInspector(inspector ports.RequestInspector) {
	c.inspectors = append(c.inspectors, inspector)
}

func (c *Chain) Inspect(ctx context.Context, r *http.Request, targetPath string) (*domain.RequestProfile, error) {
	profile := domain.NewRequestProfile(targetPath)

	for _, inspector := range c.inspectors {
		if err := inspector.Inspect(ctx, r, profile); err != nil {
			c.logger.Warn("Inspector failed, continuing chain",
				"inspector", inspector.Name(),
				"path", targetPath,
				"error", err)
			continue
		}

		c.logger.Debug("Inspector completed",
			"inspector", inspector.Name(),
			"path", targetPath,
			"supported_by", profile.SupportedBy)
	}

	return profile, nil
}
