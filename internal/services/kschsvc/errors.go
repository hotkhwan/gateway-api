// internal/services/kschsvc/errors.go
package kschsvc

import "errors"

var (
	// generic
	ErrUnauthorized = errors.New("unauthorized")
	ErrServer       = errors.New("server error")

	// validation / input
	ErrInvalidObjectID      = errors.New("invalid object id")
	ErrVideoRequired        = errors.New("video is required")
	ErrFileTooLarge         = errors.New("file too large")
	ErrUnsupportedVideoType = errors.New("unsupported video type")

	// domain
	ErrVideoNotFound = errors.New("video not found")
)
