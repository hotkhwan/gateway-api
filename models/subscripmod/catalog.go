// models/subscripmod/catalog.go
package subscripmod

import (
	"errors"
	"time"
)

var (
	ErrPlanNotFound = errors.New("plan not found")
)

// PlanCatalog provides hardcoded plan defaults (alternative to MongoDB plans)
// This acts as a single source of truth for plan configurations
type PlanCatalog interface {
	GetPlan(planId string) (*SubscriptionPlan, error)
	ListPlans() []SubscriptionPlan
}

// HardcodedPlanCatalog implements PlanCatalog with hardcoded defaults
type HardcodedPlanCatalog struct{}

func NewHardcodedPlanCatalog() *HardcodedPlanCatalog {
	return &HardcodedPlanCatalog{}
}

// GetPlan returns a plan by ID
func (c *HardcodedPlanCatalog) GetPlan(planId string) (*SubscriptionPlan, error) {
	for _, plan := range c.ListPlans() {
		if plan.PlanId == planId {
			return &plan, nil
		}
	}
	return nil, ErrPlanNotFound
}

// ListPlans returns all available plans
func (c *HardcodedPlanCatalog) ListPlans() []SubscriptionPlan {
	now := time.Now()
	return []SubscriptionPlan{
		{
			PlanId: "freemium",
			Name:   "Freemium",
			Limits: SubscriptionLimits{
				OrgCacheTtlSec:            30,
				MaxPayloadBytes:           1 * 1024 * 1024, // 1MB
				PerOrgPerSec:              10,
				PerOrgBurst:               20,
				PerIpPerMin:               300,
				StorageQuotaBytes:         10 * 1024 * 1024 * 1024, // 10GB
				MaxOrganizationsPerTenant: 2,
			},
			IsPublic:  true,
			CreatedAt: now,
			UpdatedAt: now,
		},
		{
			PlanId: "pro",
			Name:   "Pro",
			Limits: SubscriptionLimits{
				OrgCacheTtlSec:            90,
				MaxPayloadBytes:           40 * 1024 * 1024, // 40MB
				PerOrgPerSec:              100,
				PerOrgBurst:               200,
				PerIpPerMin:               3000,
				StorageQuotaBytes:         100 * 1024 * 1024 * 1024, // 100GB
				MaxOrganizationsPerTenant: 5,
			},
			IsPublic:  true,
			CreatedAt: now,
			UpdatedAt: now,
		},
		{
			PlanId: "enterprise",
			Name:   "Enterprise",
			Limits: SubscriptionLimits{
				OrgCacheTtlSec:            120,
				MaxPayloadBytes:           100 * 1024 * 1024, // 100MB (can be overridden)
				PerOrgPerSec:              1000,
				PerOrgBurst:               2000,
				PerIpPerMin:               10000,
				StorageQuotaBytes:         1024 * 1024 * 1024 * 1024, // 1TB
				MaxOrganizationsPerTenant: 10,
			},
			IsPublic:  false, // Enterprise requires license key
			CreatedAt: now,
			UpdatedAt: now,
		},
	}
}
