package proxy

import (
	"fmt"
	"sync"
	"time"

	"github.com/thushan/olla/internal/adapter/proxy/olla"
	"github.com/thushan/olla/internal/adapter/proxy/sherpa"
	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/core/ports"
	"github.com/thushan/olla/internal/logger"
)

const (
	DefaultProxySherpa = "sherpa"
	DefaultProxyOlla   = "olla"
)

// ProxyCreator is a function that creates a proxy service
type ProxyCreator func(ports.DiscoveryService, domain.EndpointSelector, ports.ProxyConfiguration, ports.StatsCollector, logger.StyledLogger) (ports.ProxyService, error)

// Factory creates proxy services using a registry pattern
type Factory struct {
	creators       map[string]ProxyCreator
	statsCollector ports.StatsCollector
	logger         logger.StyledLogger
	mu             sync.RWMutex
}

func NewFactory(statsCollector ports.StatsCollector, theLogger logger.StyledLogger) *Factory {
	factory := &Factory{
		creators:       make(map[string]ProxyCreator),
		statsCollector: statsCollector,
		logger:         theLogger,
	}

	// Register Sherpa implementation
	factory.Register(DefaultProxySherpa, func(discovery ports.DiscoveryService, selector domain.EndpointSelector, config ports.ProxyConfiguration, collector ports.StatsCollector, logger logger.StyledLogger) (ports.ProxyService, error) {
		sherpaConfig := &sherpa.Configuration{
			ProxyPrefix:         config.GetProxyPrefix(),
			ConnectionTimeout:   config.GetConnectionTimeout(),
			ConnectionKeepAlive: config.GetConnectionKeepAlive(),
			ResponseTimeout:     config.GetResponseTimeout(),
			ReadTimeout:         config.GetReadTimeout(),
			StreamBufferSize:    config.GetStreamBufferSize(),
		}
		return sherpa.NewService(discovery, selector, sherpaConfig, collector, logger)
	})

	// Register Olla implementation
	factory.Register(DefaultProxyOlla, func(discovery ports.DiscoveryService, selector domain.EndpointSelector, config ports.ProxyConfiguration, collector ports.StatsCollector, logger logger.StyledLogger) (ports.ProxyService, error) {
		ollaConfig := &olla.Configuration{
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
		return olla.NewService(discovery, selector, ollaConfig, collector, logger)
	})

	return factory
}

// Register adds a new proxy creator
func (f *Factory) Register(name string, creator ProxyCreator) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.creators[name] = creator
}

// Create creates a new proxy service of the specified type
func (f *Factory) Create(proxyType string, discoveryService ports.DiscoveryService, selector domain.EndpointSelector, config ports.ProxyConfiguration) (ports.ProxyService, error) {
	f.mu.RLock()
	creator, exists := f.creators[proxyType]
	f.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("unknown proxy implementation: %s", proxyType)
	}

	f.logger.Info("Initialising proxy service", "type", proxyType)
	return creator(discoveryService, selector, config, f.statsCollector, f.logger)
}

// GetAvailableTypes returns all registered proxy types
func (f *Factory) GetAvailableTypes() []string {
	f.mu.RLock()
	defer f.mu.RUnlock()

	types := make([]string, 0, len(f.creators))
	for name := range f.creators {
		types = append(types, name)
	}
	return types
}
