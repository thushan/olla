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
	
	// Deduplicate aliases by normalized name
	aliasMap := make(map[string]domain.AliasEntry)
	for _, alias := range unified.Aliases {
		normalized := u.normalizer.NormaliseAlias(alias.Name)
		aliasMap[normalized] = alias
	}
	for _, alias := range aliases {
		normalized := u.normalizer.NormaliseAlias(alias.Name)
		// Only add if not already present (prefer existing source attribution)
		if _, exists := aliasMap[normalized]; !exists {
			aliasMap[normalized] = alias
		}
	}
	
	// Convert back to slice
	unified.Aliases = make([]domain.AliasEntry, 0, len(aliasMap))
	for _, alias := range aliasMap {
		unified.Aliases = append(unified.Aliases, alias)
	}
	
	// Set prompt template based on heuristics
	u.setPromptTemplate(unified)

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
	// Normalise the alias for lookup
	normalizedAlias := u.normalizer.NormaliseAlias(alias)
	
	// Check alias index first with normalized alias
	if unifiedID, exists := u.aliasIndex.Load(normalizedAlias); exists {
		u.stats.cacheHits.Add(1)
		if model, exists := u.unifiedModels.Load(unifiedID); exists {
			return model, nil
		}
	}

	u.stats.cacheMisses.Add(1)

	// Search through all models
	var found *domain.UnifiedModel
	u.unifiedModels.Range(func(id string, model *domain.UnifiedModel) bool {
		// Check if normalized ID matches
		if u.normalizer.NormaliseAlias(model.ID) == normalizedAlias {
			found = model
			return false
		}
		
		// Check all aliases with normalization
		for _, aliasEntry := range model.Aliases {
			if u.normalizer.NormaliseAlias(aliasEntry.Name) == normalizedAlias {
				found = model
				return false
			}
		}
		
		return true
	})

	if found == nil {
		return nil, fmt.Errorf("no unified model found for alias: %s", alias)
	}

	// Update alias index for faster future lookups
	u.aliasIndex.Store(normalizedAlias, found.ID)
	return found, nil
}

