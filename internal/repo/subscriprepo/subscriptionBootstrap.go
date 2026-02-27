// internal/repo/subscriprepo/subscriptionBootstrap.go
package subscriprepo

import (
	"context"

	"github.com/hotkhwan/gateway-api/config"
	"github.com/hotkhwan/gateway-api/internal/repo/stomongo"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func init() {
	config.RegisterMongoBootstrap(func(ctx context.Context) error {
		// Ensure subscriptions indexes
		if err := NewSubscriptionRepo(config.DB).EnsureIndexes(ctx); err != nil {
			return err
		}
		return nil
	})
}

// EnsureIndexes ensures required indexes for subscriptions collection
func (r *SubscriptionRepo) EnsureIndexes(ctx context.Context) error {
	// Unique index on tenantId (one tenant = one active subscription)
	if err := stomongo.EnsureUniqueIndex(
		ctx,
		"subscriptions",
		bson.D{
			{Key: "tenantId", Value: 1},
		},
		"uq_tenantId",
	); err != nil {
		return err
	}

	// Index on status + updatedAt for ops/debug
	statusIdx := mongo.IndexModel{
		Keys: bson.D{
			{Key: "status", Value: 1},
			{Key: "updatedAt", Value: -1},
		},
		Options: options.Index().SetName("idx_status_updatedAt"),
	}
	if err := stomongo.CreateIndexes(ctx, "subscriptions", []mongo.IndexModel{statusIdx}); err != nil {
		return err
	}

	// Index on planId for querying by plan
	planIdx := mongo.IndexModel{
		Keys: bson.D{
			{Key: "planId", Value: 1},
		},
		Options: options.Index().SetName("idx_planId"),
	}
	if err := stomongo.CreateIndexes(ctx, "subscriptions", []mongo.IndexModel{planIdx}); err != nil {
		return err
	}

	return nil
}
