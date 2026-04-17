// internal/services/aiconfigdraftsvc/service.go
package aiconfigdraftsvc

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hotkhwan/gateway-api/internal/logger"
	"github.com/hotkhwan/gateway-api/models/ingestmod"
	"github.com/hotkhwan/gateway-api/utils/traceutil"
	"github.com/rs/zerolog"
	"go.mongodb.org/mongo-driver/bson"
)

// ErrDraftNotFound is returned when a draft cannot be located.
var ErrDraftNotFound = errors.New("aiconfigdraftsvc: draft not found")

// configDraftRepo is the persistence interface for ConfigDraft documents.
type configDraftRepo interface {
	Insert(ctx context.Context, draft *ConfigDraft) error
	FindByID(ctx context.Context, workspaceId, draftId string) (*ConfigDraft, error)
	Update(ctx context.Context, workspaceId, draftId string, update bson.M) error
}

// suggestionSource provides mapping suggestions keyed by source family.
type suggestionSource interface {
	GetByFamily(sourceFamily string) []*ingestmod.MappingSuggestion
}

// ConfigDraftService orchestrates config draft creation and refinement.
type ConfigDraftService struct {
	repo          configDraftRepo
	suggestionSvc suggestionSource
	log           zerolog.Logger
}

// NewConfigDraftService constructs a new ConfigDraftService.
func NewConfigDraftService(repo configDraftRepo, suggestionSvc suggestionSource) *ConfigDraftService {
	return &ConfigDraftService{
		repo:          repo,
		suggestionSvc: suggestionSvc,
		log:           logger.WithMeta("aiconfigdraftsvc", "ConfigDraftService"),
	}
}

