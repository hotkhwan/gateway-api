package ingestsvc

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/hotkhwan/gateway-api/config"
	"github.com/hotkhwan/gateway-api/internal/repo/cacheevt"
	"github.com/hotkhwan/gateway-api/internal/repo/ingestdetailsrepo"
	"github.com/hotkhwan/gateway-api/internal/repo/ingestmgmtrepo"
	"github.com/hotkhwan/gateway-api/models/ingestmod"
	"github.com/hotkhwan/gateway-api/utils/traceutil"
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"
)

type ApprovalService struct {
	eventMgmtRepo    *ingestmgmtrepo.EventManagementRepo
	eventDetailsRepo *ingestdetailsrepo.EventDetailsRepo
	redis            *redis.Client
	logger           zerolog.Logger
}

func NewApprovalService(
	eventMgmtRepo *ingestmgmtrepo.EventManagementRepo,
	eventDetailsRepo *ingestdetailsrepo.EventDetailsRepo,
	redis *redis.Client,
	logger zerolog.Logger,
) *ApprovalService {
	if eventMgmtRepo == nil || eventDetailsRepo == nil || redis == nil {
		panic("ApprovalService: eventMgmtRepo, eventDetailsRepo, redis and logger are required")
	}
	return &ApprovalService{
		eventMgmtRepo:    eventMgmtRepo,
		eventDetailsRepo: eventDetailsRepo,
		redis:            redis,
		logger:           logger,
	}
}

// ApproveEvent approves a pending event and moves it to event_details.
// Gate check → canonical event build → store → Kafka publish (raw.events).
func (s *ApprovalService) ApproveEvent(
	ctx context.Context,
	tenantId, orgId, eventId, approvedBy string,
	updates *ingestmod.EventUpdateInput,
) (*ingestmod.EventDetail, error) {
	ctx, end, log := traceutil.StartLite(ctx, "gateway.ingestsvc", "ApprovalService.ApproveEvent", "ingestsvc", "ApproveEvent")
	defer end()

	// 1) Fetch pending event
	pending, err := s.eventMgmtRepo.FindByEventId(ctx, tenantId, orgId, eventId)
	if err != nil {
		if err == ingestmgmtrepo.ErrNotFound {
			return nil, ErrEventNotFound
		}
		return nil, err
	}

	if pending.StatusName == "rejected" {
		return nil, ErrEventAlreadyRejected
	}

	// 2) Approval gate — validate event is ready for approval
	if err := runApprovalGate(pending); err != nil {
		log.Warn().
			Str("tenantId", tenantId).
			Str("orgId", orgId).
			Str("eventId", eventId).
			Err(err).
			Msg("❌ [ApproveEvent] approval gate blocked")
		return nil, err
	}

	// 3) Apply metadata updates if provided
	if updates != nil {
		if updates.Name != "" {
			pending.Name = updates.Name
		}
		if updates.Lat != nil {
			pending.Lat = *updates.Lat
		}
		if updates.Lng != nil {
			pending.Lng = *updates.Lng
		}
		if updates.EventType != "" {
			pending.EventType = updates.EventType
		}
		pending.UpdatedAt = time.Now().UTC()
	}

	// 4) Build canonical event
	now := time.Now().UTC()
	canonical := buildCanonicalEvent(pending, now)

	// 5) Create approved event detail
	eventDetail := &ingestmod.EventDetail{
		EventId:  pending.EventId,
		TenantId: pending.TenantId,
		OrgId:    pending.OrgId,
		Name:     pending.Name,
		Lat:      pending.Lat,
		Lng:      pending.Lng,
		EventType: pending.EventType,
		NormalizedData: map[string]any{
			"eventType":  pending.EventType,
			"occurredAt": pending.CreatedAt,
			"source":     canonical.Source,
			"location":   canonical.Location,
			"payload":    pending.RawBody,
		},
		SourceIp:       pending.SourceIp,
		IngestedAt:     pending.CreatedAt,
		ApprovedAt:     now,
		CreatedAt:      now,
		UpdatedAt:      now,
		PendingEventId: pending.ID,
	}

	// 6) Store in event_details
	if err := s.eventDetailsRepo.Insert(ctx, eventDetail); err != nil {
		return nil, fmt.Errorf("failed to store approved event: %w", err)
	}

	// 7) Update pending event status
	pending.Status = true
	pending.StatusName = "approved"
	pending.ApprovedBy = approvedBy
	pending.ApprovedAt = now

	if err := s.eventMgmtRepo.Update(ctx, pending.ID.Hex(), pending); err != nil {
		return nil, fmt.Errorf("failed to update pending event: %w", err)
	}

	// 8) Publish CanonicalEvent to raw.events (traced, non-blocking)
	s.publishCanonicalEvent(ctx, canonical, orgId, eventId)

	// 9) Update Redis cache
	if err := cacheevt.SetEventStatusApproved(ctx, tenantId, eventId); err != nil {
		log.Warn().
			Str("tenantId", tenantId).
			Str("eventId", eventId).
			Err(err).
			Msg("⚠️ [ApproveEvent] failed to update Redis cache (non-critical)")
	}

	// 10) Set device:eventType approval cache for auto-processing future events
	// 10) Cache device:eventType approved for auto-processing future events.
	//     Cache both the final eventType AND the original suggestedType (auto-detected),
	//     since new events are classified by suggestedType, not the admin-overridden eventType.
	if pending.DeviceKey != "" && pending.DeviceRef != nil {
		deviceKey := pending.DeviceRef.Type + ":" + pending.DeviceRef.ID
		for _, et := range uniqueStrings(pending.EventType, pending.SuggestedType) {
			if et == "" {
				continue
			}
			if err := cacheevt.SetDeviceEventTypeApproved(ctx, tenantId, deviceKey, et); err != nil {
				log.Warn().
					Str("tenantId", tenantId).
					Str("deviceKey", deviceKey).
					Str("eventType", et).
					Err(err).
					Msg("⚠️ [ApproveEvent] failed to set device:eventType approval cache (non-critical)")
			}
		}
	}

	log.Info().
		Str("tenantId", tenantId).
		Str("orgId", orgId).
		Str("eventId", eventId).
		Str("approvedBy", approvedBy).
		Msg("✅ [ApproveEvent] event approved")

	return eventDetail, nil
}

