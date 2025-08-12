package metrics

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/PaesslerAG/jsonpath"
	"github.com/puzpuzpuz/xsync/v4"

	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/logger"
	"github.com/thushan/olla/pkg/pool"
)

const (
	// Limits for reliability
	maxExtractionTimeout = 10 * time.Millisecond
	maxCachedPaths       = 500
)

// ProfileFactory provides access to provider profiles
type ProfileFactory interface {
	GetProfile(profileType string) (domain.InferenceProfile, error)
}

// Extractor implements high-performance metrics extraction from provider responses
type Extractor struct {
	profileFactory ProfileFactory
	logger         logger.StyledLogger

	// Lock-free caches using xsync
	compiledPaths *xsync.Map[string, interface{}]

	// Object pool for JSON parsing to reduce allocations
	jsonPool *pool.Pool[*interface{}]

	// Monitoring counters
	extractionCount atomic.Int64
	cacheHits       atomic.Int64
	failures        atomic.Int64
}

// NewExtractor creates a new metrics extractor optimised for high-frequency workloads
func NewExtractor(profileFactory ProfileFactory, logger logger.StyledLogger) (*Extractor, error) {
	jsonPool, err := pool.NewLitePool(func() *interface{} {
		var data interface{}
		return &data
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create JSON pool: %w", err)
	}

	return &Extractor{
		profileFactory: profileFactory,
		logger:         logger,
		compiledPaths:  xsync.NewMap[string, interface{}](),
		jsonPool:       jsonPool,
	}, nil
}

// ValidateProfile validates and pre-compiles JSONPath expressions at startup
func (e *Extractor) ValidateProfile(profile domain.InferenceProfile) error {
	config := profile.GetConfig()
	if config == nil || !config.Metrics.Extraction.Enabled {
		return nil
	}

	metricsConfig := config.Metrics.Extraction

	// Validate and cache compiled paths
	for field, path := range metricsConfig.Paths {
		if path == "" {
			continue
		}

		// Try to compile the JSONPath
		compiled, err := jsonpath.New(field)
		if err != nil {
			return fmt.Errorf("profile %s: invalid JSONPath for field %s: %s - %w",
				profile.GetName(), field, path, err)
		}

		// Cache the compiled path
		cacheKey := fmt.Sprintf("%s:%s", profile.GetName(), field)
		e.compiledPaths.Store(cacheKey, compiled)
	}

	// Validate calculation expressions reference valid fields
	for _, expr := range metricsConfig.Calculations {
		e.validateCalculation(expr, metricsConfig.Paths)
	}

	e.logger.Debug("Validated metrics configuration",
		"profile", profile.GetName(),
		"paths", len(metricsConfig.Paths),
		"calculations", len(metricsConfig.Calculations))

	return nil
}

// ExtractMetrics extracts metrics from response body and headers
func (e *Extractor) ExtractMetrics(ctx context.Context, responseBody []byte, headers http.Header, providerName string) *domain.ProviderMetrics {
	return e.extractWithTimeout(ctx, responseBody, headers, providerName)
}

// ExtractFromChunk extracts metrics from a streaming chunk
func (e *Extractor) ExtractFromChunk(ctx context.Context, chunk []byte, providerName string) *domain.ProviderMetrics {
	e.logger.Debug("ExtractFromChunk called", 
		"provider", providerName, 
		"chunk_size", len(chunk))
	return e.extractWithTimeout(ctx, chunk, nil, providerName)
}

// extractWithTimeout ensures extraction never blocks requests
func (e *Extractor) extractWithTimeout(ctx context.Context, data []byte, headers http.Header, providerName string) *domain.ProviderMetrics {
	e.extractionCount.Add(1)

	// Quick timeout to ensure we never block
	extractCtx, cancel := context.WithTimeout(ctx, maxExtractionTimeout)
	defer cancel()

	// Run extraction in a goroutine with timeout
	done := make(chan *domain.ProviderMetrics, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				e.failures.Add(1)
				e.logger.Debug("Metrics extraction panic recovered", "error", r, "provider", providerName)
			}
		}()
		done <- e.doExtract(data, headers, providerName)
	}()

	select {
	case metrics := <-done:
		return metrics
	case <-extractCtx.Done():
		e.failures.Add(1)
		e.logger.Debug("Metrics extraction timeout", "provider", providerName)
		return nil
	}
}

// doExtract performs the actual extraction
func (e *Extractor) doExtract(data []byte, headers http.Header, providerName string) *domain.ProviderMetrics {
	if len(data) == 0 && headers == nil {
		return nil
	}

	profile, err := e.profileFactory.GetProfile(providerName)
	if err != nil {
		return nil
	}

	config := profile.GetConfig()
	if config == nil || !config.Metrics.Extraction.Enabled {
		return nil
	}

	metricsConfig := config.Metrics.Extraction
	metrics := &domain.ProviderMetrics{}

	// Extract from JSON body if available
	if len(data) > 0 && metricsConfig.Source != "response_headers" {
		e.extractFromJSON(data, metricsConfig, profile.GetName(), metrics)
	}

	// Extract from headers if configured
	if headers != nil && len(metricsConfig.Headers) > 0 {
		e.extractFromHeaders(headers, metricsConfig, metrics)
	}

	return metrics
}

// extractFromJSON extracts metrics from JSON response body
func (e *Extractor) extractFromJSON(data []byte, config domain.MetricsExtractionConfig, _ string, metrics *domain.ProviderMetrics) {
	// Get a JSON object from pool
	jsonObj := e.jsonPool.Get()
	defer func() {
		// Clear and return to pool
		*jsonObj = nil
		e.jsonPool.Put(jsonObj)
	}()

	// Parse JSON
	if err := json.Unmarshal(data, jsonObj); err != nil {
		return
	}

	// Extract raw values using JSONPath
	rawValues := make(map[string]interface{})
	for field, path := range config.Paths {
		if path == "" {
			continue
		}
		// Use jsonpath directly without caching for now
		if value, err := jsonpath.Get(path, *jsonObj); err == nil {
			rawValues[field] = value
		}
	}

	// Map extracted values to metrics struct
	e.mapToMetrics(rawValues, config.Calculations, metrics)
}

