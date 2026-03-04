package eventsvc

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"time"

	"github.com/hotkhwan/gateway-api/config"
	"github.com/hotkhwan/gateway-api/internal/repo/cacheevt"
	"github.com/hotkhwan/gateway-api/internal/repo/eventdetailsrepo"
	"github.com/hotkhwan/gateway-api/internal/repo/eventmgmtrepo"
	"github.com/hotkhwan/gateway-api/models/eventmod"
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"
)

type ApprovalService struct {
	eventMgmtRepo    *eventmgmtrepo.EventManagementRepo
	eventDetailsRepo  *eventdetailsrepo.EventDetailsRepo
	redis             *redis.Client
	logger            zerolog.Logger
}

func NewApprovalService(
	eventMgmtRepo *eventmgmtrepo.EventManagementRepo,
	eventDetailsRepo *eventdetailsrepo.EventDetailsRepo,
	redis *redis.Client,
	logger zerolog.Logger,
) *ApprovalService {
	if eventMgmtRepo == nil || eventDetailsRepo == nil || redis == nil {
		panic("ApprovalService: eventMgmtRepo, eventDetailsRepo, redis and logger are required")
	}
	return &ApprovalService{
		eventMgmtRepo:   eventMgmtRepo,
		eventDetailsRepo: eventDetailsRepo,
		redis:          redis,
		logger:         logger,
	}
}

// ApproveEvent approves a pending event and moves it to event_details
func (s *ApprovalService) ApproveEvent(
	ctx context.Context,
	tenantId, orgId, eventId, approvedBy string,
	updates *eventmod.EventUpdateInput,
) (*eventmod.EventDetail, error) {
	// 1) Get pending event
	pending, err := s.eventMgmtRepo.FindByEventId(ctx, tenantId, orgId, eventId)
	if err != nil {
		if err == eventmgmtrepo.ErrNotFound {
			return nil, ErrEventNotFound
		}
		return nil, err
	}

	if pending.Status {
		return nil, ErrEventAlreadyApproved
	}

	if pending.StatusName == "rejected" {
		return nil, ErrEventAlreadyRejected
	}

	// 2) Apply metadata updates if provided
	if updates != nil {
		if updates.Name != "" {
			pending.Name = updates.Name
		}
		if !math.IsNaN(updates.Lat) {
			pending.Lat = updates.Lat
		}
		if !math.IsNaN(updates.Lng) {
			pending.Lng = updates.Lng
		}
		if updates.EventType != "" {
			pending.EventType = updates.EventType
		}
		pending.UpdatedAt = time.Now().UTC()
	}

	// 3) Normalize event data
	normalizedData, err := s.normalizeEvent(ctx, pending.EventType, pending.RawBody)
	if err != nil {
		return nil, fmt.Errorf("normalization failed: %w", err)
	}

	// 4) Create approved event detail
	now := time.Now().UTC()
	eventDetail := &eventmod.EventDetail{
		EventId:        pending.EventId,
		TenantId:       pending.TenantId,
		OrgId:          pending.OrgId,
		Name:           pending.Name,
		Lat:            pending.Lat,
		Lng:            pending.Lng,
		EventType:      pending.EventType,
		NormalizedData: normalizedData,
		SourceIp:       pending.SourceIp,
		IngestedAt:     pending.CreatedAt,
		ApprovedAt:     now,
		CreatedAt:      now,
		UpdatedAt:      now,
		PendingEventId: pending.ID,
	}

	// 5) Store in event_details
	if err := s.eventDetailsRepo.Insert(ctx, eventDetail); err != nil {
		return nil, fmt.Errorf("failed to store approved event: %w", err)
	}

	// 6) Update pending event status
	pending.Status = true
	pending.StatusName = "approved"
	pending.ApprovedBy = approvedBy
	pending.ApprovedAt = now

	if err := s.eventMgmtRepo.Update(ctx, pending.ID.Hex(), pending); err != nil {
		return nil, fmt.Errorf("failed to update pending event: %w", err)
	}

	// 7) Send to Kafka (normalized topic)
	topic := config.TopicEnv("KAFKA_TOPIC_NORMALIZED_EVENTS", "normalized.events")
	payload, _ := json.Marshal(eventDetail)
	if err := config.SendToKafkaWithCtx(ctx, topic, orgId, payload, nil); err != nil {
		// Log error but don't block approval
		s.logger.Error().
			Str("tenantId", tenantId).
			Str("orgId", orgId).
			Str("eventId", eventId).
			Err(err).
			Msg("kafka send failed (non-blocking)")
	}

	// 8) Update Redis cache
	if err := cacheevt.SetEventStatusApproved(ctx, tenantId, eventId); err != nil {
		s.logger.Warn().
			Str("tenantId", tenantId).
			Str("eventId", eventId).
			Err(err).
			Msg("failed to update Redis cache (non-critical)")
	}

	// 9) Set device:eventType approval cache for auto-processing future events
	// This allows future events from the same device:eventType to skip approval
	if pending.DeviceKey != "" && pending.EventType != "" {
		deviceKey := pending.DeviceRef.Type + ":" + pending.DeviceRef.ID
		if err := cacheevt.SetDeviceEventTypeApproved(ctx, tenantId, deviceKey, pending.EventType); err != nil {
			s.logger.Warn().
				Str("tenantId", tenantId).
				Str("deviceKey", deviceKey).
				Str("eventType", pending.EventType).
				Err(err).
				Msg("failed to set device:eventType approval cache (non-critical)")
		}
	}

	return eventDetail, nil
}

