// models/ingestmod/dlq.go
package ingestmod

import "time"

// DLQMessage — a failed delivery message held in the dead-letter queue.
type DLQMessage struct {
	MessageId  string    `json:"messageId"  bson:"messageId"`
	EventId    string    `json:"eventId"    bson:"eventId"`
	Reason     string    `json:"reason"     bson:"reason"`
	RetryCount int       `json:"retryCount" bson:"retryCount"`
	CreatedAt  time.Time `json:"createdAt"  bson:"createdAt"`
}
