// router/events.go
package router

import (
	"klynx/controllers/dashapi"
	"klynx/internal/middleware"

	"github.com/gofiber/fiber/v2"
)

func RegisterKcontrolDashboard(router fiber.Router) {
	router.Route("/dashboard", func(r fiber.Router) {
		r.Use(middleware.AuthBearer())

		r.Get("", middleware.AllowOnly("GET"), dashapi.DashboardSummary)
		r.Get("/", middleware.AllowOnly("GET"), dashapi.DashboardSummary)

		// ✅ PATCH blacklist by id
		r.Patch("/blacklist/:id", middleware.AllowOnly("PATCH"), dashapi.UpdateBlacklist)
	})
}
