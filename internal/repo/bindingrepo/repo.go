// internal/repo/bindingrepo/repo.go
package bindingrepo

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

var ErrBindingNotFound = errors.New("delivery binding not found")

const collectionName = "template_delivery_bindings"

type BindingRepo struct{}

func NewBindingRepo() *BindingRepo { return &BindingRepo{} }

func (r *BindingRepo) EnsureIndexes(ctx context.Context) error {
	return stomongo.CreateIndexes(ctx, collectionName, []mongo.IndexModel{
		{
			Keys: bson.D{
				{Key: "workspaceId", Value: 1},
				{Key: "dispatchStage", Value: 1},
				{Key: "enabled", Value: 1},
			},
			Options: indexOpts("idx_binding_workspace_stage"),
		},
	})
}

func indexOpts(name string) *options.IndexOptions {
	return options.Index().SetName(name)
}

func (r *BindingRepo) Insert(ctx context.Context, b *ingestmod.TemplateDeliveryBinding) error {
	b.ID = uuid.NewString()
	now := time.Now().UTC()
	b.CreatedAt = now
	b.UpdatedAt = now
	_, err := stomongo.InsertOne(ctx, collectionName, b)
	return err
}

func (r *BindingRepo) FindByID(ctx context.Context, workspaceId, id string) (*ingestmod.TemplateDeliveryBinding, error) {
	var result ingestmod.TemplateDeliveryBinding
	err := stomongo.FindOne(ctx, collectionName, bson.M{
		"_id":         id,
		"workspaceId": workspaceId,
	}, &result)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, ErrBindingNotFound
		}
		return nil, err
	}
	return &result, nil
}

// FindAllByStage returns all enabled bindings across all workspaces for a given stage.
// Used at startup to warm the realtime cache.
func (r *BindingRepo) FindAllByStage(ctx context.Context, stage string) ([]ingestmod.TemplateDeliveryBinding, error) {
	var result []ingestmod.TemplateDeliveryBinding
	opts := options.Find().SetSort(bson.D{{Key: "workspaceId", Value: 1}})
	err := stomongo.Find(ctx, collectionName, bson.M{
		"dispatchStage": stage,
		"enabled":       true,
	}, opts, &result)
	if err != nil {
		return nil, err
	}
	return result, nil
}

// FindByWorkspaceAndStage returns all enabled bindings for a workspace + dispatch stage.
func (r *BindingRepo) FindByWorkspaceAndStage(ctx context.Context, workspaceId, stage string) ([]ingestmod.TemplateDeliveryBinding, error) {
	var result []ingestmod.TemplateDeliveryBinding
	opts := options.Find().SetSort(bson.D{{Key: "createdAt", Value: -1}})
	err := stomongo.Find(ctx, collectionName, bson.M{
		"workspaceId":   workspaceId,
		"dispatchStage": stage,
		"enabled":       true,
	}, opts, &result)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (r *BindingRepo) List(
	ctx context.Context,
	workspaceId string,
	page, perPage int,
) ([]ingestmod.TemplateDeliveryBinding, *gmod.Pagination, error) {
	filter := bson.M{"workspaceId": workspaceId}
	sort := bson.D{{Key: "createdAt", Value: -1}}
	var results []ingestmod.TemplateDeliveryBinding
	pag, err := stomongo.FindWithPagination(ctx, collectionName, filter, sort, page, perPage, &results)
	if err != nil {
		return nil, nil, err
	}
	return results, &pag, nil
}

func (r *BindingRepo) Update(ctx context.Context, workspaceId, id string, fields bson.M) error {
	fields["updatedAt"] = time.Now().UTC()
	res, err := stomongo.UpdateOne(ctx, collectionName, bson.M{
		"_id":         id,
		"workspaceId": workspaceId,
	}, fields)
	if err != nil {
		return err
	}
	if res.MatchedCount == 0 {
		return ErrBindingNotFound
	}
	return nil
}

func (r *BindingRepo) Delete(ctx context.Context, workspaceId, id string) error {
	res, err := stomongo.DeleteOne(ctx, collectionName, bson.M{
		"_id":         id,
		"workspaceId": workspaceId,
	})
	if err != nil {
		return err
	}
	if res.DeletedCount == 0 {
		return ErrBindingNotFound
	}
	return nil
}
