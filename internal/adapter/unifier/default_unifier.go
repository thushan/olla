package unifier

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/puzpuzpuz/xsync/v4"
	"github.com/thushan/olla/internal/core/domain"
	"github.com/thushan/olla/internal/core/ports"
	"github.com/thushan/olla/internal/logger"
)

// DefaultUnifier implements the ModelUnifier interface with rule-based unification
type DefaultUnifier struct {
	logger        logger.StyledLogger
	rules         map[string][]ports.UnificationRule       // Platform-specific rules
	unifiedModels *xsync.Map[string, *domain.UnifiedModel] // ID -> UnifiedModel
	aliasIndex    *xsync.Map[string, string]               // Alias -> UnifiedID
	normalizer    ports.ModelNormalizer
	detector      ports.PlatformDetector
	stats         unifierStats
	mu            sync.RWMutex
}

type unifierStats struct {
	totalUnified      atomic.Int64
	totalErrors       atomic.Int64
	cacheHits         atomic.Int64
	cacheMisses       atomic.Int64
	lastUnificationAt atomic.Value // time.Time
	totalUnifyTimeNs  atomic.Int64
	unificationCount  atomic.Int64
}

// NewDefaultUnifier creates a new default unifier implementation
func NewDefaultUnifier(logger logger.StyledLogger) *DefaultUnifier {
	u := &DefaultUnifier{
		logger:        logger,
		rules:         make(map[string][]ports.UnificationRule),
		unifiedModels: xsync.NewMap[string, *domain.UnifiedModel](),
		aliasIndex:    xsync.NewMap[string, string](),
		normalizer:    NewModelNormalizer(),
		detector:      NewPlatformDetector(),
	}

	// Register default rules
	u.registerDefaultRules()

	return u
}

// UnifyModel converts a platform-specific model to unified format
func (u *DefaultUnifier) UnifyModel(ctx context.Context, sourceModel *domain.ModelInfo, endpointURL string) (*domain.UnifiedModel, error) {
	start := time.Now()
	defer func() {
		u.stats.totalUnifyTimeNs.Add(time.Since(start).Nanoseconds())
		u.stats.unificationCount.Add(1)
		u.stats.lastUnificationAt.Store(time.Now())
	}()

	if sourceModel == nil {
		u.stats.totalErrors.Add(1)
		return nil, domain.NewUnificationError("unify", "unknown", "", fmt.Errorf("source model is nil"))
	}

	// Detect platform type
	platformType := u.detector.DetectPlatform(sourceModel)

	// Apply platform-specific rules
	unified, err := u.applyRules(sourceModel, platformType)
	if err != nil {
		u.stats.totalErrors.Add(1)
		return nil, domain.NewUnificationError("apply_rules", platformType, sourceModel.Name, err)
	}

	// Add source endpoint information
	endpoint := domain.SourceEndpoint{
		EndpointURL: endpointURL,
		NativeName:  sourceModel.Name,
		State:       "unknown",
		LastSeen:    time.Now(),
		DiskSize:    sourceModel.Size,
	}

	if sourceModel.Details != nil && sourceModel.Details.State != nil {
		endpoint.State = *sourceModel.Details.State
	}

	unified.AddOrUpdateEndpoint(endpoint)
	unified.LastSeen = time.Now()
	unified.DiskSize = unified.GetTotalDiskSize()

	// Generate aliases
	aliases := u.normalizer.GenerateAliases(unified, platformType, sourceModel.Name)
	unified.Aliases = append(unified.Aliases, aliases...)

	// Cache the unified model
	u.cacheUnifiedModel(unified)
	u.stats.totalUnified.Add(1)

	return unified, nil
}

