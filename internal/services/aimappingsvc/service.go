// internal/services/aimappingsvc/service.go
package aimappingsvc

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/hotkhwan/gateway-api/internal/crypto/secretbox"
	"github.com/hotkhwan/gateway-api/internal/gateways/aiprovider"
	"github.com/hotkhwan/gateway-api/internal/logger"
	"github.com/hotkhwan/gateway-api/internal/repo/aiconfigrepo"
	"github.com/hotkhwan/gateway-api/internal/repo/aisuggestauditrepo"
	"github.com/hotkhwan/gateway-api/internal/services/mappingsuggestionsvc"
	"github.com/hotkhwan/gateway-api/models/ingestmod"
	"github.com/hotkhwan/gateway-api/models/workspacemod"
	"github.com/hotkhwan/gateway-api/utils/traceutil"
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"
)

const (
	dedupCacheTTL       = 60 * time.Second
	defaultTimeoutMs    = 30_000
	defaultMaxBytes     = 12_288 // 12 KB
	minConfidence       = 0.0
	maxConfidence       = 1.0
	modeAIAugmented     = "aiAugmented"
	modeSystemOnly      = "systemOnly"
	modeAIFailedFallback = "aiFailedFallback"
	auditExpiry         = 90 * 24 * time.Hour
)

// AIMappingService orchestrates AI-assisted field mapping suggestions.
type AIMappingService struct {
	configRepo    *aiconfigrepo.AIConfigRepo
	auditRepo     *aisuggestauditrepo.AISuggestAuditRepo
	suggestionSvc *mappingsuggestionsvc.MappingSuggestionService
	redis         *redis.Client
	keyring       *secretbox.JSONKeyring
	log           zerolog.Logger
}

// NewAIMappingService constructs a new AIMappingService.
func NewAIMappingService(
	configRepo *aiconfigrepo.AIConfigRepo,
	auditRepo *aisuggestauditrepo.AISuggestAuditRepo,
	suggestionSvc *mappingsuggestionsvc.MappingSuggestionService,
	redisClient *redis.Client,
	keyring *secretbox.JSONKeyring,
) *AIMappingService {
	return &AIMappingService{
		configRepo:    configRepo,
		auditRepo:     auditRepo,
		suggestionSvc: suggestionSvc,
		redis:         redisClient,
		keyring:       keyring,
		log:           logger.WithMeta("aimappingsvc", "AIMappingService"),
	}
}

