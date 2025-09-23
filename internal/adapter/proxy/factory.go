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
type ProxyCreator func(ports.DiscoveryService, domain.EndpointSelector, ports.ProxyConfiguration, ports.StatsCollector, ports.MetricsExtractor, logger.StyledLogger) (ports.ProxyService, error)

// Factory creates proxy services using a registry pattern
type Factory struct {
	creators         map[string]ProxyCreator
	statsCollector   ports.StatsCollector
	metricsExtractor ports.MetricsExtractor
	logger           logger.StyledLogger
	mu               sync.RWMutex
}

func NewFactory(statsCollector ports.StatsCollector, metricsExtractor ports.MetricsExtractor, theLogger logger.StyledLogger) *Factory {
	factory := &Factory{
		creators:         make(map[string]ProxyCreator),
		statsCollector:   statsCollector,
		metricsExtractor: metricsExtractor,
		logger:           theLogger,
	}

	// Register Sherpa implementation
	factory.Register(DefaultProxySherpa, func(discovery ports.DiscoveryService, selector domain.EndpointSelector, config ports.ProxyConfiguration, collector ports.StatsCollector, metricsExtractor ports.MetricsExtractor, logger logger.StyledLogger) (ports.ProxyService, error) {
		sherpaConfig := &sherpa.Configuration{}
		sherpaConfig.ProxyPrefix = config.GetProxyPrefix()
		sherpaConfig.ConnectionTimeout = config.GetConnectionTimeout()
		sherpaConfig.ConnectionKeepAlive = config.GetConnectionKeepAlive()
		sherpaConfig.ResponseTimeout = config.GetResponseTimeout()
		sherpaConfig.ReadTimeout = config.GetReadTimeout()
		sherpaConfig.StreamBufferSize = config.GetStreamBufferSize()
		sherpaConfig.Profile = config.GetProxyProfile()
		return sherpa.NewService(discovery, selector, sherpaConfig, collector, metricsExtractor, logger)
	})

	// Register Olla implementation
	factory.Register(DefaultProxyOlla, func(discovery ports.DiscoveryService, selector domain.EndpointSelector, config ports.ProxyConfiguration, collector ports.StatsCollector, metricsExtractor ports.MetricsExtractor, logger logger.StyledLogger) (ports.ProxyService, error) {
		ollaConfig := &olla.Configuration{}
		ollaConfig.ProxyPrefix = config.GetProxyPrefix()
		ollaConfig.ConnectionTimeout = config.GetConnectionTimeout()
		ollaConfig.ConnectionKeepAlive = config.GetConnectionKeepAlive()
		ollaConfig.ResponseTimeout = config.GetResponseTimeout()
		ollaConfig.ReadTimeout = config.GetReadTimeout()
		ollaConfig.StreamBufferSize = config.GetStreamBufferSize()
		ollaConfig.Profile = config.GetProxyProfile()

		if ollaSpecific, ok := config.(interface {
			GetMaxIdleConns() int
			GetIdleConnTimeout() time.Duration
			GetMaxConnsPerHost() int
		}); ok {
			ollaConfig.MaxIdleConns = ollaSpecific.GetMaxIdleConns()
			ollaConfig.IdleConnTimeout = ollaSpecific.GetIdleConnTimeout()
			ollaConfig.MaxConnsPerHost = ollaSpecific.GetMaxConnsPerHost()
		} else {
			// fallback option with defaults
			ollaConfig.MaxIdleConns = 200
			ollaConfig.IdleConnTimeout = 90 * time.Second
			ollaConfig.MaxConnsPerHost = 50
		}

		return olla.NewService(discovery, selector, ollaConfig, collector, metricsExtractor, logger)
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
	return creator(discoveryService, selector, config, f.statsCollector, f.metricsExtractor, f.logger)
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