// UnifyModels batch processes multiple models for efficiency
func (u *DefaultUnifier) UnifyModels(ctx context.Context, sourceModels []*domain.ModelInfo, endpointURL string) ([]*domain.UnifiedModel, error) {
	if len(sourceModels) == 0 {
		return []*domain.UnifiedModel{}, nil
	}

	// Process models concurrently but with a limit
	const maxConcurrent = 10
	sem := make(chan struct{}, maxConcurrent)
	results := make([]*domain.UnifiedModel, len(sourceModels))
	errors := make([]error, len(sourceModels))
	var wg sync.WaitGroup

	for i, model := range sourceModels {
		wg.Add(1)
		go func(idx int, m *domain.ModelInfo) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			unified, err := u.UnifyModel(ctx, m, endpointURL)
			results[idx] = unified
			errors[idx] = err
		}(i, model)
	}

	wg.Wait()

	// Check for errors
	var finalResults []*domain.UnifiedModel
	for i, result := range results {
		if errors[i] != nil {
			u.logger.ErrorWithEndpoint(endpointURL, "Failed to unify model", errors[i])
			continue
		}
		if result != nil {
			finalResults = append(finalResults, result)
		}
	}

	return finalResults, nil
}

// ResolveAlias finds unified model by any known alias
func (u *DefaultUnifier) ResolveAlias(ctx context.Context, alias string) (*domain.UnifiedModel, error) {
	// Check alias index first
	if unifiedID, exists := u.aliasIndex.Load(alias); exists {
		u.stats.cacheHits.Add(1)
		if model, exists := u.unifiedModels.Load(unifiedID); exists {
			return model, nil
		}
	}

	u.stats.cacheMisses.Add(1)

	// Search through all models
	var found *domain.UnifiedModel
	u.unifiedModels.Range(func(id string, model *domain.UnifiedModel) bool {
		if model.HasAlias(alias) || model.ID == alias {
			found = model
			return false // Stop iteration
		}
		return true
	})

	if found == nil {
		return nil, fmt.Errorf("no unified model found for alias: %s", alias)
	}

	// Update alias index for faster future lookups
	u.aliasIndex.Store(alias, found.ID)
	return found, nil
}

// GetAliases returns all known aliases for a unified model ID
func (u *DefaultUnifier) GetAliases(ctx context.Context, unifiedID string) ([]string, error) {
	model, exists := u.unifiedModels.Load(unifiedID)
	if !exists {
		return nil, fmt.Errorf("unified model not found: %s", unifiedID)
	}

	return model.Aliases, nil
}

// RegisterCustomRule allows platform-specific unification rules
func (u *DefaultUnifier) RegisterCustomRule(platformType string, rule ports.UnificationRule) error {
	u.mu.Lock()
	defer u.mu.Unlock()

	if rule == nil {
		return fmt.Errorf("rule cannot be nil")
	}

	u.rules[platformType] = append(u.rules[platformType], rule)

	// Sort rules by priority
	u.sortRulesByPriority(platformType)

	return nil
}

// GetStats returns unification performance metrics
func (u *DefaultUnifier) GetStats() domain.UnificationStats {
	lastUnification, _ := u.stats.lastUnificationAt.Load().(time.Time)

	var avgUnifyTime float64
	count := u.stats.unificationCount.Load()
	if count > 0 {
		avgUnifyTime = float64(u.stats.totalUnifyTimeNs.Load()) / float64(count) / 1e6 // Convert to ms
	}

	return domain.UnificationStats{
		TotalUnified:       u.stats.totalUnified.Load(),
		TotalErrors:        u.stats.totalErrors.Load(),
		CacheHits:          u.stats.cacheHits.Load(),
		CacheMisses:        u.stats.cacheMisses.Load(),
		LastUnificationAt:  lastUnification,
		AverageUnifyTimeMs: avgUnifyTime,
	}
}

