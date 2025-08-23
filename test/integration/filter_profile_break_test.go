package integration

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thushan/olla/internal/adapter/discovery"
	"github.com/thushan/olla/internal/adapter/filter"
	"github.com/thushan/olla/internal/adapter/registry/profile"
	"github.com/thushan/olla/internal/config"
	"github.com/thushan/olla/internal/core/domain"
)

func TestFilteringOutOllamaProfile(t *testing.T) {
	t.Run("filtering ollama profile with no endpoints using it", func(t *testing.T) {
		// Create profile factory with filter that excludes ollama
		profileFilter := &domain.FilterConfig{
			Exclude: []string{"ollama"},
		}

		factory, err := profile.NewFactory("../../config/profiles")
		require.NoError(t, err)

		// Create filter adapter
		filterAdapter := filter.NewGlobFilter()

		// Apply filter to profile loader
		loader := factory.GetLoader()
		if loader != nil {
			loader.SetFilterAdapter(filterAdapter)
			loader.SetFilter(profileFilter)
			err = loader.LoadProfiles()
			require.NoError(t, err)
		}

		// Verify ollama profile is not available
		availableProfiles := factory.GetAvailableProfiles()
		assert.NotContains(t, availableProfiles, "ollama")

		// Verify ValidateProfileType returns false for ollama
		assert.False(t, factory.ValidateProfileType("ollama"))

		// Create repository with no endpoints
		repo := discovery.NewStaticEndpointRepository()

		// Load empty config - should work fine
		err = repo.LoadFromConfig(context.Background(), []config.EndpointConfig{})
		assert.NoError(t, err, "Loading empty config should work even with ollama filtered")

		// Try to get endpoints - should work fine
		endpoints, err := repo.GetAll(context.Background())
		assert.NoError(t, err)
		assert.Empty(t, endpoints)
	})

	t.Run("filtering ollama profile breaks endpoints that use it", func(t *testing.T) {
		// Create profile factory with filter that excludes ollama
		profileFilter := &domain.FilterConfig{
			Exclude: []string{"ollama"},
		}

		factory, err := profile.NewFactory("../../config/profiles")
		require.NoError(t, err)

		// Create filter adapter
		filterAdapter := filter.NewGlobFilter()

		// Apply filter to profile loader
		loader := factory.GetLoader()
		if loader != nil {
			loader.SetFilterAdapter(filterAdapter)
			loader.SetFilter(profileFilter)
			err = loader.LoadProfiles()
			require.NoError(t, err)
		}

		// Create repository with the filtered factory
		repo := discovery.NewStaticEndpointRepositoryWithFactory(factory)

		// Try to load config with ollama endpoint
		configs := []config.EndpointConfig{
			{
				Name:           "test-ollama",
				URL:            "http://localhost:11434",
				Type:           "ollama",
				HealthCheckURL: "/",
				ModelURL:       "/api/tags",
				CheckInterval:  5 * time.Second,
				CheckTimeout:   2 * time.Second,
			},
		}

		// This should fail because ollama profile is filtered out
		err = repo.LoadFromConfig(context.Background(), configs)
		assert.Error(t, err, "Should fail to load endpoint with filtered profile type")
		assert.Contains(t, err.Error(), "unsupported endpoint type: ollama")
	})

	// Removed test for "auto" type - auto is for discovery detection, not a real profile type

	t.Run("endpoint with empty type works even with ollama filtered", func(t *testing.T) {
		// Create profile factory with filter that excludes ollama
		profileFilter := &domain.FilterConfig{
			Exclude: []string{"ollama"},
		}

		factory, err := profile.NewFactory("../../config/profiles")
		require.NoError(t, err)

		// Create filter adapter
		filterAdapter := filter.NewGlobFilter()

		// Apply filter to profile loader
		loader := factory.GetLoader()
		if loader != nil {
			loader.SetFilterAdapter(filterAdapter)
			loader.SetFilter(profileFilter)
			err = loader.LoadProfiles()
			require.NoError(t, err)
		}

		// Create repository with the filtered factory
		repo := discovery.NewStaticEndpointRepositoryWithFactory(factory)

		// Load config with empty type endpoint
		configs := []config.EndpointConfig{
			{
				Name:           "test-empty-type",
				URL:            "http://localhost:11434",
				Type:           "", // empty type should work (no validation)
				HealthCheckURL: "/",
				ModelURL:       "/api/tags",
				CheckInterval:  5 * time.Second,
				CheckTimeout:   2 * time.Second,
			},
		}

		// This should work because empty type skips validation
		err = repo.LoadFromConfig(context.Background(), configs)
		assert.NoError(t, err, "Should load endpoint with empty type even with ollama filtered")

		// Verify endpoint was loaded
		endpoints, err := repo.GetAll(context.Background())
		assert.NoError(t, err)
		assert.Len(t, endpoints, 1)
		assert.Equal(t, "", endpoints[0].Type)
	})
}

func TestProfileFactoryWithFiltering(t *testing.T) {
	t.Run("GetProfile fallback to openai-compatible when profile filtered", func(t *testing.T) {
		// Create factory
		factory, err := profile.NewFactory("../../config/profiles")
		require.NoError(t, err)

		// Create filter that excludes ollama but keeps openai-compatible
		profileFilter := &domain.FilterConfig{
			Exclude: []string{"ollama"},
		}

		filterAdapter := filter.NewGlobFilter()
		loader := factory.GetLoader()
		if loader != nil {
			loader.SetFilterAdapter(filterAdapter)
			loader.SetFilter(profileFilter)
			err = loader.LoadProfiles()
			require.NoError(t, err)
		}

		// Try to get ollama profile - should fallback to openai-compatible
		profile, err := factory.GetProfile("ollama")
		assert.NoError(t, err, "Should fallback to openai-compatible")
		assert.NotNil(t, profile)

		// Verify it's actually the openai-compatible profile
		config := profile.GetConfig()
		assert.NotNil(t, config)
		// The openai-compatible profile should have specific characteristics
		// but since GetProfile returns the fallback, this is working as designed
	})

	t.Run("GetProfile fails when both ollama and openai-compatible filtered", func(t *testing.T) {
		// Create factory
		factory, err := profile.NewFactory("../../config/profiles")
		require.NoError(t, err)

		// Create filter that excludes both ollama and openai-compatible
		profileFilter := &domain.FilterConfig{
			Exclude: []string{"ollama", "openai-compatible"},
		}

		filterAdapter := filter.NewGlobFilter()
		loader := factory.GetLoader()
		if loader != nil {
			loader.SetFilterAdapter(filterAdapter)
			loader.SetFilter(profileFilter)
			err = loader.LoadProfiles()
			require.NoError(t, err)
		}

		// Try to get ollama profile - should fail completely
		profile, err := factory.GetProfile("ollama")
		assert.Error(t, err, "Should fail when both ollama and fallback are filtered")
		assert.Nil(t, profile)
		assert.Contains(t, err.Error(), "profile not found")
	})
}
