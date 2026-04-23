package ingestdetailsrepo

import (
	"context"
	"errors"
	"regexp"
	"time"

	"github.com/hotkhwan/gateway-api/internal/repo/stomongo"
	"github.com/hotkhwan/gateway-api/models/gmod"
	"github.com/hotkhwan/gateway-api/models/ingestmod"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// ListApprovedFilter carries optional filter values for ListApproved.
// Zero values are treated as "no filter" for each dimension.
type ListApprovedFilter struct {
	EventType    string
	SourceFamily string
	Search       string    // matches eventType / source.deviceName / source.deviceId (case-insensitive substring)
	StartDate    time.Time // inclusive lower bound on occurredAt (zero = no lower bound)
	EndDate      time.Time // inclusive upper bound on occurredAt (zero = no upper bound)
}

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

// FindBinaryRefsByEventId returns the binaryRefs array stored on the normalized
// event_details document for a single eventId. Filters by eventId only because
// the event_details collection's canonical tenantId is set at ingest time
// (e.g. "klynx") and may not match the tenantId carried in downstream consumer
// messages where tenantId was replaced with orgId by a bridging service.
// eventId is a UUID so the single-field filter is safe against cross-tenant
// collisions.
func (r *EventDetailsRepo) FindBinaryRefsByEventId(ctx context.Context, eventId string) ([]ingestmod.BinaryRef, error) {
	if eventId == "" {
		return nil, nil
	}
	var doc struct {
		BinaryRefs []ingestmod.BinaryRef `bson:"binaryRefs"`
	}
	err := stomongo.FindOne(ctx, "event_details", bson.M{"eventId": eventId}, &doc)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return doc.BinaryRefs, nil
}

// FindByEventId finds an approved event by eventId
func (r *EventDetailsRepo) FindByEventId(
	ctx context.Context,
	tenantId, orgId, eventId string,
) (*ingestmod.EventDetail, error) {
	var result ingestmod.EventDetail
	err := stomongo.FindOne(ctx, "event_details", bson.M{
		"tenantId":          tenantId,
		"source.workspaceId": orgId,
		"eventId":           eventId,
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

// ListApproved lists approved events with pagination.
func (r *EventDetailsRepo) ListApproved(
	ctx context.Context,
	tenantId, orgId string,
	f ListApprovedFilter,
	page, perPage int,
	sortField, sortOrder string,
) ([]*ingestmod.EventDetail, *gmod.Pagination, error) {
	filter := bson.M{"tenantId": tenantId, "source.workspaceId": orgId}
	if f.EventType != "" {
		filter["eventType"] = f.EventType
	}
	if f.SourceFamily != "" {
		filter["source.sourceFamily"] = f.SourceFamily
	}
	if f.Search != "" {
		rx := bson.M{"$regex": regexp.QuoteMeta(f.Search), "$options": "i"}
		filter["$or"] = bson.A{
			bson.M{"eventType": rx},
			bson.M{"source.deviceName": rx},
			bson.M{"source.deviceId": rx},
		}
	}
	if !f.StartDate.IsZero() || !f.EndDate.IsZero() {
		occ := bson.M{}
		if !f.StartDate.IsZero() {
			occ["$gte"] = f.StartDate
		}
		if !f.EndDate.IsZero() {
			occ["$lte"] = f.EndDate
		}
		filter["occurredAt"] = occ
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

// FindNormalizedByEventID finds a NormalizedEvent by eventId scoped to workspaceId (source.orgId).
// Used by the gRPC EventService to serve klynx GetEvent requests.
func (r *EventDetailsRepo) FindNormalizedByEventID(ctx context.Context, workspaceId, eventId string) (*ingestmod.NormalizedEvent, error) {
	var result ingestmod.NormalizedEvent
	err := stomongo.FindOne(ctx, "event_details", bson.M{
		"source.workspaceId": workspaceId,
		"eventId":            eventId,
	}, &result)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &result, nil
}

// FindNormalizedByEventIDs finds multiple NormalizedEvents by eventIds scoped to workspaceId.
// Missing IDs are silently omitted — caller checks NotFound list.
func (r *EventDetailsRepo) FindNormalizedByEventIDs(ctx context.Context, workspaceId string, eventIds []string) ([]*ingestmod.NormalizedEvent, error) {
	var results []*ingestmod.NormalizedEvent
	err := stomongo.Find(ctx, "event_details", bson.M{
		"source.workspaceId": workspaceId,
		"eventId":            bson.M{"$in": eventIds},
	}, nil, &results)
	if err != nil {
		return nil, err
	}
	return results, nil
}

// EnsureIndexes creates indexes for performance
func (r *EventDetailsRepo) EnsureIndexes(ctx context.Context) error {
	// Migrate legacy source.orgId → source.workspaceId for documents written before the rename.
	_, _ = stomongo.UpdateManyPipeline(ctx, "event_details",
		bson.M{"source.orgId": bson.M{"$exists": true}},
		mongo.Pipeline{
			bson.D{{Key: "$set", Value: bson.M{"source.workspaceId": "$source.orgId"}}},
			bson.D{{Key: "$unset", Value: "source.orgId"}},
		},
	)

	indexes := []mongo.IndexModel{
		// Unique lookup by eventId
		{
			Keys:    bson.D{{Key: "eventId", Value: 1}},
			Options: options.Index().SetUnique(true),
		},
		// Standard list by tenant/workspace
		{
			Keys: bson.D{{Key: "tenantId", Value: 1}, {Key: "source.workspaceId", Value: 1}, {Key: "occurredAt", Value: -1}},
		},
		// Event type filter
		{
			Keys: bson.D{{Key: "tenantId", Value: 1}, {Key: "source.workspaceId", Value: 1}, {Key: "eventType", Value: 1}, {Key: "occurredAt", Value: -1}},
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
