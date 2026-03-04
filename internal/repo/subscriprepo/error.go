// internal/repo/subscriprepo/error.go
package subscriprepo

import "errors"

var (
	ErrNotFound           = errors.New("subscription not found")
	ErrTenantHasActiveSub = errors.New("tenant already has an active subscription")
)
