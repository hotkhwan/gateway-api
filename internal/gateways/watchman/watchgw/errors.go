// internal/gateways/watchman/watchgw/errors.go
package watchgw

import "errors"

var (
	ErrNotFound     = errors.New("watchman: not found")
	ErrUnauthorized = errors.New("unauthorized")
	ErrServer       = errors.New("server error")
	ErrDuplicate    = errors.New("duplicate")
)
