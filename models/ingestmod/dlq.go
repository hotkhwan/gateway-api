// models/ingestmod/dlq.go
package ingestmod

import "time"

// DLQMessage — a failed delivery message held in the dead-letter queue.
// Collection: dlq_events
type DLQMessage struct {
	MessageId           string         `json:"messageId"           bson:"messageId"`
	EventId             string         `json:"eventId"             bson:"eventId"`
	TenantId            string         `json:"tenantId"            bson:"tenantId"`
	OrgId               string         `json:"orgId"               bson:"orgId"`
	TemplateId          string         `json:"templateId,omitempty" bson:"templateId,omitempty"` // source template (if matched)
	Topic               string         `json:"topic"               bson:"topic"`                 // Kafka topic that failed
	Stage               string         `json:"stage"               bson:"stage"`                 // "normalize" | "deliver" | "webhook"
	Reason              string         `json:"reason"              bson:"reason"`
	Payload             map[string]any `json:"payload"             bson:"payload"` // original message payload
	RetryCount          int            `json:"retryCount"          bson:"retryCount"`
	MaxRetries          int            `json:"maxRetries"          bson:"maxRetries"`          // from template.DLQ.MaxRetries
	RetryTimeoutSeconds int            `json:"retryTimeoutSeconds" bson:"retryTimeoutSeconds"` // from template.DLQ.RetryTimeoutSeconds
	Status              string         `json:"status"              bson:"status"`              // "pending" | "retrying" | "resolved" | "abandoned"
	LastErrorAt         time.Time      `json:"lastErrorAt"         bson:"lastErrorAt"`
	CreatedAt           time.Time      `json:"createdAt"           bson:"createdAt"`
	UpdatedAt           time.Time      `json:"updatedAt"           bson:"updatedAt"`
}
