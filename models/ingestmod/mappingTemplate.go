// models/ingestmod/mappingTemplate.go
package ingestmod

import "time"

// DLQConfig — per-template DLQ behaviour.
// Controls whether failed events for this template go to the dead-letter queue,
// how many times to retry, and how long to wait between retries.
type DLQConfig struct {
	Enabled             bool `json:"enabled"             bson:"enabled"`             // false = skip DLQ entirely for this template
	MaxRetries          int  `json:"maxRetries"          bson:"maxRetries"`          // max retry attempts before abandoning (default 3)
	RetryTimeoutSeconds int  `json:"retryTimeoutSeconds" bson:"retryTimeoutSeconds"` // seconds between retry attempts (default 60)
}

// PayloadCondition — one filter predicate applied to a normalized event field.
// Only "eq" and "in" operators are supported for now.
//
// Field path resolution (see internal/services/classification and
// klynx-api/docs/contracts/template-classification-rules.md §5A.3):
//   - "payload.<key>"  recommended convention; resolves to event.Payload[<key>]
//   - "<key>"          legacy bare path; also resolves to event.Payload[<key>]
//   - "source.*" / "event.*" / "meta.*" are reserved roots rejected at PATCH-time
type PayloadCondition struct {
	Field    string   `json:"field"    bson:"field"`    // e.g. "payload.listType" or "listType"
	Operator string   `json:"operator" bson:"operator"` // "eq" | "in"
	Values   []string `json:"values"   bson:"values"`   // list of raw values to match
}

// ClassificationSet — canonical fields set by a classification rule.
type ClassificationSet struct {
	EventClass    string `json:"eventClass,omitempty"    bson:"eventClass,omitempty"`
	EventSeverity string `json:"eventSeverity,omitempty" bson:"eventSeverity,omitempty"`
}

// ClassificationRule — derives canonical eventClass/eventSeverity from payload fields.
// Evaluated in Order (ascending); first matching rule wins.
// If no rule matches, defaults are eventClass="unknown", eventSeverity="none".
type ClassificationRule struct {
	Name  string             `json:"name"            bson:"name"`
	When  []PayloadCondition `json:"when"            bson:"when"`
	Set   ClassificationSet  `json:"set"             bson:"set"`
	Order int                `json:"order,omitempty" bson:"order,omitempty"`
}

// TemplateDeliveryTarget — binds a DeliveryTarget to a template with optional filters.
//
// Filter: payload field conditions (AND logic). Empty = pass all.
// EventClasses: whitelist of eventClass values this target accepts. Empty = accept all.
// EventSeverities: whitelist of eventSeverity values this target accepts. Empty = accept all.
// MessageTemplateKey: key to select a MessageTemplate for message channels (line/discord/telegram).
// Webhook targets ignore this field and send raw JSON.
//
// After Wide slice Phase-E, webhook body = raw gw.events.normalized.v1 payload.
// The schemaVersion field added by narrow-v2 has been removed (contract §5.5).
type TemplateDeliveryTarget struct {
	TargetId           string             `json:"targetId"                      bson:"targetId"`
	Filter             []PayloadCondition `json:"filter,omitempty"              bson:"filter,omitempty"`
	EventClasses       []string           `json:"eventClasses,omitempty"        bson:"eventClasses,omitempty"`
	EventSeverities    []string           `json:"eventSeverities,omitempty"     bson:"eventSeverities,omitempty"`
	MessageTemplateKey string             `json:"messageTemplateKey,omitempty"  bson:"messageTemplateKey,omitempty"`
}

// MessageTemplate — locale-aware notification text for a specific channel type.
// channelType: "line" | "discord" | "telegram"
// Go text/template syntax; render context includes .eventId, .eventType, .eventClass, .eventSeverity, .occurredAt, .payload.*, .source.*
// Fallback chain: TargetConfig.Locale → MappingTemplate.DefaultLocale → "en" → minimal default
//
// Key: optional unique identifier within a template for messageTemplateKey selection.
type MessageTemplate struct {
	Key         string            `json:"key,omitempty"  bson:"key,omitempty"`
	ChannelType string            `json:"channelType"    bson:"channelType"`
	Locale      string            `json:"locale"         bson:"locale"`
	Title       string            `json:"title"          bson:"title"`
	Body        string            `json:"body"           bson:"body"`
	Extras      map[string]string `json:"extras,omitempty" bson:"extras,omitempty"`
}

