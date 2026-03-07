// internal/repo/ingestrepo/template.go
package ingestrepo

import (
	"context"
	"errors"
	"regexp"

	"github.com/hotkhwan/gateway-api/internal/repo/stomongo"
	"github.com/hotkhwan/gateway-api/models/gmod"
	"github.com/hotkhwan/gateway-api/models/ingestmod"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const colTemplate = "mapping_templates"

var ErrTemplateNotFound = errors.New("template not found")

type MappingTemplateRepo struct{}

func NewMappingTemplateRepo() *MappingTemplateRepo {
	return &MappingTemplateRepo{}
}

// Insert stores a new mapping template.
func (r *MappingTemplateRepo) Insert(ctx context.Context, t *ingestmod.MappingTemplate) error {
	_, err := stomongo.InsertOne(ctx, colTemplate, t)
	return err
}

// FindById returns a template by orgId + templateId.
func (r *MappingTemplateRepo) FindById(ctx context.Context, orgId, templateId string) (*ingestmod.MappingTemplate, error) {
	var result ingestmod.MappingTemplate
	err := stomongo.FindOne(ctx, colTemplate, bson.M{
		"orgId":      orgId,
		"templateId": templateId,
	}, &result)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, ErrTemplateNotFound
		}
		return nil, err
	}
	return &result, nil
}

// List returns paginated templates for an org, with optional name search.
func (r *MappingTemplateRepo) List(
	ctx context.Context,
	orgId, search string,
	page, perPage int,
	sortField, sortOrder string,
) ([]*ingestmod.MappingTemplate, *gmod.Pagination, error) {
	filter := bson.M{"orgId": orgId}
	if search != "" {
		filter["name"] = bson.M{"$regex": regexp.QuoteMeta(search), "$options": "i"}
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

	var result []*ingestmod.MappingTemplate
	pagination, err := stomongo.FindWithPagination(ctx, colTemplate, filter, sort, page, perPage, &result)
	if err != nil {
		return nil, nil, err
	}
	pagination.SortField = sortField
	pagination.SortOrder = sortOrder
	return result, &pagination, nil
}

// Update patches a template by orgId + templateId.
// setFields must be a bson.M of fields to update (updatedAt is added automatically).
func (r *MappingTemplateRepo) Update(ctx context.Context, orgId, templateId string, setFields bson.M) error {
	res, err := stomongo.UpdateOne(ctx, colTemplate, bson.M{
		"orgId":      orgId,
		"templateId": templateId,
	}, setFields)
	if err != nil {
		return err
	}
	if res.MatchedCount == 0 {
		return ErrTemplateNotFound
	}
	return nil
}

// Delete removes a template by orgId + templateId.
func (r *MappingTemplateRepo) Delete(ctx context.Context, orgId, templateId string) error {
	res, err := stomongo.DeleteOne(ctx, colTemplate, bson.M{
		"orgId":      orgId,
		"templateId": templateId,
	})
	if err != nil {
		return err
	}
	if res.DeletedCount == 0 {
		return ErrTemplateNotFound
	}
	return nil
}

// FindByMatch returns the first template whose match rule fits the incoming event.
//
// Matching is flexible: if a template's match field is empty, it acts as a wildcard.
// Criteria checked (all must pass):
//   - match.eventType:       template value must be "" OR equal eventType
//   - match.rawBodyKeyHash:  template value must be "" OR equal keyHash
//   - match.deviceType:      template value must be "" OR equal deviceType (when deviceType provided)
//
// Returns ErrTemplateNotFound when no template matches.
func (r *MappingTemplateRepo) FindByMatch(ctx context.Context, orgId, eventType, keyHash, deviceType, deviceId string) (*ingestmod.MappingTemplate, error) {
	and := bson.A{
		// eventType: template "" = wildcard matches any eventType
		bson.M{"$or": bson.A{
			bson.M{"match.eventType": ""},
			bson.M{"match.eventType": bson.M{"$exists": false}},
			bson.M{"match.eventType": eventType},
		}},
		// rawBodyKeyHash: template "" = wildcard matches any schema
		bson.M{"$or": bson.A{
			bson.M{"match.rawBodyKeyHash": ""},
			bson.M{"match.rawBodyKeyHash": bson.M{"$exists": false}},
			bson.M{"match.rawBodyKeyHash": keyHash},
		}},
	}
	// deviceType: only enforce when the incoming event provides one
	if deviceType != "" {
		and = append(and, bson.M{"$or": bson.A{
			bson.M{"match.deviceType": ""},
			bson.M{"match.deviceType": bson.M{"$exists": false}},
			bson.M{"match.deviceType": deviceType},
		}})
	}
	// deviceId: only enforce when the incoming event provides one
	if deviceId != "" {
		and = append(and, bson.M{"$or": bson.A{
			bson.M{"match.deviceId": ""},
			bson.M{"match.deviceId": bson.M{"$exists": false}},
			bson.M{"match.deviceId": deviceId},
		}})
	}

	filter := bson.M{"orgId": orgId, "$and": and}

	var result ingestmod.MappingTemplate
	err := stomongo.FindOne(ctx, colTemplate, filter, &result)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, ErrTemplateNotFound
		}
		return nil, err
	}
	return &result, nil
}

// EnsureIndexes creates indexes on mapping_templates collection.
func (r *MappingTemplateRepo) EnsureIndexes(ctx context.Context) error {
	return stomongo.CreateIndexes(ctx, colTemplate, []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "orgId", Value: 1}, {Key: "templateId", Value: 1}},
			Options: options.Index().SetUnique(true),
		},
		{
			Keys: bson.D{{Key: "orgId", Value: 1}, {Key: "name", Value: 1}},
		},
		// Fingerprint match index (PR5): hot path lookup on every ingest
		{
			Keys: bson.D{
				{Key: "orgId", Value: 1},
				{Key: "match.eventType", Value: 1},
				{Key: "match.rawBodyKeyHash", Value: 1},
			},
		},
		{
			Keys: bson.D{{Key: "createdAt", Value: -1}},
		},
	})
}