// RejectEvent marks a pending event as rejected.
func (s *ApprovalService) RejectEvent(
	ctx context.Context,
	tenantId, orgId, eventId, rejectedBy string,
) error {
	ctx, end, log := traceutil.StartLite(ctx, "gateway.ingestsvc", "ApprovalService.RejectEvent", "ingestsvc", "RejectEvent")
	defer end()

	// 1) Fetch pending event
	pending, err := s.eventMgmtRepo.FindByEventId(ctx, tenantId, orgId, eventId)
	if err != nil {
		if err == ingestmgmtrepo.ErrNotFound {
			return ErrEventNotFound
		}
		return err
	}

	// 2) Update status
	now := time.Now().UTC()
	pending.StatusName = "rejected"
	pending.UpdatedAt = now
	pending.ApprovedBy = rejectedBy
	pending.ApprovedAt = now

	if err := s.eventMgmtRepo.Update(ctx, pending.ID.Hex(), pending); err != nil {
		return fmt.Errorf("failed to update event: %w", err)
	}

	// 3) Update Redis event status cache
	if err := cacheevt.SetEventStatusRejected(ctx, tenantId, eventId); err != nil {
		log.Warn().
			Str("tenantId", tenantId).
			Str("eventId", eventId).
			Err(err).
			Msg("⚠️ [RejectEvent] failed to update Redis cache (non-critical)")
	}

	// 4) Clear device:eventType auto-approval cache so future events re-enter the pending queue.
	//    Required when rejecting a previously-approved event that had its device cached.
	if pending.DeviceKey != "" && pending.EventType != "" {
		deviceKey := pending.DeviceKey
		if pending.DeviceRef != nil {
			deviceKey = pending.DeviceRef.Type + ":" + pending.DeviceRef.ID
		}
		if err := cacheevt.InvalidateDeviceEventTypeApproval(ctx, tenantId, deviceKey, pending.EventType); err != nil {
			log.Warn().
				Str("tenantId", tenantId).
				Str("deviceKey", deviceKey).
				Str("eventType", pending.EventType).
				Err(err).
				Msg("⚠️ [RejectEvent] failed to invalidate device:eventType approval cache (non-critical)")
		}
	}

	log.Info().
		Str("tenantId", tenantId).
		Str("orgId", orgId).
		Str("eventId", eventId).
		Str("rejectedBy", rejectedBy).
		Msg("✅ [RejectEvent] event rejected")

	return nil
}

