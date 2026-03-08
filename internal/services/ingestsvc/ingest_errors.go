// internal/services/ingestsvc/ingest_errors.go
package ingestsvc

import "errors"

var (
	ErrOrgNotFound             = errors.New("org not found or disabled")
	ErrRateLimited             = errors.New("rate limit exceeded")
	ErrPayloadTooLarge         = errors.New("payload too large")
	ErrEmptyBody               = errors.New("request body is empty")
	ErrSubscriptionUnavailable = errors.New("subscription temporarily unavailable")
	ErrSourceFamilyLocked      = errors.New("source family is not enabled")
	ErrSourceFamilyComingSoon  = errors.New("source family is coming soon")
)
