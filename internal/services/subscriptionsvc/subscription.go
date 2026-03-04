// internal/services/subscriptionsvc/subscription.go
package subscriptionsvc

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/hotkhwan/gateway-api/config"
	"github.com/hotkhwan/gateway-api/internal/repo/subscriprepo"
	"github.com/hotkhwan/gateway-api/models/subscripmod"

	"github.com/redis/go-redis/v9"
)

// EffectiveLimits represents the actual limits after considering plan + overrides
type EffectiveLimits struct {
	PlanId                    string
	OrgCacheTtlSec            int64
	MaxPayloadBytes           int64
	PerOrgPerSec              int
	PerOrgBurst               int
	PerIpPerMin               int
	StorageQuotaBytes         int64
	MaxOrganizationsPerTenant int
}

type SubscriptionService struct {
	subRepo     *subscriprepo.SubscriptionRepo
	licenseRepo *subscriprepo.LicenseRepo
	planCatalog subscripmod.PlanCatalog
	redis       *redis.Client
}

func NewSubscriptionService(
	subRepo *subscriprepo.SubscriptionRepo,
	licenseRepo *subscriprepo.LicenseRepo,
	redis *redis.Client,
) *SubscriptionService {
	return &SubscriptionService{
		subRepo:     subRepo,
		licenseRepo: licenseRepo,
		planCatalog: subscripmod.NewHardcodedPlanCatalog(),
		redis:       redis,
	}
}

// GetEffectiveLimits returns the effective limits for a tenant
// This is the source of truth - combine plan + overrides
func (s *SubscriptionService) GetEffectiveLimits(
	ctx context.Context,
	tenantId string,
) (*EffectiveLimits, error) {
	// 1. Get tenant subscription
	sub, err := s.subRepo.FindByTenantId(ctx, tenantId)
	if err != nil {
		return nil, fmt.Errorf("failed to get subscription: %w", err)
	}

	// 2. Get plan defaults
	plan, err := s.planCatalog.GetPlan(sub.PlanId)
	if err != nil {
		return nil, fmt.Errorf("failed to get plan %s: %w", sub.PlanId, err)
	}

	// 3. Merge plan limits with overrides
	limits := s.mergeLimits(&plan.Limits, sub.Overrides)

	// 4. Ensure MaxPayloadBytes doesn't exceed Kafka safe limit
	kafkaSafeMax := config.GetKafkaSafeMaxPayloadBytes()
	if limits.MaxPayloadBytes > kafkaSafeMax {
		limits.MaxPayloadBytes = kafkaSafeMax
	}

	return &EffectiveLimits{
		PlanId:                    sub.PlanId,
		OrgCacheTtlSec:            limits.OrgCacheTtlSec,
		MaxPayloadBytes:           limits.MaxPayloadBytes,
		PerOrgPerSec:              limits.PerOrgPerSec,
		PerOrgBurst:               limits.PerOrgBurst,
		PerIpPerMin:               limits.PerIpPerMin,
		StorageQuotaBytes:         limits.StorageQuotaBytes,
		MaxOrganizationsPerTenant: limits.MaxOrganizationsPerTenant,
	}, nil
}

// GetTenantLimitsCached returns effective limits with Redis caching
func (s *SubscriptionService) GetTenantLimitsCached(
	ctx context.Context,
	tenantId string,
) (*EffectiveLimits, error) {
	cacheKey := fmt.Sprintf("subcache:limits:%s", tenantId)

	// Try cache first
	if val, err := s.redis.Get(ctx, cacheKey).Bytes(); err == nil {
		// TODO: Unmarshal and return cached limits
		// For now, bypass cache to implement base functionality first
		_ = val
	}

	// Cache miss - fetch from repo
	limits, err := s.GetEffectiveLimits(ctx, tenantId)
	if err != nil {
		return nil, err
	}

	// Cache the limits (TTL from plan limits)
	ttl := limits.OrgCacheTtlSec
	if ttl <= 0 {
		ttl = 30 // Default 30 seconds
	}

	// TODO: Marshal and cache limits
	_ = ttl

	return limits, nil
}

