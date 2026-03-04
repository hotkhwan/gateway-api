// internal/repo/subscriprepo/subscriptionRepo.go
package subscriprepo

import (
	"context"
	"time"

	"github.com/hotkhwan/gateway-api/internal/repo/stomongo"
	"github.com/hotkhwan/gateway-api/models/subscripmod"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

type SubscriptionRepo struct {
	col *mongo.Collection
}

func NewSubscriptionRepo(db *mongo.Database) *SubscriptionRepo {
	return &SubscriptionRepo{col: db.Collection("subscriptions")}
}

// FindByTenantId finds subscription by tenantId (one tenant = one active subscription)
func (r *SubscriptionRepo) FindByTenantId(ctx context.Context, tenantId string) (*subscripmod.Subscription, error) {
	var result subscripmod.Subscription
	err := stomongo.FindOne(ctx, r.col.Name(), bson.M{"tenantId": tenantId}, &result)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

// FindById finds subscription by ID
func (r *SubscriptionRepo) FindById(ctx context.Context, id string) (*subscripmod.Subscription, error) {
	var result subscripmod.Subscription

	objId, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return nil, err
	}

	err = stomongo.FindOne(ctx, r.col.Name(), bson.M{"_id": objId}, &result)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

// UpsertDefaultIfMissing creates a default freemium subscription if tenant doesn't have one
func (r *SubscriptionRepo) UpsertDefaultIfMissing(ctx context.Context, tenantId string) (*subscripmod.Subscription, error) {
	// Check if already exists
	existing, err := r.FindByTenantId(ctx, tenantId)
	if err == nil {
		return existing, nil // Already has subscription
	}

	// Create default freemium
	now := time.Now().UTC()
	sub := &subscripmod.Subscription{
		ID:           primitive.NewObjectID(),
		IDString:     generateSubscriptionId(),
		TenantId:     tenantId,
		PlanId:       "freemium",
		Status:       subscripmod.SubscriptionStatusActive,
		BillingCycle: subscripmod.BillingCycleNone,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	_, err = r.col.InsertOne(ctx, sub)
	if err != nil {
		return nil, err
	}

	return sub, nil
}

// UpdatePlan updates subscription plan and billing cycle
func (r *SubscriptionRepo) UpdatePlan(ctx context.Context, tenantId, planId string, billingCycle subscripmod.BillingCycle) error {
	now := time.Now().UTC()
	update := bson.M{
		"$set": bson.M{
			"planId":       planId,
			"billingCycle": billingCycle,
			"updatedAt":    now,
		},
	}

	if billingCycle != subscripmod.BillingCycleNone {
		update["$set"].(bson.M)["currentPeriodStart"] = now
	}

	_, err := r.col.UpdateOne(ctx, bson.M{"tenantId": tenantId}, update)
	return err
}

// ActivateEnterprise activates enterprise plan with license key
func (r *SubscriptionRepo) ActivateEnterprise(
	ctx context.Context,
	tenantId string,
	licenseKey string,
	licenseKeyHash string,
	limits *subscripmod.SubscriptionLimits,
) error {
	now := time.Now().UTC()

	update := bson.M{
		"$set": bson.M{
			"planId":         "enterprise",
			"billingCycle":   subscripmod.BillingCycleYearly,
			"status":         subscripmod.SubscriptionStatusActive,
			"licenseKeyHash": &licenseKeyHash,
			"updatedAt":      now,
		},
	}

	if limits != nil {
		update["$set"].(bson.M)["overrides"] = limits
	}

	_, err := r.col.UpdateOne(ctx, bson.M{"tenantId": tenantId}, update)
	return err
}

// UpdateOverrides updates subscription limits overrides
func (r *SubscriptionRepo) UpdateOverrides(ctx context.Context, tenantId string, overrides *subscripmod.SubscriptionLimits) error {
	now := time.Now().UTC()
	_, err := r.col.UpdateOne(ctx, bson.M{"tenantId": tenantId}, bson.M{
		"$set": bson.M{
			"overrides": overrides,
			"updatedAt": now,
		},
	})
	return err
}

// generateSubscriptionId generates a unique subscription ID string
func generateSubscriptionId() string {
	return "sub_" + primitive.NewObjectID().Hex()
}
