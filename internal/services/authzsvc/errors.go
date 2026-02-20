// internal/services/authzsvc/errors.go
package authzsvc

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/hotkhwan/gateway-api/internal/repo/authzrepo"
	"github.com/hotkhwan/gateway-api/models/gmod"
)

var (
	// Error Request
	ErrBadRequest    = errors.New("bad request")
	ErrUnauthorized  = errors.New("unauthorized")
	ErrNotFound      = errors.New("not found")
	ErrConflict      = errors.New("conflict")
	ErrForbidden     = errors.New("forbidden")
	ErrHasChildren   = errors.New("has children")
	ErrRootImmutable = errors.New("root immutable")
	ErrInvalidParent = errors.New("invalid parent")

	// Error Invite
	ErrForbiddenInvite        = errors.New("forbidden: no manage permission")
	ErrInvalidInviteArgs      = errors.New("invalid invite args")
	ErrInviteRoleNotAllowed   = errors.New("invalid role")
	ErrInviteAlreadyMember    = errors.New("user already in organization")
	ErrInviteSelfAdminBlocked = errors.New("cannot self-invite as admin")
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
	case errors.Is(err, authzrepo.ErrOrgNameAlreadyExists):
		return http.StatusConflict, gmod.CodeConflict
	case errors.Is(err, authzrepo.ErrNotFound),
		errors.Is(err, ErrNotFound):
		return http.StatusNotFound, gmod.CodeNotFound
	case errors.Is(err, ErrHasChildren),
		errors.Is(err, ErrConflict):
		return http.StatusConflict, gmod.CodeConflict
	case errors.Is(err, ErrBadRequest):
		return http.StatusBadRequest, gmod.CodeBadRequest
	case errors.Is(err, ErrUnauthorized):
		return http.StatusUnauthorized, gmod.CodeUnauthorized
	default:
		return http.StatusInternalServerError, gmod.CodeInternalError
	}
}

func Fail(err error, format string, args ...any) error {
	// helper: wrap + formatted message (ไม่ทำให้เสีย errors.Is)
	msg := fmt.Sprintf(format, args...)
	return Wrap(err, msg)
}
