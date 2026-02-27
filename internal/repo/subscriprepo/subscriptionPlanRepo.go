// internal/repo/subscriprepo/subscriptionPlanRepo.go
package subscriprepo

import (
	"context"
	"time"

	"github.com/hotkhwan/gateway-api/internal/repo/stomongo"
	"github.com/hotkhwan/gateway-api/models/subscripmod"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

// SubscriptionPlanRepo is optional - for MongoDB-backed plans
// If you prefer hardcoded catalog, you can skip this
type SubscriptionPlanRepo struct {
	col *mongo.Collection
}

func NewSubscriptionPlanRepo(db *mongo.Database) *SubscriptionPlanRepo {
	return &SubscriptionPlanRepo{col: db.Collection("subscriptionPlans")}
}

// FindById finds plan by ID
func (r *SubscriptionPlanRepo) FindById(ctx context.Context, planId string) (*subscripmod.SubscriptionPlan, error) {
	var result subscripmod.SubscriptionPlan
	err := stomongo.FindOne(ctx, r.col.Name(), bson.M{"id": planId}, &result)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

// ListAll returns all plans
func (r *SubscriptionPlanRepo) ListAll(ctx context.Context) ([]subscripmod.SubscriptionPlan, error) {
	var result []subscripmod.SubscriptionPlan
	err := stomongo.Find(ctx, r.col.Name(), bson.M{}, nil, &result)
	return result, err
}

// ListPublic returns public plans available for self-upgrade
func (r *SubscriptionPlanRepo) ListPublic(ctx context.Context) ([]subscripmod.SubscriptionPlan, error) {
	var result []subscripmod.SubscriptionPlan
	err := stomongo.Find(ctx, r.col.Name(), bson.M{"isPublic": true}, nil, &result)
	return result, err
}

// Upsert creates or updates a plan
func (r *SubscriptionPlanRepo) Upsert(ctx context.Context, plan *subscripmod.SubscriptionPlan) error {
	now := time.Now().UTC()
	plan.UpdatedAt = now

	filter := bson.M{"id": plan.PlanId}
	update := bson.M{"$set": plan}
	_, err := r.col.UpdateOne(ctx, filter, update, nil)
	if err != nil {
		return err
	}

	return nil
}

// EnsureIndexes ensures required indexes for subscriptionPlans collection
func (r *SubscriptionPlanRepo) EnsureIndexes(ctx context.Context) error {
	// Unique index on id
	if err := stomongo.EnsureUniqueIndex(
		ctx,
		"subscriptionPlans",
		bson.D{
			{Key: "id", Value: 1},
		},
		"uq_planId",
	); err != nil {
		return err
	}

	return nil
}
