// internal/repo/kctrlregistryrepo/repo.go
//
// Persistence for kctrl_registry. Each approved kcontrol device gets one row
// here, written by klynx-api outbound PATCH per
// klynx-api/docs/contracts/kcontrol-gw-managed-registry.md §3.
package kctrlregistryrepo

import (
	"context"
	"errors"
	"time"

	"github.com/hotkhwan/gateway-api/internal/repo/stomongo"
	"github.com/hotkhwan/gateway-api/models/kctrlmod"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const colKctrlRegistry = "kctrl_registry"

// ErrNotFound is returned when the requested hwId has no registry row.
var ErrNotFound = errors.New("kctrl registry row not found")

// KctrlRegistryRepo wraps the kctrl_registry Mongo collection.
type KctrlRegistryRepo struct{}

func NewKctrlRegistryRepo() *KctrlRegistryRepo { return &KctrlRegistryRepo{} }

// FindByHwId returns the registry row for hwId, or ErrNotFound.
func (r *KctrlRegistryRepo) FindByHwId(ctx context.Context, hwId string) (*kctrlmod.KctrlRegistry, error) {
	var out kctrlmod.KctrlRegistry
	if err := stomongo.FindOne(ctx, colKctrlRegistry, bson.M{"hwId": hwId}, &out); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &out, nil
}

// Upsert sets the row identified by hwId. Returns the updated document.
//
// klynx-api's PATCH payload populates orgId/approved/approvedAt/approvedBy
// plus the bookkeeping fields workspaceId / lastSyncFromKlynxAt /
// lastOutboundHash that gateway-api manages. The caller passes the full
// expected document; this method does a $set on every field.
func (r *KctrlRegistryRepo) Upsert(ctx context.Context, doc *kctrlmod.KctrlRegistry) (*kctrlmod.KctrlRegistry, error) {
	if doc == nil || doc.HwId == "" {
		return nil, errors.New("kctrlregistryrepo: hwId required")
	}
	setFields := bson.M{
		"orgId":               doc.OrgId,
		"workspaceId":         doc.WorkspaceId,
		"approved":            doc.Approved,
		"approvedAt":          doc.ApprovedAt,
		"approvedBy":          doc.ApprovedBy,
		"lastSyncFromKlynxAt": doc.LastSyncFromKlynxAt,
		"lastOutboundHash":    doc.LastOutboundHash,
	}
	_, err := stomongo.UpsertByFilter(ctx, colKctrlRegistry,
		bson.M{"hwId": doc.HwId},
		setFields,
		bson.M{"hwId": doc.HwId},
	)
	if err != nil {
		return nil, err
	}
	return r.FindByHwId(ctx, doc.HwId)
}

// Delete removes the row for hwId. Idempotent — missing row is not an error
// per contract §4.2.
func (r *KctrlRegistryRepo) Delete(ctx context.Context, hwId string) error {
	if hwId == "" {
		return errors.New("kctrlregistryrepo: hwId required")
	}
	_, err := stomongo.DeleteOne(ctx, colKctrlRegistry, bson.M{"hwId": hwId})
	return err
}

// DriftFilter scopes the drift query per contract §4.3.
type DriftFilter struct {
	StaleSince time.Time // rows whose lastSyncFromKlynxAt is before this
}

// DriftRow is the projection returned to operators.
type DriftRow struct {
	HwId                string    `json:"hwId"                  bson:"hwId"`
	OrgId               string    `json:"orgId"                 bson:"orgId"`
	LastSyncFromKlynxAt time.Time `json:"lastSyncFromKlynxAt"   bson:"lastSyncFromKlynxAt"`
}

// ListDrift returns rows where lastSyncFromKlynxAt < StaleSince.
func (r *KctrlRegistryRepo) ListDrift(ctx context.Context, f DriftFilter) ([]DriftRow, error) {
	filter := bson.M{"lastSyncFromKlynxAt": bson.M{"$lt": f.StaleSince}}
	opts := options.Find().SetSort(bson.D{{Key: "lastSyncFromKlynxAt", Value: 1}})
	var out []DriftRow
	if err := stomongo.Find(ctx, colKctrlRegistry, filter, opts, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// CountAll returns the total number of rows. Used in the drift summary.
func (r *KctrlRegistryRepo) CountAll(ctx context.Context) (int64, error) {
	return stomongo.Count(ctx, colKctrlRegistry, bson.M{})
}

// EnsureIndexes installs the unique hwId index plus the drift-scan index.
// Called from bootstrap.go init() through config.RegisterMongoBootstrap.
func (r *KctrlRegistryRepo) EnsureIndexes(ctx context.Context) error {
	return stomongo.CreateIndexes(ctx, colKctrlRegistry, []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "hwId", Value: 1}},
			Options: options.Index().SetUnique(true).SetName("hwId_unique"),
		},
		{
			Keys: bson.D{
				{Key: "approved", Value: 1},
				{Key: "lastSyncFromKlynxAt", Value: 1},
			},
			Options: options.Index().SetName("approved_lastSync"),
		},
	})
}
