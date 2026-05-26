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
// See klynx-api/docs/contracts/event-severity-forwarding.md for the
// severity + eventClass rollout (Layer C — klynx-api 4.51.0 ships the
// consumer-side mirror + projection; this PR ships the producer side).
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

	// --- Classification (Layer C — klynx-api docs/contracts/event-severity-forwarding.md) ---
	// Severity is admin-classified per ClassificationRule on the matched
	// MappingTemplate. Canonical vocab: high | medium | low | info | none
	// (pass-through — producer may emit any string; klynx FE Phase 2 maps
	// unknown to "none" gray badge). Carried on the wire so klynx consumer
	// projects to event_refs.severity without re-running classification rules.
	//
	// Defaults to "none" when no rule matches (set by classification.Apply
	// in the normalizer producer hot path — see
	// internal/kafka/normalizedcons/consumer.go step 6a). The omitempty tag
	// is retained for backwards-compatibility with pre-Phase-1.0 wire bytes
	// only — post-Phase-1.0 every emitted event carries a non-empty value.
	Severity string `json:"severity,omitempty"`
	// EventClass is admin-classified category per ClassificationRule.
	// Free-form (admin-configurable). Phase 2 dashboard donut groups by
	// this field on the klynx side. Defaults to "unknown" when no rule
	// matches (same producer hot path as Severity above); omitempty retained
	// for backwards-compatibility with pre-Phase-1.0 wire bytes only.
	EventClass string `json:"eventClass,omitempty"`

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

	// TemplateID is the matching MappingTemplate.templateId from gateway-api ingest.
	// Forwarded so klynx-api delivery consumer can resolve deliveryTargets / messageTemplates.
	// Empty when ingest matched no template (event was suggestion-applied or pending).
	TemplateID string `json:"templateId,omitempty"`

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
