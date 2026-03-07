// internal/kafka/normalizedcons/types.go
package normalizedcons

import (
	"github.com/hotkhwan/gateway-api/internal/geoboundary"
	"github.com/hotkhwan/gateway-api/internal/repo/dlqrepo"
	"github.com/hotkhwan/gateway-api/internal/repo/ingestdetailsrepo"
	"github.com/hotkhwan/gateway-api/internal/repo/ingestrepo"
	"github.com/rs/zerolog"
)

// ConsumerDeps holds all dependencies injected into the normalizer consumer.
type ConsumerDeps struct {
	EventDetailsRepo *ingestdetailsrepo.EventDetailsRepo
	TemplateRepo     *ingestrepo.MappingTemplateRepo
	DLQRepo          *dlqrepo.DLQRepo
	GeoCfg           GeoConfig
	Logger           zerolog.Logger
	// S3BucketKey is the bucket key used for binary field uploads (empty = skip S3)
	S3BucketKey string
}

// GeoConfig controls geo enrichment behaviour in the normalizer.
type GeoConfig struct {
	DefaultCountry string                    // "TH"
	AdminLevel     int                       // 1 = province
	IdScheme       string                    // "ISO_3166_2"
	GeoCellScheme  string                    // "geohash"
	GeoCellPrec    int                       // 5 = ~5km²
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
