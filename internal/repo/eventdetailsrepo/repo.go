package eventdetailsrepo

import (
	"context"
	"errors"

	"github.com/hotkhwan/gateway-api/internal/repo/stomongo"
	"github.com/hotkhwan/gateway-api/models/eventmod"
	"github.com/hotkhwan/gateway-api/models/gmod"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var (
	ErrNotFound = errors.New("event detail not found")
)

type EventDetailsRepo struct{}

func NewEventDetailsRepo() *EventDetailsRepo {
	return &EventDetailsRepo{}
}

// Insert stores an approved event
func (r *EventDetailsRepo) Insert(ctx context.Context, event *eventmod.EventDetail) error {
	_, err := stomongo.InsertOne(ctx, "event_details", event)
	return err
}

// FindByEventId finds an approved event by eventId
func (r *EventDetailsRepo) FindByEventId(
	ctx context.Context,
	tenantId, orgId, eventId string,
) (*eventmod.EventDetail, error) {
	var result eventmod.EventDetail
	err := stomongo.FindOne(ctx, "event_details", bson.M{
		"tenantId": tenantId,
		"orgId":    orgId,
		"eventId":   eventId,
	}, &result)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &result, nil
}

// ListApproved lists approved events with pagination
func (r *EventDetailsRepo) ListApproved(
	ctx context.Context,
	tenantId, orgId string,
	eventType string, // optional filter
	page, perPage int,
	sortField, sortOrder string,
) ([]*eventmod.EventDetail, *gmod.Pagination, error) {
	filter := bson.M{"tenantId": tenantId, "orgId": orgId}
	if eventType != "" {
		filter["eventType"] = eventType
	}

	sort := bson.D{}
	if sortField != "" {
		order := 1
		if sortOrder == "desc" {
			order = -1
		}
		sort = append(sort, bson.E{Key: sortField, Value: order})
	} else {
		sort = append(sort, bson.E{Key: "approvedAt", Value: -1})
	}

	var result []*eventmod.EventDetail
	pagination, err := stomongo.FindWithPagination(
		ctx,
		"event_details",
		filter,
		sort,
		page, perPage,
		&result,
	)
	if err != nil {
		return nil, nil, err
	}

	// Update pagination with sort info
	pagination.SortField = sortField
	pagination.SortOrder = sortOrder

	return result, &pagination, nil
}

// EnsureIndexes creates indexes for performance
func (r *EventDetailsRepo) EnsureIndexes(ctx context.Context) error {
	indexes := []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "tenantId", Value: 1}, {Key: "orgId", Value: 1}, {Key: "eventId", Value: 1}},
			Options: options.Index().SetUnique(true),
		},
		{
			Keys: bson.D{{Key: "tenantId", Value: 1}, {Key: "orgId", Value: 1}, {Key: "eventType", Value: 1}, {Key: "approvedAt", Value: -1}},
		},
		{
			Keys: bson.D{{Key: "approvedAt", Value: -1}},
		},
	}

	err := stomongo.CreateIndexes(ctx, "event_details", indexes)
	return err
}
