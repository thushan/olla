package sherpa

import (
	"github.com/thushan/olla/internal/core/constants"
	"time"
)

// Configuration holds proxy settings
type Configuration struct {
	ProxyPrefix         string
	Profile             string
	ConnectionTimeout   time.Duration
	ConnectionKeepAlive time.Duration
	ResponseTimeout     time.Duration
	ReadTimeout         time.Duration
	StreamBufferSize    int
}

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

func (c *Configuration) GetProxyPrefix() string {
	if c.ProxyPrefix == "" {
		return constants.DefaultOllaProxyPathPrefix
	}
	return c.ProxyPrefix
}

func (c *Configuration) GetConnectionTimeout() time.Duration {
	if c.ConnectionTimeout == 0 {
		return DefaultTimeout
	}
	return c.ConnectionTimeout
}

func (c *Configuration) GetConnectionKeepAlive() time.Duration {
	if c.ConnectionKeepAlive == 0 {
		return DefaultKeepAlive
	}
	return c.ConnectionKeepAlive
}

func (c *Configuration) GetResponseTimeout() time.Duration {
	return c.ResponseTimeout
}

func (c *Configuration) GetReadTimeout() time.Duration {
	if c.ReadTimeout == 0 {
		return DefaultReadTimeout
	}
	return c.ReadTimeout
}

func (c *Configuration) GetStreamBufferSize() int {
	if c.StreamBufferSize == 0 {
		return DefaultStreamBufferSize
	}
	return c.StreamBufferSize
}
