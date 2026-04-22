// internal/services/targetsvc/errors.go
package targetsvc

import (
	"errors"
	"net/http"

	"github.com/hotkhwan/gateway-api/internal/repo/ingestrepo"
	"github.com/hotkhwan/gateway-api/models/gmod"
)

const CodeTargetInUse = "TARGET_IN_USE"

var (
	ErrNotFound              = errors.New("not found")
	ErrConflict              = errors.New("conflict: name already exists")
	ErrForbidden             = errors.New("forbidden: insufficient permission")
	ErrBadRequest            = errors.New("bad request")
	ErrUnauthorized          = errors.New("unauthorized")
	ErrPlanLimitExceeded     = errors.New("plan limit exceeded")
	ErrTargetInUse           = errors.New("target is still assigned to one or more mapping templates")
	ErrMissingBotToken       = errors.New("botToken is required for telegram targets")
	ErrMissingChatId         = errors.New("chatId is required for telegram targets")
	ErrMissingChannelToken   = errors.New("channelAccessToken is required for line targets")
	ErrMissingRecipients     = errors.New("to is required for line targets")
	ErrMissingURL            = errors.New("url is required for webhook and discord targets")
	ErrKlynxModeWithURL      = errors.New("mode=klynx target must not have url field")
	ErrKlynxModeWithHMAC     = errors.New("mode=klynx target must not have hmac/signing config")
	ErrKlynxModeInSaasPublic = errors.New("mode=klynx target is not supported in saasPublic profile")
)

// TargetInUseError wraps ErrTargetInUse with the list of templates still
// referencing the target, so the controller can surface them in `details`.
type TargetInUseError struct {
	Templates []ingestrepo.TemplateUsageRef
}

func (e *TargetInUseError) Error() string { return ErrTargetInUse.Error() }
func (e *TargetInUseError) Unwrap() error { return ErrTargetInUse }

// MapSvcError แปลง service error → HTTP status + error code
func MapSvcError(err error) (int, string) {
	switch {
	case errors.Is(err, ErrNotFound):
		return http.StatusNotFound, gmod.CodeNotFound
	case errors.Is(err, ErrConflict):
		return http.StatusConflict, gmod.CodeConflict
	case errors.Is(err, ErrTargetInUse):
		return http.StatusConflict, CodeTargetInUse
	case errors.Is(err, ErrForbidden):
		return http.StatusForbidden, gmod.CodeForbidden
	case errors.Is(err, ErrBadRequest):
		return http.StatusBadRequest, gmod.CodeBadRequest
	case errors.Is(err, ErrUnauthorized):
		return http.StatusUnauthorized, gmod.CodeUnauthorized
	case errors.Is(err, ErrPlanLimitExceeded):
		return http.StatusPaymentRequired, "PLAN_LIMIT_EXCEEDED"
	case errors.Is(err, ErrKlynxModeWithURL), errors.Is(err, ErrKlynxModeWithHMAC), errors.Is(err, ErrKlynxModeInSaasPublic):
		return http.StatusBadRequest, gmod.CodeBadRequest
	case errors.Is(err, ErrMissingBotToken),
		errors.Is(err, ErrMissingChatId),
		errors.Is(err, ErrMissingChannelToken),
		errors.Is(err, ErrMissingRecipients),
		errors.Is(err, ErrMissingURL):
		return http.StatusBadRequest, gmod.CodeBadRequest
	default:
		return http.StatusInternalServerError, gmod.CodeInternalError
	}
}
