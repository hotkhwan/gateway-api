package eventmgmtrepo

import (
	"context"
	"fmt"
	"time"

	"github.com/hotkhwan/gateway-api/internal/repo/stomongo"
	"github.com/hotkhwan/gateway-api/models/ingestmod"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// DashboardStatsResponse สรุปสถิติจาก event_management
type DashboardStatsResponse struct {
	PendingCount  int            `json:"pendingCount"`
	RejectedCount int            `json:"rejectedCount"`
	ByEventType   map[string]int `json:"byEventType"`
	ByPriority    map[string]int `json:"byPriority"`
}

// GetStatsForDashboard ดึงสถิติจาก event_management สำหรับ dashboard
func (r *EventManagementRepo) GetStatsForDashboard(
	ctx context.Context,
	tenantId, orgId string,
	start, end time.Time,
	status, eventType string,
) (*DashboardStatsResponse, error) {
	filter := bson.M{
		"tenantId": tenantId,
		"orgId":    orgId,
		"createdAt": bson.M{
			"$gte": start,
			"$lt":  end,
		},
	}

	// Filter by status
	if status != "" && status != "all" {
		filter["statusName"] = status
	}

	// Filter by event type
	if eventType != "" {
		filter["eventType"] = eventType
	}

	// Aggregate pipeline for stats
	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: filter}},
		{
			{Key: "$group", Value: bson.M{
				"_id": bson.M{
					"statusName": "$statusName",
					"eventType":  "$eventType",
					"priority":   "$priority",
				},
				"count": bson.M{"$sum": 1},
			}},
		},
	}

	type aggResult struct {
		ID struct {
			StatusName string `bson:"statusName"`
			EventType  string `bson:"eventType"`
			Priority   string `bson:"priority"`
		} `bson:"_id"`
		Count int `bson:"count"`
	}

	var aggResults []aggResult
	if err := stomongo.Aggregate(ctx, "event_management", pipeline, &aggResults); err != nil {
		return nil, err
	}

	response := &DashboardStatsResponse{
		ByEventType: make(map[string]int),
		ByPriority:  make(map[string]int),
	}

	for _, result := range aggResults {
		// Count by status
		switch result.ID.StatusName {
		case "pending":
			response.PendingCount += result.Count
		case "rejected":
			response.RejectedCount += result.Count
		}

		// Count by event type
		if result.ID.EventType != "" {
			response.ByEventType[result.ID.EventType] += result.Count
		}

		// Count by priority
		if result.ID.Priority != "" {
			response.ByPriority[result.ID.Priority] += result.Count
		}
	}

	return response, nil
}

// GetGeoCellBuckets ดึง stats ตาม geo cell (geohash)
func (r *EventManagementRepo) GetGeoCellBuckets(
	ctx context.Context,
	tenantId, orgId string,
	start, end time.Time,
	status, eventType string,
	precision int,
) ([]ingestmod.GeoCellBucket, error) {
	filter := bson.M{
		"tenantId": tenantId,
		"orgId":    orgId,
		"createdAt": bson.M{
			"$gte": start,
			"$lt":  end,
		},
	}

	// Filter by status
	if status != "" && status != "all" {
		filter["statusName"] = status
	}

	// Filter by event type
	if eventType != "" {
		filter["eventType"] = eventType
	}

	// Aggregate pipeline for geo cell buckets
	// สำหรับ geohash precision 5, เราจะ group ตาม lat/lng ที่ปัดเศษ
	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: filter}},
		{
			{Key: "$group", Value: bson.M{
				"_id": bson.M{
					"lat": bson.M{"$round": bson.A{"$lat", precision}},
					"lng": bson.M{"$round": bson.A{"$lng", precision}},
				},
				"count":    bson.M{"$sum": 1},
				"firstLat": bson.M{"$first": "$lat"},
				"firstLng": bson.M{"$first": "$lng"},
			}},
		},
	}

	type geoCellAgg struct {
		ID struct {
			Lat float64 `bson:"lat"`
			Lng float64 `bson:"lng"`
		} `bson:"_id"`
		Count    int     `bson:"count"`
		FirstLat float64 `bson:"firstLat"`
		FirstLng float64 `bson:"firstLng"`
	}

	var aggResults []geoCellAgg
	if err := stomongo.Aggregate(ctx, "event_management", pipeline, &aggResults); err != nil {
		return nil, err
	}

	var buckets []ingestmod.GeoCellBucket
	for _, result := range aggResults {
		// สร้าง geohash cell ID จาก lat/lng (simplified version)
		cellID := fmt.Sprintf("%.5f,%.5f", result.ID.Lat, result.ID.Lng)

		buckets = append(buckets, ingestmod.GeoCellBucket{
			Cell:  cellID,
			Count: result.Count,
			Lat:   result.FirstLat,
			Lng:   result.FirstLng,
		})
	}

	return buckets, nil
}

// GetAdminArea1Buckets ดึง stats ตาม admin area level 1
func (r *EventManagementRepo) GetAdminArea1Buckets(
	ctx context.Context,
	tenantId, orgId string,
	start, end time.Time,
	status, eventType string,
) (map[string]int, error) {
	filter := bson.M{
		"tenantId": tenantId,
		"orgId":    orgId,
		"createdAt": bson.M{
			"$gte": start,
			"$lt":  end,
		},
	}

	// Filter by status
	if status != "" && status != "all" {
		filter["statusName"] = status
	}

	// Filter by event type
	if eventType != "" {
		filter["eventType"] = eventType
	}

	// สำหรับ admin area 1 เบื้องต้น เราจะคืน empty map
	// (ต้องมี field adminAreaCode ใน DB หรือทำ point-in-polygon ตอน query)
	buckets := make(map[string]int)

	// TODO: ในอนาคต เพิ่ม logic สำหรับ resolve admin area จาก lat/lng
	// หรือใช้ field adminAreaCode ถ้ามีใน DB

	// For now, return empty map
	return buckets, nil
}

// GetRecentEvents ดึง events ล่าสุด
func (r *EventManagementRepo) GetRecentEvents(
	ctx context.Context,
	tenantId, orgId string,
	start, end time.Time,
	status, eventType string,
	limit int,
) ([]*EventWithLocation, error) {
	filter := bson.M{
		"tenantId": tenantId,
		"orgId":    orgId,
		"createdAt": bson.M{
			"$gte": start,
			"$lt":  end,
		},
	}

	// Filter by status
	if status != "" && status != "all" {
		filter["statusName"] = status
	}

	// Filter by event type
	if eventType != "" {
		filter["eventType"] = eventType
	}

	findOpts := options.Find().
		SetSort(bson.D{{Key: "createdAt", Value: -1}}).
		SetLimit(int64(limit))

	var events []*EventWithLocation
	if err := stomongo.Find(ctx, "event_management", filter, findOpts, &events); err != nil {
		return nil, err
	}

	return events, nil
}

// EventWithLocation event พร้อม location สำหรับ dashboard
type EventWithLocation struct {
	ID        string    `bson:"_id" json:"id"`
	EventId   string    `bson:"eventId" json:"eventId"`
	Name      string    `bson:"name" json:"name"`
	EventType string    `bson:"eventType" json:"eventType"`
	Status    string    `bson:"statusName" json:"statusName"`
	Priority  string    `bson:"priority,omitempty" json:"priority,omitempty"`
	CreatedAt time.Time `bson:"createdAt" json:"createdAt"`
	Lat       float64   `bson:"lat" json:"lat"`
	Lng       float64   `bson:"lng" json:"lng"`
	SourceIp  string    `bson:"sourceIp" json:"sourceIp,omitempty"`
}
