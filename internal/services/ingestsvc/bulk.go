// internal/services/ingestsvc/bulk.go
package ingestsvc

import (
	"context"
	"strings"

	"github.com/hotkhwan/gateway-api/internal/repo/ingestrepo"
	"github.com/hotkhwan/gateway-api/models/ingestmod"
	"github.com/hotkhwan/gateway-api/utils/traceutil"
	"github.com/rs/zerolog"
)

const maxBulkSize = 100

// BulkService handles bulk approve/reject/delete/applyTemplate operations on pending events.
type BulkService struct {
	approvalSvc  *ApprovalService
	tmplMatcher  *TemplateMatcher
	templateRepo *ingestrepo.MappingTemplateRepo
	log          zerolog.Logger
}

// NewBulkService creates a BulkService.
func NewBulkService(
	approvalSvc *ApprovalService,
	tmplMatcher *TemplateMatcher,
	templateRepo *ingestrepo.MappingTemplateRepo,
	log zerolog.Logger,
) *BulkService {
	if approvalSvc == nil || tmplMatcher == nil || templateRepo == nil {
		panic("BulkService: approvalSvc, tmplMatcher, templateRepo are required")
	}
	return &BulkService{
		approvalSvc:  approvalSvc,
		tmplMatcher:  tmplMatcher,
		templateRepo: templateRepo,
		log:          log,
	}
}

// BulkApprove approves up to maxBulkSize pending events. Partial success is reported via BulkResult.
func (s *BulkService) BulkApprove(ctx context.Context, tenantId, orgId, callerUserId string, eventIds []string) *BulkResult {
	_, end, log := traceutil.StartLite(ctx, "gateway.ingestsvc", "BulkService.BulkApprove", "ingestsvc", "BulkApprove")
	defer end()

	log.Info().Str("orgId", orgId).Int("count", len(eventIds)).Msg("📥 [BulkApprove] starting bulk approve")

	result := &BulkResult{Succeeded: []string{}, Failed: []BulkFailItem{}}
	for _, id := range bulkSlice(eventIds) {
		if _, err := s.approvalSvc.ApproveEvent(ctx, tenantId, orgId, id, callerUserId, nil); err != nil {
			result.Failed = append(result.Failed, BulkFailItem{EventId: id, Reason: err.Error()})
			log.Warn().Str("orgId", orgId).Str("eventId", id).Err(err).Msg("❌ [BulkApprove] failed")
		} else {
			result.Succeeded = append(result.Succeeded, id)
		}
	}

	log.Info().Str("orgId", orgId).Int("succeeded", len(result.Succeeded)).Int("failed", len(result.Failed)).Msg("✅ [BulkApprove] done")
	return result
}

// BulkReject rejects up to maxBulkSize pending events. Partial success is reported via BulkResult.
func (s *BulkService) BulkReject(ctx context.Context, tenantId, orgId, callerUserId string, eventIds []string) *BulkResult {
	_, end, log := traceutil.StartLite(ctx, "gateway.ingestsvc", "BulkService.BulkReject", "ingestsvc", "BulkReject")
	defer end()

	log.Info().Str("orgId", orgId).Int("count", len(eventIds)).Msg("📥 [BulkReject] starting bulk reject")

	result := &BulkResult{Succeeded: []string{}, Failed: []BulkFailItem{}}
	for _, id := range bulkSlice(eventIds) {
		if err := s.approvalSvc.RejectEvent(ctx, tenantId, orgId, id, callerUserId); err != nil {
			result.Failed = append(result.Failed, BulkFailItem{EventId: id, Reason: err.Error()})
			log.Warn().Str("orgId", orgId).Str("eventId", id).Err(err).Msg("❌ [BulkReject] failed")
		} else {
			result.Succeeded = append(result.Succeeded, id)
		}
	}

	log.Info().Str("orgId", orgId).Int("succeeded", len(result.Succeeded)).Int("failed", len(result.Failed)).Msg("✅ [BulkReject] done")
	return result
}