// ListPending lists pending events with pagination.
func (s *ApprovalService) ListPending(
	ctx context.Context,
	input *ingestmod.ListEventsInput,
) (*ingestmod.PaginatedResult, error) {
	ctx, end, _ := traceutil.StartLite(ctx, "gateway.ingestsvc", "ApprovalService.ListPending", "ingestsvc", "ListPending")
	defer end()

	if input.Page < 1 {
		input.Page = 1
	}
	if input.PerPage <= 0 {
		input.PerPage = 10
	}
	if input.PerPage > 100 {
		input.PerPage = 100
	}

	items, pagination, err := s.eventMgmtRepo.ListPending(
		ctx,
		input.TenantId,
		input.OrgId,
		input.StatusName,
		input.EventType,
		input.Page,
		input.PerPage,
		input.SortField,
		input.SortOrder,
	)
	if err != nil {
		return nil, err
	}

	return &ingestmod.PaginatedResult{
		Items:       items,
		Total:       int64(pagination.TotalRecords),
		TotalPages:  pagination.TotalPages,
		CurrentPage: pagination.Page,
	}, nil
}

// GetPendingEvent gets a single pending event.
func (s *ApprovalService) GetPendingEvent(
	ctx context.Context,
	tenantId, orgId, eventId string,
) (*ingestmod.EventManagement, error) {
	ctx, end, _ := traceutil.StartLite(ctx, "gateway.ingestsvc", "ApprovalService.GetPendingEvent", "ingestsvc", "GetPendingEvent")
	defer end()
	return s.eventMgmtRepo.FindByEventId(ctx, tenantId, orgId, eventId)
}

// ListApproved lists approved events with pagination.
func (s *ApprovalService) ListApproved(
	ctx context.Context,
	input *ingestmod.ListEventsInput,
) (*ingestmod.PaginatedResult, error) {
	ctx, end, _ := traceutil.StartLite(ctx, "gateway.ingestsvc", "ApprovalService.ListApproved", "ingestsvc", "ListApproved")
	defer end()

	if input.Page < 1 {
		input.Page = 1
	}
	if input.PerPage <= 0 {
		input.PerPage = 10
	}
	if input.PerPage > 100 {
		input.PerPage = 100
	}

	items, pagination, err := s.eventDetailsRepo.ListApproved(
		ctx,
		input.TenantId,
		input.OrgId,
		input.EventType,
		input.Page,
		input.PerPage,
		input.SortField,
		input.SortOrder,
	)
	if err != nil {
		return nil, err
	}

	return &ingestmod.PaginatedResult{
		Items:       items,
		Total:       int64(pagination.TotalRecords),
		TotalPages:  pagination.TotalPages,
		CurrentPage: pagination.Page,
	}, nil
}

// GetApprovedEvent gets a single approved event.
func (s *ApprovalService) GetApprovedEvent(
	ctx context.Context,
	tenantId, orgId, eventId string,
) (*ingestmod.EventDetail, error) {
	ctx, end, _ := traceutil.StartLite(ctx, "gateway.ingestsvc", "ApprovalService.GetApprovedEvent", "ingestsvc", "GetApprovedEvent")
	defer end()

	event, err := s.eventDetailsRepo.FindByEventId(ctx, tenantId, orgId, eventId)
	if err != nil {
		if err == ingestdetailsrepo.ErrNotFound {
			return nil, ErrEventNotFound
		}
		return nil, err
	}
	return event, nil
}

// UpdatePendingEvent updates a pending event's metadata.
func (s *ApprovalService) UpdatePendingEvent(
	ctx context.Context,
	tenantId, orgId, eventId, callerUserId string,
	updates *ingestmod.EventUpdateInput,
) error {
	ctx, end, log := traceutil.StartLite(ctx, "gateway.ingestsvc", "ApprovalService.UpdatePendingEvent", "ingestsvc", "UpdatePendingEvent")
	defer end()

	// 1) Fetch pending event
	pending, err := s.eventMgmtRepo.FindByEventId(ctx, tenantId, orgId, eventId)
	if err != nil {
		if err == ingestmgmtrepo.ErrNotFound {
			return ErrEventNotFound
		}
		return err
	}

	// 2) Apply updates
	if updates.Name != "" {
		pending.Name = updates.Name
	}
	if updates.Description != nil {
		pending.Description = updates.Description
	}
	if updates.Lat != nil {
		pending.Lat = *updates.Lat
	}
	if updates.Lng != nil {
		pending.Lng = *updates.Lng
	}
	if updates.EventType != "" {
		pending.EventType = updates.EventType
	}
	if updates.Priority != "" {
		pending.Priority = updates.Priority
	}
	if updates.Tags != nil {
		pending.Tags = updates.Tags
	}

	// 3) Persist
	if err := s.eventMgmtRepo.Update(ctx, pending.ID.Hex(), pending); err != nil {
		log.Error().Str("orgId", orgId).Str("eventId", eventId).Err(err).Msg("❌ [UpdatePendingEvent] db update failed")
		return err
	}

	// 4) Invalidate Redis cache
	return cacheevt.InvalidateEventStatus(ctx, tenantId, eventId)
}