// RejectEvent marks a pending event as rejected
func (s *ApprovalService) RejectEvent(
	ctx context.Context,
	tenantId, orgId, eventId, rejectedBy string,
) error {
	// 1) Get pending event
	pending, err := s.eventMgmtRepo.FindByEventId(ctx, tenantId, orgId, eventId)
	if err != nil {
		if err == eventmgmtrepo.ErrNotFound {
			return ErrEventNotFound
		}
		return err
	}

	if pending.Status {
		return ErrEventAlreadyApproved
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

	// 3) Update Redis cache
	if err := cacheevt.SetEventStatusRejected(ctx, tenantId, eventId); err != nil {
		s.logger.Warn().
			Str("tenantId", tenantId).
			Str("eventId", eventId).
			Err(err).
			Msg("failed to update Redis cache (non-critical)")
	}

	return nil
}

// ListPending lists pending events with pagination
func (s *ApprovalService) ListPending(
	ctx context.Context,
	input *eventmod.ListEventsInput,
) (*eventmod.PaginatedResult, error) {
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

	return &eventmod.PaginatedResult{
		Items:       items,
		Total:       int64(pagination.TotalRecords),
		TotalPages:  pagination.TotalPages,
		CurrentPage: pagination.Page,
	}, nil
}

// GetPendingEvent gets a single pending event
func (s *ApprovalService) GetPendingEvent(
	ctx context.Context,
	tenantId, orgId, eventId string,
) (*eventmod.EventManagement, error) {
	return s.eventMgmtRepo.FindByEventId(ctx, tenantId, orgId, eventId)
}

// ListApproved lists approved events with pagination
func (s *ApprovalService) ListApproved(
	ctx context.Context,
	input *eventmod.ListEventsInput,
) (*eventmod.PaginatedResult, error) {
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

	return &eventmod.PaginatedResult{
		Items:       items,
		Total:       int64(pagination.TotalRecords),
		TotalPages:  pagination.TotalPages,
		CurrentPage: pagination.Page,
	}, nil
}

// GetApprovedEvent gets a single approved event
func (s *ApprovalService) GetApprovedEvent(
	ctx context.Context,
	tenantId, orgId, eventId string,
) (*eventmod.EventDetail, error) {
	event, err := s.eventDetailsRepo.FindByEventId(ctx, tenantId, orgId, eventId)
	if err != nil {
		if err == eventdetailsrepo.ErrNotFound {
			return nil, ErrEventNotFound
		}
		return nil, err
	}
	return event, nil
}

// UpdatePendingEvent updates a pending event's metadata
func (s *ApprovalService) UpdatePendingEvent(
	ctx context.Context,
	tenantId, orgId, eventId, callerUserId string,
	updates *eventmod.EventUpdateInput,
) error {
	// 1) Get pending event
	pending, err := s.eventMgmtRepo.FindByEventId(ctx, tenantId, orgId, eventId)
	if err != nil {
		if err == eventmgmtrepo.ErrNotFound {
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
	if !math.IsNaN(updates.Lat) {
		pending.Lat = updates.Lat
	}
	if !math.IsNaN(updates.Lng) {
		pending.Lng = updates.Lng
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

	// 3) Update
	if err := s.eventMgmtRepo.Update(ctx, pending.ID.Hex(), pending); err != nil {
		return err
	}

	// 4) Invalidate Redis cache (optional - can be kept for pending)
	return cacheevt.InvalidateEventStatus(ctx, tenantId, eventId)
}

// DeletePendingEvent deletes a pending event
func (s *ApprovalService) DeletePendingEvent(
	ctx context.Context,
	tenantId, orgId, eventId, callerUserId string,
) error {
	// 1) Get pending event
	pending, err := s.eventMgmtRepo.FindByEventId(ctx, tenantId, orgId, eventId)
	if err != nil {
		if err == eventmgmtrepo.ErrNotFound {
			return ErrEventNotFound
		}
		return err
	}

	// 2) Can only delete pending or rejected events
	if pending.Status {
		return ErrEventAlreadyApproved
	}

	// 3) Delete
	if err := s.eventMgmtRepo.Delete(ctx, pending.ID.Hex()); err != nil {
		return err
	}

	// 4) Invalidate Redis cache
	return cacheevt.InvalidateEventStatus(ctx, tenantId, eventId)
}

// normalizeEvent applies normalization to raw data
// For now, returns raw data with metadata
// Future: Apply type-specific normalization templates
func (s *ApprovalService) normalizeEvent(
	_ context.Context,
	eventType string,
	rawBody json.RawMessage,
) (json.RawMessage, error) {
	// Build normalized result
	result := map[string]interface{}{
		"eventType": eventType,
		"normalized": true,
		"rawData":   rawBody,
	}

	// Future enhancement: Apply event type specific templates
	// This is where you would add logic to transform the raw data
	// based on the event type (LPR_Brand, FACE_Brand, etc.)

	return json.Marshal(result)
}