// BulkDelete deletes up to maxBulkSize pending events. Partial success is reported via BulkResult.
func (s *BulkService) BulkDelete(ctx context.Context, tenantId, orgId, callerUserId string, eventIds []string) *BulkResult {
	_, end, log := traceutil.StartLite(ctx, "gateway.ingestsvc", "BulkService.BulkDelete", "ingestsvc", "BulkDelete")
	defer end()

	log.Info().Str("orgId", orgId).Int("count", len(eventIds)).Msg("📥 [BulkDelete] starting bulk delete")

	result := &BulkResult{Succeeded: []string{}, Failed: []BulkFailItem{}}
	for _, id := range bulkSlice(eventIds) {
		if err := s.approvalSvc.DeletePendingEvent(ctx, tenantId, orgId, id, callerUserId); err != nil {
			result.Failed = append(result.Failed, BulkFailItem{EventId: id, Reason: err.Error()})
			log.Warn().Str("orgId", orgId).Str("eventId", id).Err(err).Msg("❌ [BulkDelete] failed")
		} else {
			result.Succeeded = append(result.Succeeded, id)
		}
	}

	log.Info().Str("orgId", orgId).Int("succeeded", len(result.Succeeded)).Int("failed", len(result.Failed)).Msg("✅ [BulkDelete] done")
	return result
}

// BulkApplyTemplate applies templateId's FieldMappings to each pending event then auto-approves it.
// Up to maxBulkSize events per call. Partial success is reported via BulkResult.
func (s *BulkService) BulkApplyTemplate(ctx context.Context, tenantId, orgId, callerUserId, templateId string, eventIds []string) *BulkResult {
	_, end, log := traceutil.StartLite(ctx, "gateway.ingestsvc", "BulkService.BulkApplyTemplate", "ingestsvc", "BulkApplyTemplate")
	defer end()

	log.Info().Str("orgId", orgId).Str("templateId", templateId).Int("count", len(eventIds)).Msg("📥 [BulkApplyTemplate] starting bulk apply template")

	result := &BulkResult{Succeeded: []string{}, Failed: []BulkFailItem{}}

	// Load template once for the whole batch
	tmpl, err := s.templateRepo.FindById(ctx, orgId, templateId)
	if err != nil {
		log.Error().Str("orgId", orgId).Str("templateId", templateId).Err(err).Msg("❌ [BulkApplyTemplate] template not found")
		for _, id := range eventIds {
			result.Failed = append(result.Failed, BulkFailItem{EventId: id, Reason: ErrTemplateNotFound.Error()})
		}
		return result
	}

	for _, id := range bulkSlice(eventIds) {
		// Fetch pending event
		pending, err := s.approvalSvc.GetPendingEvent(ctx, tenantId, orgId, id)
		if err != nil {
			result.Failed = append(result.Failed, BulkFailItem{EventId: id, Reason: err.Error()})
			log.Warn().Str("orgId", orgId).Str("eventId", id).Err(err).Msg("❌ [BulkApplyTemplate] fetch failed")
			continue
		}

		// Apply template field mappings
		mapped, missing := s.tmplMatcher.ApplyMappings(pending.RawBody, tmpl.Mappings)
		if len(missing) > 0 {
			reason := "missing required fields: " + strings.Join(missing, ", ")
			result.Failed = append(result.Failed, BulkFailItem{EventId: id, Reason: reason})
			log.Warn().Str("orgId", orgId).Str("eventId", id).Strs("missing", missing).Msg("❌ [BulkApplyTemplate] missing required fields")
			continue
		}

		// Build EventUpdateInput from mapped fields
		updates := &ingestmod.EventUpdateInput{}
		if v, ok := mapped["eventType"].(string); ok && v != "" {
			updates.EventType = v
		}
		if v, ok := mapped["name"].(string); ok && v != "" {
			updates.Name = v
		}
		if v, ok := mapped["location.lat"].(float64); ok {
			updates.Lat = &v
		}
		if v, ok := mapped["location.lng"].(float64); ok {
			updates.Lng = &v
		}

		// Auto-approve with mapped metadata
		if _, err := s.approvalSvc.ApproveEvent(ctx, tenantId, orgId, id, callerUserId, updates); err != nil {
			result.Failed = append(result.Failed, BulkFailItem{EventId: id, Reason: err.Error()})
			log.Warn().Str("orgId", orgId).Str("eventId", id).Err(err).Msg("❌ [BulkApplyTemplate] approve failed")
			continue
		}

		log.Info().
			Str("orgId", orgId).
			Str("eventId", id).
			Str("templateId", templateId).
			Msg("✅ [BulkApplyTemplate] mapped and approved")
		result.Succeeded = append(result.Succeeded, id)
	}

	log.Info().Str("orgId", orgId).Int("succeeded", len(result.Succeeded)).Int("failed", len(result.Failed)).Msg("✅ [BulkApplyTemplate] done")
	return result
}

// bulkSlice caps the slice at maxBulkSize.
func bulkSlice(ids []string) []string {
	if len(ids) > maxBulkSize {
		return ids[:maxBulkSize]
	}
	return ids
}
