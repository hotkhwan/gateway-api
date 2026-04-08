// internal/repo/workspacerepo/repo.go
package workspacerepo

import (
	"context"
	"errors"
	"time"

	"github.com/hotkhwan/gateway-api/config"
	"github.com/hotkhwan/gateway-api/internal/repo/stomongo"
	"github.com/hotkhwan/gateway-api/models/workspacemod"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var ErrWorkspaceNotFound = errors.New("workspace not found")

const collection = "workspaces"

type WorkspaceRepo struct{}

func NewWorkspaceRepo() *WorkspaceRepo { return &WorkspaceRepo{} }

// EnsureIndexes creates unique indexes on workspaceId and klynxOrgId.
func (r *WorkspaceRepo) EnsureIndexes(ctx context.Context) error {
	if err := stomongo.EnsureUniqueIndex(ctx, collection, bson.D{{Key: "workspaceId", Value: 1}}, "idx_workspace_id"); err != nil {
		return err
	}
	return stomongo.EnsureUniqueIndex(ctx, collection, bson.D{{Key: "klynxOrgId", Value: 1}}, "idx_klynx_org_id")
}

// Upsert inserts or updates a workspace record by workspaceId.
func (r *WorkspaceRepo) Upsert(ctx context.Context, ws *workspacemod.Workspace) error {
	col := config.DB.Collection(collection)
	filter := bson.M{"workspaceId": ws.WorkspaceID}
	update := bson.M{
		"$set": bson.M{
			"workspaceId":  ws.WorkspaceID,
			"klynxOrgId":   ws.KlynxOrgID,
			"tenantId":     ws.TenantID,
			"ownerUserId":  ws.OwnerUserID,
			"name":         ws.Name,
			"status":       ws.Status,
			"eventUri":     ws.EventURI,
			"updatedAt":    time.Now().UTC(),
		},
		"$setOnInsert": bson.M{"createdAt": ws.CreatedAt},
	}
	opts := options.Update().SetUpsert(true)
	_, err := col.UpdateOne(ctx, filter, update, opts)
	return err
}

// FindByWorkspaceID returns the workspace for a given workspaceId.
func (r *WorkspaceRepo) FindByWorkspaceID(ctx context.Context, workspaceID string) (*workspacemod.Workspace, error) {
	col := config.DB.Collection(collection)
	var ws workspacemod.Workspace
	if err := col.FindOne(ctx, bson.M{"workspaceId": workspaceID}).Decode(&ws); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, ErrWorkspaceNotFound
		}
		return nil, err
	}
	return &ws, nil
}

// FindByKlynxOrgID returns the workspace linked to a klynx org.
func (r *WorkspaceRepo) FindByKlynxOrgID(ctx context.Context, klynxOrgID string) (*workspacemod.Workspace, error) {
	col := config.DB.Collection(collection)
	var ws workspacemod.Workspace
	if err := col.FindOne(ctx, bson.M{"klynxOrgId": klynxOrgID}).Decode(&ws); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, ErrWorkspaceNotFound
		}
		return nil, err
	}
	return &ws, nil
}

// UpdateStatus sets the workspace status (active | suspended | archived).
func (r *WorkspaceRepo) UpdateStatus(ctx context.Context, workspaceID, status string) error {
	col := config.DB.Collection(collection)
	_, err := col.UpdateOne(ctx,
		bson.M{"workspaceId": workspaceID},
		bson.M{"$set": bson.M{"status": status, "updatedAt": time.Now().UTC()}},
	)
	return err
}

func init() {
	config.RegisterMongoBootstrap(func(ctx context.Context) error {
		return NewWorkspaceRepo().EnsureIndexes(ctx)
	})
}
