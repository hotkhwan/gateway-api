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

// GetKlynxOrgID returns the klynx orgId for a given workspaceId.
// Returns "" (no error) when the workspace exists but has no klynxOrgId.
// Returns ErrWorkspaceNotFound when the workspace does not exist.
func (r *WorkspaceRepo) GetKlynxOrgID(ctx context.Context, workspaceId string) (string, error) {
	ws, err := r.FindByWorkspaceID(ctx, workspaceId)
	if err != nil {
		return "", err
	}
	return ws.KlynxOrgID, nil
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

// ListByIDs returns workspaces matching the given workspaceId list.
// Used by WorkspaceSvc to list workspaces visible to a user (IDs come from Permify lookup).
func (r *WorkspaceRepo) ListByIDs(ctx context.Context, ids []string) ([]*workspacemod.Workspace, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	col := config.DB.Collection(collection)
	cursor, err := col.Find(ctx, bson.M{"workspaceId": bson.M{"$in": ids}})
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	var out []*workspacemod.Workspace
	if err := cursor.All(ctx, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// ListByTenantID returns every workspace belonging to a tenant. Used by the
// platform-license activation path to invalidate runtime entitlement caches
// across all workspaces owned by the activating tenant.
func (r *WorkspaceRepo) ListByTenantID(ctx context.Context, tenantID string) ([]*workspacemod.Workspace, error) {
	if tenantID == "" {
		return nil, nil
	}
	col := config.DB.Collection(collection)
	cursor, err := col.Find(ctx, bson.M{"tenantId": tenantID})
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	var out []*workspacemod.Workspace
	if err := cursor.All(ctx, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// ListAll returns all workspaces regardless of ownership. Used by platform admins.
func (r *WorkspaceRepo) ListAll(ctx context.Context) ([]*workspacemod.Workspace, error) {
	col := config.DB.Collection(collection)
	cursor, err := col.Find(ctx, bson.M{})
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	var out []*workspacemod.Workspace
	if err := cursor.All(ctx, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// UpdateName patches the workspace name (and updatedAt).
func (r *WorkspaceRepo) UpdateName(ctx context.Context, workspaceID, name string) error {
	col := config.DB.Collection(collection)
	_, err := col.UpdateOne(ctx,
		bson.M{"workspaceId": workspaceID},
		bson.M{"$set": bson.M{"name": name, "updatedAt": time.Now().UTC()}},
	)
	return err
}

// UpdateIngestKey sets the ingestKey on the workspace's ingestConfig subdocument.
func (r *WorkspaceRepo) UpdateIngestKey(ctx context.Context, workspaceID, ingestKey string) error {
	col := config.DB.Collection(collection)
	_, err := col.UpdateOne(ctx,
		bson.M{"workspaceId": workspaceID},
		bson.M{"$set": bson.M{
			"ingestConfig.ingestKey": ingestKey,
			"updatedAt":              time.Now().UTC(),
		}},
	)
	return err
}

// Delete hard-deletes a workspace record. Use only for standalone (non-klynx-provisioned) workspaces.
func (r *WorkspaceRepo) Delete(ctx context.Context, workspaceID string) error {
	col := config.DB.Collection(collection)
	_, err := col.DeleteOne(ctx, bson.M{"workspaceId": workspaceID})
	return err
}

func init() {
	config.RegisterMongoBootstrap(func(ctx context.Context) error {
		return NewWorkspaceRepo().EnsureIndexes(ctx)
	})
}
