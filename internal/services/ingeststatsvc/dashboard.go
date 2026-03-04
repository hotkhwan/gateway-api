package ingeststatsvc

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/hotkhwan/gateway-api/internal/repo/eventdetailsrepo"
	"github.com/hotkhwan/gateway-api/internal/repo/eventmgmtrepo"
	"github.com/hotkhwan/gateway-api/models/ingestmod"
	"github.com/rs/zerolog"
)

var (
	ErrInvalidDateRange = errors.New("end date must be after start date")
	ErrDateRangeTooLong = errors.New("date range exceeds maximum allowed period (31 days)")
)

type DashboardStatsService struct {
	eventMgmtRepo   *eventmgmtrepo.EventManagementRepo
	eventDetailsRepo *eventdetailsrepo.EventDetailsRepo
	logger          zerolog.Logger
}

func NewDashboardStatsService(
	eventMgmtRepo *eventmgmtrepo.EventManagementRepo,
	eventDetailsRepo *eventdetailsrepo.EventDetailsRepo,
	logger zerolog.Logger,
) *DashboardStatsService {
	return &DashboardStatsService{
		eventMgmtRepo:   eventMgmtRepo,
		eventDetailsRepo: eventDetailsRepo,
		logger:          logger,
	}
}

// GetIngestDashboardStats ดึงข้อมูล ingest dashboard stats
func (s *DashboardStatsService) GetIngestDashboardStats(
	ctx context.Context,
	input *ingestmod.GetIngestDashboardInput,
) (*ingestmod.IngestDashboardStats, error) {
	// 1. Parse dateTime range (RFC3339)
	start, end, err := parseDateTimeRange(input.StartDate, input.EndDate)
	if err != nil {
		return nil, err
	}

	s.logger.Info().
		Str("tenantId", input.TenantId).
		Str("orgId", input.OrgId).
		Time("start", start).
		Time("end", end).
		Str("status", input.Status).
		Str("eventType", input.EventType).
		Msg("📊 Building ingest dashboard stats")

	// 2. Build core stats (counts + groupings)
	stats, err := s.buildStats(ctx, input.TenantId, input.OrgId, start, end, input.Status, input.EventType)
	if err != nil {
		return nil, fmt.Errorf("failed to build stats: %w", err)
	}

	// 3. Map meta (set defaults)
	stats.Geo = ingestmod.GeoMeta{
		CountryCode: "TH",
		AdminLevel:  1,
		IdScheme:    "ISO_3166_2",
	}
	stats.GeoCell = ingestmod.GeoCellMeta{
		Scheme:    "geohash",
		Precision: 5,
	}

	// Set DateTime range ที่ใช้งาน (RFC3339)
	stats.StartDate = start.UTC().Format(time.RFC3339)
	stats.EndDate = end.UTC().Format(time.RFC3339)

	// 4. Map aggregates
	byCell, err := s.buildGeoCellBuckets(ctx, input.TenantId, input.OrgId, start, end, input.Status, input.EventType, stats.GeoCell.Precision)
	if err != nil {
		s.logger.Warn().Err(err).Msg("⚠️ failed to build geo cell buckets")
	} else {
		stats.ByGeoCell = byCell
	}

	byAdmin, err := s.buildAdminArea1Buckets(ctx, input.TenantId, input.OrgId, start, end, input.Status, input.EventType)
	if err != nil {
		// อันนี้ล้มได้ถ้ายังไม่มี provinceCode ใน DB — ให้ warn และคืน empty
		s.logger.Warn().Err(err).Msg("⚠️ failed to build admin area buckets")
	} else {
		stats.ByAdminArea1 = byAdmin
	}

	// 5. Recent events (ล่าสุด 10 รายการ)
	recent, err := s.getRecentEvents(ctx, input.TenantId, input.OrgId, start, end, input.Status, input.EventType, 10)
	if err != nil {
		s.logger.Warn().Err(err).Msg("⚠️ failed to get recent events")
	} else {
		stats.RecentEvents = recent
	}

	return stats, nil
}

