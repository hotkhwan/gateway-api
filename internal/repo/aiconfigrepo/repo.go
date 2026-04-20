// internal/repo/aiconfigrepo/repo.go
package aiconfigrepo

import (
	"context"
	"errors"
	"time"

	"github.com/hotkhwan/gateway-api/config"
	"github.com/hotkhwan/gateway-api/internal/repo/stomongo"
	"github.com/hotkhwan/gateway-api/models/workspacemod"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

var ErrNotFound = errors.New("ai config not found")

const collection = "workspace_ai_configs"

type AIConfigRepo struct{}

func NewAIConfigRepo() *AIConfigRepo { return &AIConfigRepo{} }

// EnsureIndexes creates a unique index on workspaceId.
func (r *AIConfigRepo) EnsureIndexes(ctx context.Context) error {
	return stomongo.EnsureUniqueIndex(ctx, collection, bson.D{{Key: "workspaceId", Value: 1}}, "idx_workspace_ai_config_workspace_id")
}

// FindByWorkspaceID returns the AI config for a given workspaceId.
// Returns ErrNotFound if no document exists.
func (r *AIConfigRepo) FindByWorkspaceID(ctx context.Context, workspaceID string) (*workspacemod.WorkspaceAIConfig, error) {
	var cfg workspacemod.WorkspaceAIConfig
	err := stomongo.FindOne(ctx, collection, bson.M{"workspaceId": workspaceID}, &cfg)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &cfg, nil
}

// Upsert inserts or updates the AI config document by workspaceId.
func (r *AIConfigRepo) Upsert(ctx context.Context, cfg *workspacemod.WorkspaceAIConfig) error {
	now := time.Now().UTC()
	filter := bson.M{"workspaceId": cfg.WorkspaceID}
	setFields := bson.M{
		"workspaceId":          cfg.WorkspaceID,
		"enabled":              cfg.Enabled,
		"provider":             cfg.Provider,
		"model":                cfg.Model,
		"encryptedApiKey":      cfg.EncryptedApiKey,
		"providerMode":         cfg.ProviderMode,
		"defaultTimeoutMs":     cfg.DefaultTimeoutMs,
		"maxInputBytes":        cfg.MaxInputBytes,
		"updatedBy":            cfg.UpdatedBy,
		"lastValidatedAt":      cfg.LastValidatedAt,
		"lastValidationStatus": cfg.LastValidationStatus,
		"lastValidationError":  cfg.LastValidationError,
		"updatedAt":            now,
	}
	onInsert := bson.M{
		"createdBy": cfg.CreatedBy,
		"createdAt": now,
	}
	_, err := stomongo.UpsertByFilter(ctx, collection, filter, setFields, onInsert)
	return err
}

// ClearKey sets encryptedApiKey to null, switches providerMode to "freeSharedProvider",
// and records who cleared the key and when.
func (r *AIConfigRepo) ClearKey(ctx context.Context, workspaceID, updatedBy string) error {
	col := config.DB.Collection(collection)
	_, err := col.UpdateOne(ctx,
		bson.M{"workspaceId": workspaceID},
		bson.M{
			"$set": bson.M{
				"providerMode": "freeSharedProvider",
				"updatedBy":    updatedBy,
				"updatedAt":    time.Now().UTC(),
			},
			"$unset": bson.M{"encryptedApiKey": ""},
		},
	)
	return err
}

func init() {
	config.RegisterMongoBootstrap(func(ctx context.Context) error {
		return NewAIConfigRepo().EnsureIndexes(ctx)
	})
}
