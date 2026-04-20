// internal/repo/ingesttmplrepo/repo.go
package ingesttmplrepo

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/hotkhwan/gateway-api/internal/repo/stomongo"
	"github.com/hotkhwan/gateway-api/models/gmod"
	"github.com/hotkhwan/gateway-api/models/ingestmod"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var ErrTemplateNotFound = errors.New("ingest template not found")
var ErrTemplateNameExists = errors.New("ingest template name already exists in this workspace")

const collectionName = "ingest_templates"

type IngestTemplateRepo struct{}

func NewIngestTemplateRepo() *IngestTemplateRepo { return &IngestTemplateRepo{} }

func (r *IngestTemplateRepo) EnsureIndexes(ctx context.Context) error {
	return stomongo.EnsureUniqueIndex(ctx, collectionName, bson.D{
		{Key: "workspaceId", Value: 1},
		{Key: "name", Value: 1},
	}, "uq_ingest_template_name_per_workspace")
}

func (r *IngestTemplateRepo) Insert(ctx context.Context, t *ingestmod.IngestTemplate) error {
	t.ID = uuid.NewString()
	now := time.Now().UTC()
	t.CreatedAt = now
	t.UpdatedAt = now
	_, err := stomongo.InsertOne(ctx, collectionName, t)
	return err
}

func (r *IngestTemplateRepo) FindByID(ctx context.Context, workspaceId, id string) (*ingestmod.IngestTemplate, error) {
	var result ingestmod.IngestTemplate
	err := stomongo.FindOne(ctx, collectionName, bson.M{
		"_id":         id,
		"workspaceId": workspaceId,
	}, &result)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, ErrTemplateNotFound
		}
		return nil, err
	}
	return &result, nil
}

func (r *IngestTemplateRepo) ExistsByName(ctx context.Context, workspaceId, name, excludeID string) (bool, error) {
	filter := bson.M{
		"workspaceId": workspaceId,
		"name":        name,
	}
	if excludeID != "" {
		filter["_id"] = bson.M{"$ne": excludeID}
	}
	count, err := stomongo.Count(ctx, collectionName, filter)
	return count > 0, err
}

func (r *IngestTemplateRepo) List(
	ctx context.Context,
	workspaceId string,
	page, perPage int,
) ([]ingestmod.IngestTemplate, *gmod.Pagination, error) {
	filter := bson.M{"workspaceId": workspaceId}
	sort := bson.D{{Key: "createdAt", Value: -1}}
	var results []ingestmod.IngestTemplate
	pag, err := stomongo.FindWithPagination(ctx, collectionName, filter, sort, page, perPage, &results)
	if err != nil {
		return nil, nil, err
	}
	return results, &pag, nil
}

func (r *IngestTemplateRepo) Update(ctx context.Context, workspaceId, id string, fields bson.M) error {
	fields["updatedAt"] = time.Now().UTC()
	res, err := stomongo.UpdateOne(ctx, collectionName, bson.M{
		"_id":         id,
		"workspaceId": workspaceId,
	}, fields)
	if err != nil {
		return err
	}
	if res.MatchedCount == 0 {
		return ErrTemplateNotFound
	}
	return nil
}

func (r *IngestTemplateRepo) Delete(ctx context.Context, workspaceId, id string) error {
	res, err := stomongo.DeleteOne(ctx, collectionName, bson.M{
		"_id":         id,
		"workspaceId": workspaceId,
	})
	if err != nil {
		return err
	}
	if res.DeletedCount == 0 {
		return ErrTemplateNotFound
	}
	return nil
}

// FindEnabledByWorkspaceAndFamily returns all enabled templates for a workspace + sourceFamily.
func (r *IngestTemplateRepo) FindEnabledByWorkspaceAndFamily(ctx context.Context, workspaceId, sourceFamily string) ([]ingestmod.IngestTemplate, error) {
	var result []ingestmod.IngestTemplate
	opts := options.Find().SetSort(bson.D{{Key: "createdAt", Value: -1}})
	err := stomongo.Find(ctx, collectionName, bson.M{
		"workspaceId":  workspaceId,
		"sourceFamily": sourceFamily,
		"enabled":      true,
	}, opts, &result)
	if err != nil {
		return nil, err
	}
	return result, nil
}