// Suggest returns an AI-augmented or system-only mapping suggestion.
func (s *AIMappingService) Suggest(ctx context.Context, input AISuggestInput) (*AISuggestResult, error) {
	ctx, end, log := traceutil.StartLite(ctx,
		"github.com/hotkhwan/gateway-api/aimappingsvc",
		"aimappingsvc.Suggest",
		"aimappingsvc", "Suggest",
	)
	defer end()

	incRequests()

	// 1. Validate input.
	if input.SourceFamily == "" {
		return nil, fmt.Errorf("aimappingsvc: sourceFamily is required")
	}
	if len(input.SamplePayload) == 0 {
		return nil, fmt.Errorf("aimappingsvc: samplePayload is required")
	}

	// 2. Normalize payload — sanitize BSON key chars at the top level.
	normalized := sanitizePayload(input.SamplePayload)

	// 3. Extract observedPaths.
	observedPaths := extractPaths(normalized, "", 0)

	// 4. Run system suggestion matcher.
	systemSuggestions := s.suggestionSvc.GetByFamily(input.SourceFamily)
	systemMappings, systemRules := buildSystemMappings(systemSuggestions)
	hasSysSuggestion := len(systemMappings) > 0 || len(systemRules) > 0

	log.Debug().
		Str("sourceFamily", input.SourceFamily).
		Int("systemMappings", len(systemMappings)).
		Int("observedPaths", len(observedPaths)).
		Msg("system suggestion loaded")

	// 5. Load WorkspaceAIConfig.
	cfg, err := s.configRepo.FindByWorkspaceID(ctx, input.WorkspaceID)
	if err != nil {
		if errors.Is(err, aiconfigrepo.ErrNotFound) {
			// No config → system only.
			return s.buildSystemOnlyResult(systemMappings, systemRules, observedPaths, hasSysSuggestion), nil
		}
		log.Warn().Err(err).Str("workspaceId", input.WorkspaceID).Msg("failed to load ai config; falling back to system-only")
		return s.buildSystemOnlyResult(systemMappings, systemRules, observedPaths, hasSysSuggestion), nil
	}
	if !cfg.Enabled {
		return s.buildSystemOnlyResult(systemMappings, systemRules, observedPaths, hasSysSuggestion), nil
	}

	// 6. Check Redis dedup cache.
	cacheKey := dedupCacheKey(input.WorkspaceID, input.SourceFamily, normalized)
	if cached := s.getFromCache(ctx, cacheKey); cached != nil {
		log.Debug().Str("cacheKey", cacheKey).Msg("returning cached AI suggestion")
		return cached, nil
	}

	// 7. Reduce payload.
	maxBytes := cfg.MaxInputBytes
	if maxBytes <= 0 {
		maxBytes = defaultMaxBytes
	}
	reduced := ReducePayload(normalized, maxBytes)

	// 8. Check maxBytes limit — warn but proceed.
	if reduced.ReducedBytes > maxBytes {
		log.Warn().Int("reducedBytes", reduced.ReducedBytes).Int("maxBytes", maxBytes).Msg("reduced payload still exceeds maxBytes limit")
	}

	// 9. Resolve enum context.
	enumBundle := resolveEnumContext(systemSuggestions, reduced.ObservedPaths, input.SourceFamily)

	// 10. Build prompt.
	prompt := buildPrompt(input.SourceFamily, reduced, enumBundle, systemMappings)

	// 11. Create AI provider via factory.
	provider, err := aiprovider.NewProvider(cfg, s.keyring)
	if err != nil {
		log.Error().Err(err).Msg("failed to create ai provider; falling back")
		incAIFailures()
		incFallback()
		return s.buildFallbackResult(systemMappings, systemRules, observedPaths, hasSysSuggestion, cfg), nil
	}

	// 12. Call provider.Complete with timeout.
	timeoutMs := cfg.DefaultTimeoutMs
	if timeoutMs <= 0 {
		timeoutMs = defaultTimeoutMs
	}
	callCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutMs)*time.Millisecond)
	defer cancel()

	aiResp, aiErr := provider.Complete(callCtx, aiprovider.CompletionRequest{
		Prompt:    prompt,
		MaxTokens: 2048,
	})
	if aiErr != nil {
		log.Warn().Err(aiErr).Str("provider", provider.Name()).Msg("ai provider call failed; falling back")
		incAIFailures()
		incFallback()
		s.saveAudit(ctx, input, cfg, modeAIFailedFallback, 0, false, 0)
		return s.buildFallbackResult(systemMappings, systemRules, observedPaths, hasSysSuggestion, cfg), nil
	}

	// 13. Unmarshal AI response → AISuggestRawResult.
	var rawResult aiprovider.AISuggestRawResult
	if jsonErr := json.Unmarshal([]byte(aiResp.RawJSON), &rawResult); jsonErr != nil {
		log.Warn().Err(jsonErr).Str("raw", aiResp.RawJSON).Msg("failed to parse ai json response; falling back")
		incParseFailures()
		incFallback()
		s.saveAudit(ctx, input, cfg, modeAIFailedFallback, aiResp.LatencyMs, false, 0)
		return s.buildFallbackResult(systemMappings, systemRules, observedPaths, hasSysSuggestion, cfg), nil
	}

	// Normalize suggestedEventType.
	rawResult.SuggestedEventType = NormalizeSuggestedEventType(rawResult.SuggestedEventType)

	// 14. Validate AI output.
	validationErrs := ValidateAIOutput(rawResult, reduced.ObservedPaths)
	if len(validationErrs) > 0 {
		log.Warn().Interface("validationErrors", validationErrs).Msg("ai output validation failed; falling back")
		incFallback()
		s.saveAudit(ctx, input, cfg, modeAIFailedFallback, aiResp.LatencyMs, false, 0)
		return s.buildFallbackResult(systemMappings, systemRules, observedPaths, hasSysSuggestion, cfg), nil
	}

	// 15. Merge with policy.
	mergedMappings, mergedRules, conflicts := MergeWithPolicy(systemMappings, systemRules, rawResult, reduced.ObservedPaths)

	// 16. Compute confidence.
	confidence := computeConfidence(rawResult, mergedMappings, conflicts)

	// 17. Save audit entry (no payload, no AI output).
	s.saveAudit(ctx, input, cfg, modeAIAugmented, aiResp.LatencyMs, true, len(conflicts))

	// 18. Build and cache result.
	result := &AISuggestResult{
		Mode:               modeAIAugmented,
		Provider:           provider.Name(),
		Model:              cfg.Model,
		PromptVersion:      aiSuggestPromptVersion,
		SchemaVersion:      aiprovider.AISuggestSchemaVersion,
		SuggestedEventType: rawResult.SuggestedEventType,
		FieldMappings:      mergedMappings,
		MatchRules:         mergedRules,
		Conflicts:          conflicts,
		Diagnostics: SuggestDiagnostics{
			SystemSuggestionUsed:  hasSysSuggestion,
			AIParseSuccess:        true,
			AIValidationSuccess:   true,
			ObservedPathsCount:    len(reduced.ObservedPaths),
			AIOutputFieldsCount:   len(rawResult.FieldMappings),
			PayloadReducedBytes:   reduced.ReducedBytes,
			AILatencyMs:           aiResp.LatencyMs,
			EnumContextFieldsUsed: len(enumBundle.FieldDictionaries),
		},
		Confidence: confidence,
	}

	s.setCache(ctx, cacheKey, result)

	log.Info().
		Str("workspaceId", input.WorkspaceID).
		Str("sourceFamily", input.SourceFamily).
		Str("mode", modeAIAugmented).
		Int("conflicts", len(conflicts)).
		Float64("confidence", confidence).
		Msg("ai mapping suggestion completed")

	return result, nil
}

