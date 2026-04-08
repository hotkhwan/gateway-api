// internal/kafka/normalizedcons/types.go
package normalizedcons

import (
	"context"

	"github.com/hotkhwan/gateway-api/internal/eventschema"
	"github.com/hotkhwan/gateway-api/internal/geoboundary"
	"github.com/hotkhwan/gateway-api/internal/repo/dlqrepo"
	"github.com/hotkhwan/gateway-api/internal/repo/ingestdetailsrepo"
	"github.com/hotkhwan/gateway-api/internal/repo/ingestrepo"
	"github.com/rs/zerolog"
)

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

// ConsumerDeps holds all dependencies injected into the normalizer consumer.
type ConsumerDeps struct {
	EventDetailsRepo *ingestdetailsrepo.EventDetailsRepo
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
