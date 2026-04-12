// internal/services/ingesttmplsvc/errors.go
package ingesttmplsvc

import (
	"errors"
	"net/http"
)

var (
	ErrNotFound   = errors.New("ingest template not found")
	ErrBadRequest = errors.New("invalid input")
	ErrConflict   = errors.New("template name already exists")
)

// MapSvcError maps an ingesttmplsvc error to HTTP status + code string.
func MapSvcError(err error) (int, string) {
	switch {
	case errors.Is(err, ErrNotFound):
		return http.StatusNotFound, "NOT_FOUND"
	case errors.Is(err, ErrConflict):
		return http.StatusConflict, "CONFLICT"
	case errors.Is(err, ErrBadRequest):
		return http.StatusBadRequest, "BAD_REQUEST"
	default:
		return http.StatusInternalServerError, "INTERNAL_ERROR"
	}
}
