// internal/repo/subscriprepo/licenseRepo.go
package subscriprepo

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/hotkhwan/gateway-api/internal/repo/stomongo"
	"github.com/hotkhwan/gateway-api/models/subscripmod"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var (
	ErrLicenseNotFound        = errors.New("license key not found")
	ErrLicenseAlreadyActivated = errors.New("license key already activated")
	ErrLicenseRevoked         = errors.New("license key has been revoked")
)

type LicenseRepo struct {
	col *mongo.Collection
}

func NewLicenseRepo(db *mongo.Database) *LicenseRepo {
	return &LicenseRepo{col: db.Collection("license_keys")}
}

// FindByKey finds license by key (case-insensitive)
func (r *LicenseRepo) FindByKey(ctx context.Context, key string) (*subscripmod.LicenseKey, error) {
	var result subscripmod.LicenseKey
	filter := bson.M{"key": key}

	err := stomongo.FindOne(ctx, r.col.Name(), filter, &result)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, ErrLicenseNotFound
		}
		return nil, err
	}
	return &result, nil
}

// FindById finds license by ID
func (r *LicenseRepo) FindById(ctx context.Context, id primitive.ObjectID) (*subscripmod.LicenseKey, error) {
	var result subscripmod.LicenseKey

	err := stomongo.FindOne(ctx, r.col.Name(), bson.M{"_id": id}, &result)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, ErrLicenseNotFound
		}
		return nil, err
	}
	return &result, nil
}

// Create creates a new license key
func (r *LicenseRepo) Create(ctx context.Context, license *subscripmod.LicenseKey) error {
	_, err := r.col.InsertOne(ctx, license)
	return err
}

// MarkLicenseActivated marks a license as activated for a tenant
func (r *LicenseRepo) MarkLicenseActivated(
	ctx context.Context,
	id primitive.ObjectID,
	tenantId string,
	activatedAt time.Time,
) error {
	now := time.Now().UTC()

	// Check if already activated first
	existing, err := r.FindById(ctx, id)
	if err != nil {
		return err
	}

	if existing.Status == subscripmod.LicenseStatusActivated {
		return ErrLicenseAlreadyActivated
	}

	if existing.Status == subscripmod.LicenseStatusRevoked {
		return ErrLicenseRevoked
	}

	update := bson.M{
		"$set": bson.M{
			"status":       subscripmod.LicenseStatusActivated,
			"tenantId":     tenantId,
			"activatedAt":  activatedAt,
			"updatedAt":    now,
		},
	}

	_, err = r.col.UpdateOne(ctx, bson.M{"_id": id}, update)
	return err
}

// Revoke revokes a license key
func (r *LicenseRepo) Revoke(ctx context.Context, id primitive.ObjectID) error {
	now := time.Now().UTC()

	update := bson.M{
		"$set": bson.M{
			"status":    subscripmod.LicenseStatusRevoked,
			"updatedAt": now,
		},
	}

	_, err := r.col.UpdateOne(ctx, bson.M{"_id": id}, update)
	return err
}

// List lists all license keys (with optional filters)
func (r *LicenseRepo) List(ctx context.Context, filter bson.M) ([]*subscripmod.LicenseKey, error) {
	opts := options.Find().SetSort(bson.M{"createdAt": -1})

	cursor, err := r.col.Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var results []*subscripmod.LicenseKey
	if err := cursor.All(ctx, &results); err != nil {
		return nil, err
	}

	return results, nil
}

// CountByStatus counts licenses by status
func (r *LicenseRepo) CountByStatus(ctx context.Context, status subscripmod.LicenseStatus) (int64, error) {
	count, err := r.col.CountDocuments(ctx, bson.M{"status": status})
	return count, err
}

// FindByTenantId finds activated license for a tenant
func (r *LicenseRepo) FindByTenantId(ctx context.Context, tenantId string) (*subscripmod.LicenseKey, error) {
	var result subscripmod.LicenseKey

	err := stomongo.FindOne(ctx, r.col.Name(), bson.M{"tenantId": tenantId}, &result)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, ErrLicenseNotFound
		}
		return nil, err
	}
	return &result, nil
}

// ValidateLicenseKey validates if a license key can be activated
func (r *LicenseRepo) ValidateLicenseKey(ctx context.Context, key string) (*subscripmod.LicenseKey, error) {
	license, err := r.FindByKey(ctx, key)
	if err != nil {
		if errors.Is(err, ErrLicenseNotFound) {
			return nil, fmt.Errorf("invalid license key")
		}
		return nil, fmt.Errorf("failed to validate license: %w", err)
	}

	switch license.Status {
	case subscripmod.LicenseStatusAvailable:
		return license, nil
	case subscripmod.LicenseStatusActivated:
		return nil, ErrLicenseAlreadyActivated
	case subscripmod.LicenseStatusRevoked:
		return nil, ErrLicenseRevoked
	default:
		return nil, fmt.Errorf("unknown license status: %s", license.Status)
	}
}
