package registry

import (
	"fmt"

	"github.com/thushan/olla/internal/logger"

	"github.com/thushan/olla/internal/core/domain"
)

type RegistryConfig struct {
	Type           string `yaml:"type"`
	EnableUnifier  bool   `yaml:"enable_unifier"`
}

func NewModelRegistry(config RegistryConfig, logger logger.StyledLogger) (domain.ModelRegistry, error) {
	switch config.Type {
	case "memory", "":
		if config.EnableUnifier {
			return NewUnifiedMemoryModelRegistry(logger), nil
		}
		return NewMemoryModelRegistry(logger), nil
	default:
		return nil, fmt.Errorf("unsupported registry type: %s", config.Type)
	}
}
