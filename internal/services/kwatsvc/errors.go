// internal/services/kwatsvc/errors.go
package kwatsvc

import "errors"

var (
	// generic
	ErrUnauthorized = errors.New("unauthorized")
	ErrServer       = errors.New("server error")

	// validation / input
	ErrInvalidObjectID = errors.New("invalid object id")
	ErrPhotoRequired   = errors.New("photo is required")

	// domain
	ErrDuplicateIDCard   = errors.New("duplicate idcard")
	ErrBadPhoto          = errors.New("bad photo")
	ErrNoFace            = errors.New("no face detected")
	ErrWatchlistNotFound = errors.New("watchlist not found")
)
