// internal/repo/configdraftrepo/repo.go
package configdraftrepo

import (
	"context"
	"errors"

	"github.com/hotkhwan/gateway-api/config"
	"github.com/hotkhwan/gateway-api/internal/repo/stomongo"
	"github.com/hotkhwan/gateway-api/internal/services/aiconfigdraftsvc"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

const collection = "ai_config_drafts"

// ConfigDraftRepo provides persistence for ConfigDraft documents.
type ConfigDraftRepo struct{}

// NewConfigDraftRepo returns a new ConfigDraftRepo.
func NewConfigDraftRepo() *ConfigDraftRepo { return &ConfigDraftRepo{} }

// EnsureIndexes creates a unique compound index on (workspaceId, draftId).
func (r *ConfigDraftRepo) EnsureIndexes(ctx context.Context) error {
	return stomongo.EnsureUniqueIndex(
		ctx,
		collection,
		bson.D{{Key: "workspaceId", Value: 1}, {Key: "draftId", Value: 1}},
		"idx_ai_config_drafts_workspace_draft",
	)
}

// Insert persists a new ConfigDraft document.
func (r *ConfigDraftRepo) Insert(ctx context.Context, draft *aiconfigdraftsvc.ConfigDraft) error {
	_, err := stomongo.InsertOne(ctx, collection, draft)
	return err
}

// FindByID retrieves a ConfigDraft by workspaceId and draftId.
// Returns aiconfigdraftsvc.ErrDraftNotFound when the document does not exist.
func (r *ConfigDraftRepo) FindByID(ctx context.Context, workspaceId, draftId string) (*aiconfigdraftsvc.ConfigDraft, error) {
	var draft aiconfigdraftsvc.ConfigDraft
	err := stomongo.FindOne(ctx, collection, bson.M{
		"workspaceId": workspaceId,
		"draftId":     draftId,
	}, &draft)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, aiconfigdraftsvc.ErrDraftNotFound
		}
		return nil, err
	}
	return &draft, nil
}

// Update applies a $set update to a ConfigDraft identified by workspaceId and draftId.
func (r *ConfigDraftRepo) Update(ctx context.Context, workspaceId, draftId string, update bson.M) error {
	_, err := stomongo.UpdateOne(ctx, collection, bson.M{
		"workspaceId": workspaceId,
		"draftId":     draftId,
	}, update)
	return err
}

func init() {
	config.RegisterMongoBootstrap(func(ctx context.Context) error {
		return NewConfigDraftRepo().EnsureIndexes(ctx)
	})
}
