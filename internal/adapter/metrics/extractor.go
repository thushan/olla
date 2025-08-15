package metrics

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/vm"
	"github.com/tidwall/gjson"

	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/logger"
	"github.com/thushan/olla/internal/util"
)

const (
	// maxExtractionTimeout limits extraction time to prevent blocking
	maxExtractionTimeout = 10 * time.Millisecond
)

// ProfileFactory provides access to provider profiles
type ProfileFactory interface {
	GetProfile(profileType string) (domain.InferenceProfile, error)
}

// Extractor implements high-performance metrics extraction using robust libraries
type Extractor struct {
	profileFactory ProfileFactory
	logger         logger.StyledLogger

	// Pre-compiled expressions cache
	compiledExprs sync.Map // map[string]*vm.Program

	// Pre-parsed field paths cache
	fieldPaths sync.Map // map[string]string - profile:field -> gjson path

	// Track which profiles have been validated
	validatedProfiles sync.Map // map[string]bool

	// Monitoring counters
	extractionCount atomic.Int64
	failures        atomic.Int64
}

// NewExtractor creates a new metrics extractor using robust libraries
func NewExtractor(profileFactory ProfileFactory, logger logger.StyledLogger) (*Extractor, error) {
	return &Extractor{
		profileFactory: profileFactory,
		logger:         logger,
	}, nil
}

// ValidateProfile validates and pre-compiles expressions at startup
func (e *Extractor) ValidateProfile(profile domain.InferenceProfile) error {
	config := profile.GetConfig()
	if config == nil || !config.Metrics.Extraction.Enabled {
		return nil
	}

	metricsConfig := config.Metrics.Extraction

	// Cache field paths for gjson
	for field, path := range metricsConfig.Paths {
		if path == "" {
			continue
		}

		// Convert JSONPath to gjson syntax if needed
		gjsonPath := convertToGjsonPath(path)
		cacheKey := fmt.Sprintf("%s:%s", profile.GetName(), field)
		e.fieldPaths.Store(cacheKey, gjsonPath)
	}

	// Pre-compile calculation expressions using expr library
	for field, expression := range metricsConfig.Calculations {
		if expression == "" {
			continue
		}

		// Create environment with expected variables
		env := map[string]interface{}{
			"input_tokens":       0,
			"output_tokens":      0,
			"eval_duration_ns":   0,
			"prompt_duration_ns": 0,
			"total_duration_ns":  0,
			"load_duration_ns":   0,
		}

		// Add all path fields as potential variables
		for fieldName := range metricsConfig.Paths {
			env[fieldName] = 0
		}

		// Compile the expression
		program, err := expr.Compile(expression,
			expr.Env(env),
			expr.AsFloat64(), // Ensure output is float64
		)
		if err != nil {
			return fmt.Errorf("profile %s: invalid calculation for field %s: %s - %w",
				profile.GetName(), field, expression, err)
		}

		cacheKey := fmt.Sprintf("%s:%s", profile.GetName(), field)
		e.compiledExprs.Store(cacheKey, program)
	}

	e.logger.Debug("Validated metrics configuration with robust libraries",
		"profile", profile.GetName(),
		"paths", len(metricsConfig.Paths),
		"calculations", len(metricsConfig.Calculations))

	return nil
}

