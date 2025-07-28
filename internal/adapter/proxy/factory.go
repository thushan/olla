package proxy

import (
	"fmt"
	"sync"
	"time"

	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/core/ports"
	"github.com/thushan/olla/internal/logger"
)

const (
	DefaultProxySherpa = "sherpa"
	DefaultProxyOlla   = "olla"
)

type Factory struct {
	creators       map[string]func(ports.DiscoveryService, domain.EndpointSelector, ports.ProxyConfiguration, ports.StatsCollector, logger.StyledLogger) ports.ProxyService
	statsCollector ports.StatsCollector
	logger         logger.StyledLogger
	mu             sync.RWMutex
}

func NewFactory(statsCollector ports.StatsCollector, theLogger logger.StyledLogger) *Factory {
	factory := &Factory{
		creators:       make(map[string]func(ports.DiscoveryService, domain.EndpointSelector, ports.ProxyConfiguration, ports.StatsCollector, logger.StyledLogger) ports.ProxyService),
		statsCollector: statsCollector,
		logger:         theLogger,
	}

	factory.Register(DefaultProxySherpa, func(discovery ports.DiscoveryService, selector domain.EndpointSelector, config ports.ProxyConfiguration, collector ports.StatsCollector, logger logger.StyledLogger) ports.ProxyService {
		sherpaConfig := &Configuration{
			ProxyPrefix:         config.GetProxyPrefix(),
			ConnectionTimeout:   config.GetConnectionTimeout(),
			ConnectionKeepAlive: config.GetConnectionKeepAlive(),
			ResponseTimeout:     config.GetResponseTimeout(),
			ReadTimeout:         config.GetReadTimeout(),
			StreamBufferSize:    config.GetStreamBufferSize(),
		}
		return NewSherpaService(discovery, selector, sherpaConfig, collector, logger)
	})

	factory.Register(DefaultProxyOlla, func(discovery ports.DiscoveryService, selector domain.EndpointSelector, config ports.ProxyConfiguration, collector ports.StatsCollector, logger logger.StyledLogger) ports.ProxyService {
		ollaConfig := &Configuration{
			ProxyPrefix:         config.GetProxyPrefix(),
			ConnectionTimeout:   config.GetConnectionTimeout(),
			ConnectionKeepAlive: config.GetConnectionKeepAlive(),
			ResponseTimeout:     config.GetResponseTimeout(),
			ReadTimeout:         config.GetReadTimeout(),
			StreamBufferSize:    config.GetStreamBufferSize(),
			MaxIdleConns:        200,
			IdleConnTimeout:     90 * time.Second,
			MaxConnsPerHost:     50,
		}
		return NewOllaService(discovery, selector, ollaConfig, collector, logger)
	})
	return factory
}

func (f *Factory) Register(name string, creator func(ports.DiscoveryService, domain.EndpointSelector, ports.ProxyConfiguration, ports.StatsCollector, logger.StyledLogger) ports.ProxyService) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.creators[name] = creator
}

func (f *Factory) Create(proxyType string, discoveryService ports.DiscoveryService, selector domain.EndpointSelector, config ports.ProxyConfiguration) (ports.ProxyService, error) {
	f.mu.RLock()
	creator, exists := f.creators[proxyType]
	f.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("unknown proxy implementation: %s", proxyType)
	}

	f.logger.Info("Initialising proxy service", "type", proxyType)
	return creator(discoveryService, selector, config, f.statsCollector, f.logger), nil
}

func (f *Factory) GetAvailableTypes() []string {
	f.mu.RLock()
	defer f.mu.RUnlock()

	types := make([]string, 0, len(f.creators))
	for name := range f.creators {
		types = append(types, name)
	}
	return types
}
