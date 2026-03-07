// internal/services/dlqsvc/errors.go
package dlqsvc

import "errors"

var (
	ErrDLQMessageNotFound  = errors.New("dlq message not found")
	ErrAlreadyResolved     = errors.New("dlq message is already resolved")
	ErrMaxRetriesExceeded  = errors.New("max retries exceeded; use replay or abandon instead")
	ErrRetryTooSoon        = errors.New("retry timeout has not elapsed yet")
)
