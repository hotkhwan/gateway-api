// models/subscripmod/plan.go
package subscripmod

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// SubscriptionPlan represents a plan template
type SubscriptionPlan struct {
	ID        primitive.ObjectID `bson:"_id,omitempty"`
	PlanId    string             `bson:"id"`
	Name      string             `bson:"name"`
	Limits    SubscriptionLimits `bson:"limits"`
	IsPublic  bool               `bson:"isPublic"` // if true, available for self-upgrade
	CreatedAt time.Time          `bson:"createdAt"`
	UpdatedAt time.Time          `bson:"updatedAt"`
}
