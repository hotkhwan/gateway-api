// models/subscripmod/subscription.go
package subscripmod

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// BillingCycle defines subscription billing cycle
type BillingCycle string

const (
	BillingCycleNone      BillingCycle = "none"
	BillingCycleMonthly   BillingCycle = "monthly"
	BillingCycleQuarterly BillingCycle = "quarterly"
	BillingCycleYearly    BillingCycle = "yearly"
)

// SubscriptionStatus defines subscription status
type SubscriptionStatus string

const (
	SubscriptionStatusActive    SubscriptionStatus = "active"
	SubscriptionStatusInactive  SubscriptionStatus = "inactive"
	SubscriptionStatusSuspended SubscriptionStatus = "suspended"
	SubscriptionStatusExpired   SubscriptionStatus = "expired"
)

// SubscriptionLimits defines rate and quota limits
type SubscriptionLimits struct {
	OrgCacheTtlSec            int64 `bson:"orgCacheTtlSec" json:"orgCacheTtlSec"`
	MaxPayloadBytes           int64 `bson:"maxPayloadBytes" json:"maxPayloadBytes"`
	PerOrgPerSec              int   `bson:"perOrgPerSec" json:"perOrgPerSec"`
	PerOrgBurst               int   `bson:"perOrgBurst" json:"perOrgBurst"`
	PerIpPerMin               int   `bson:"perIpPerMin" json:"perIpPerMin"`
	StorageQuotaBytes         int64 `bson:"storageQuotaBytes" json:"storageQuotaBytes"`
	MaxOrganizationsPerTenant int   `bson:"maxOrganizationsPerTenant" json:"maxOrganizationsPerTenant"`
}

// Subscription represents a tenant's subscription
type Subscription struct {
	ID       primitive.ObjectID `bson:"_id,omitempty"`
	IDString string             `bson:"id,omitempty" json:"id"` // external stable id

	TenantId     string             `bson:"tenantId"`
	PlanId       string             `bson:"planId"`
	Status       SubscriptionStatus `bson:"status"`
	BillingCycle BillingCycle       `bson:"billingCycle"`

	// Period for billing (null for lifetime plans)
	CurrentPeriodStart *time.Time `bson:"currentPeriodStart,omitempty"`
	CurrentPeriodEnd   *time.Time `bson:"currentPeriodEnd,omitempty"`

	// Enterprise license activation
	LicenseKeyHash *string `bson:"licenseKeyHash,omitempty"`

	// Overrides allow customizing limits per subscription
	Overrides *SubscriptionLimits `bson:"overrides,omitempty"`

	CreatedAt time.Time `bson:"createdAt"`
	UpdatedAt time.Time `bson:"updatedAt"`
}

// UpdatePlanRequest is the request body for updating subscription plan
type UpdatePlanRequest struct {
	PlanId       string       `json:"planId" example:"premium"`
	BillingCycle BillingCycle `json:"billingCycle" example:"monthly"`
}

// ActivateEnterpriseRequest is the request body for activating enterprise plan
type ActivateEnterpriseRequest struct {
	LicenseKey string              `json:"licenseKey" example:"ENT-XXXX-XXXX-XXXX"`
	Limits     *SubscriptionLimits `json:"limits,omitempty"`
}
