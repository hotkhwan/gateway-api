// internal/eventschema/normalized.go
package eventschema

import "time"

// NormalizedEvent is the canonical cross-service event forwarded to klynx-api.
// Transport-agnostic: used over Kafka (appliance profile) and HTTPS webhook (saasPublic profile).
//
// Schema version: "1.0"
// IMPORTANT: This struct is a copy-by-convention mirror of klynx-api's
// internal/eventbridge/types.go NormalizedEvent.
// Any field add/rename/remove MUST be PR'd on BOTH repos and deployed together.
//
// Trace propagation MUST go through transport headers (Kafka message headers or HTTP headers),
// NOT embedded in this struct. TraceID is for business correlation logging only.
type NormalizedEvent struct {
	// --- Envelope ---
	EventID        string    `json:"eventId"`
	SchemaVersion  string    `json:"schemaVersion"`
	WorkspaceID    string    `json:"workspaceId"`
	OrgID          string    `json:"orgId,omitempty"`
	SourceType     string    `json:"sourceType"`
	SourceCategory string    `json:"sourceCategory,omitempty"`
	SourceAction   string    `json:"sourceAction,omitempty"`
	SourceFamily   string    `json:"sourceFamily"`
	OccurredAt     time.Time `json:"occurredAt"`
	ReceivedAt     time.Time `json:"receivedAt"`

	// --- Device identity (grouped) ---
	Source *NormalizedSource `json:"source,omitempty"`

	// --- Geo enrichment ---
	Location    *NormalizedLocation `json:"location,omitempty"`
	Geo         *NormalizedGeo      `json:"geo,omitempty"`
	GeoCell     *NormalizedGeoCell  `json:"geoCell,omitempty"`
	ByAdminArea map[string]any      `json:"byAdminArea,omitempty"`

	// --- Payload and binaries ---
	Payload       map[string]any        `json:"payload"`
	BinaryRefs    []NormalizedBinaryRef `json:"binaryRefs,omitempty"`
	RawPayloadRef string                `json:"rawPayloadRef,omitempty"`

	TraceID string `json:"traceId,omitempty"`
}

// NormalizedSource identifies the originating device and its context.
type NormalizedSource struct {
	WorkspaceID  string `json:"workspaceId,omitempty"`
	OrgID        string `json:"orgId,omitempty"`
	SourceType   string `json:"sourceType,omitempty"`
	SourceFamily string `json:"sourceFamily,omitempty"`
	DeviceID     string `json:"deviceId,omitempty"`
	DeviceMgmtID string `json:"deviceMgmtId,omitempty"`
	SN           string `json:"sn,omitempty"`       // device serial number
	EdgeName     string `json:"edgeName,omitempty"` // edge device name
}

// NormalizedLocation holds the physical location of the originating device.
type NormalizedLocation struct {
	Lat  float64 `json:"lat,omitempty"`
	Lng  float64 `json:"lng,omitempty"`
	Site string  `json:"site,omitempty"`
	Zone string  `json:"zone,omitempty"`
}

// NormalizedGeo is the reverse-geocoded administrative region from lat/lng.
type NormalizedGeo struct {
	CountryCode string `json:"countryCode,omitempty"`
	AdminLevel  int    `json:"adminLevel,omitempty"`
	AdminName   string `json:"adminName,omitempty"`
	AdminCode   string `json:"adminCode,omitempty"`
	IdScheme    string `json:"idScheme,omitempty"`
}

// NormalizedGeoCell is a discrete spatial cell for aggregation queries (heat maps, area summaries).
type NormalizedGeoCell struct {
	Cell      string `json:"cell,omitempty"`
	Scheme    string `json:"scheme,omitempty"`    // "geohash" | "h3" | "s2"
	Precision int    `json:"precision,omitempty"`
}

// NormalizedBinaryRef is a pointer to a binary object stored in S3.
type NormalizedBinaryRef struct {
	ObjectID    string `json:"objectId"`
	Bucket      string `json:"bucket,omitempty"`
	ContentType string `json:"contentType,omitempty"`
	Kind        string `json:"kind,omitempty"`  // "image" | "video" | "binary"
	Role        string `json:"role,omitempty"`  // "full" | "snapshot" | "thumbnail" | "clip" | "capture"
	SourceIndex int    `json:"sourceIndex"`
}