// buildStats สร้างสถิติหลักจาก event_management (pending) และ event_details (approved)
func (s *DashboardStatsService) buildStats(
	ctx context.Context,
	tenantId, orgId string,
	start, end time.Time,
	status, eventType string,
) (*ingestmod.IngestDashboardStats, error) {
	stats := &ingestmod.IngestDashboardStats{
		ByEventType: make(map[string]int),
		ByPriority:  make(map[string]int),
		ByAdminArea1: make(map[string]int),
		ByGeoCell:    []ingestmod.GeoCellBucket{},
	}

	// Query จาก event_management (pending/rejected) และ event_details (approved)
	// สำหรับเบื้องต้น เราจะ query จาก event_management ก่อน
	pendingStats, err := s.eventMgmtRepo.GetStatsForDashboard(ctx, tenantId, orgId, start, end, status, eventType)
	if err != nil {
		return nil, fmt.Errorf("failed to get pending stats: %w", err)
	}

	stats.PendingEvents = pendingStats.PendingCount
	stats.RejectedEvents = pendingStats.RejectedCount

	// รวม byEventType และ byPriority จาก pending events
	for k, v := range pendingStats.ByEventType {
		stats.ByEventType[k] += v
	}
	for k, v := range pendingStats.ByPriority {
		stats.ByPriority[k] += v
	}

	// Query จาก event_details (approved)
	approvedStats, err := s.eventDetailsRepo.GetStatsForDashboard(ctx, tenantId, orgId, start, end, status, eventType)
	if err != nil {
		return nil, fmt.Errorf("failed to get approved stats: %w", err)
	}

	stats.ApprovedEvents = approvedStats.ApprovedCount

	// รวม byEventType และ byPriority จาก approved events
	for k, v := range approvedStats.ByEventType {
		stats.ByEventType[k] += v
	}
	for k, v := range approvedStats.ByPriority {
		stats.ByPriority[k] += v
	}

	stats.TotalEvents = stats.PendingEvents + stats.ApprovedEvents + stats.RejectedEvents

	return stats, nil
}

// buildGeoCellBuckets สร้าง bucket ตาม geo cell (geohash precision 5)
func (s *DashboardStatsService) buildGeoCellBuckets(
	ctx context.Context,
	tenantId, orgId string,
	start, end time.Time,
	status, eventType string,
	precision int,
) ([]ingestmod.GeoCellBucket, error) {
	// สำหรับเบื้องต้น เราจะ query จาก event_management ก่อน
	buckets, err := s.eventMgmtRepo.GetGeoCellBuckets(ctx, tenantId, orgId, start, end, status, eventType, precision)
	if err != nil {
		return nil, err
	}
	return buckets, nil
}

// buildAdminArea1Buckets สร้าง bucket ตาม admin area level 1
func (s *DashboardStatsService) buildAdminArea1Buckets(
	ctx context.Context,
	tenantId, orgId string,
	start, end time.Time,
	status, eventType string,
) (map[string]int, error) {
	// สำหรับเบื้องต้น เราจะ query จาก event_management ก่อน
	buckets, err := s.eventMgmtRepo.GetAdminArea1Buckets(ctx, tenantId, orgId, start, end, status, eventType)
	if err != nil {
		return nil, err
	}
	return buckets, nil
}

// getRecentEvents ดึง events ล่าสุด
func (s *DashboardStatsService) getRecentEvents(
	ctx context.Context,
	tenantId, orgId string,
	start, end time.Time,
	status, eventType string,
	limit int,
) ([]ingestmod.RecentEventSummary, error) {
	// สำหรับเบื้องต้น เราจะ query จาก event_management ก่อน
	events, err := s.eventMgmtRepo.GetRecentEvents(ctx, tenantId, orgId, start, end, status, eventType, limit)
	if err != nil {
		return nil, err
	}

	// Convert to RecentEventSummary
	summaries := make([]ingestmod.RecentEventSummary, 0, len(events))
	for _, e := range events {
		summaries = append(summaries, ingestmod.RecentEventSummary{
			ID:        e.ID,
			EventId:   e.EventId,
			Name:      e.Name,
			EventType: e.EventType,
			Status:    e.Status,
			Priority:  e.Priority,
			CreatedAt: e.CreatedAt,
			Lat:       e.Lat,
			Lng:       e.Lng,
			SourceIp:  e.SourceIp,
		})
	}

	return summaries, nil
}

// parseDateTimeRange parse ISO 8601 datetime range
// format: 2026-01-20T00:00:00Z,2026-01-20T16:59:59Z
func parseDateTimeRange(startDate, endDate string) (time.Time, time.Time, error) {
	loc, err := time.LoadLocation("Asia/Bangkok")
	if err != nil || loc == nil {
		loc = time.UTC
	}

	now := time.Now().In(loc)

	var start time.Time
	var end time.Time

	// Parse start date
	if startDate != "" {
		t, err := time.Parse(time.RFC3339, startDate)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("invalid startDate format (use RFC3339): %w", err)
		}
		start = t
	} else {
		// Default: วันนี้ 00:00
		start = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	}

	// Parse end date
	if endDate != "" {
		t, err := time.Parse(time.RFC3339, endDate)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("invalid endDate format (use RFC3339): %w", err)
		}
		end = t
	} else {
		// Default: now
		end = now
	}

	// Guard: end >= start
	if end.Before(start) {
		return time.Time{}, time.Time{}, ErrInvalidDateRange
	}

	// Optional: Guard against too long range (31 days)
	maxDays := 31 * 24 * time.Hour
	if end.Sub(start) > maxDays {
		return time.Time{}, time.Time{}, ErrDateRangeTooLong
	}

	return start, end, nil
}
