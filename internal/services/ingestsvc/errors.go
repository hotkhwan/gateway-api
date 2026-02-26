// internal/services/ingestsvc/errors.go
package ingestsvc

import "errors"

var (
	ErrOrgNotFound    = errors.New("org not found or disabled")
	ErrRateLimited    = errors.New("rate limit exceeded")
	ErrPayloadTooLarge = errors.New("payload too large (max 256KB)")
	ErrEmptyBody      = errors.New("request body is empty")
)