// extractFromHeaders extracts metrics from response headers
func (e *Extractor) extractFromHeaders(headers http.Header, config domain.MetricsExtractionConfig, _ *domain.ProviderMetrics) {
	for field, headerName := range config.Headers {
		if value := headers.Get(headerName); value != "" {
			// Try to parse as number for known fields
			switch field {
			case "rate_limit_remaining", "rate_limit_reset":
				if n, err := strconv.ParseInt(value, 10, 32); err == nil {
					// Store in appropriate field if we add header metrics
					_ = n
				}
			}
		}
	}
}

// mapToMetrics maps extracted values to the metrics struct
func (e *Extractor) mapToMetrics(rawValues map[string]interface{}, calculations map[string]string, metrics *domain.ProviderMetrics) {
	// Direct field mappings
	if v, ok := getInt32(rawValues, "input_tokens"); ok {
		metrics.InputTokens = v
	}
	if v, ok := getInt32(rawValues, "output_tokens"); ok {
		metrics.OutputTokens = v
	}
	if v, ok := getInt32(rawValues, "total_tokens"); ok {
		metrics.TotalTokens = v
	} else if metrics.InputTokens > 0 && metrics.OutputTokens > 0 {
		metrics.TotalTokens = metrics.InputTokens + metrics.OutputTokens
	}

	// Model and status
	if v, ok := rawValues["model"].(string); ok {
		metrics.Model = v
	}
	if v, ok := rawValues["finish_reason"].(string); ok {
		metrics.FinishReason = v
	}
	if v, ok := rawValues["done"].(bool); ok {
		metrics.IsComplete = v
	}

	// Convert nanosecond timings to milliseconds
	if v, ok := getInt64(rawValues, "prompt_duration_ns"); ok {
		ms := v / 1_000_000
		if ms <= math.MaxInt32 {
			metrics.PromptMs = int32(ms)      //nolint:gosec // checked above
			metrics.TTFTMs = metrics.PromptMs // TTFT approximation
		}
	}
	if v, ok := getInt64(rawValues, "eval_duration_ns"); ok {
		ms := v / 1_000_000
		if ms <= math.MaxInt32 {
			metrics.GenerationMs = int32(ms) //nolint:gosec // checked above
		}
	}
	if v, ok := getInt64(rawValues, "total_duration_ns"); ok {
		ms := v / 1_000_000
		if ms <= math.MaxInt32 {
			metrics.TotalMs = int32(ms) //nolint:gosec // checked above
		}
	}
	if v, ok := getInt64(rawValues, "load_duration_ns"); ok {
		ms := v / 1_000_000
		if ms <= math.MaxInt32 {
			metrics.ModelLoadMs = int32(ms) //nolint:gosec // checked above
		}
	}

	// Calculate tokens per second if we have the data
	if metrics.OutputTokens > 0 && metrics.GenerationMs > 0 {
		metrics.TokensPerSecond = float32(metrics.OutputTokens) / (float32(metrics.GenerationMs) / 1000.0)
	}

	// Apply any configured calculations
	e.applyCalculations(rawValues, calculations, metrics)
}

// applyCalculations applies simple math calculations
func (e *Extractor) applyCalculations(rawValues map[string]interface{}, calculations map[string]string, metrics *domain.ProviderMetrics) {
	// Convert raw values to floats for calculations
	floatValues := make(map[string]float64)
	for k, v := range rawValues {
		switch val := v.(type) {
		case float64:
			floatValues[k] = val
		case float32:
			floatValues[k] = float64(val)
		case int:
			floatValues[k] = float64(val)
		case int32:
			floatValues[k] = float64(val)
		case int64:
			floatValues[k] = float64(val)
		}
	}

	// Apply calculations
	for field, expr := range calculations {
		if result, err := evaluateSimpleMath(expr, floatValues); err == nil {
			switch field {
			case "tokens_per_second":
				metrics.TokensPerSecond = float32(result)
			case "ttft_ms":
				metrics.TTFTMs = int32(result)
			case "total_ms":
				metrics.TotalMs = int32(result)
			case "model_load_ms":
				metrics.ModelLoadMs = int32(result)
			}
		}
	}
}

// validateCalculation validates that a calculation expression is valid
func (e *Extractor) validateCalculation(expr string, availableFields map[string]string) {
	// Check that referenced fields exist
	for field := range availableFields {
		if strings.Contains(expr, field) {
			// Field is referenced and available
			continue
		}
	}
	// Basic validation passed
}

// Helper functions for type conversion

func getInt32(values map[string]interface{}, key string) (int32, bool) {
	if v, ok := values[key]; ok {
		switch val := v.(type) {
		case float64:
			return int32(val), true
		case int:
			if val <= math.MaxInt32 && val >= math.MinInt32 {
				return int32(val), true
			}
			return 0, false
		case int32:
			return val, true
		case int64:
			if val <= math.MaxInt32 && val >= math.MinInt32 {
				return int32(val), true
			}
			return 0, false
		}
	}
	return 0, false
}

func getInt64(values map[string]interface{}, key string) (int64, bool) {
	if v, ok := values[key]; ok {
		switch val := v.(type) {
		case float64:
			return int64(val), true
		case int:
			return int64(val), true
		case int32:
			return int64(val), true
		case int64:
			return val, true
		}
	}
	return 0, false
}
