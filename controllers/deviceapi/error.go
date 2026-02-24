// controllers/deviceapi/errors.go
package deviceapi

import (
	"errors"

	"github.com/gofiber/fiber/v2"
	"github.com/hotkhwan/gateway-api/internal/repo/devicerepo"
	"github.com/hotkhwan/gateway-api/internal/services/devicesvc"
	"github.com/hotkhwan/gateway-api/models/gmod"
)

// handleErr — maps service/repo errors → HTTP status + code
func handleErr(c *fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, devicesvc.ErrForbidden):
		return c.Status(403).JSON(gmod.ApiErrorResponse{
			Code: gmod.CodeForbidden, Message: "forbidden", Status: false,
		})
	case errors.Is(err, devicesvc.ErrInvalidArgs):
		return c.Status(400).JSON(gmod.ApiErrorResponse{
			Code: gmod.CodeBadRequest, Message: err.Error(), Status: false,
		})
	case errors.Is(err, devicesvc.ErrPermifySyncFailed):
		return c.Status(502).JSON(gmod.ApiErrorResponse{
			Code: "PERMIFY_SYNC_FAILED", Message: "authorization sync failed, device was rolled back", Status: false,
		})
	case errors.Is(err, devicerepo.ErrNotFound):
		return c.Status(404).JSON(gmod.ApiErrorResponse{
			Code: gmod.CodeNotFound, Message: "not found", Status: false,
		})
	case errors.Is(err, devicerepo.ErrCrossOrgAccess):
		return c.Status(403).JSON(gmod.ApiErrorResponse{
			Code: gmod.CodeForbidden, Message: "cross-org access denied", Status: false,
		})
	case errors.Is(err, devicerepo.ErrGroupNameAlreadyExists):
		return c.Status(409).JSON(gmod.ApiErrorResponse{
			Code: "CONFLICT", Message: "name already exists in this org", Status: false,
		})
	case errors.Is(err, devicesvc.ErrCameraNameAlreadyExists):
		return c.Status(409).JSON(gmod.ApiErrorResponse{
			Code: "CONFLICT", Message: "camera name already exists in this org", Status: false,
		})
	case errors.Is(err, devicesvc.ErrResourceGroupNameAlreadyExists):
		return c.Status(409).JSON(gmod.ApiErrorResponse{
			Code: "CONFLICT", Message: "resource group name already exists in this org", Status: false,
		})
	default:
		return c.Status(500).JSON(gmod.ApiErrorResponse{
			Code: gmod.CodeInternalError, Message: "internal server error", Status: false,
		})
	}
}
