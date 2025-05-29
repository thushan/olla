package discovery

import (
	"context"
	"github.com/thushan/olla/internal/adapter/health"
	"github.com/thushan/olla/internal/config"
	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/logger"
)

type SimpleDiscoveryService struct {
	Repository    *StaticEndpointRepository
	HealthChecker *health.HTTPHealthChecker
	Endpoints     []config.EndpointConfig
	Logger        *logger.StyledLogger
}

func (s *SimpleDiscoveryService) GetEndpoints(ctx context.Context) ([]*domain.Endpoint, error) {
	return s.Repository.GetAll(ctx)
}

func (s *SimpleDiscoveryService) GetHealthyEndpoints(ctx context.Context) ([]*domain.Endpoint, error) {
	routable, err := s.Repository.GetRoutable(ctx)
	if err != nil {
		return nil, err
	}

	if len(routable) > 0 {
		return routable, nil
	}

	s.Logger.Warn("No routable endpoints available, falling back to all endpoints")
	return s.Repository.GetAll(ctx)
}

func (s *SimpleDiscoveryService) RefreshEndpoints(ctx context.Context) error {
	_, err := s.Repository.UpsertFromConfig(ctx, s.Endpoints)
	return err
}