// GetConfig returns the workspace AI config (without the actual API key).
func (s *AIMappingService) GetConfig(ctx context.Context, workspaceID string) (*workspacemod.WorkspaceAIConfig, bool, error) {
	ctx, end, log := traceutil.StartLite(ctx,
		"github.com/hotkhwan/gateway-api/aimappingsvc",
		"aimappingsvc.GetConfig",
		"aimappingsvc", "GetConfig",
	)
	defer end()

	cfg, err := s.configRepo.FindByWorkspaceID(ctx, workspaceID)
	if err != nil {
		if errors.Is(err, aiconfigrepo.ErrNotFound) {
			return nil, false, nil
		}
		log.Error().Err(err).Str("workspaceId", workspaceID).Msg("failed to load ai config")
		return nil, false, fmt.Errorf("aimappingsvc: get config: %w", err)
	}

	hasApiKey := cfg.EncryptedApiKey != nil
	// Never return the actual key.
	cfg.EncryptedApiKey = nil

	return cfg, hasApiKey, nil
}

// UpsertConfig creates or updates a workspace AI config.
func (s *AIMappingService) UpsertConfig(ctx context.Context, workspaceID, userID string, req UpsertConfigRequest) error {
	ctx, end, log := traceutil.StartLite(ctx,
		"github.com/hotkhwan/gateway-api/aimappingsvc",
		"aimappingsvc.UpsertConfig",
		"aimappingsvc", "UpsertConfig",
	)
	defer end()

	// Load existing or build new config.
	cfg, err := s.configRepo.FindByWorkspaceID(ctx, workspaceID)
	if err != nil {
		if !errors.Is(err, aiconfigrepo.ErrNotFound) {
			log.Error().Err(err).Str("workspaceId", workspaceID).Msg("failed to load existing ai config")
			return fmt.Errorf("aimappingsvc: upsert config: %w", err)
		}
		// Not found — start fresh.
		cfg = &workspacemod.WorkspaceAIConfig{
			WorkspaceID: workspaceID,
			CreatedBy:   userID,
		}
	}

	cfg.Enabled = req.Enabled
	cfg.Provider = req.Provider
	cfg.Model = req.Model
	cfg.UpdatedBy = userID

	if req.DefaultTimeoutMs > 0 {
		cfg.DefaultTimeoutMs = req.DefaultTimeoutMs
	}
	if req.MaxInputBytes > 0 {
		cfg.MaxInputBytes = req.MaxInputBytes
	}

	// Encrypt API key if provided.
	if req.ApiKey != "" {
		if s.keyring == nil {
			return fmt.Errorf("aimappingsvc: keyring is nil — cannot encrypt api key")
		}
		blob, encErr := secretbox.EncryptString(s.keyring, req.ApiKey)
		if encErr != nil {
			log.Error().Err(encErr).Msg("failed to encrypt api key")
			return fmt.Errorf("aimappingsvc: encrypt api key: %w", encErr)
		}
		cfg.EncryptedApiKey = blob
		cfg.ProviderMode = "workspaceManagedProvider"
	} else if cfg.EncryptedApiKey == nil {
		cfg.ProviderMode = "freeSharedProvider"
	}

	if err := s.configRepo.Upsert(ctx, cfg); err != nil {
		log.Error().Err(err).Str("workspaceId", workspaceID).Msg("failed to upsert ai config")
		return fmt.Errorf("aimappingsvc: upsert config: %w", err)
	}

	log.Info().Str("workspaceId", workspaceID).Str("updatedBy", userID).Msg("ai config upserted")
	return nil
}

