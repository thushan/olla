package registry

import (
	"fmt"

	"github.com/thushan/olla/internal/logger"

	"github.com/thushan/olla/internal/config"
	"github.com/thushan/olla/internal/core/domain"
)

type RegistryConfig struct {
	UnificationConf *config.UnificationConfig `yaml:"unification"`
	Type            string                    `yaml:"type"`
	EnableUnifier   bool                      `yaml:"enable_unifier"`
}

func NewModelRegistry(regConfig RegistryConfig, logger logger.StyledLogger) (domain.ModelRegistry, error) {
	switch regConfig.Type {
	case "memory", "":
		if regConfig.EnableUnifier {
			return NewUnifiedMemoryModelRegistry(logger, regConfig.UnificationConf), nil
		}
		return NewMemoryModelRegistry(logger), nil
	default:
		return nil, fmt.Errorf("unsupported registry type: %s", regConfig.Type)
	}
}
