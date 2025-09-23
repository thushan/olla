package sherpa

import (
	"github.com/thushan/olla/internal/adapter/proxy/config"
	"github.com/thushan/olla/internal/core/constants"
)

// Configuration holds Sherpa proxy settings
type Configuration struct {
	config.BaseProxyConfig
}

// GetProxyProfile overrides base to default to streaming profile for Sherpa
func (c *Configuration) GetProxyProfile() string {
	if c.Profile == "" {
		// Majority of users will use streaming profile
		return constants.ConfigurationProxyProfileStreaming
	}
	return c.Profile
}

// shouldFlush determines if we should flush the response
func (s *Service) shouldFlush(state *streamState) bool {
	return state.isStreaming
}
