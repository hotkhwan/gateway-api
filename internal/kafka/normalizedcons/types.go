// internal/kafka/normalizedcons/types.go
package normalizedcons

import (
	"context"

	"github.com/hotkhwan/gateway-api/internal/eventschema"
	"github.com/hotkhwan/gateway-api/internal/geoboundary"
	"github.com/hotkhwan/gateway-api/internal/repo/dlqrepo"
	"github.com/hotkhwan/gateway-api/models/authzmod"
	"github.com/hotkhwan/gateway-api/models/ingestmod"
	"github.com/rs/zerolog"
)

// eventDetailsRepoI is the repo subset used by the normalizer consumer.
// *ingestdetailsrepo.EventDetailsRepo satisfies this interface.
type eventDetailsRepoI interface {
	Upsert(ctx context.Context, event *ingestmod.NormalizedEvent) error
}

// templateRepoI is the MappingTemplateRepo subset used by the normalizer
// consumer. *ingestrepo.MappingTemplateRepo satisfies this interface.
// Introduced for Phase 1.0 so producer-side classification can be regression-
// tested against a stubbed template without spinning up Mongo. The production
// matcher (ingestsvc.NewTemplateMatcher) still requires the concrete repo,
// so applyTemplate falls back to "no field mapping" when given a stub —
// classification still runs because it only needs the ClassificationRules
// slice on the returned *ingestmod.MappingTemplate.
type templateRepoI interface {
	FindById(ctx context.Context, orgId, templateId string) (*ingestmod.MappingTemplate, error)
}

// EntitlementChecker gates ingest based on workspace quota.
// Defined locally to avoid circular import with entitlementsvc.
type EntitlementChecker interface {
	CheckIngestAllowed(ctx context.Context, workspaceId string, payloadBytes int) error
}

// IngestAuthzChecker gates ingest based on Permify workspace permissions.
// Defined locally to avoid circular import with authzgw.
type IngestAuthzChecker interface {
	CanIngest(ctx context.Context, workspaceId, sourceId string) (bool, error)
}

// EventBridgePublisher forwards normalized events to klynx-api (appliance only).
// Defined locally to avoid circular import with the eventbridge package.
type EventBridgePublisher interface {
	Publish(ctx context.Context, event eventschema.NormalizedEvent) error
}

// CanonicalNotifier sends the MQTT canonical-notify after a NormalizedEvent is written.
// Defined locally to keep the consumer independent of the inframsg/alertmsg packages.
// Nil = skip MQTT notify (e.g. in tests or when MQTT is not configured).
type CanonicalNotifier interface {
	PublishCanonicalNotify(workspaceID, sourceFamily, eventID string) error
}

// BindingQuerier looks up normalize-stage delivery bindings for a workspace.
// Defined locally to avoid circular import with bindingsvc.
type BindingQuerier interface {
	GetNormalizeBindings(ctx context.Context, workspaceId string) ([]ingestmod.TemplateDeliveryBinding, error)
}

// KlynxTargetChecker reports whether a workspace has at least one enabled klynx-mode target.
// Defined locally to avoid circular import with targetrepo.
type KlynxTargetChecker interface {
	HasKlynxTarget(ctx context.Context, orgId string) (bool, error)
}

// TargetLookup fetches a delivery target by ID + org for normalize-stage binding dispatch.
// Defined locally to avoid circular import with targetrepo.
type TargetLookup interface {
	FindByIDAndOrg(ctx context.Context, targetId, tenantId, orgId string) (*authzmod.DeliveryTarget, error)
}

// KlynxOrgLookup resolves the klynx orgId from a gateway workspaceId.
// Used to populate OrgID in EventBridge events so klynx consumers can route by orgId.
// Nil = OrgID falls back to workspaceId.
type KlynxOrgLookup interface {
	GetKlynxOrgID(ctx context.Context, workspaceId string) (string, error)
}

// DeviceMgmtResolver resolves device_management enrichment (e.g. deviceMgmtId UUID) for an ingest event.
// Nil = skip enrichment (deviceMgmtId omitted from normalized event).
type DeviceMgmtResolver interface {
	Resolve(ctx context.Context, tenantId, orgId, sourceFamily, entityType, entityId string) *ingestmod.DeviceManagement
}

// ConsumerDeps holds all dependencies injected into the normalizer consumer.
type ConsumerDeps struct {
	EventDetailsRepo eventDetailsRepoI // *ingestdetailsrepo.EventDetailsRepo satisfies this
	TemplateRepo     templateRepoI    // *ingestrepo.MappingTemplateRepo satisfies this
	DLQRepo          *dlqrepo.DLQRepo
	GeoCfg           GeoConfig
	Logger           zerolog.Logger
	// S3BucketKey is the bucket key used for binary field uploads (empty = skip S3)
	S3BucketKey string

	// Optional gates — both are non-fatal (log + continue) during parallel mode.
	EntitlementSvc EntitlementChecker
	IngestAuthzGw  IngestAuthzChecker
	// EventBridgePub forwards to klynx-api via Kafka (appliance profile only).
	// Nil = disabled (saasPublic profile or not yet wired).
	EventBridgePub EventBridgePublisher

	// CanonicalNotifier sends MQTT canonical-notify after event is written (non-fatal).
	// Nil = skip MQTT notify.
	CanonicalNotifier CanonicalNotifier
	// BindingQuerier looks up normalize-stage bindings for webhook dispatch.
	// Nil = normalize-stage binding dispatch disabled.
	BindingQuerier BindingQuerier
	// KlynxTargetChecker gates EventBridge publish to workspaces with a klynx target.
	// Nil = skip check (publish unconditionally when EventBridgePub is set).
	KlynxTargetChecker KlynxTargetChecker
	// TargetLookup fetches a delivery target by ID + org for normalize-stage binding dispatch.
	// Nil = binding dispatch skipped even when BindingQuerier is set.
	TargetLookup TargetLookup
	// KlynxOrgLookup resolves klynx orgId from workspaceId for EventBridge event routing.
	// Nil = OrgID falls back to workspaceId.
	KlynxOrgLookup KlynxOrgLookup
	// DeviceMgmtResolver enriches normalized events with deviceMgmtId UUID.
	// Nil = deviceMgmtId omitted from event payload.
	DeviceMgmtResolver DeviceMgmtResolver
}

// GeoConfig controls geo enrichment behaviour in the normalizer.
type GeoConfig struct {
	DefaultCountry string // "TH"
	AdminLevel     int    // 1 = province
	IdScheme       string // "ISO_3166_2"
	GeoCellScheme  string // "geohash"
	GeoCellPrec    int    // 5 = ~5km²
	BoundaryIndex  geoboundary.BoundaryQuerier
}

// DefaultGeoConfig returns a GeoConfig populated from ENV / sensible defaults.
func DefaultGeoConfig() GeoConfig {
	return GeoConfig{
		DefaultCountry: "TH",
		AdminLevel:     1,
		IdScheme:       "ISO_3166_2",
		GeoCellScheme:  "geohash",
		GeoCellPrec:    5,
		BoundaryIndex:  geoboundary.LoadEmbedded(),
	}
}

// ============================================================
// Production CanonicalNotifier — wraps alertmsg
// ============================================================

// alertmsgNotifier is the production CanonicalNotifier that delegates to alertmsg.
type alertmsgNotifier struct{}

// NewAlertmsgNotifier returns the production CanonicalNotifier for use in container wiring.
func NewAlertmsgNotifier() CanonicalNotifier { return &alertmsgNotifier{} }
