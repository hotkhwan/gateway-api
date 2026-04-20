// models/ingestmod/normalization.go
package ingestmod

import "time"

// CanonicalEvent — raw event published to Kafka raw.events after approval gate.
// Produced by IngestService (approval path); consumed by the Normalizer consumer.
// This is the INPUT to the normalizer — NOT the final stored shape.
type CanonicalEvent struct {
	EventId      string         `json:"eventId"              bson:"eventId"`
	TenantId     string         `json:"tenantId"             bson:"tenantId"`
	SourceFamily string         `json:"sourceFamily"         bson:"sourceFamily"`
	EventType    string         `json:"eventType"            bson:"eventType"`
	OccurredAt   time.Time      `json:"occurredAt"           bson:"occurredAt"`
	Source       SourceInfo     `json:"source"               bson:"source"`
	Location     LocationInfo   `json:"location"             bson:"location"`
	Payload      map[string]any `json:"payload"              bson:"payload"`
	TemplateId   string         `json:"templateId,omitempty" bson:"templateId,omitempty"`
	CreatedAt    time.Time      `json:"createdAt"            bson:"createdAt"`
}

// SourceInfo identifies the originating device.
type SourceInfo struct {
	DeviceId          string `json:"deviceId"                     bson:"deviceId"`
	DeviceType        string `json:"deviceType"                   bson:"deviceType"`
	DeviceName        string `json:"deviceName,omitempty"         bson:"deviceName,omitempty"`
	DeviceDescription string `json:"deviceDescription,omitempty"  bson:"deviceDescription,omitempty"`
	SubType           string `json:"subType,omitempty"            bson:"subType,omitempty"`
	Vendor            string `json:"vendor,omitempty"             bson:"vendor,omitempty"`
	Protocol          string `json:"protocol,omitempty"           bson:"protocol,omitempty"`
	WorkspaceId        string `json:"workspaceId"                  bson:"workspaceId"`
}

// LocationInfo holds the resolved geographic coordinates.
// Values derived from fieldMappings targeting location.lat / location.lng.
type LocationInfo struct {
	Lat  float64 `json:"lat"            bson:"lat"`
	Lng  float64 `json:"lng"            bson:"lng"`
	Site string  `json:"site,omitempty" bson:"site,omitempty"`
	Zone string  `json:"zone,omitempty" bson:"zone,omitempty"`
}

// NormalizedEvent — final stored shape in event_details (MongoDB) and
// published to Kafka normalized.events after geo enrichment.
// Normalizer computes Geo + GeoCell from location.lat / location.lng.
type NormalizedEvent struct {
	EventId       string    `json:"eventId"                    bson:"eventId"`
	TenantId      string    `json:"tenantId"                   bson:"tenantId"`
	EventType     string    `json:"eventType"                  bson:"eventType"`
	EventCategory string    `json:"eventCategory,omitempty"    bson:"eventCategory,omitempty"`
	EventAction   string    `json:"eventAction,omitempty"      bson:"eventAction,omitempty"`
	EventClass    string    `json:"eventClass,omitempty"       bson:"eventClass,omitempty"`
	EventSeverity string    `json:"eventSeverity,omitempty"    bson:"eventSeverity,omitempty"`
	OccurredAt    time.Time `json:"occurredAt"                 bson:"occurredAt"`

	Source    SourceInfo    `json:"source"    bson:"source"`
	Ownership OwnershipInfo `json:"ownership" bson:"ownership"`
	Location  LocationInfo  `json:"location"  bson:"location"`

	// Geo enrichment — computed by Normalizer from location.lat / location.lng
	Geo         GeoInfo         `json:"geo"         bson:"geo"`
	GeoCell     GeoCellInfo     `json:"geoCell"     bson:"geoCell"`
	ByAdminArea ByAdminAreaInfo `json:"byAdminArea" bson:"byAdminArea"`

	Payload    map[string]any `json:"payload"    bson:"payload"`
	BinaryRefs []BinaryRef    `json:"binaryRefs" bson:"binaryRefs"`

	Meta NormalizationMeta `json:"meta" bson:"meta"`
}

// OwnershipInfo identifies the downstream service that owns/subscribes to this event.
type OwnershipInfo struct {
	OwnerService string `json:"ownerService,omitempty" bson:"ownerService,omitempty"`
}

// GeoInfo — reverse-geocoded administrative region from lat/lng.
// Computed by Normalizer using embedded GeoJSON boundary dataset.
type GeoInfo struct {
	CountryCode string `json:"countryCode" bson:"countryCode"` // ISO 3166-1 alpha-2, e.g. "TH"
	AdminLevel  int    `json:"adminLevel"  bson:"adminLevel"`  // 1 = province, 2 = district
	AdminName   string `json:"adminName"   bson:"adminName"`   // e.g. "Bangkok", "Chiang Mai"
	AdminCode   string `json:"adminCode"   bson:"adminCode"`   // ISO 3166-2, e.g. "TH-10"
	IdScheme    string `json:"idScheme"    bson:"idScheme"`    // "ISO_3166_2"
}

// GeoCellInfo — discrete spatial cell for aggregation queries (heat maps, area summaries).
type GeoCellInfo struct {
	Scheme    string `json:"scheme"    bson:"scheme"`    // "geohash" | "h3" | "s2"
	Precision int    `json:"precision" bson:"precision"` // geohash: 1-12; h3: 0-15
	Cell      string `json:"cell"      bson:"cell"`      // e.g. geohash "w3gv2"
}

// ByAdminAreaInfo — index keys for fast admin-area dashboard aggregation.
// Key = admin code (e.g. "TH-10"), value = reserved for downstream aggregator.
type ByAdminAreaInfo map[string]any

// BinaryRef — pointer to a binary object extracted from rawBody and stored in S3.
type BinaryRef struct {
	ObjectId    string `json:"objectId"    bson:"objectId"`    // S3 key (includes extension: .jpg, .mp4, …)
	Bucket      string `json:"bucket"      bson:"bucket"`      // S3 bucket name
	ContentType string `json:"contentType" bson:"contentType"` // "image/jpeg" | "video/mp4" | …
	FieldName   string `json:"fieldName"   bson:"fieldName"`   // source field in rawBody
	Kind        string `json:"kind"        bson:"kind"`        // "image" | "video" | "binary"
	Role        string `json:"role"        bson:"role"`        // "full" | "snapshot" | "thumbnail" | "clip" | "capture"
	SourceIndex int    `json:"sourceIndex" bson:"sourceIndex"` // position in source array (0 for scalar fields)
}

// NormalizationMeta — audit and tracing metadata added by Normalizer.
type NormalizationMeta struct {
	SchemaVersion string    `json:"schemaVersion"        bson:"schemaVersion"`
	TraceId       string    `json:"traceId"              bson:"traceId"`
	TemplateId    string    `json:"templateId,omitempty" bson:"templateId,omitempty"`
	NormalizedAt  time.Time `json:"normalizedAt"         bson:"normalizedAt"`
}
