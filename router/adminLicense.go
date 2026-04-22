// router/adminLicense.go
package router

import (
	"os"
	"strings"

	"github.com/gofiber/fiber/v3"
	"github.com/hotkhwan/gateway-api/internal/app"
	"github.com/hotkhwan/gateway-api/internal/middleware"
)

func envFlag(name string) bool {
	return strings.EqualFold(os.Getenv(name), "true")
}

// RegisterAdminLicenseRoutes mounts the minimal enterprise-license issuance
// surface. Only registered when LICENSE_ADMIN_ENABLED=true so customer
// deployments ship without issuance endpoints.
func RegisterAdminLicenseRoutes(router fiber.Router, c *app.Container) {
	if !envFlag("LICENSE_ADMIN_ENABLED") {
		return
	}
	if c.LicenseAdminController == nil {
		return
	}

	router.Route("/admin/licenses", func(r fiber.Router) {
		r.Use(middleware.AuthBearer())
		r.Use(middleware.RequireRoles([]string{"administrator"}))

		r.All("/", middleware.AllowMethods("GET", "POST"))
		r.Get("/", c.LicenseAdminController.List)
		r.Post("/", c.LicenseAdminController.Issue)

		r.All("/:id", middleware.AllowMethods("GET"))
		r.Get("/:id", c.LicenseAdminController.Get)

		r.All("/:id/revoke", middleware.AllowMethods("POST"))
		r.Post("/:id/revoke", c.LicenseAdminController.Revoke)
	})
}

// RegisterPlatformLicenseRoutes mounts the deployment-side activation surface.
// Only registered when PLATFORM_LICENSE_ENABLED=true.
func RegisterPlatformLicenseRoutes(router fiber.Router, c *app.Container) {
	if !envFlag("PLATFORM_LICENSE_ENABLED") {
		return
	}
	if c.PlatformLicenseController == nil {
		return
	}

	router.Route("/admin/platformLicense", func(r fiber.Router) {
		r.Use(middleware.AuthBearer())
		r.Use(middleware.RequireRoles([]string{"administrator"}))

		r.All("/", middleware.AllowMethods("GET"))
		r.Get("/", c.PlatformLicenseController.Current)

		r.All("/validate", middleware.AllowMethods("POST"))
		r.Post("/validate", c.PlatformLicenseController.Validate)

		r.All("/activate", middleware.AllowMethods("POST"))
		r.Post("/activate", c.PlatformLicenseController.Activate)
	})
}