// MappingTemplate — collection: mapping_templates
// Defines how to map rawBody fields to canonical targets for a given device/event signature.
type MappingTemplate struct {
	TemplateId          string                   `json:"templateId"                        bson:"templateId"`
	WorkspaceId         string                   `json:"workspaceId"                       bson:"workspaceId"`
	Enabled             bool                     `json:"enabled"                           bson:"enabled"`
	SourceFamily        string                   `json:"sourceFamily,omitempty"            bson:"sourceFamily,omitempty"`
	FinalEventType      string                   `json:"finalEventType,omitempty"          bson:"finalEventType,omitempty"`
	Name                string                   `json:"name"                              bson:"name"`
	Match               MatchRule                `json:"match"                             bson:"match"`
	MatchAll            []MatchCondition         `json:"matchAll,omitempty"                bson:"matchAll,omitempty"`
	MatchAny            []MatchCondition         `json:"matchAny,omitempty"                bson:"matchAny,omitempty"`
	DeliveryMatchAll    []MatchCondition         `json:"deliveryMatchAll,omitempty"        bson:"deliveryMatchAll,omitempty"`
	DeliveryMatchAny    []MatchCondition         `json:"deliveryMatchAny,omitempty"        bson:"deliveryMatchAny,omitempty"`
	Priority            int                      `json:"priority,omitempty"                bson:"priority,omitempty"`
	Mappings            []FieldMapping           `json:"mappings"                          bson:"mappings"`
	DLQ                 DLQConfig                `json:"dlq"                               bson:"dlq"`
	DefaultLocale       string                   `json:"defaultLocale,omitempty"           bson:"defaultLocale,omitempty"`
	MessageTemplates    []MessageTemplate        `json:"messageTemplates,omitempty"        bson:"messageTemplates,omitempty"`
	ClassificationRules []ClassificationRule     `json:"classificationRules,omitempty"     bson:"classificationRules,omitempty"`
	DeliveryTargets     []TemplateDeliveryTarget `json:"deliveryTargets,omitempty"         bson:"deliveryTargets,omitempty"`
	CreatedAt           time.Time                `json:"createdAt"                         bson:"createdAt"`
	UpdatedAt           time.Time                `json:"updatedAt"                         bson:"updatedAt"`
}

// MatchCondition — V2 condition for matchAll/matchAny template matching.
type MatchCondition struct {
	Field    string   `json:"field"    bson:"field"`    // e.g. "raw.type", "raw.typeValue"
	Operator string   `json:"operator" bson:"operator"` // "eq" | "in" | "contains" | "prefix"
	Values   []string `json:"values"   bson:"values"`   // values to match against
}

// MatchRule defines criteria used to auto-bind a template to an incoming event (V1 legacy).
// Fingerprint matching uses: vendor|protocol|deviceType|deviceId|subType|eventType|rawSchemaVersion|rawBodyKeyHash
type MatchRule struct {
	Vendor           string `json:"vendor,omitempty"           bson:"vendor,omitempty"`
	Protocol         string `json:"protocol,omitempty"         bson:"protocol,omitempty"`
	DeviceType       string `json:"deviceType,omitempty"       bson:"deviceType,omitempty"`
	DeviceId         string `json:"deviceId,omitempty"         bson:"deviceId,omitempty"`
	SubType          string `json:"subType,omitempty"          bson:"subType,omitempty"`
	EventType        string `json:"eventType,omitempty"        bson:"eventType,omitempty"`
	RawSchemaVersion string `json:"rawSchemaVersion,omitempty" bson:"rawSchemaVersion,omitempty"`
	RawBodyKeyHash   string `json:"rawBodyKeyHash,omitempty"   bson:"rawBodyKeyHash,omitempty"`
}
