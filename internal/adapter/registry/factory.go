package registry

import (
	"fmt"

	"github.com/thushan/olla/internal/core/domain"
)

type RegistryConfig struct {
	Type string `yaml:"type"`
}

func NewModelRegistry(config RegistryConfig) (domain.ModelRegistry, error) {
	switch config.Type {
	case "memory", "":
		return NewMemoryModelRegistry(), nil
	default:
		return nil, fmt.Errorf("unsupported registry type: %s", config.Type)
	}
}
