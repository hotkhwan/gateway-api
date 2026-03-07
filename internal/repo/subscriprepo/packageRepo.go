// internal/repo/subscriprepo/packageRepo.go
package subscriprepo

import (
	"context"
	"errors"
	"time"

	"github.com/hotkhwan/gateway-api/internal/repo/stomongo"
	"github.com/hotkhwan/gateway-api/models/subscripmod"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const colPackages = "subscription_packages"

var ErrPackageNotFound = errors.New("subscription package not found")

// PackageRepo — CRUD for subscription_packages collection.
// Package catalog is seeded at bootstrap and updated via admin API only.
type PackageRepo struct{}

func NewPackageRepo() *PackageRepo {
	return &PackageRepo{}
}

// FindById returns a package by its planId/code (e.g. "freemium", "pro", "enterprise")
func (r *PackageRepo) FindById(ctx context.Context, packageId string) (*subscripmod.SubscriptionPackage, error) {
	var result subscripmod.SubscriptionPackage
	err := stomongo.FindOne(ctx, colPackages, bson.M{"id": packageId}, &result)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, ErrPackageNotFound
		}
		return nil, err
	}
	return &result, nil
}

// ListActive returns all active packages ordered by sortOrder asc
func (r *PackageRepo) ListActive(ctx context.Context, publicOnly bool) ([]subscripmod.SubscriptionPackage, error) {
	filter := bson.M{"status": "active"}
	if publicOnly {
		filter["isPublic"] = true
	}
	var results []subscripmod.SubscriptionPackage
	opts := options.Find().SetSort(bson.D{{Key: "sortOrder", Value: 1}})
	err := stomongo.Find(ctx, colPackages, filter, opts, &results)
	return results, err
}

// Upsert creates or updates a package by id (used for seed and admin updates).
// Uses $set so existing documents are updated in-place on every startup.
func (r *PackageRepo) Upsert(ctx context.Context, pkg *subscripmod.SubscriptionPackage) error {
	now := time.Now().UTC()
	pkg.UpdatedAt = now
	_, err := stomongo.UpsertByFilter(
		ctx,
		colPackages,
		bson.M{"id": pkg.PackageId},
		bson.M{
			"code":        pkg.Code,
			"name":        pkg.Name,
			"description": pkg.Description,
			"status":      pkg.Status,
			"sortOrder":   pkg.SortOrder,
			"isPublic":    pkg.IsPublic,
			"billing":     pkg.Billing,
			"limits":      pkg.Limits,
			"features":    pkg.Features,
			"ui":          pkg.UI,
			"updatedAt":   now,
		},
		bson.M{
			"id":        pkg.PackageId,
			"createdAt": pkg.CreatedAt,
		},
	)
	return err
}

// EnsureIndexes creates required indexes for subscription_packages
func (r *PackageRepo) EnsureIndexes(ctx context.Context) error {
	if err := stomongo.EnsureUniqueIndex(ctx, colPackages, bson.D{
		{Key: "id", Value: 1},
	}, "uq_pkg_id"); err != nil {
		return err
	}
	if err := stomongo.EnsureUniqueIndex(ctx, colPackages, bson.D{
		{Key: "code", Value: 1},
	}, "uq_pkg_code"); err != nil {
		return err
	}
	return stomongo.CreateIndexes(ctx, colPackages, []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "status", Value: 1}, {Key: "sortOrder", Value: 1}},
			Options: options.Index().SetName("idx_pkg_status_sort"),
		},
	})
}