// MergeUnifiedModels merges models from different endpoints
func (u *DefaultUnifier) MergeUnifiedModels(ctx context.Context, models []*domain.UnifiedModel) (*domain.UnifiedModel, error) {
	if len(models) == 0 {
		return nil, fmt.Errorf("no models to merge")
	}

	if len(models) == 1 {
		return models[0], nil
	}

	// Use the first model as base
	merged := &domain.UnifiedModel{
		ID:               models[0].ID,
		Family:           models[0].Family,
		Variant:          models[0].Variant,
		ParameterSize:    models[0].ParameterSize,
		ParameterCount:   models[0].ParameterCount,
		Quantization:     models[0].Quantization,
		Format:           models[0].Format,
		Aliases:          []string{},
		SourceEndpoints:  []domain.SourceEndpoint{},
		Capabilities:     []string{},
		MaxContextLength: models[0].MaxContextLength,
		Metadata:         make(map[string]interface{}),
	}

	// Merge aliases (deduplicate)
	aliasSet := make(map[string]bool)
	for _, model := range models {
		for _, alias := range model.Aliases {
			aliasSet[alias] = true
		}
	}
	for alias := range aliasSet {
		merged.Aliases = append(merged.Aliases, alias)
	}

	// Merge endpoints
	endpointMap := make(map[string]domain.SourceEndpoint)
	for _, model := range models {
		for _, endpoint := range model.SourceEndpoints {
			endpointMap[endpoint.EndpointURL] = endpoint
		}
	}
	for _, endpoint := range endpointMap {
		merged.SourceEndpoints = append(merged.SourceEndpoints, endpoint)
	}

	// Merge capabilities (deduplicate)
	capSet := make(map[string]bool)
	for _, model := range models {
		for _, cap := range model.Capabilities {
			capSet[cap] = true
		}
	}
	for cap := range capSet {
		merged.Capabilities = append(merged.Capabilities, cap)
	}

	// Update metadata
	merged.LastSeen = time.Now()
	merged.DiskSize = merged.GetTotalDiskSize()

	// Cache the merged model
	u.cacheUnifiedModel(merged)

	return merged, nil
}

// Clear removes all cached unified models
func (u *DefaultUnifier) Clear(ctx context.Context) error {
	u.unifiedModels.Clear()
	u.aliasIndex.Clear()
	return nil
}

// applyRules applies platform-specific rules to unify a model
func (u *DefaultUnifier) applyRules(sourceModel *domain.ModelInfo, platformType string) (*domain.UnifiedModel, error) {
	// Get platform-specific rules
	u.mu.RLock()
	rules := u.rules[platformType]
	u.mu.RUnlock()

	// Try each rule in priority order
	for _, rule := range rules {
		if rule.CanHandle(sourceModel) {
			return rule.Apply(sourceModel)
		}
	}

	// If no specific rule matches, use default unification
	return u.defaultUnification(sourceModel, platformType)
}

// defaultUnification provides fallback unification logic
func (u *DefaultUnifier) defaultUnification(sourceModel *domain.ModelInfo, platformType string) (*domain.UnifiedModel, error) {
	// Extract family and variant
	var platformFamily string
	if sourceModel.Details != nil && sourceModel.Details.Family != nil {
		platformFamily = *sourceModel.Details.Family
	}
	family, variant := u.normalizer.NormalizeFamily(sourceModel.Name, platformFamily)

	// Extract and normalize size
	var sizeStr string
	if sourceModel.Details != nil && sourceModel.Details.ParameterSize != nil {
		sizeStr = *sourceModel.Details.ParameterSize
	}
	normalizedSize, paramCount := u.normalizer.NormalizeSize(sizeStr)

	// Extract and normalize quantization
	var quantStr string
	if sourceModel.Details != nil && sourceModel.Details.QuantizationLevel != nil {
		quantStr = *sourceModel.Details.QuantizationLevel
	}
	normalizedQuant := u.normalizer.NormalizeQuantization(quantStr)

	// Generate canonical ID
	canonicalID := u.normalizer.GenerateCanonicalID(family, variant, normalizedSize, normalizedQuant)

	// Extract format
	format := "unknown"
	if sourceModel.Details != nil && sourceModel.Details.Format != nil {
		format = *sourceModel.Details.Format
	}

	// Extract capabilities
	capabilities := u.inferCapabilities(sourceModel)

	// Extract max context length
	var maxContext *int64
	if sourceModel.Details != nil && sourceModel.Details.MaxContextLength != nil {
		maxContext = sourceModel.Details.MaxContextLength
	}

	// Create metadata
	metadata := make(map[string]interface{})
	if sourceModel.Details != nil {
		if sourceModel.Details.Publisher != nil {
			metadata["publisher"] = *sourceModel.Details.Publisher
		}
		if sourceModel.Details.Type != nil {
			metadata["type"] = *sourceModel.Details.Type
		}
		if sourceModel.Details.Digest != nil {
			metadata["digest"] = *sourceModel.Details.Digest
		}
	}

	return &domain.UnifiedModel{
		ID:               canonicalID,
		Family:           family,
		Variant:          variant,
		ParameterSize:    normalizedSize,
		ParameterCount:   paramCount,
		Quantization:     normalizedQuant,
		Format:           format,
		Aliases:          []string{sourceModel.Name}, // Start with original name
		SourceEndpoints:  []domain.SourceEndpoint{},
		Capabilities:     capabilities,
		MaxContextLength: maxContext,
		Metadata:         metadata,
	}, nil
}

