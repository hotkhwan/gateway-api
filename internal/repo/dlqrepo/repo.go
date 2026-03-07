// internal/repo/dlqrepo/repo.go
package dlqrepo

import (
	"context"
	"errors"
	"time"

	"github.com/hotkhwan/gateway-api/internal/repo/stomongo"
	"github.com/hotkhwan/gateway-api/models/gmod"
	"github.com/hotkhwan/gateway-api/models/ingestmod"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const colDLQ = "dlq_events"

var ErrNotFound = errors.New("dlq message not found")

type DLQRepo struct{}

func NewDLQRepo() *DLQRepo { return &DLQRepo{} }

// Insert stores a new DLQ message.
func (r *DLQRepo) Insert(ctx context.Context, msg *ingestmod.DLQMessage) error {
	_, err := stomongo.InsertOne(ctx, colDLQ, msg)
	return err
}

// FindById returns a DLQ message by tenantId + messageId.
func (r *DLQRepo) FindById(ctx context.Context, tenantId, messageId string) (*ingestmod.DLQMessage, error) {
	var result ingestmod.DLQMessage
	err := stomongo.FindOne(ctx, colDLQ, bson.M{
		"tenantId":  tenantId,
		"messageId": messageId,
	}, &result)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &result, nil
}

// List returns paginated DLQ messages with optional filters.
func (r *DLQRepo) List(
	ctx context.Context,
	tenantId, orgId, stage, status string,
	page, perPage int,
) ([]*ingestmod.DLQMessage, *gmod.Pagination, error) {
	filter := bson.M{"tenantId": tenantId}
	if orgId != "" {
		filter["orgId"] = orgId
	}
	if stage != "" {
		filter["stage"] = stage
	}
	if status != "" {
		filter["status"] = status
	}

	sort := bson.D{{Key: "createdAt", Value: -1}}

	var result []*ingestmod.DLQMessage
	pagination, err := stomongo.FindWithPagination(ctx, colDLQ, filter, sort, page, perPage, &result)
	if err != nil {
		return nil, nil, err
	}
	return result, &pagination, nil
}

// UpdateStatus updates status, retryCount, and lastErrorAt for a DLQ message.
func (r *DLQRepo) UpdateStatus(ctx context.Context, tenantId, messageId, status string, retryCount int) error {
	now := time.Now().UTC()
	_, err := stomongo.UpsertByFilter(
		ctx, colDLQ,
		bson.M{"tenantId": tenantId, "messageId": messageId},
		bson.M{
			"status":      status,
			"retryCount":  retryCount,
			"lastErrorAt": now,
			"updatedAt":   now,
		},
		bson.M{},
	)
	return err
}

// Stats returns counts by stage and status for a tenant/org.
func (r *DLQRepo) Stats(ctx context.Context, tenantId, orgId string) (map[string]any, error) {
	match := bson.M{"tenantId": tenantId}
	if orgId != "" {
		match["orgId"] = orgId
	}

	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: match}},
		{{Key: "$facet", Value: bson.M{
			"byStage": bson.A{
				bson.M{"$group": bson.M{"_id": "$stage", "count": bson.M{"$sum": 1}}},
			},
			"byStatus": bson.A{
				bson.M{"$group": bson.M{"_id": "$status", "count": bson.M{"$sum": 1}}},
			},
			"total": bson.A{
				bson.M{"$count": "count"},
			},
		}}},
	}

	var results []bson.M
	if err := stomongo.Aggregate(ctx, colDLQ, pipeline, &results); err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return map[string]any{"total": 0, "byStage": map[string]int{}, "byStatus": map[string]int{}}, nil
	}

	raw := results[0]

	total := 0
	if totals, ok := raw["total"].(bson.A); ok && len(totals) > 0 {
		if t, ok := totals[0].(bson.M); ok {
			if c, ok := t["count"].(int32); ok {
				total = int(c)
			}
		}
	}

	byStage := collapseGroupResult(raw["byStage"])
	byStatus := collapseGroupResult(raw["byStatus"])

	return map[string]any{
		"total":    total,
		"byStage":  byStage,
		"byStatus": byStatus,
	}, nil
}

func collapseGroupResult(v any) map[string]int {
	out := map[string]int{}
	arr, ok := v.(bson.A)
	if !ok {
		return out
	}
	for _, item := range arr {
		m, ok := item.(bson.M)
		if !ok {
			continue
		}
		key, _ := m["_id"].(string)
		count, _ := m["count"].(int32)
		if key != "" {
			out[key] = int(count)
		}
	}
	return out
}

// EnsureIndexes creates indexes for the dlq_events collection.
func (r *DLQRepo) EnsureIndexes(ctx context.Context) error {
	indexes := []mongo.IndexModel{
		// Primary lookup by messageId
		{
			Keys:    bson.D{{Key: "messageId", Value: 1}},
			Options: options.Index().SetUnique(true),
		},
		// List by tenant/org
		{
			Keys: bson.D{{Key: "tenantId", Value: 1}, {Key: "orgId", Value: 1}, {Key: "createdAt", Value: -1}},
		},
		// Filter by status (pending/retrying need attention)
		{
			Keys: bson.D{{Key: "tenantId", Value: 1}, {Key: "status", Value: 1}},
		},
		// Filter by stage + status
		{
			Keys: bson.D{{Key: "tenantId", Value: 1}, {Key: "stage", Value: 1}, {Key: "status", Value: 1}},
		},
		// TTL: auto-delete resolved/abandoned after 30 days
		{
			Keys:    bson.D{{Key: "updatedAt", Value: 1}},
			Options: options.Index().SetExpireAfterSeconds(30 * 24 * 60 * 60),
		},
	}
	return stomongo.CreateIndexes(ctx, colDLQ, indexes)
}