// ClearApiKey removes the encrypted API key from a workspace AI config.
func (s *AIMappingService) ClearApiKey(ctx context.Context, workspaceID, userID string) error {
	ctx, end, log := traceutil.StartLite(ctx,
		"github.com/hotkhwan/gateway-api/aimappingsvc",
		"aimappingsvc.ClearApiKey",
		"aimappingsvc", "ClearApiKey",
	)
	defer end()

	if err := s.configRepo.ClearKey(ctx, workspaceID, userID); err != nil {
		log.Error().Err(err).Str("workspaceId", workspaceID).Msg("failed to clear api key")
		return fmt.Errorf("aimappingsvc: clear api key: %w", err)
	}

	log.Info().Str("workspaceId", workspaceID).Str("clearedBy", userID).Msg("ai api key cleared")
	return nil
}

// ValidateConnection tests the AI provider connection and updates LastValidatedAt.
func (s *AIMappingService) ValidateConnection(ctx context.Context, workspaceID string) error {
	ctx, end, log := traceutil.StartLite(ctx,
		"github.com/hotkhwan/gateway-api/aimappingsvc",
		"aimappingsvc.ValidateConnection",
		"aimappingsvc", "ValidateConnection",
	)
	defer end()

	cfg, err := s.configRepo.FindByWorkspaceID(ctx, workspaceID)
	if err != nil {
		if errors.Is(err, aiconfigrepo.ErrNotFound) {
			return fmt.Errorf("aimappingsvc: no ai config found for workspace %s", workspaceID)
		}
		return fmt.Errorf("aimappingsvc: load config: %w", err)
	}

	provider, err := aiprovider.NewProvider(cfg, s.keyring)
	if err != nil {
		return fmt.Errorf("aimappingsvc: create provider: %w", err)
	}

	// Send a minimal test prompt.
	testPrompt := `Respond with valid JSON: {"ok": true}`
	timeoutMs := cfg.DefaultTimeoutMs
	if timeoutMs <= 0 {
		timeoutMs = defaultTimeoutMs
	}
	callCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutMs)*time.Millisecond)
	defer cancel()

	_, callErr := provider.Complete(callCtx, aiprovider.CompletionRequest{
		Prompt:    testPrompt,
		MaxTokens: 32,
	})

	now := time.Now().UTC()
	cfg.LastValidatedAt = &now
	if callErr != nil {
		log.Warn().Err(callErr).Str("workspaceId", workspaceID).Msg("ai connection validation failed")
		cfg.LastValidationStatus = "failed"
		cfg.LastValidationError = callErr.Error()
	} else {
		cfg.LastValidationStatus = "ok"
		cfg.LastValidationError = ""
	}

	if upsertErr := s.configRepo.Upsert(ctx, cfg); upsertErr != nil {
		log.Error().Err(upsertErr).Msg("failed to persist validation status")
		return fmt.Errorf("aimappingsvc: persist validation status: %w", upsertErr)
	}

	if callErr != nil {
		return fmt.Errorf("aimappingsvc: connection validation failed: %w", callErr)
	}

	log.Info().Str("workspaceId", workspaceID).Str("provider", provider.Name()).Msg("ai connection validated successfully")
	return nil
}