// DeletePendingEvent deletes a pending or rejected event.
func (s *ApprovalService) DeletePendingEvent(
	ctx context.Context,
	tenantId, orgId, eventId, callerUserId string,
) error {
	ctx, end, log := traceutil.StartLite(ctx, "gateway.ingestsvc", "ApprovalService.DeletePendingEvent", "ingestsvc", "DeletePendingEvent")
	defer end()

	// 1) Fetch pending event
	pending, err := s.eventMgmtRepo.FindByEventId(ctx, tenantId, orgId, eventId)
	if err != nil {
		if err == ingestmgmtrepo.ErrNotFound {
			return ErrEventNotFound
		}
		return err
	}

	// 3) Delete
	if err := s.eventMgmtRepo.Delete(ctx, pending.ID.Hex()); err != nil {
		log.Error().Str("orgId", orgId).Str("eventId", eventId).Err(err).Msg("❌ [DeletePendingEvent] delete failed")
		return err
	}

	// 4) Invalidate Redis cache
	return cacheevt.InvalidateEventStatus(ctx, tenantId, eventId)
}

// ============================================================
// Kafka publish
// ============================================================

// publishCanonicalEvent publishes a CanonicalEvent to raw.events (traced, non-blocking).
// The raw.events topic is consumed by the normalizer service which produces to normalized.events.
func (s *ApprovalService) publishCanonicalEvent(ctx context.Context, canonical *ingestmod.CanonicalEvent, orgId, eventId string) {
	kCtx, kEnd, kLog := traceutil.StartLite(ctx, "gateway.ingestsvc", "ApprovalService.kafkaPublish", "ingestsvc", "kafkaPublish")
	defer kEnd()

	topic := config.TopicEnv("KAFKA_TOPIC_RAW_EVENTS", "raw.events")
	payload, err := json.Marshal(canonical)
	if err != nil {
		kLog.Error().Str("eventId", eventId).Err(err).Msg("❌ [kafkaPublish] marshal failed")
		return
	}

	headers := map[string]string{
		"eventId":   eventId,
		"eventType": canonical.EventType,
		"orgId":     orgId,
		"tenantId":  canonical.TenantId,
	}

	if err := config.SendToKafkaWithCtx(kCtx, topic, orgId, payload, headers); err != nil {
		kLog.Error().
			Str("topic", topic).
			Str("orgId", orgId).
			Str("eventId", eventId).
			Err(err).
			Msg("❌ [kafkaPublish] send failed (non-blocking)")
		return
	}

	kLog.Info().
		Str("topic", topic).
		Str("orgId", orgId).
		Str("eventId", eventId).
		Str("eventType", canonical.EventType).
		Msg("✅ [kafkaPublish] CanonicalEvent published to raw.events")
}

// ============================================================
// Canonical event builder
// ============================================================

// buildCanonicalEvent constructs a CanonicalEvent from a pending event.
// Published to raw.events; consumed by the downstream normalizer service.
func buildCanonicalEvent(pending *ingestmod.EventManagement, now time.Time) *ingestmod.CanonicalEvent {
	deviceId := ""
	deviceType := ""
	if pending.DeviceRef != nil {
		deviceId = pending.DeviceRef.ID
		deviceType = pending.DeviceRef.Type
	}

	return &ingestmod.CanonicalEvent{
		EventId:    pending.EventId,
		TenantId:   pending.TenantId,
		EventType:  pending.EventType,
		OccurredAt: pending.CreatedAt,
		Source: ingestmod.SourceInfo{
			DeviceId:   deviceId,
			DeviceType: deviceType,
			OrgId:      pending.OrgId,
		},
		Location: ingestmod.LocationInfo{
			Lat: pending.Lat,
			Lng: pending.Lng,
		},
		Payload:   pending.RawBody,
		CreatedAt: now,
	}
}

// uniqueStrings returns the input strings deduplicated and preserving order.
func uniqueStrings(vals ...string) []string {
	seen := make(map[string]struct{}, len(vals))
	out := make([]string, 0, len(vals))
	for _, v := range vals {
		if _, ok := seen[v]; !ok {
			seen[v] = struct{}{}
			out = append(out, v)
		}
	}
	return out
}
