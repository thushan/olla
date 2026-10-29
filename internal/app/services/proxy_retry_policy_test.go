package services

import (
	"testing"

	proxyconfig "github.com/thushan/olla/internal/adapter/proxy/config"
	appconfig "github.com/thushan/olla/internal/config"
)

// The YAML-facing RetryConfig is positive (enabled, non_idempotent) while the
// engine policy is inverted so its zero value is today's behaviour; this pins
// the mapping between them.
func TestCreateProxyConfiguration_RetryPolicy(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   appconfig.RetryConfig
		want proxyconfig.RetryPolicy
	}{
		{name: "defaults", in: appconfig.DefaultConfig().Proxy.Retry, want: proxyconfig.RetryPolicy{}},
		{name: "disabled", in: appconfig.RetryConfig{Enabled: false, NonIdempotent: true}, want: proxyconfig.RetryPolicy{Disabled: true}},
		{name: "capped", in: appconfig.RetryConfig{Enabled: true, MaxAttempts: 2, NonIdempotent: true}, want: proxyconfig.RetryPolicy{MaxAttempts: 2}},
		{name: "no non-idempotent", in: appconfig.RetryConfig{Enabled: true}, want: proxyconfig.RetryPolicy{NoNonIdempotent: true}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg := appconfig.DefaultConfig()
			cfg.Proxy.Retry = tt.in
			got := NewProxyServiceWrapper(&cfg.Proxy, newTestLogger()).createProxyConfiguration().GetRetryPolicy()
			if got != tt.want {
				t.Errorf("policy = %+v, want %+v", got, tt.want)
			}
		})
	}
}