// FromPrompt parses a natural-language prompt, resolves entities, detects missing
// fields, builds a ConfigDraft, and persists it.
func (s *ConfigDraftService) FromPrompt(ctx context.Context, workspaceId, userId, prompt string) (*ConfigDraft, error) {
	ctx, end, log := traceutil.StartLite(ctx,
		"github.com/hotkhwan/gateway-api/aiconfigdraftsvc",
		"aiconfigdraftsvc.FromPrompt",
		"aiconfigdraftsvc", "FromPrompt",
	)
	defer end()

	if strings.TrimSpace(prompt) == "" {
		return nil, fmt.Errorf("aiconfigdraftsvc: prompt is required")
	}

	// 1. Parse intent.
	intent := parseIntent(prompt)

	// 2. Resolve entities against suggestions.
	suggestions := s.suggestionSvc.GetByFamily(intent.SourceFamily)
	resolveEntities(&intent, suggestions)

	// 3. Detect missing fields.
	missing := detectMissing(intent)

	// 4. Build match conditions from resolved conditions.
	var conditions []ingestmod.SuggestionRuleItem
	for _, cond := range intent.Conditions {
		if cond.Resolved {
			conditions = append(conditions, ingestmod.SuggestionRuleItem{
				Field:    cond.ResolvedPath,
				Operator: cond.Operator,
				Value:    cond.ResolvedValue,
			})
		} else {
			conditions = append(conditions, ingestmod.SuggestionRuleItem{
				Field:    cond.FieldHint,
				Operator: cond.Operator,
				Value:    cond.ValueHint,
			})
		}
	}

	// 5. Determine status.
	status := "ready"
	if len(missing) > 0 {
		status = "incomplete"
	}

	// 6. Build warnings.
	var warnings []string
	for _, action := range intent.DeliveryActions {
		warnings = append(warnings, fmt.Sprintf("action type: %s", action.Type))
	}

	// 7. Assemble draft.
	now := time.Now().UTC()
	draft := &ConfigDraft{
		DraftID:         uuid.NewString(),
		WorkspaceID:     workspaceId,
		Status:          status,
		SourceFamily:    intent.SourceFamily,
		MatchConditions: conditions,
		MissingFields:   missing,
		Warnings:        warnings,
		ReviewSummary:   []string{},
		RedactedPrompt:  intent.RawPrompt,
		CreatedBy:       userId,
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	// 8. Persist.
	if err := s.repo.Insert(ctx, draft); err != nil {
		log.Error().Err(err).Str("workspaceId", workspaceId).Msg("failed to insert config draft")
		return nil, fmt.Errorf("aiconfigdraftsvc: insert draft: %w", err)
	}

	log.Info().
		Str("workspaceId", workspaceId).
		Str("draftId", draft.DraftID).
		Str("status", draft.Status).
		Msg("config draft created from prompt")

	return draft, nil
}

// Refine applies user-supplied answers to missing field hints, then re-evaluates
// whether all required fields are now resolved.
func (s *ConfigDraftService) Refine(ctx context.Context, workspaceId, draftId string, req RefineRequest) (*ConfigDraft, error) {
	ctx, end, log := traceutil.StartLite(ctx,
		"github.com/hotkhwan/gateway-api/aiconfigdraftsvc",
		"aiconfigdraftsvc.Refine",
		"aiconfigdraftsvc", "Refine",
	)
	defer end()

	draft, err := s.repo.FindByID(ctx, workspaceId, draftId)
	if err != nil {
		return nil, mapRepoErr(err, draftId)
	}

	// Apply answers: remove hints whose field is in the answers map.
	var remaining []MissingFieldHint
	for _, hint := range draft.MissingFields {
		if _, answered := req.Answers[hint.Field]; !answered {
			remaining = append(remaining, hint)
		}
	}
	draft.MissingFields = remaining

	// Update status.
	if len(remaining) == 0 {
		draft.Status = "ready"
	}
	draft.UpdatedAt = time.Now().UTC()

	update := bson.M{
		"missingFields": draft.MissingFields,
		"status":        draft.Status,
		"updatedAt":     draft.UpdatedAt,
	}

	if err := s.repo.Update(ctx, workspaceId, draftId, update); err != nil {
		log.Error().Err(err).Str("draftId", draftId).Msg("failed to update draft after refine")
		return nil, fmt.Errorf("aiconfigdraftsvc: refine draft: %w", err)
	}

	log.Info().
		Str("workspaceId", workspaceId).
		Str("draftId", draftId).
		Str("status", draft.Status).
		Int("remainingMissing", len(remaining)).
		Msg("config draft refined")

	return draft, nil
}

// DryRun loads a draft and simulates it against a sample payload.
func (s *ConfigDraftService) DryRun(ctx context.Context, workspaceId, draftId string, samplePayload map[string]any) (*DryRunResult, error) {
	ctx, end, log := traceutil.StartLite(ctx,
		"github.com/hotkhwan/gateway-api/aiconfigdraftsvc",
		"aiconfigdraftsvc.DryRun",
		"aiconfigdraftsvc", "DryRun",
	)
	defer end()

	draft, err := s.repo.FindByID(ctx, workspaceId, draftId)
	if err != nil {
		return nil, mapRepoErr(err, draftId)
	}

	result := DryRun(draft, samplePayload)

	log.Info().
		Str("workspaceId", workspaceId).
		Str("draftId", draftId).
		Bool("matched", result.Matched).
		Msg("config draft dry-run complete")

	return &result, nil
}

// Save marks the draft status as "ready" and persists the change.
func (s *ConfigDraftService) Save(ctx context.Context, workspaceId, draftId, userId string) (*ConfigDraft, error) {
	ctx, end, log := traceutil.StartLite(ctx,
		"github.com/hotkhwan/gateway-api/aiconfigdraftsvc",
		"aiconfigdraftsvc.Save",
		"aiconfigdraftsvc", "Save",
	)
	defer end()

	draft, err := s.repo.FindByID(ctx, workspaceId, draftId)
	if err != nil {
		return nil, mapRepoErr(err, draftId)
	}

	draft.Status = "ready"
	draft.UpdatedAt = time.Now().UTC()

	update := bson.M{
		"status":    draft.Status,
		"updatedAt": draft.UpdatedAt,
	}

	if err := s.repo.Update(ctx, workspaceId, draftId, update); err != nil {
		log.Error().Err(err).Str("draftId", draftId).Msg("failed to save draft")
		return nil, fmt.Errorf("aiconfigdraftsvc: save draft: %w", err)
	}

	log.Info().
		Str("workspaceId", workspaceId).
		Str("draftId", draftId).
		Str("savedBy", userId).
		Msg("config draft saved")

	return draft, nil
}

// mapRepoErr converts a repo-layer not-found error to the service sentinel.
func mapRepoErr(err error, draftId string) error {
	if errors.Is(err, ErrDraftNotFound) {
		return ErrDraftNotFound
	}
	return fmt.Errorf("aiconfigdraftsvc: load draft %s: %w", draftId, err)
}