// mergeLimits combines plan limits with subscription overrides
// Overrides take precedence but cannot exceed plan limits (safety check)
func (s *SubscriptionService) mergeLimits(
	plan *subscripmod.SubscriptionLimits,
	overrides *subscripmod.SubscriptionLimits,
) *subscripmod.SubscriptionLimits {
	if overrides == nil {
		return plan
	}

	// Create a copy of plan limits
	result := *plan

	// Apply overrides
	if overrides.MaxPayloadBytes > 0 {
		// Safety: don't allow overrides to exceed plan significantly
		if overrides.MaxPayloadBytes <= plan.MaxPayloadBytes {
			result.MaxPayloadBytes = overrides.MaxPayloadBytes
		}
	}
	if overrides.PerOrgPerSec > 0 {
		if overrides.PerOrgPerSec <= plan.PerOrgPerSec {
			result.PerOrgPerSec = overrides.PerOrgPerSec
		}
	}
	if overrides.PerOrgBurst > 0 {
		if overrides.PerOrgBurst <= plan.PerOrgBurst {
			result.PerOrgBurst = overrides.PerOrgBurst
		}
	}
	if overrides.PerIpPerMin > 0 {
		if overrides.PerIpPerMin <= plan.PerIpPerMin {
			result.PerIpPerMin = overrides.PerIpPerMin
		}
	}
	if overrides.StorageQuotaBytes > 0 {
		if overrides.StorageQuotaBytes <= plan.StorageQuotaBytes {
			result.StorageQuotaBytes = overrides.StorageQuotaBytes
		}
	}
	if overrides.MaxOrganizationsPerTenant > 0 {
		if overrides.MaxOrganizationsPerTenant <= plan.MaxOrganizationsPerTenant {
			result.MaxOrganizationsPerTenant = overrides.MaxOrganizationsPerTenant
		}
	}

	return &result
}

// BootstrapSubscription creates default freemium if tenant doesn't have one
func (s *SubscriptionService) BootstrapSubscription(ctx context.Context, tenantId string) (*subscripmod.Subscription, error) {
	return s.subRepo.UpsertDefaultIfMissing(ctx, tenantId)
}

// UpdatePlan updates subscription plan
func (s *SubscriptionService) UpdatePlan(
	ctx context.Context,
	tenantId, planId string,
	billingCycle subscripmod.BillingCycle,
) error {
	// Validate plan exists
	_, err := s.planCatalog.GetPlan(planId)
	if err != nil {
		return fmt.Errorf("invalid plan: %w", err)
	}

	return s.subRepo.UpdatePlan(ctx, tenantId, planId, billingCycle)
}

// ActivateEnterprise activates enterprise plan with license key
func (s *SubscriptionService) ActivateEnterprise(
	ctx context.Context,
	tenantId string,
	licenseKey string,
	limits *subscripmod.SubscriptionLimits,
) error {
	// 1. Validate license key exists and is available
	license, err := s.licenseRepo.ValidateLicenseKey(ctx, licenseKey)
	if err != nil {
		return ErrInvalidLicenseKey
	}

	// 2. Use limits from license if not provided in request
	finalLimits := limits
	if finalLimits == nil && license.Limits != nil {
		finalLimits = license.Limits
	}

	// 3. Validate limits against Kafka safe max
	if finalLimits != nil && finalLimits.MaxPayloadBytes > 0 {
		kafkaSafeMax := config.GetKafkaSafeMaxPayloadBytes()
		if finalLimits.MaxPayloadBytes > kafkaSafeMax {
			return fmt.Errorf("maxPayloadBytes %d exceeds Kafka safe limit %d (set KAFKA_MAX_MESSAGE_BYTES to increase)",
				finalLimits.MaxPayloadBytes, kafkaSafeMax)
		}
	}

	// 4. Hash the license key (store hash, not plain key)
	hasher := sha256.New()
	hasher.Write([]byte(licenseKey))
	licenseKeyHash := hex.EncodeToString(hasher.Sum(nil))

	// 5. Mark license as activated
	now := time.Now().UTC()
	if err := s.licenseRepo.MarkLicenseActivated(ctx, license.ID, tenantId, now); err != nil {
		return fmt.Errorf("failed to mark license as activated: %w", err)
	}

	// 6. Activate enterprise for tenant
	return s.subRepo.ActivateEnterprise(ctx, tenantId, licenseKey, licenseKeyHash, finalLimits)
}

// CheckOrganizationLimit checks if tenant can create more organizations
func (s *SubscriptionService) CheckOrganizationLimit(
	ctx context.Context,
	tenantId string,
	currentOrgCount int,
) error {
	limits, err := s.GetEffectiveLimits(ctx, tenantId)
	if err != nil {
		return err
	}

	if currentOrgCount >= limits.MaxOrganizationsPerTenant {
		return ErrOrganizationLimitReached
	}

	return nil
}
