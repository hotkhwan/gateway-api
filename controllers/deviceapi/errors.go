// controllers/deviceapi/errors.go
package deviceapi

import (
	"errors"

	"github.com/gofiber/fiber/v2"
	"github.com/hotkhwan/gateway-api/internal/repo/devicerepo"
	"github.com/hotkhwan/gateway-api/internal/services/devicesvc"
	"github.com/hotkhwan/gateway-api/utils/httputil"
)

// handleErr — maps service/repo errors → HTTP status + code
func handleErr(c *fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, devicesvc.ErrForbidden):
		return httputil.FailForbidden(c, "forbidden")
	case errors.Is(err, devicesvc.ErrInvalidArgs):
		return httputil.FailBadRequest(c, err.Error())
	case errors.Is(err, devicesvc.ErrPermifySyncFailed):
		return httputil.Fail(c, fiber.StatusBadGateway, "PERMIFY_SYNC_FAILED", "authorization sync failed, device was rolled back")
	case errors.Is(err, devicerepo.ErrNotFound):
		return httputil.FailNotFound(c, "not found")
	case errors.Is(err, devicerepo.ErrCrossOrgAccess):
		return httputil.FailForbidden(c, "cross-org access denied")
	case errors.Is(err, devicerepo.ErrGroupNameAlreadyExists):
		return httputil.FailConflict(c, "name already exists in this org")
	case errors.Is(err, devicesvc.ErrCameraNameAlreadyExists):
		return httputil.FailConflict(c, "camera name already exists in this org")
	case errors.Is(err, devicesvc.ErrResourceGroupNameAlreadyExists):
		return httputil.FailConflict(c, "resource group name already exists in this org")
	default:
		return httputil.FailInternal(c, "internal server error")
	}
}