// GetAliases returns all known aliases for a unified model ID
func (u *DefaultUnifier) GetAliases(ctx context.Context, unifiedID string) ([]string, error) {
	model, exists := u.unifiedModels.Load(unifiedID)
	if !exists {
		return nil, fmt.Errorf("unified model not found: %s", unifiedID)
	}

	return model.GetAliasStrings(), nil
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

	// Deduplicate by digest first
	digestMap := make(map[string][]*domain.UnifiedModel)
	var noDigestModels []*domain.UnifiedModel
	
	for _, model := range models {
		if digest, ok := model.Metadata["digest"].(string); ok && digest != "" {
			digestMap[digest] = append(digestMap[digest], model)
		} else {
			noDigestModels = append(noDigestModels, model)
		}
	}
	
	// Process digest groups, preferring Ollama > LM Studio > others
	var dedupedModels []*domain.UnifiedModel
	for digest, group := range digestMap {
		selected := u.selectPreferredModel(group)
		if len(group) > 1 {
			u.logger.Debug(fmt.Sprintf("Deduped %d models with digest %s, selected from %s", 
				len(group), digest, selected.SourceEndpoints[0].EndpointURL))
		}
		dedupedModels = append(dedupedModels, selected)
	}
	
	// Add models without digest
	dedupedModels = append(dedupedModels, noDigestModels...)
	
	if len(dedupedModels) == 1 {
		return dedupedModels[0], nil
	}

	// Use the first model as base
	merged := &domain.UnifiedModel{
		ID:               dedupedModels[0].ID,
		Family:           dedupedModels[0].Family,
		Variant:          dedupedModels[0].Variant,
		ParameterSize:    dedupedModels[0].ParameterSize,
		ParameterCount:   dedupedModels[0].ParameterCount,
		Quantization:     dedupedModels[0].Quantization,
		Format:           dedupedModels[0].Format,
		Aliases:          []domain.AliasEntry{},
		SourceEndpoints:  []domain.SourceEndpoint{},
		Capabilities:     []string{},
		MaxContextLength: dedupedModels[0].MaxContextLength,
		Metadata:         make(map[string]interface{}),
		PromptTemplateID: dedupedModels[0].PromptTemplateID,
	}

	// Merge aliases (deduplicate by normalized name)
	aliasMap := make(map[string]domain.AliasEntry)
	for _, model := range dedupedModels {
		for _, alias := range model.Aliases {
			normalized := u.normalizer.NormaliseAlias(alias.Name)
			// Keep first occurrence to preserve original source attribution
			if _, exists := aliasMap[normalized]; !exists {
				aliasMap[normalized] = alias
			}
		}
	}
	for _, alias := range aliasMap {
		merged.Aliases = append(merged.Aliases, alias)
	}

	// Merge endpoints
	endpointMap := make(map[string]domain.SourceEndpoint)
	for _, model := range dedupedModels {
		for _, endpoint := range model.SourceEndpoints {
			endpointMap[endpoint.EndpointURL] = endpoint
		}
	}
	for _, endpoint := range endpointMap {
		merged.SourceEndpoints = append(merged.SourceEndpoints, endpoint)
	}

	// Merge capabilities (deduplicate)
	capSet := make(map[string]bool)
	for _, model := range dedupedModels {
		for _, cap := range model.Capabilities {
			capSet[cap] = true
		}
	}
	for cap := range capSet {
		merged.Capabilities = append(merged.Capabilities, cap)
	}
	
	// Merge metadata, preserving important fields
	for _, model := range dedupedModels {
		for k, v := range model.Metadata {
			if _, exists := merged.Metadata[k]; !exists {
				merged.Metadata[k] = v
			}
		}
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

	unified := &domain.UnifiedModel{
		ID:               canonicalID,
		Family:           family,
		Variant:          variant,
		ParameterSize:    normalizedSize,
		ParameterCount:   paramCount,
		Quantization:     normalizedQuant,
		Format:           format,
		Aliases:          []domain.AliasEntry{{Name: sourceModel.Name, Source: platformType}}, // Start with original name
		SourceEndpoints:  []domain.SourceEndpoint{},
		Capabilities:     capabilities,
		MaxContextLength: maxContext,
		Metadata:         metadata,
	}
	
	// Set prompt template
	u.setPromptTemplate(unified)
	
	return unified, nil
}

// inferCapabilities attempts to determine model capabilities
func (u *DefaultUnifier) inferCapabilities(model *domain.ModelInfo) []string {
	capabilitySet := make(map[string]bool)
	
	// Apply capability rules
	for _, rule := range CapabilityRules {
		matches := false
		
		// Check model type
		if rule.ModelType != "" && model.Details != nil && model.Details.Type != nil {
			if strings.EqualFold(rule.ModelType, *model.Details.Type) {
				matches = true
			}
		}
		
		// Check architecture
		if rule.Architecture != "" && model.Details != nil {
			// Check if family matches architecture
			if model.Details.Family != nil && strings.EqualFold(rule.Architecture, *model.Details.Family) {
				matches = true
			}
			// Also check if it's in the model name
			if matchesPattern(model.Name, rule.Architecture) {
				matches = true
			}
		}
		
		// Check name patterns
		if len(rule.NamePatterns) > 0 && containsAny(model.Name, rule.NamePatterns) {
			matches = true
		}
		
		// Add capabilities if rule matches
		if matches {
			for _, cap := range rule.Capabilities {
				capabilitySet[cap] = true
			}
		}
	}
	
	// Legacy pattern matching for backward compatibility
	nameLower := strings.ToLower(model.Name)
	if strings.Contains(nameLower, "chat") || strings.Contains(nameLower, "instruct") {
		capabilitySet["chat"] = true
	}
	if strings.Contains(nameLower, "code") || strings.Contains(nameLower, "coder") {
		capabilitySet["code"] = true
	}
	if strings.Contains(nameLower, "vision") || strings.Contains(nameLower, "vlm") {
		capabilitySet["vision"] = true
	}
	
	// Convert set to slice
	var capabilities []string
	for cap := range capabilitySet {
		capabilities = append(capabilities, cap)
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

	// Update alias index with normalized aliases
	for _, alias := range model.Aliases {
		normalized := u.normalizer.NormaliseAlias(alias.Name)
		u.aliasIndex.Store(normalized, model.ID)
	}

	// Also index the canonical ID as an alias
	u.aliasIndex.Store(u.normalizer.NormaliseAlias(model.ID), model.ID)
}

// selectPreferredModel selects the preferred model from a group based on platform preference
func (u *DefaultUnifier) selectPreferredModel(models []*domain.UnifiedModel) *domain.UnifiedModel {
	if len(models) == 1 {
		return models[0]
	}
	
	// Platform preference: ollama > lmstudio > others
	platformPriority := map[string]int{
		"ollama":   3,
		"lmstudio": 2,
		"*":        1,
	}
	
	var selected *domain.UnifiedModel
	highestPriority := 0
	
	for _, model := range models {
		// Determine platform from source attribution
		var platform string
		for _, alias := range model.Aliases {
			if alias.Source != "" && alias.Source != "generated" {
				platform = alias.Source
				break
			}
		}
		
		priority := platformPriority[platform]
		if priority == 0 {
			priority = 1 // default priority
		}
		
		if priority > highestPriority {
			selected = model
			highestPriority = priority
		}
	}
	
	if selected == nil {
		selected = models[0]
	}
	
	return selected
}

// setPromptTemplate sets the prompt template ID based on heuristics
func (u *DefaultUnifier) setPromptTemplate(model *domain.UnifiedModel) {
	// Check if already set
	if model.PromptTemplateID != "" {
		return
	}
	
	// Apply heuristics
	nameLower := strings.ToLower(model.ID)
	
	// Check all aliases for patterns
	var hasInstruct, hasChat bool
	for _, alias := range model.Aliases {
		aliasLower := strings.ToLower(alias.Name)
		if strings.Contains(aliasLower, "instruct") {
			hasInstruct = true
		}
		if strings.Contains(aliasLower, "chat") && !strings.Contains(aliasLower, "instruct") {
			hasChat = true
		}
	}
	
	// Check family-specific patterns
	if model.Family == "llama" {
		if hasInstruct || strings.Contains(nameLower, "instruct") {
			model.PromptTemplateID = "llama3-instruct"
			return
		}
	}
	
	// Check variant
	if strings.Contains(model.Variant, "chat") || hasChat {
		model.PromptTemplateID = "chatml"
		return
	}
	
	// Check model type in metadata
	if modelType, ok := model.Metadata["type"].(string); ok {
		if modelType == "code" {
			model.PromptTemplateID = "plain"
			return
		}
	}
	
	// Check capabilities
	for _, cap := range model.Capabilities {
		if cap == "code" {
			model.PromptTemplateID = "plain"
			return
		}
	}
	
	// Default for chat-capable models
	for _, cap := range model.Capabilities {
		if cap == "chat" {
			model.PromptTemplateID = "chatml"
			return
		}
	}
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
	var candidates []FamilyCandidate
	
	// 1. Check publisher-based rules (highest priority)
	if publisher := extractPublisher(modelName); publisher != "" {
		for _, rule := range PublisherFamilyRules {
			for _, pub := range rule.Publishers {
				if strings.EqualFold(publisher, pub) {
					candidates = append(candidates, FamilyCandidate{
						Family:   rule.Family,
						Priority: rule.Priority,
						Source:   "publisher",
					})
					break
				}
			}
		}
	}
	
	// 2. Check name pattern rules
	for _, rule := range NamePatternRules {
		if matchesPattern(modelName, rule.Pattern) {
			candidates = append(candidates, FamilyCandidate{
				Family:   rule.Family,
				Variant:  rule.Variant,
				Priority: rule.Priority,
				Source:   "name_pattern",
			})
		}
	}
	
	// 3. Check common regex patterns
	nameLower := strings.ToLower(modelName)
	patterns := []struct {
		regex   *regexp.Regexp
		family  string
		variant int // 0 means use captured group
		priority int
	}{
		{regexp.MustCompile(`phi[\-_]?(\d+(?:\.\d+)?)`), "phi", 0, 7},
		{regexp.MustCompile(`llama[\-_]?(\d+(?:\.\d+)?)`), "llama", 0, 7},
		{regexp.MustCompile(`qwen[\-_]?(\d+(?:\.\d+)?)`), "qwen", 0, 7},
		{regexp.MustCompile(`gemma[\-_]?(\d+)`), "gemma", 0, 7},
		{regexp.MustCompile(`mistral[\-_]?(\d+)`), "mistral", 0, 7},
		{regexp.MustCompile(`deepseek[\-_]?(?:r)?(\d+)`), "deepseek", 0, 7},
		{regexp.MustCompile(`falcon[\-_]?(\d+)`), "falcon", 0, 7},
	}

	for _, pattern := range patterns {
		if matches := pattern.regex.FindStringSubmatch(nameLower); len(matches) > 1 {
			candidates = append(candidates, FamilyCandidate{
				Family:   pattern.family,
				Variant:  matches[1],
				Priority: pattern.priority,
				Source:   "regex_pattern",
			})
		}
	}
	
	// 4. Use platform-provided family (lower priority)
	if platformFamily != "" && platformFamily != "unknown" {
		candidates = append(candidates, FamilyCandidate{
			Family:   strings.ToLower(platformFamily),
			Priority: 5,
			Source:   "platform",
		})
	}
	
	// 5. Extract from model name (fallback)
	parts := strings.FieldsFunc(modelName, func(r rune) bool {
		return r == '-' || r == '_' || r == '/' || r == ':'
	})
	if len(parts) > 0 {
		candidates = append(candidates, FamilyCandidate{
			Family:   strings.ToLower(parts[0]),
			Priority: 3,
			Source:   "name_extraction",
		})
	}
	
	// Select highest priority candidate
	var selected FamilyCandidate
	for _, candidate := range candidates {
		if candidate.Priority > selected.Priority {
			selected = candidate
		}
	}
	
	if selected.Family != "" {
		family = selected.Family
		variant = selected.Variant
		
		// If no variant specified, try to extract it
		if variant == "" {
			variant = extractVersionFromName(modelName, family)
		}
		
		if variant == "" {
			variant = "unknown"
		}
		
		return
	}
	
	// Default fallback
	return "unknown", "unknown"
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

func (n *defaultNormalizer) GenerateAliases(unified *domain.UnifiedModel, platformType string, nativeName string) []domain.AliasEntry {
	// Start with native name to ensure it's always first
	aliases := []domain.AliasEntry{{Name: nativeName, Source: platformType}}
	aliasSet := make(map[string]bool)
	aliasSet[nativeName] = true

	// Add common variations (using : as standard separator)
	baseID := fmt.Sprintf("%s%s", unified.Family, unified.Variant)
	if unified.Variant != "unknown" && unified.Variant != "" {
		candidates := []string{
			fmt.Sprintf("%s:%s", baseID, unified.ParameterSize),
			fmt.Sprintf("%s:%s-%s", baseID, unified.ParameterSize, unified.Quantization),
		}
		for _, c := range candidates {
			if !aliasSet[c] {
				aliases = append(aliases, domain.AliasEntry{Name: c, Source: "generated"})
				aliasSet[c] = true
			}
		}
	}

	// Platform-specific aliases
	switch platformType {
	case "ollama":
		candidates := []string{}
		if unified.Variant != "unknown" && unified.Variant != "" {
			candidates = append(candidates, fmt.Sprintf("%s:latest", baseID))
		}
		candidates = append(candidates, fmt.Sprintf("%s:%s", unified.Family, unified.ParameterSize))
		
		for _, c := range candidates {
			if !aliasSet[c] {
				aliases = append(aliases, domain.AliasEntry{Name: c, Source: platformType})
				aliasSet[c] = true
			}
		}
	case "lmstudio":
		// LM Studio often uses vendor prefixes
		if publisher, ok := unified.Metadata["publisher"].(string); ok && publisher != "" {
			candidates := []string{
				fmt.Sprintf("%s/%s", publisher, baseID),
				fmt.Sprintf("%s/%s-%s", publisher, baseID, unified.ParameterSize),
			}
			for _, c := range candidates {
				if !aliasSet[c] {
					aliases = append(aliases, domain.AliasEntry{Name: c, Source: platformType})
					aliasSet[c] = true
				}
			}
		}
	}

	return aliases
}

func (n *defaultNormalizer) NormaliseAlias(alias string) string {
	return strings.ToLower(strings.ReplaceAll(alias, "-", ":"))
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

