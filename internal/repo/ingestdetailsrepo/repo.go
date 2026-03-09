package ingestdetailsrepo

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

var (
	ErrNotFound = errors.New("event detail not found")
)

type EventDetailsRepo struct{}

func NewEventDetailsRepo() *EventDetailsRepo {
	return &EventDetailsRepo{}
}

// Insert stores an approved event and sets the generated _id back on the struct.
func (r *EventDetailsRepo) Insert(ctx context.Context, event *ingestmod.EventDetail) error {
	id, err := stomongo.InsertOne(ctx, "event_details", event)
	if err != nil {
		return err
	}
	event.ID = id
	return nil
}

// Upsert stores a NormalizedEvent idempotently (insert or replace by eventId).
// Used by the Normalizer consumer to write event_details from raw.events.
func (r *EventDetailsRepo) Upsert(ctx context.Context, event *ingestmod.NormalizedEvent) error {
	// Convert struct to bson.M via marshal/unmarshal so bson tags are respected
	raw, err := bson.Marshal(event)
	if err != nil {
		return err
	}
	var doc bson.M
	if err := bson.Unmarshal(raw, &doc); err != nil {
		return err
	}
	delete(doc, "_id")

	filter := bson.M{"eventId": event.EventId}
	onInsert := bson.M{"createdAt": time.Now().UTC()}
	_, err = stomongo.UpsertByFilter(ctx, "event_details", filter, doc, onInsert)
	return err
}

// FindByEventId finds an approved event by eventId
func (r *EventDetailsRepo) FindByEventId(
	ctx context.Context,
	tenantId, orgId, eventId string,
) (*ingestmod.EventDetail, error) {
	var result ingestmod.EventDetail
	err := stomongo.FindOne(ctx, "event_details", bson.M{
		"tenantId":     tenantId,
		"source.orgId": orgId,
		"eventId":      eventId,
	}, &result)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	result.ProjectFlatFields()
	return &result, nil
}

// ListApproved lists approved events with pagination
func (r *EventDetailsRepo) ListApproved(
	ctx context.Context,
	tenantId, orgId string,
	eventType string, // optional filter
	page, perPage int,
	sortField, sortOrder string,
) ([]*ingestmod.EventDetail, *gmod.Pagination, error) {
	filter := bson.M{"tenantId": tenantId, "source.orgId": orgId}
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

	var result []*ingestmod.EventDetail
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

	// Project flat convenience fields for FE
	for _, item := range result {
		item.ProjectFlatFields()
	}

	// Update pagination with sort info
	pagination.SortField = sortField
	pagination.SortOrder = sortOrder

	return result, &pagination, nil
}

// EnsureIndexes creates indexes for performance
func (r *EventDetailsRepo) EnsureIndexes(ctx context.Context) error {
	indexes := []mongo.IndexModel{
		// Unique lookup by eventId
		{
			Keys:    bson.D{{Key: "eventId", Value: 1}},
			Options: options.Index().SetUnique(true),
		},
		// Standard list by tenant/org
		{
			Keys: bson.D{{Key: "tenantId", Value: 1}, {Key: "orgId", Value: 1}, {Key: "occurredAt", Value: -1}},
		},
		// Event type filter
		{
			Keys: bson.D{{Key: "tenantId", Value: 1}, {Key: "orgId", Value: 1}, {Key: "eventType", Value: 1}, {Key: "occurredAt", Value: -1}},
		},
		// Geo cell lookup (heat maps)
		{
			Keys: bson.D{{Key: "geoCell.cell", Value: 1}, {Key: "tenantId", Value: 1}},
		},
		// Admin area filter (wildcard covers all admin codes)
		{
			Keys: bson.D{{Key: "byAdminArea.$**", Value: 1}},
		},
		// Country + admin level filter (dashboard by province)
		{
			Keys: bson.D{{Key: "tenantId", Value: 1}, {Key: "geo.countryCode", Value: 1}, {Key: "geo.adminCode", Value: 1}, {Key: "occurredAt", Value: -1}},
		},
	}

	err := stomongo.CreateIndexes(ctx, "event_details", indexes)
	return err
}
