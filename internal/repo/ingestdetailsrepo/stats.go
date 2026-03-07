package ingestdetailsrepo

import (
	"context"
	"time"

	"github.com/hotkhwan/gateway-api/internal/repo/stomongo"
	"github.com/hotkhwan/gateway-api/models/ingestmod"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// DashboardStatsResponse สรุปสถิติจาก event_details
type DashboardStatsResponse struct {
	ApprovedCount int            `json:"approvedCount"`
	ByEventType   map[string]int `json:"byEventType"`
	ByPriority    map[string]int `json:"byPriority"`
}

// GetStatsForDashboard ดึงสถิติจาก event_details สำหรับ dashboard
func (r *EventDetailsRepo) GetStatsForDashboard(
	ctx context.Context,
	tenantId, orgId string,
	start, end time.Time,
	status, eventType string,
) (*DashboardStatsResponse, error) {
	filter := bson.M{
		"tenantId":     tenantId,
		"source.orgId": orgId,
		"approvedAt": bson.M{
			"$gte": start,
			"$lt":  end,
		},
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
					"eventType": "$eventType",
					"priority":  "$priority",
				},
				"count": bson.M{"$sum": 1},
			}},
		},
	}

	type aggResult struct {
		ID struct {
			EventType string `bson:"eventType"`
			Priority  string `bson:"priority"`
		} `bson:"_id"`
		Count int `bson:"count"`
	}

	var aggResults []aggResult
	if err := stomongo.Aggregate(ctx, "event_details", pipeline, &aggResults); err != nil {
		return nil, err
	}

	response := &DashboardStatsResponse{
		ApprovedCount: 0,
		ByEventType:   make(map[string]int),
		ByPriority:    make(map[string]int),
	}

	for _, result := range aggResults {
		// Total approved count
		response.ApprovedCount += result.Count

		// Count by event type
		if result.ID.EventType != "" {
			response.ByEventType[result.ID.EventType] += result.Count
		}

		// Count by priority (event_details อาจไม่มี priority, skip)
		if result.ID.Priority != "" {
			response.ByPriority[result.ID.Priority] += result.Count
		}
	}

	return response, nil
}

// GetRecentEvents ดึง events ล่าสุดจาก event_details
func (r *EventDetailsRepo) GetRecentEvents(
	ctx context.Context,
	tenantId, orgId string,
	start, end time.Time,
	eventType string,
	limit int,
) ([]ingestmod.RecentEventSummary, error) {
	filter := bson.M{
		"tenantId":     tenantId,
		"source.orgId": orgId,
		"createdAt": bson.M{
			"$gte": start,
			"$lt":  end,
		},
	}

	if eventType != "" {
		filter["eventType"] = eventType
	}

	findOpts := options.Find().
		SetSort(bson.D{{Key: "createdAt", Value: -1}}).
		SetLimit(int64(limit))

	type eventDoc struct {
		ID         string                `bson:"_id" json:"id"`
		EventId    string                `bson:"eventId" json:"eventId"`
		EventType  string                `bson:"eventType" json:"eventType"`
		Location   ingestmod.LocationInfo `bson:"location" json:"location"`
		Source     ingestmod.SourceInfo   `bson:"source" json:"source"`
		OccurredAt time.Time             `bson:"occurredAt" json:"occurredAt"`
		CreatedAt  time.Time             `bson:"createdAt" json:"createdAt"`
	}

	var docs []eventDoc
	if err := stomongo.Find(ctx, "event_details", filter, findOpts, &docs); err != nil {
		return nil, err
	}

	summaries := make([]ingestmod.RecentEventSummary, 0, len(docs))
	for _, d := range docs {
		summaries = append(summaries, ingestmod.RecentEventSummary{
			ID:        d.ID,
			EventId:   d.EventId,
			EventType: d.EventType,
			Status:    "approved",
			CreatedAt: d.CreatedAt,
			Lat:       d.Location.Lat,
			Lng:       d.Location.Lng,
		})
	}

	return summaries, nil
}
