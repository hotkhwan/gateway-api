// internal/repo/msgtmplrepo/repo.go
package msgtmplrepo

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
)

var ErrMsgTemplateNotFound = errors.New("message template not found")
var ErrMsgTemplateNameExists = errors.New("message template name already exists in this workspace")

const collectionName = "workspace_message_templates"

type MsgTemplateRepo struct{}

func NewMsgTemplateRepo() *MsgTemplateRepo { return &MsgTemplateRepo{} }

func (r *MsgTemplateRepo) EnsureIndexes(ctx context.Context) error {
	return stomongo.EnsureUniqueIndex(ctx, collectionName, bson.D{
		{Key: "workspaceId", Value: 1},
		{Key: "name", Value: 1},
	}, "uq_msg_template_name_per_workspace")
}

func (r *MsgTemplateRepo) Insert(ctx context.Context, t *ingestmod.WorkspaceMessageTemplate) error {
	t.ID = uuid.NewString()
	now := time.Now().UTC()
	t.CreatedAt = now
	t.UpdatedAt = now
	_, err := stomongo.InsertOne(ctx, collectionName, t)
	return err
}

func (r *MsgTemplateRepo) FindByID(ctx context.Context, workspaceId, id string) (*ingestmod.WorkspaceMessageTemplate, error) {
	var result ingestmod.WorkspaceMessageTemplate
	err := stomongo.FindOne(ctx, collectionName, bson.M{
		"_id":         id,
		"workspaceId": workspaceId,
	}, &result)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, ErrMsgTemplateNotFound
		}
		return nil, err
	}
	return &result, nil
}

func (r *MsgTemplateRepo) ExistsByName(ctx context.Context, workspaceId, name, excludeID string) (bool, error) {
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

func (r *MsgTemplateRepo) List(
	ctx context.Context,
	workspaceId string,
	page, perPage int,
) ([]ingestmod.WorkspaceMessageTemplate, *gmod.Pagination, error) {
	filter := bson.M{"workspaceId": workspaceId}
	sort := bson.D{{Key: "createdAt", Value: -1}}
	var results []ingestmod.WorkspaceMessageTemplate
	pag, err := stomongo.FindWithPagination(ctx, collectionName, filter, sort, page, perPage, &results)
	if err != nil {
		return nil, nil, err
	}
	return results, &pag, nil
}

func (r *MsgTemplateRepo) Update(ctx context.Context, workspaceId, id string, fields bson.M) error {
	fields["updatedAt"] = time.Now().UTC()
	res, err := stomongo.UpdateOne(ctx, collectionName, bson.M{
		"_id":         id,
		"workspaceId": workspaceId,
	}, fields)
	if err != nil {
		return err
	}
	if res.MatchedCount == 0 {
		return ErrMsgTemplateNotFound
	}
	return nil
}

func (r *MsgTemplateRepo) Delete(ctx context.Context, workspaceId, id string) error {
	res, err := stomongo.DeleteOne(ctx, collectionName, bson.M{
		"_id":         id,
		"workspaceId": workspaceId,
	})
	if err != nil {
		return err
	}
	if res.DeletedCount == 0 {
		return ErrMsgTemplateNotFound
	}
	return nil
}
