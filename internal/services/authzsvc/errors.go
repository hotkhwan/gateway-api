// internal/services/authzsvc/errors.go
package authzsvc

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/hotkhwan/gateway-api/models/gmod"
)

var (
	ErrBadRequest    = errors.New("bad request")
	ErrUnauthorized  = errors.New("unauthorized")
	ErrNotFound      = errors.New("not found")
	ErrConflict      = errors.New("conflict")
	ErrForbidden     = errors.New("forbidden")
	ErrHasChildren   = errors.New("has children")
	ErrRootImmutable = errors.New("root immutable")
	ErrInvalidParent = errors.New("invalid parent")
)

type SvcError struct {
	err    error
	msg    string
	detail map[string]any
}

func (e *SvcError) Error() string {
	if e.msg != "" {
		return e.msg
	}
	if e.err != nil {
		return e.err.Error()
	}
	return "service error"
}

func (e *SvcError) Unwrap() error { return e.err }

func Wrap(err error, msg string) error {
	if err == nil {
		return nil
	}
	return &SvcError{err: err, msg: msg}
}

func WithDetail(err error, detail map[string]any) error {
	if err == nil {
		return nil
	}
	if se, ok := err.(*SvcError); ok {
		se.detail = detail
		return se
	}
	return &SvcError{err: err, detail: detail}
}

func MapSvcError(err error) (status int, code string) {
	if err == nil {
		return http.StatusOK, gmod.CodeSuccess
	}

	switch {
	case errors.Is(err, ErrBadRequest), errors.Is(err, ErrInvalidParent):
		return http.StatusBadRequest, gmod.CodeBadRequest
	case errors.Is(err, ErrUnauthorized):
		return http.StatusUnauthorized, gmod.CodeUnauthorized
	case errors.Is(err, ErrForbidden):
		return http.StatusForbidden, gmod.CodeForbidden
	case errors.Is(err, ErrNotFound):
		return http.StatusNotFound, gmod.CodeNotFound
	case errors.Is(err, ErrConflict), errors.Is(err, ErrHasChildren), errors.Is(err, ErrRootImmutable):
		return http.StatusConflict, gmod.CodeConflict
	default:
		return http.StatusInternalServerError, gmod.CodeInternalError
	}
}

func Fail(err error, format string, args ...any) error {
	// helper: wrap + formatted message (ไม่ทำให้เสีย errors.Is)
	msg := fmt.Sprintf(format, args...)
	return Wrap(err, msg)
}
