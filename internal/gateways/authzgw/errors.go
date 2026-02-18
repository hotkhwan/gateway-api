// internal/gateways/authzgw/errors.go
package authzgw

import "errors"

var (
	ErrNilRestClient      = errors.New("authzgw rest client is nil")
	ErrEmptyPermifyBaseURL = errors.New("permify baseURL is empty")
	ErrInvalidArgs        = errors.New("invalid args")
	ErrPermifyRejected    = errors.New("permify rejected request")
)