// inferCapabilities attempts to determine model capabilities
func (u *DefaultUnifier) inferCapabilities(model *domain.ModelInfo) []string {
	var capabilities []string

	// Check if it's a vision model
	if model.Details != nil && model.Details.Type != nil && *model.Details.Type == "vlm" {
		capabilities = append(capabilities, "vision")
	}

	// Check model name for common patterns
	nameLower := strings.ToLower(model.Name)
	if strings.Contains(nameLower, "chat") || strings.Contains(nameLower, "instruct") {
		capabilities = append(capabilities, "chat")
	}
	if strings.Contains(nameLower, "code") || strings.Contains(nameLower, "coder") {
		capabilities = append(capabilities, "code")
	}
	if strings.Contains(nameLower, "vision") || strings.Contains(nameLower, "vlm") {
		capabilities = append(capabilities, "vision")
	}

	// Default to completion if no specific capabilities found
	if len(capabilities) == 0 {
		capabilities = append(capabilities, "completion")
	}

	return capabilities
}

// cacheUnifiedModel stores the unified model and updates indices
func (u *DefaultUnifier) cacheUnifiedModel(model *domain.UnifiedModel) {
	u.unifiedModels.Store(model.ID, model)

	// Update alias index
	for _, alias := range model.Aliases {
		u.aliasIndex.Store(alias, model.ID)
	}

	// Also index the canonical ID as an alias
	u.aliasIndex.Store(model.ID, model.ID)
}

// sortRulesByPriority sorts rules in descending priority order
func (u *DefaultUnifier) sortRulesByPriority(platformType string) {
	rules := u.rules[platformType]
	for i := 0; i < len(rules)-1; i++ {
		for j := i + 1; j < len(rules); j++ {
			if rules[i].GetPriority() < rules[j].GetPriority() {
				rules[i], rules[j] = rules[j], rules[i]
			}
		}
	}
}

// registerDefaultRules sets up built-in unification rules
func (u *DefaultUnifier) registerDefaultRules() {
	// Ollama-specific rules
	u.RegisterCustomRule("ollama", &ollamaPhiFamilyRule{normalizer: u.normalizer})
	u.RegisterCustomRule("ollama", &ollamaHuggingFaceRule{normalizer: u.normalizer})

	// LM Studio rules
	u.RegisterCustomRule("lmstudio", &lmstudioVendorPrefixRule{normalizer: u.normalizer})

	// Generic rules for all platforms
	u.RegisterCustomRule("*", &genericModelRule{normalizer: u.normalizer})
}

// ModelNormalizer implementation
type defaultNormalizer struct {
	sizePattern  *regexp.Regexp
	quantPattern *regexp.Regexp
}

func NewModelNormalizer() ports.ModelNormalizer {
	return &defaultNormalizer{
		sizePattern:  regexp.MustCompile(`(\d+(?:\.\d+)?)\s*([BbMmKk]?)(?:illion)?`),
		quantPattern: regexp.MustCompile(`[Qq](\d+)_?([A-Za-z0-9_]+)`),
	}
}

