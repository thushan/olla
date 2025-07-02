package services

import (
	"fmt"
)

// ServiceRegistry facilitates runtime service discovery and dependency injection
// after the registration phase completes.
type ServiceRegistry struct {
	services map[string]ManagedService
}

// NewServiceRegistry creates a new service registry
func NewServiceRegistry() *ServiceRegistry {
	return &ServiceRegistry{
		services: make(map[string]ManagedService),
	}
}

func (r *ServiceRegistry) Register(name string, service ManagedService) {
	r.services[name] = service
}

func (r *ServiceRegistry) Get(name string) (ManagedService, error) {
	service, exists := r.services[name]
	if !exists {
		return nil, fmt.Errorf("service %s not found", name)
	}
	return service, nil
}

func (r *ServiceRegistry) GetStats() (*StatsService, error) {
	service, err := r.Get("stats")
	if err != nil {
		return nil, err
	}
	stats, ok := service.(*StatsService)
	if !ok {
		return nil, fmt.Errorf("service stats is not a StatsService")
	}
	return stats, nil
}

func (r *ServiceRegistry) GetSecurity() (*SecurityService, error) {
	service, err := r.Get("security")
	if err != nil {
		return nil, err
	}
	security, ok := service.(*SecurityService)
	if !ok {
		return nil, fmt.Errorf("service security is not a SecurityService")
	}
	return security, nil
}

func (r *ServiceRegistry) GetDiscovery() (*DiscoveryService, error) {
	service, err := r.Get("discovery")
	if err != nil {
		return nil, err
	}
	discovery, ok := service.(*DiscoveryService)
	if !ok {
		return nil, fmt.Errorf("service discovery is not a DiscoveryService")
	}
	return discovery, nil
}

func (r *ServiceRegistry) GetProxy() (*ProxyServiceWrapper, error) {
	service, err := r.Get("proxy")
	if err != nil {
		return nil, err
	}
	proxy, ok := service.(*ProxyServiceWrapper)
	if !ok {
		return nil, fmt.Errorf("service proxy is not a ProxyServiceWrapper")
	}
	return proxy, nil
}

func (r *ServiceRegistry) GetHTTP() (*HTTPService, error) {
	service, err := r.Get("http")
	if err != nil {
		return nil, err
	}
	http, ok := service.(*HTTPService)
	if !ok {
		return nil, fmt.Errorf("service http is not a HTTPService")
	}
	return http, nil
}
