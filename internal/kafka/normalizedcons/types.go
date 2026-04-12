// internal/kafka/normalizedcons/types.go
package normalizedcons

import (
	"context"

	"github.com/hotkhwan/gateway-api/internal/eventschema"
	"github.com/hotkhwan/gateway-api/internal/geoboundary"
	"github.com/hotkhwan/gateway-api/internal/repo/dlqrepo"
	"github.com/hotkhwan/gateway-api/internal/repo/ingestrepo"
	"github.com/hotkhwan/gateway-api/models/authzmod"
	"github.com/hotkhwan/gateway-api/models/ingestmod"
	"github.com/rs/zerolog"
)

// eventDetailsRepoI is the repo subset used by the normalizer consumer.
// *ingestdetailsrepo.EventDetailsRepo satisfies this interface.
type eventDetailsRepoI interface {
	Upsert(ctx context.Context, event *ingestmod.NormalizedEvent) error
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

// ConsumerDeps holds all dependencies injected into the normalizer consumer.
type ConsumerDeps struct {
	EventDetailsRepo eventDetailsRepoI // *ingestdetailsrepo.EventDetailsRepo satisfies this
	TemplateRepo     *ingestrepo.MappingTemplateRepo
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
