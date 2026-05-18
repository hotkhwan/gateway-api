// router/adminKctrlRegistry.go
//
// Routes the kctrl-registry inbound surface from klynx-api Phase B + the
// operator drift / retry endpoints. Wiring described in
// klynx-api/docs/contracts/kcontrol-gw-managed-registry.md §4 and the
// companion gateway-api plan docs/plan/kctrl-registry-phase-a.md.
package router

import (
	"github.com/gofiber/fiber/v3"
	"github.com/hotkhwan/gateway-api/internal/app"
	"github.com/hotkhwan/gateway-api/internal/middleware"
)

// RegisterAdminKctrlRegistryRoutes mounts:
//
//	PATCH  /admin/kctrl-registry/:hwId            (klynx → gw upsert)
//	DELETE /admin/kctrl-registry/:hwId            (klynx → gw delete)
//	GET    /admin/system/kctrlRegistryDrift       (operator)
//	POST   /admin/system/kctrlRegistryRetry/:hwId (operator)
func RegisterAdminKctrlRegistryRoutes(router fiber.Router, c *app.Container) {
	if c.KctrlRegistryController == nil {
		return
	}

	router.Route("/admin/kctrl-registry", func(r fiber.Router) {
		r.Use(middleware.AuthBearer())
		r.Use(middleware.ActiveWorkspace())

		r.All("/:hwId", middleware.AllowMethods("PATCH", "DELETE"))
		r.Patch("/:hwId", c.KctrlRegistryController.Upsert)
		r.Delete("/:hwId", c.KctrlRegistryController.Delete)
	})

	router.Route("/admin/system", func(r fiber.Router) {
		r.Use(middleware.AuthBearer())
		r.Use(middleware.RequireRoles([]string{"administrator"}))

		r.All("/kctrlRegistryDrift", middleware.AllowMethods("GET"))
		r.Get("/kctrlRegistryDrift", c.KctrlRegistryController.Drift)

		r.All("/kctrlRegistryRetry/:hwId", middleware.AllowMethods("POST"))
		r.Post("/kctrlRegistryRetry/:hwId", c.KctrlRegistryController.Retry)
	})
}