func (n *defaultNormalizer) NormalizeFamily(modelName string, platformFamily string) (family string, variant string) {
	// Try to extract from model name first
	nameLower := strings.ToLower(modelName)

	// Common patterns: phi4, llama3.3, qwen3, etc.
	patterns := []struct {
		regex   *regexp.Regexp
		family  string
		variant int // 0 means use captured group
	}{
		{regexp.MustCompile(`phi[\-_]?(\d+)`), "phi", 0},
		{regexp.MustCompile(`llama[\-_]?(\d+(?:\.\d+)?)`), "llama", 0},
		{regexp.MustCompile(`qwen[\-_]?(\d+(?:\.\d+)?)`), "qwen", 0},
		{regexp.MustCompile(`gemma[\-_]?(\d+)`), "gemma", 0},
		{regexp.MustCompile(`mistral[\-_]?(\d+)`), "mistral", 0},
		{regexp.MustCompile(`deepseek[\-_]?(?:r)?(\d+)`), "deepseek", 0},
		{regexp.MustCompile(`falcon[\-_]?(\d+)`), "falcon", 0},
	}

	for _, pattern := range patterns {
		if matches := pattern.regex.FindStringSubmatch(nameLower); len(matches) > 1 {
			family = pattern.family
			variant = matches[1]
			return
		}
	}

	// Fall back to platform-provided family
	if platformFamily != "" {
		family = strings.ToLower(platformFamily)
		// Try to extract version from name
		versionRegex := regexp.MustCompile(`v?(\d+(?:\.\d+)?)`)
		if matches := versionRegex.FindStringSubmatch(modelName); len(matches) > 1 {
			variant = matches[1]
		} else {
			variant = "unknown"
		}
		return
	}

	// Last resort: use first part of name as family
	parts := strings.FieldsFunc(modelName, func(r rune) bool {
		return r == '-' || r == '_' || r == '/' || r == ':'
	})
	if len(parts) > 0 {
		family = strings.ToLower(parts[0])
		variant = "unknown"
	} else {
		family = "unknown"
		variant = "unknown"
	}

	return
}

func (n *defaultNormalizer) NormalizeSize(size string) (normalised string, parameterCount int64) {
	if size == "" {
		return "unknown", 0
	}

	// Extract number and unit
	matches := n.sizePattern.FindStringSubmatch(size)
	if len(matches) < 2 {
		return strings.ToLower(size), 0
	}

	num, err := strconv.ParseFloat(matches[1], 64)
	if err != nil {
		return strings.ToLower(size), 0
	}

	unit := strings.ToLower(matches[2])
	if unit == "" {
		unit = "b" // Default to billion
	}

	// Convert to billions and calculate parameter count
	var billions float64
	switch unit {
	case "k":
		billions = num / 1_000_000
		parameterCount = int64(num * 1_000)
	case "m":
		billions = num / 1_000
		parameterCount = int64(num * 1_000_000)
	case "b":
		billions = num
		parameterCount = int64(num * 1_000_000_000)
	default:
		billions = num
		parameterCount = int64(num * 1_000_000_000)
	}

	// Format normalized size
	if billions == float64(int(billions)) {
		normalised = fmt.Sprintf("%db", int(billions))
	} else {
		normalised = fmt.Sprintf("%.1fb", billions)
	}

	return
}

