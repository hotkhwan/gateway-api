// internal/services/devicesvc/errors.go
package devicesvc

import "errors"

var (
	ErrForbidden                      = errors.New("forbidden")
	ErrInvalidArgs                    = errors.New("invalid arguments")
	ErrNotFound                       = errors.New("not found")
	ErrPermifySyncFailed              = errors.New("permify sync failed")
	ErrHasChildren                    = errors.New("has children")
	ErrCameraNameAlreadyExists        = errors.New("camera name already exists in this org")
	ErrResourceGroupNameAlreadyExists = errors.New("resource group name already exists in this org")
)
