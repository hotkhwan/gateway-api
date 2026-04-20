// internal/services/subscriptionsvc/packages.go
package subscriptionsvc

import (
	"context"
	"errors"
	"fmt"

	"github.com/hotkhwan/gateway-api/internal/repo/subscriprepo"
	"github.com/hotkhwan/gateway-api/models/authzmod"
	"github.com/hotkhwan/gateway-api/models/subscripmod"
	"github.com/hotkhwan/gateway-api/utils/traceutil"
)

// TargetCounter is the repo subset required for quota enforcement.
// *targetrepo.TargetRepo satisfies this interface.
type TargetCounter interface {
	CountByTypeAndOrg(ctx context.Context, tenantId, orgId, targetType string) (int64, error)
	CountMessageChannelsByOrg(ctx context.Context, tenantId, orgId string) (int64, error)
}

// ListPackages returns all active packages from DB, ordered by sortOrder.
// publicOnly=true filters to isPublic=true only (for pricing page).
func (s *SubscriptionService) ListPackages(ctx context.Context, publicOnly bool) ([]subscripmod.SubscriptionPackage, error) {
	ctx, end, _ := traceutil.StartLite(ctx, "gateway.subscriptionsvc", "ListPackages", "subscriptionsvc", "ListPackages")
	defer end()

	pkgRepo := subscriprepo.NewPackageRepo()
	return pkgRepo.ListActive(ctx, publicOnly)
}

// GetCurrentEffective resolves the tenant's active subscription merged with package limits + overrides.
// This is the authoritative source for quota enforcement.
func (s *SubscriptionService) GetCurrentEffective(ctx context.Context, tenantId string) (*subscripmod.EffectiveSubscription, error) {
	ctx, end, _ := traceutil.StartLite(ctx, "gateway.subscriptionsvc", "GetCurrentEffective", "subscriptionsvc", "GetCurrentEffective")
	defer end()

	sub, err := s.subRepo.FindByTenantId(ctx, tenantId)
	if err != nil {
		// Auto-bootstrap freemium subscription if not found
		sub, err = s.subRepo.UpsertDefaultIfMissing(ctx, tenantId)
		if err != nil {
			return nil, fmt.Errorf("subscription not found and bootstrap failed: %w", err)
		}
	}

	pkgRepo := subscriprepo.NewPackageRepo()
	pkg, err := pkgRepo.FindById(ctx, sub.PlanId)
	if err != nil {
		if errors.Is(err, subscriprepo.ErrPackageNotFound) {
			// Fallback to hardcoded catalog if package not seeded yet
			plan, catErr := s.planCatalog.GetPlan(sub.PlanId)
			if catErr != nil {
				return nil, fmt.Errorf("plan %q not found in DB or catalog: %w", sub.PlanId, err)
			}
			return &subscripmod.EffectiveSubscription{
				TenantId:     tenantId,
				PlanId:       sub.PlanId,
				Status:       string(sub.Status),
				BillingCycle: string(sub.BillingCycle),
				Limits:       *s.mergeLimits(&plan.Limits, sub.Overrides),
			}, nil
		}
		return nil, err
	}

	merged := s.mergeLimits(&pkg.Limits, sub.Overrides)
	return &subscripmod.EffectiveSubscription{
		TenantId:     tenantId,
		PlanId:       sub.PlanId,
		Status:       string(sub.Status),
		BillingCycle: string(sub.BillingCycle),
		Limits:       *merged,
		Features:     pkg.Features,
	}, nil
}

// ValidateDeliveryTargetQuota checks if an org can create another target of the given type.
// Returns ErrDeliveryQuotaExceeded if the plan limit is reached.
// -1 = unlimited.
func (s *SubscriptionService) ValidateDeliveryTargetQuota(
	ctx context.Context,
	tenantId, orgId, targetType string,
	tgtRepo TargetCounter,
) error {
	ctx, end, _ := traceutil.StartLite(ctx, "gateway.subscriptionsvc", "ValidateDeliveryTargetQuota", "subscriptionsvc", "ValidateDeliveryTargetQuota")
	defer end()

	eff, err := s.GetCurrentEffective(ctx, tenantId)
	if err != nil {
		return err
	}
	lim := eff.Limits

	// Resolve per-type limit
	var typeLimit int
	switch targetType {
	case authzmod.TargetTypeWebhook:
		typeLimit = lim.WebhooksPerOrg
	case authzmod.TargetTypeLine:
		typeLimit = lim.LineTargetsPerOrg
	case authzmod.TargetTypeDiscord:
		typeLimit = lim.DiscordTargetsPerOrg
	case authzmod.TargetTypeTelegram:
		typeLimit = lim.TelegramTargetsPerOrg
	default:
		return nil // unknown type — let targetsvc validate separately
	}

	if typeLimit == -1 {
		return nil // unlimited
	}

	// Count existing targets of this type for the org
	count, err := tgtRepo.CountByTypeAndOrg(ctx, tenantId, orgId, targetType)
	if err != nil {
		return err
	}
	if int(count) >= typeLimit {
		return fmt.Errorf("%w: plan allows %d %s target(s) per org", ErrDeliveryQuotaExceeded, typeLimit, targetType)
	}

	// For message channels (line+discord+telegram), also check combined total
	if targetType != authzmod.TargetTypeWebhook && lim.MessageChannelsPerOrg != -1 {
		msgCount, err := tgtRepo.CountMessageChannelsByOrg(ctx, tenantId, orgId)
		if err != nil {
			return err
		}
		if int(msgCount) >= lim.MessageChannelsPerOrg {
			return fmt.Errorf("%w: plan allows %d message channel(s) per org total", ErrDeliveryQuotaExceeded, lim.MessageChannelsPerOrg)
		}
	}

	return nil
}
