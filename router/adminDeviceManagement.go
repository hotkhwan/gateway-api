// router/adminDeviceManagement.go
package router

import (
	"github.com/gofiber/fiber/v3"
	"github.com/hotkhwan/gateway-api/internal/app"
	"github.com/hotkhwan/gateway-api/internal/middleware"
)

// RegisterAdminDeviceManagementRoutes mounts the admin-scoped surfaces for
// the canonical device_management store. v1 ships the klynx-camera overlay
// inbound PATCH endpoint per klynx-api/docs/contracts/camera-gw-managed-overlay.md §8.
// v1.11 adds the DELETE (deregister/soft-tombstone) sibling per
// klynx-api/docs/contracts/device-camera-domain.md §5.10.
func RegisterAdminDeviceManagementRoutes(router fiber.Router, c *app.Container) {
	if c.CameraOverlayInboundController == nil {
		return
	}

	router.Route("/admin/device-management", func(r fiber.Router) {
		r.Use(middleware.AuthBearer())
		r.Use(middleware.ActiveWorkspace())

		r.Route("/cameras", func(rc fiber.Router) {
			rc.All("/:gwDeviceMgmtId", middleware.AllowMethods("PATCH", "DELETE"))
			rc.Patch("/:gwDeviceMgmtId", c.CameraOverlayInboundController.Apply)
			rc.Delete("/:gwDeviceMgmtId", c.CameraOverlayInboundController.Delete)
		})

		// Per-camera device-identity provisioning (camID Phase 1) —
		// klynx-api pre-creates the identity before a camera uploads.
		if c.DeviceIdentityController != nil {
			r.All("/identities", middleware.AllowMethods("POST"))
			r.Post("/identities", c.DeviceIdentityController.Provision)
		}
	})
}
