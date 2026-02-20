// internal/repo/authzrepo/error.go
package authzrepo

import "errors"

var (
	ErrOrgNameAlreadyExists = errors.New("organization name already exists")
	ErrNotFound             = errors.New("not found")
)