func (n *defaultNormalizer) NormalizeQuantization(quant string) string {
	if quant == "" || strings.ToLower(quant) == "unknown" {
		return "unk"
	}

	// Common mappings
	mappings := map[string]string{
		"q4_k_m": "q4km",
		"q4_k_s": "q4ks",
		"q3_k_l": "q3kl",
		"q3_k_m": "q3km",
		"q3_k_s": "q3ks",
		"q5_k_m": "q5km",
		"q5_k_s": "q5ks",
		"q6_k":   "q6k",
		"q8_0":   "q8",
		"q4_0":   "q4",
		"q4_1":   "q4_1",
		"q5_0":   "q5",
		"q5_1":   "q5_1",
		"f16":    "f16",
		"f32":    "f32",
	}

	quantLower := strings.ToLower(quant)
	quantLower = strings.ReplaceAll(quantLower, "-", "_")

	if normalized, exists := mappings[quantLower]; exists {
		return normalized
	}

	// Try to extract pattern
	if matches := n.quantPattern.FindStringSubmatch(quant); len(matches) > 1 {
		bits := matches[1]
		suffix := strings.ToLower(matches[2])
		suffix = strings.ReplaceAll(suffix, "_", "")
		return fmt.Sprintf("q%s%s", bits, suffix)
	}

	// Return lowercase version as fallback
	return strings.ToLower(strings.ReplaceAll(quant, "_", ""))
}

func (n *defaultNormalizer) GenerateCanonicalID(family, variant, size, quant string) string {
	// Format: {family}/{variant}:{size}-{quant}
	if variant == "unknown" || variant == "" {
		return fmt.Sprintf("%s:%s-%s", family, size, quant)
	}
	return fmt.Sprintf("%s/%s:%s-%s", family, variant, size, quant)
}

func (n *defaultNormalizer) GenerateAliases(unified *domain.UnifiedModel, platformType string, nativeName string) []string {
	aliases := []string{nativeName} // Always include native name

	// Add common variations
	baseID := fmt.Sprintf("%s%s", unified.Family, unified.Variant)
	if unified.Variant != "unknown" {
		aliases = append(aliases,
			fmt.Sprintf("%s:%s", baseID, unified.ParameterSize),
			fmt.Sprintf("%s-%s", baseID, unified.ParameterSize),
			fmt.Sprintf("%s:%s-%s", baseID, unified.ParameterSize, unified.Quantization),
			fmt.Sprintf("%s-%s-%s", baseID, unified.ParameterSize, unified.Quantization),
		)
	}

	// Platform-specific aliases
	switch platformType {
	case "ollama":
		aliases = append(aliases,
			fmt.Sprintf("%s:latest", baseID),
			fmt.Sprintf("%s:%s", unified.Family, unified.ParameterSize),
		)
	case "lmstudio":
		// LM Studio often uses vendor prefixes
		if publisher, ok := unified.Metadata["publisher"].(string); ok {
			aliases = append(aliases,
				fmt.Sprintf("%s/%s", publisher, baseID),
				fmt.Sprintf("%s/%s-%s", publisher, baseID, unified.ParameterSize),
			)
		}
	}

	// Deduplicate aliases
	seen := make(map[string]bool)
	var unique []string
	for _, alias := range aliases {
		if !seen[alias] {
			seen[alias] = true
			unique = append(unique, alias)
		}
	}

	return unique
}

// PlatformDetector implementation
type defaultPlatformDetector struct{}

func NewPlatformDetector() ports.PlatformDetector {
	return &defaultPlatformDetector{}
}

func (d *defaultPlatformDetector) DetectPlatform(modelInfo *domain.ModelInfo) string {
	// Check for platform-specific patterns
	nameLower := strings.ToLower(modelInfo.Name)

	// LM Studio specific indicators - check first as it's more specific
	if modelInfo.Details != nil {
		if modelInfo.Details.Type != nil && (*modelInfo.Details.Type == "llm" || *modelInfo.Details.Type == "vlm") {
			if modelInfo.Details.MaxContextLength != nil {
				return "lmstudio"
			}
		}
	}

	// Hugging Face models with explicit hf.co prefix
	if strings.Contains(modelInfo.Name, "hf.co/") {
		return "huggingface"
	}

	// Ollama patterns with colon tags
	if strings.Contains(nameLower, ":latest") || strings.Contains(nameLower, ":") {
		return "ollama"
	}

	// Models with "/" could be either HuggingFace or LM Studio vendor prefixes
	// Without more context, default to generic handling
	if strings.Count(modelInfo.Name, "/") >= 1 {
		return "huggingface"
	}

	// Default to ollama for now
	return "ollama"
}

