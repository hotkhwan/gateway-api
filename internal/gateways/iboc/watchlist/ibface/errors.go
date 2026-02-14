// internal/gateways/iboc/watchlist/ibface/errors.go
package ibface

import (
	"errors"
)

var (
	ErrNoFaceDetected = errors.New("no face detected")
	ErrUnauthorized   = errors.New("unauthorized")
	ErrServer         = errors.New("server error")
	ErrDuplicate      = errors.New("duplicate")
)