// ExtractMetrics extracts metrics from response data and headers
func (e *Extractor) ExtractMetrics(ctx context.Context, data []byte, headers http.Header, providerName string) *domain.ProviderMetrics {
	if len(data) == 0 && headers == nil {
		return nil
	}

	e.extractionCount.Add(1)

	// Create extraction context with timeout
	extractCtx, cancel := context.WithTimeout(ctx, maxExtractionTimeout)
	defer cancel()

	done := make(chan *domain.ProviderMetrics, 1)

	go func() {
		metrics := e.doExtract(data, headers, providerName)
		select {
		case done <- metrics:
		case <-extractCtx.Done():
		}
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

// ExtractFromChunk extracts metrics from a response chunk
func (e *Extractor) ExtractFromChunk(ctx context.Context, chunk []byte, providerName string) *domain.ProviderMetrics {
	return e.ExtractMetrics(ctx, chunk, nil, providerName)
}

// doExtract performs the actual extraction using gjson for speed
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

	// Ensure profile is validated (only validate once per profile)
	profileName := profile.GetName()
	if _, validated := e.validatedProfiles.Load(profileName); !validated {
		if err := e.ValidateProfile(profile); err == nil {
			e.validatedProfiles.Store(profileName, true)
		}
	}

	metricsConfig := config.Metrics.Extraction
	metrics := &domain.ProviderMetrics{}

	// Parse JSON once using gjson (much faster than encoding/json)
	var jsonResult gjson.Result
	if len(data) > 0 && metricsConfig.Source != "response_headers" {
		jsonResult = gjson.ParseBytes(data)
		if !jsonResult.Exists() {
			return nil
		}
	}

	// Extract values using cached gjson paths
	values := make(map[string]interface{})
	for field := range metricsConfig.Paths {
		cacheKey := fmt.Sprintf("%s:%s", profile.GetName(), field)
		if pathObj, ok := e.fieldPaths.Load(cacheKey); ok {
			path, ok := pathObj.(string)
			if !ok {
				continue
			}
			result := jsonResult.Get(path)
			if result.Exists() {
				// Store the raw value for calculations
				values[field] = result.Value()

				// Also map directly to metrics fields
				e.mapFieldToMetrics(field, result, metrics)
			}
		}
	}

	// Execute pre-compiled calculations
	e.runCalculations(profile.GetName(), values, metricsConfig.Calculations, metrics)

	// Extract from headers if configured
	if headers != nil && len(metricsConfig.Headers) > 0 {
		e.extractFromHeaders(headers, metricsConfig, metrics)
	}

	return metrics
}

// mapFieldToMetrics maps extracted field values directly to metrics struct
func (e *Extractor) mapFieldToMetrics(field string, result gjson.Result, metrics *domain.ProviderMetrics) {
	switch field {
	case "input_tokens":
		metrics.InputTokens = util.SafeInt32(result.Int())
	case "output_tokens":
		metrics.OutputTokens = util.SafeInt32(result.Int())
	case "total_tokens":
		metrics.TotalTokens = util.SafeInt32(result.Int())
	case "model":
		metrics.Model = result.String()
	case "finish_reason":
		metrics.FinishReason = result.String()
	case "done":
		metrics.IsComplete = result.Bool()
	case "prompt_duration_ns", "prompt_eval_duration":
		ms := result.Int() / 1_000_000
		metrics.PromptMs = util.SafeInt32(ms)
		metrics.TTFTMs = metrics.PromptMs // TTFT approximation
	case "eval_duration_ns", "eval_duration":
		ms := result.Int() / 1_000_000
		metrics.GenerationMs = util.SafeInt32(ms)
	case "total_duration_ns":
		ms := result.Int() / 1_000_000
		metrics.TotalMs = util.SafeInt32(ms)
	case "load_duration_ns":
		ms := result.Int() / 1_000_000
		metrics.ModelLoadMs = util.SafeInt32(ms)
	}
}

// runCalculations executes pre-compiled expr expressions
func (e *Extractor) runCalculations(profileName string, values map[string]interface{}, calculations map[string]string, metrics *domain.ProviderMetrics) {
	// Convert all values to float64 for calculations
	env := make(map[string]interface{})
	for k, v := range values {
		switch val := v.(type) {
		case float64:
			env[k] = val
		case float32:
			env[k] = float64(val)
		case int:
			env[k] = float64(val)
		case int32:
			env[k] = float64(val)
		case int64:
			env[k] = float64(val)
		default:
			env[k] = 0.0
		}
	}

	// Execute each pre-compiled calculation
	for field := range calculations {
		cacheKey := fmt.Sprintf("%s:%s", profileName, field)
		if programObj, ok := e.compiledExprs.Load(cacheKey); ok {
			program, ok := programObj.(*vm.Program)
			if !ok {
				continue
			}

			result, err := expr.Run(program, env)
			if err == nil {
				if val, ok := result.(float64); ok {
					switch field {
					case "tokens_per_second":
						metrics.TokensPerSecond = float32(val)
					case "ttft_ms":
						metrics.TTFTMs = util.SafeInt32(int64(val))
					case "total_ms":
						metrics.TotalMs = util.SafeInt32(int64(val))
					case "model_load_ms":
						metrics.ModelLoadMs = util.SafeInt32(int64(val))
					}
				}
			}
		}
	}

	// Calculate tokens per second if we have the data and no calculation provided
	if metrics.TokensPerSecond == 0 && metrics.OutputTokens > 0 && metrics.GenerationMs > 0 {
		metrics.TokensPerSecond = float32(metrics.OutputTokens) / (float32(metrics.GenerationMs) / 1000.0)
	}

	// Ensure total tokens is set if we have input and output
	if metrics.TotalTokens == 0 && metrics.InputTokens > 0 && metrics.OutputTokens > 0 {
		metrics.TotalTokens = metrics.InputTokens + metrics.OutputTokens
	}
}

// extractFromHeaders extracts metrics from response headers
func (e *Extractor) extractFromHeaders(headers http.Header, config domain.MetricsExtractionConfig, _ *domain.ProviderMetrics) {
	// Implementation remains similar
	for field, headerName := range config.Headers {
		if value := headers.Get(headerName); value != "" {
			// Parse and store header values as needed
			_ = field // TODO: Map header fields to metrics
		}
	}
}

// convertToGjsonPath converts JSONPath to gjson syntax
// gjson uses simpler dot notation: $.foo.bar becomes foo.bar
func convertToGjsonPath(jsonPath string) string {
	if jsonPath == "$" || jsonPath == "" {
		return ""
	}

	s := jsonPath

	// remove the leasing "$." or "$"
	if strings.HasPrefix(s, "$.") {
		s = s[2:]
	} else if strings.HasPrefix(s, "$") {
		s = s[1:]
	}

	// convert bracket array indices: foo[0].bar -> foo.0.bar
	s = regexp.MustCompile(`\[(\d+)\]`).ReplaceAllString(s, `.$1`)

	// convert bracket keys: ['field'] -> .field
	s = regexp.MustCompile(`\['([^']+)'\]`).ReplaceAllString(s, `.$1`)

	// trim any accidental leading dots, this sometimes happens in our lab
	s = strings.TrimPrefix(s, ".")

	return s
}

// GetStats returns extraction statistics
func (e *Extractor) GetStats() (int64, int64) {
	return e.extractionCount.Load(), e.failures.Load()
}