// --- internal helpers ---

// buildSystemOnlyResult assembles a system-only result with no AI involvement.
func (s *AIMappingService) buildSystemOnlyResult(
	systemMappings []SuggestionFieldMap,
	systemRules []MatchRuleSuggestion,
	observedPaths []string,
	hasSysSuggestion bool,
) *AISuggestResult {
	return &AISuggestResult{
		Mode:          modeSystemOnly,
		PromptVersion: aiSuggestPromptVersion,
		SchemaVersion: aiprovider.AISuggestSchemaVersion,
		FieldMappings: systemMappings,
		MatchRules:    systemRules,
		Diagnostics: SuggestDiagnostics{
			SystemSuggestionUsed: hasSysSuggestion,
			ObservedPathsCount:   len(observedPaths),
		},
		Confidence: computeSystemConfidence(systemMappings),
	}
}

// buildFallbackResult assembles a fallback result when AI fails.
func (s *AIMappingService) buildFallbackResult(
	systemMappings []SuggestionFieldMap,
	systemRules []MatchRuleSuggestion,
	observedPaths []string,
	hasSysSuggestion bool,
	cfg *workspacemod.WorkspaceAIConfig,
) *AISuggestResult {
	provider := ""
	model := ""
	if cfg != nil {
		provider = cfg.Provider
		model = cfg.Model
	}
	return &AISuggestResult{
		Mode:          modeAIFailedFallback,
		Provider:      provider,
		Model:         model,
		PromptVersion: aiSuggestPromptVersion,
		SchemaVersion: aiprovider.AISuggestSchemaVersion,
		FieldMappings: systemMappings,
		MatchRules:    systemRules,
		Warnings:      []string{"AI suggestion failed; showing system suggestions only"},
		Diagnostics: SuggestDiagnostics{
			SystemSuggestionUsed: hasSysSuggestion,
			ObservedPathsCount:   len(observedPaths),
		},
		Confidence: computeSystemConfidence(systemMappings),
	}
}

// saveAudit persists an audit record. Errors are logged but not returned.
func (s *AIMappingService) saveAudit(
	ctx context.Context,
	input AISuggestInput,
	cfg *workspacemod.WorkspaceAIConfig,
	mode string,
	latencyMs int64,
	parseSuccess bool,
	conflictCount int,
) {
	provider := ""
	model := ""
	if cfg != nil {
		provider = cfg.Provider
		model = cfg.Model
	}
	entry := aisuggestauditrepo.AISuggestAuditEntry{
		WorkspaceID:   input.WorkspaceID,
		UserID:        input.UserID,
		Provider:      provider,
		Model:         model,
		PromptVersion: aiSuggestPromptVersion,
		Mode:          mode,
		SourceFamily:  input.SourceFamily,
		LatencyMs:     latencyMs,
		ParseSuccess:  parseSuccess,
		ConflictCount: conflictCount,
		CreatedAt:     time.Now().UTC(),
		ExpiresAt:     time.Now().UTC().Add(auditExpiry),
	}
	if err := s.auditRepo.Save(ctx, entry); err != nil {
		log := logger.FromCtx(ctx, "aimappingsvc", "saveAudit")
		log.Warn().Err(err).Msg("failed to save ai suggest audit entry")
	}
}

// getFromCache retrieves a cached AISuggestResult from Redis.
func (s *AIMappingService) getFromCache(ctx context.Context, key string) *AISuggestResult {
	if s.redis == nil {
		return nil
	}
	val, err := s.redis.Get(ctx, key).Bytes()
	if err != nil {
		return nil
	}
	var result AISuggestResult
	if err := json.Unmarshal(val, &result); err != nil {
		return nil
	}
	return &result
}

// setCache stores an AISuggestResult in Redis with TTL.
func (s *AIMappingService) setCache(ctx context.Context, key string, result *AISuggestResult) {
	if s.redis == nil {
		return
	}
	data, err := json.Marshal(result)
	if err != nil {
		return
	}
	if err := s.redis.Set(ctx, key, data, dedupCacheTTL).Err(); err != nil {
		log := logger.FromCtx(ctx, "aimappingsvc", "setCache")
		log.Warn().Err(err).Str("cacheKey", key).Msg("failed to cache ai suggestion")
	}
}

