// internal/services/dlqsvc/types.go
package dlqsvc

// RetryResult is returned after a successful retry scheduling.
type RetryResult struct {
	MessageId  string `json:"messageId"`
	RetryCount int    `json:"retryCount"`
	Status     string `json:"status"`
}

// ReplayResult is returned after a force-replay (bypasses max-retry check).
type ReplayResult struct {
	MessageId string `json:"messageId"`
	Status    string `json:"status"`
}
