package factory

import (
	"net/http"
	"time"
)

type SharedClientFactory struct {
	healthClient    *http.Client
	discoveryClient *http.Client
}

const (
	HealthCheckTimeout = 5 * time.Second
	DiscoveryTimeout   = 30 * time.Second
)

func NewSharedClientFactory() *SharedClientFactory {
	sharedTransport := &http.Transport{
		MaxIdleConns:        10,
		MaxIdleConnsPerHost: 5,
		IdleConnTimeout:     30 * time.Second,
		TLSHandshakeTimeout: 10 * time.Second,
		DisableCompression:  false,
	}

	return &SharedClientFactory{
		healthClient: &http.Client{
			Timeout:   HealthCheckTimeout,
			Transport: sharedTransport,
		},
		discoveryClient: &http.Client{
			Timeout:   DiscoveryTimeout,
			Transport: sharedTransport,
		},
	}
}

func (f *SharedClientFactory) GetHealthClient() *http.Client {
	return f.healthClient
}

func (f *SharedClientFactory) GetDiscoveryClient() *http.Client {
	return f.discoveryClient
}
