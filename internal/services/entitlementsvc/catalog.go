// internal/services/entitlementsvc/catalog.go
package entitlementsvc

// RuntimeEntitlementCatalog returns full RuntimeEntitlement shapes keyed by
// plan code or deployment profile. It is intentionally checked in so that the
// service can always synthesize a complete snapshot when Redis has not yet
// received a klynx.entitlement.snapshot.v1 message for a workspace.
//
// Values mirror the commercial plans in models/subscripmod/catalog.go where
// they overlap; fields that only exist on RuntimeEntitlement (source families,
// retention days, export/asset-tracking flags) are set here.
type RuntimeEntitlementCatalog struct{}

// NewRuntimeEntitlementCatalog returns the default checked-in catalog.
func NewRuntimeEntitlementCatalog() *RuntimeEntitlementCatalog {
	return &RuntimeEntitlementCatalog{}
}

// Default returns the RuntimeEntitlement for the requested plan code.
// Falls back to freemium when the plan code is unknown so the caller always
// receives a product-neutral snapshot.
func (c *RuntimeEntitlementCatalog) Default(planCode string) RuntimeEntitlement {
	switch planCode {
	case "pro":
		return proDefault()
	case "enterprise":
		return enterpriseDefault()
	case "appliance":
		return applianceDefault()
	default:
		return freemiumDefault()
	}
}

// ForProfile returns the RuntimeEntitlement default tied to the deployment
// profile. appliance and enterprise share the same unlimited-tier defaults so
// co-located deployments work out of the box even before a platform license
// narrows them.
func (c *RuntimeEntitlementCatalog) ForProfile(profile string) RuntimeEntitlement {
	switch profile {
	case "appliance":
		return applianceDefault()
	case "enterprise":
		return enterpriseDefault()
	default:
		// saasPublic and unknown profiles fall back to freemium until a local
		// subscription overlays narrower or wider limits on top.
		return freemiumDefault()
	}
}

func freemiumDefault() RuntimeEntitlement {
	return RuntimeEntitlement{
		PlanCode:              "freemium",
		MaxEventsPerSecond:    10,
		MaxPayloadBytes:       1 * 1024 * 1024,
		MaxAssets:             100,
		MaxSources:            10,
		MaxPipelines:          5,
		MaxSites:              1,
		AllowedSourceFamilies: []string{"http", "mqtt"},
		RetentionDays:         7,
		WebhookTargetsLimit:   1,
		EventExportEnabled:    false,
		AssetTrackingEnabled:  false,
	}
}

func proDefault() RuntimeEntitlement {
	return RuntimeEntitlement{
		PlanCode:              "pro",
		MaxEventsPerSecond:    100,
		MaxPayloadBytes:       40 * 1024 * 1024,
		MaxAssets:             1000,
		MaxSources:            100,
		MaxPipelines:          25,
		MaxSites:              10,
		AllowedSourceFamilies: []string{"http", "mqtt", "stream"},
		RetentionDays:         30,
		WebhookTargetsLimit:   2,
		EventExportEnabled:    true,
		AssetTrackingEnabled:  true,
	}
}

func enterpriseDefault() RuntimeEntitlement {
	return RuntimeEntitlement{
		PlanCode:              "enterprise",
		MaxEventsPerSecond:    1000,
		MaxPayloadBytes:       100 * 1024 * 1024,
		MaxAssets:             -1,
		MaxSources:            -1,
		MaxPipelines:          -1,
		MaxSites:              -1,
		AllowedSourceFamilies: []string{"*"},
		RetentionDays:         365,
		WebhookTargetsLimit:   3,
		EventExportEnabled:    true,
		AssetTrackingEnabled:  true,
	}
}

// applianceDefault mirrors enterprise — co-located deployments run unlimited by
// default; a platform license (if activated) can narrow these later.
func applianceDefault() RuntimeEntitlement {
	ent := enterpriseDefault()
	ent.PlanCode = "appliance"
	return ent
}
