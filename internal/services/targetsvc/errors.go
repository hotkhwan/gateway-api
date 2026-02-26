// internal/services/targetsvc/errors.go
package targetsvc

import (
	"errors"
	"net/http"

	"github.com/hotkhwan/gateway-api/models/gmod"
)

var (
	ErrNotFound     = errors.New("not found")
	ErrConflict     = errors.New("conflict: name already exists")
	ErrForbidden    = errors.New("forbidden: insufficient permission")
	ErrBadRequest   = errors.New("bad request")
	ErrUnauthorized = errors.New("unauthorized")
)

// MapSvcError แปลง service error → HTTP status + error code
func MapSvcError(err error) (int, string) {
	switch {
	case errors.Is(err, ErrNotFound):
		return http.StatusNotFound, gmod.CodeNotFound
	case errors.Is(err, ErrConflict):
		return http.StatusConflict, gmod.CodeConflict
	case errors.Is(err, ErrForbidden):
		return http.StatusForbidden, gmod.CodeForbidden
	case errors.Is(err, ErrBadRequest):
		return http.StatusBadRequest, gmod.CodeBadRequest
	case errors.Is(err, ErrUnauthorized):
		return http.StatusUnauthorized, gmod.CodeUnauthorized
	default:
		return http.StatusInternalServerError, gmod.CodeInternalError
	}
}
