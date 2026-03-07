package ingestmgmtrepo

import (
	"context"
	"errors"

	"github.com/hotkhwan/gateway-api/internal/repo/stomongo"
	"github.com/hotkhwan/gateway-api/models/ingestmod"
	"github.com/hotkhwan/gateway-api/models/gmod"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var (
	ErrNotFound           = errors.New("event not found")
	ErrDuplicateDeviceKey = errors.New("device already has a pending event")
)

type EventManagementRepo struct{}

func NewEventManagementRepo() *EventManagementRepo {
	return &EventManagementRepo{}
}

// Insert stores a new pending event.
// Returns ErrDuplicateDeviceKey if the unique index fires (device already has a pending event).
func (r *EventManagementRepo) Insert(ctx context.Context, event *ingestmod.EventManagement) error {
	_, err := stomongo.InsertOne(ctx, "event_management", event)
	if err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return ErrDuplicateDeviceKey
		}
		return err
	}
	return nil
}

// FindByEventId finds a pending event by eventId
func (r *EventManagementRepo) FindByEventId(
	ctx context.Context,
	tenantId, orgId, eventId string,
) (*ingestmod.EventManagement, error) {
	var result ingestmod.EventManagement
	err := stomongo.FindOne(ctx, "event_management", bson.M{
		"tenantId": tenantId,
		"orgId":    orgId,
		"eventId":  eventId,
	}, &result)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &result, nil
}

// ListPending lists pending events with pagination
func (r *EventManagementRepo) ListPending(
	ctx context.Context,
	tenantId, orgId string,
	statusName string, // "pending", "approved", "rejected", "all"
	eventType string, // optional filter
	page, perPage int,
	sortField, sortOrder string,
) ([]*ingestmod.EventManagement, *gmod.Pagination, error) {
	filter := bson.M{"tenantId": tenantId, "orgId": orgId}
	if statusName != "all" {
		filter["statusName"] = statusName
	}
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
		sort = append(sort, bson.E{Key: "createdAt", Value: -1})
	}

	var result []*ingestmod.EventManagement
	pagination, err := stomongo.FindWithPagination(
		ctx,
		"event_management",
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

// Update updates a pending event
func (r *EventManagementRepo) Update(
	ctx context.Context,
	id string,
	event *ingestmod.EventManagement,
) error {
	objID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return err
	}

	setFields := bson.M{
		"name":       event.Name,
		"lat":        event.Lat,
		"lng":        event.Lng,
		"eventType":  event.EventType,
		"status":     event.Status,
		"statusName": event.StatusName,
	}
	if event.ApprovedBy != "" {
		setFields["approvedBy"] = event.ApprovedBy
	}
	if !event.ApprovedAt.IsZero() {
		setFields["approvedAt"] = event.ApprovedAt
	}

	res, err := stomongo.UpdateByID(ctx, "event_management", objID, setFields)
	if err != nil {
		return err
	}
	if res.MatchedCount == 0 {
		return ErrNotFound
	}
	return nil
}

// FindByDeviceKey finds a pending event by device key (for device pending lock)
// Returns first pending event with matching deviceKey
func (r *EventManagementRepo) FindByDeviceKey(
	ctx context.Context,
	tenantId, orgId, deviceKey string,
) (*ingestmod.EventManagement, error) {
	var result ingestmod.EventManagement
	err := stomongo.FindOne(ctx, "event_management", bson.M{
		"tenantId":   tenantId,
		"orgId":      orgId,
		"deviceKey":  deviceKey,
		"statusName": "pending",
	}, &result)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &result, nil
}

// Delete removes a pending event
func (r *EventManagementRepo) Delete(ctx context.Context, id string) error {
	objID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return err
	}

	res, err := stomongo.DeleteByID(ctx, "event_management", objID)
	if err != nil {
		return err
	}
	if res.DeletedCount == 0 {
		return ErrNotFound
	}
	return nil
}

// EnsureIndexes creates indexes for performance
func (r *EventManagementRepo) EnsureIndexes(ctx context.Context) error {
	indexes := []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "tenantId", Value: 1}, {Key: "orgId", Value: 1}, {Key: "eventId", Value: 1}},
			Options: options.Index().SetUnique(true),
		},
		{
			Keys: bson.D{{Key: "createdAt", Value: -1}},
		},
		// Partial unique index for device pending lock.
		// Applies ONLY to documents where statusName=="pending" AND deviceKey is non-empty.
		// Excludes null/empty deviceKey so anonymous events (no device ID) never conflict.
		// NOTE: if the old index (without deviceKey filter) exists in MongoDB, drop it first:
		//   db.event_management.dropIndex("tenantId_1_orgId_1_deviceKey_1_statusName_1")
		{
			Keys: bson.D{
				{Key: "tenantId", Value: 1},
				{Key: "orgId", Value: 1},
				{Key: "deviceKey", Value: 1},
				{Key: "statusName", Value: 1},
			},
			Options: options.Index().
				SetUnique(true).
				SetName("idx_device_pending_lock").
				SetPartialFilterExpression(bson.M{
					"statusName": "pending",
					"deviceKey":  bson.M{"$gt": ""},
				}),
		},
	}

	err := stomongo.CreateIndexes(ctx, "event_management", indexes)
	return err
}