// dedupCacheKey builds a Redis key for deduplication.
func dedupCacheKey(workspaceID, sourceFamily string, payload map[string]any) string {
	encoded, _ := json.Marshal(payload)
	h := sha256.Sum256(encoded)
	return fmt.Sprintf("aimapping:dedup:%s:%s:%x", workspaceID, sourceFamily, h[:8])
}

// sanitizePayload sanitizes BSON-unsafe key chars in a payload map (top level).
func sanitizePayload(raw map[string]any) map[string]any {
	out := make(map[string]any, len(raw))
	for k, v := range raw {
		clean := strings.ReplaceAll(k, "$", "_")
		clean = strings.ReplaceAll(clean, ".", "_")
		out[clean] = v
	}
	return out
}

// extractPaths returns all dotted field paths found in a map up to maxObjectDepth.
func extractPaths(obj map[string]any, prefix string, depth int) []string {
	if depth >= maxObjectDepth {
		return nil
	}
	var paths []string
	for k, v := range obj {
		path := buildPath(prefix, k)
		paths = append(paths, path)
		switch child := v.(type) {
		case map[string]any:
			paths = append(paths, extractPaths(child, path, depth+1)...)
		case []any:
			for _, item := range child {
				if m, ok := item.(map[string]any); ok {
					paths = append(paths, extractPaths(m, path+"[]", depth+1)...)
				}
			}
		}
	}
	return paths
}

// buildSystemMappings converts ingest-model system suggestions into service-layer types.
func buildSystemMappings(suggestions []*ingestmod.MappingSuggestion) ([]SuggestionFieldMap, []MatchRuleSuggestion) {
	var mappings []SuggestionFieldMap
	var rules []MatchRuleSuggestion

	seen := make(map[string]bool)
	rulesSeen := make(map[string]bool)

	for _, sug := range suggestions {
		if sug == nil {
			continue
		}
		for _, fm := range sug.FieldMappings {
			if seen[fm.SourceField] {
				continue
			}
			seen[fm.SourceField] = true
			vcSource := ""
			if len(fm.ValueCodes) > 0 {
				vcSource = "system"
			}
			mappings = append(mappings, SuggestionFieldMap{
				SourceField:     fm.SourceField,
				TargetField:     fm.TargetField,
				ValueCodes:      fm.ValueCodes,
				ValueCodeSource: vcSource,
				Source:          "system",
			})
		}
		for _, rule := range sug.MatchRule.Rules {
			if rulesSeen[rule.Field] {
				continue
			}
			rulesSeen[rule.Field] = true
			rules = append(rules, MatchRuleSuggestion{
				FieldPath: rule.Field,
				Operator:  rule.Operator,
				Value:     rule.Value,
				Required:  true,
				Source:    "system",
			})
		}
	}

	return mappings, rules
}

// computeConfidence returns a confidence score [0.0, 1.0] for an AI-augmented result.
func computeConfidence(raw aiprovider.AISuggestRawResult, merged []SuggestionFieldMap, conflicts []SuggestConflict) float64 {
	if len(merged) == 0 {
		return 0.0
	}
	// Base: ratio of AI fields that had no conflicts.
	conflictSet := make(map[string]bool, len(conflicts))
	for _, c := range conflicts {
		conflictSet[c.FieldPath] = true
	}

	aiCount := len(raw.FieldMappings)
	if aiCount == 0 {
		return 0.5
	}
	conflicted := len(conflicts)
	score := 1.0 - float64(conflicted)/float64(aiCount)
	// Add bonus for having a valid event type.
	if raw.SuggestedEventType != "" {
		score += 0.1
	}
	// Add bonus for match rules.
	if len(raw.MatchRules) >= 2 {
		score += 0.1
	}
	if score > maxConfidence {
		score = maxConfidence
	}
	if score < minConfidence {
		score = minConfidence
	}
	return score
}

// computeSystemConfidence returns a simple confidence score for system-only results.
func computeSystemConfidence(mappings []SuggestionFieldMap) float64 {
	if len(mappings) == 0 {
		return 0.0
	}
	return 0.6
}
