// internal/services/entitlementsvc/types.go
package entitlementsvc

// RuntimeEntitlement is the phibek-specific runtime enforcement snapshot.
// It is received from klynx-api via klynx.entitlement.snapshot.v1 Kafka topic
// and cached in Redis with a TTL. phibek uses it to gate ingest without
// querying klynx on every event.
//
// This struct is intentionally product-neutral — it contains no reference to
// klynx commercial plan names, billing fields, or seat counts.
type RuntimeEntitlement struct {
	WorkspaceID           string   `json:"workspaceId"`
	PlanCode              string   `json:"planCode"`
	MaxEventsPerSecond    int      `json:"maxEventsPerSecond"`
	MaxPayloadBytes       int      `json:"maxPayloadBytes"`
	MaxAssets             int      `json:"maxAssets"`
	MaxSources            int      `json:"maxSources"`
	MaxPipelines          int      `json:"maxPipelines"`
	MaxSites              int      `json:"maxSites"`
	AllowedSourceFamilies []string `json:"allowedSourceFamilies"`
	RetentionDays         int      `json:"retentionDays"`
	WebhookTargetsLimit   int      `json:"webhookTargetsLimit"`
	EventExportEnabled    bool     `json:"eventExportEnabled"`
	AssetTrackingEnabled  bool     `json:"assetTrackingEnabled"`
}
