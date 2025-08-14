package discovery

import "github.com/thushan/olla/internal/core/domain"

// TestStaticEndpointRepository provides test-specific repository extensions
type TestStaticEndpointRepository struct {
	*StaticEndpointRepository
}

// NewTestStaticEndpointRepository creates a repository with test helpers
func NewTestStaticEndpointRepository() *TestStaticEndpointRepository {
	return &TestStaticEndpointRepository{
		StaticEndpointRepository: NewStaticEndpointRepository(),
	}
}

// AddTestEndpoint bypasses normal validation for test fixture creation
func (r *TestStaticEndpointRepository) AddTestEndpoint(endpoint *domain.Endpoint) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if endpoint.URL != nil {
		key := endpoint.URL.String()
		r.endpoints[key] = endpoint
	}
}
