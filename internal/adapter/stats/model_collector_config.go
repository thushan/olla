package stats

import "time"

// ModelCollectorConfig provides configuration options for the ModelCollector
type ModelCollectorConfig struct {

	// PercentileTrackerType specifies which percentile tracker to use ("reservoir", "simple", default: "reservoir")
	PercentileTrackerType string

	// MaxTrackedModels is the maximum number of models to track (default: 50)
	MaxTrackedModels int

	// ModelStatsTTL is how long to keep model statistics (default: 6h)
	ModelStatsTTL time.Duration

	// ModelCleanupInterval is how often to clean up old data (default: 30m)
	ModelCleanupInterval time.Duration

	// MaxUniqueClientsPerModel is the maximum number of unique clients to track per model (default: 100)
	MaxUniqueClientsPerModel int

	// PercentileSampleSize is the size of the reservoir for percentile calculation (default: 100)
	PercentileSampleSize int

	// ClientIPRetentionTime is how long to keep client IP records (default: 1h)
	ClientIPRetentionTime time.Duration

	// EnableDetailedStats enables detailed model-endpoint statistics tracking (default: false)
	EnableDetailedStats bool
}

// DefaultModelCollectorConfig returns the default configuration
func DefaultModelCollectorConfig() *ModelCollectorConfig {
	return &ModelCollectorConfig{
		MaxTrackedModels:         50,               // Reduced from 100
		ModelStatsTTL:            6 * time.Hour,    // Reduced from 24h
		ModelCleanupInterval:     30 * time.Minute, // More frequent cleanup
		MaxUniqueClientsPerModel: 100,
		PercentileSampleSize:     100,
		EnableDetailedStats:      false,
		PercentileTrackerType:    "reservoir",
		ClientIPRetentionTime:    1 * time.Hour,
	}
}

// Validate ensures the configuration values are reasonable
func (c *ModelCollectorConfig) Validate() *ModelCollectorConfig {
	if c.MaxTrackedModels <= 0 {
		c.MaxTrackedModels = 50
	}
	if c.ModelStatsTTL <= 0 {
		c.ModelStatsTTL = 6 * time.Hour
	}
	if c.ModelCleanupInterval <= 0 {
		c.ModelCleanupInterval = 30 * time.Minute
	}
	if c.MaxUniqueClientsPerModel <= 0 {
		c.MaxUniqueClientsPerModel = 100
	}
	if c.PercentileSampleSize <= 0 {
		c.PercentileSampleSize = 100
	}
	if c.PercentileTrackerType == "" {
		c.PercentileTrackerType = "reservoir"
	}
	if c.ClientIPRetentionTime <= 0 {
		c.ClientIPRetentionTime = 1 * time.Hour
	}
	return c
}
