// internal/eventschema/normalized.go
package eventschema

import "time"

// NormalizedEvent is the canonical cross-service event contract between phibek and downstream consumers.
// Transport-agnostic: used over Kafka (appliance profile) and webhook (saasPublic profile).
//
// Trace propagation MUST go through transport headers (Kafka message headers or HTTP headers),
// NOT embedded in this struct. TraceID here is for business correlation logging only.
//
// Schema version: "1.0"
type NormalizedEvent struct {
	// --- Envelope (stable, required) ---
	EventID       string    `json:"eventId"`              // deduplication key
	SchemaVersion string    `json:"schemaVersion"`        // "1.0"
	WorkspaceID   string    `json:"workspaceId"`          // phibek workspace (≠ orgId of klynx)
	OrgID         string    `json:"orgId,omitempty"`      // klynx org ref (if mapped)
	SourceType    string    `json:"sourceType"`           // "iot", "analytic", "streamzkt", ...
	SourceFamily  string    `json:"sourceFamily"`
	OccurredAt    time.Time `json:"occurredAt"`           // event timestamp (RFC3339 UTC)
	ReceivedAt    time.Time `json:"receivedAt"`           // phibek ingest timestamp

	// --- Normalized Fields (strongly named) ---
	AssetID  string `json:"assetId,omitempty"`
	SiteID   string `json:"siteId,omitempty"`
	DeviceID string `json:"deviceId,omitempty"`
	CamID    string `json:"camId,omitempty"`

	// --- Payload (raw + mapped) ---
	MappedFields  map[string]any `json:"mappedFields"`
	RawPayloadRef string         `json:"rawPayloadRef,omitempty"` // S3 key

	// --- Metadata ---
	TraceID string `json:"traceId,omitempty"` // business correlation only — not trace context
}
