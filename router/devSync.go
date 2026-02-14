// router/devSync.go
package router

import (
	"klynx/controllers/devsyncapi"
	"klynx/internal/middleware"

	"github.com/gofiber/fiber/v2"
)

func RegisterDeviceSyncRoutes(router fiber.Router) {
	router.Route("/devsync", func(r fiber.Router) {
		r.Use(middleware.AuthBearer())
		r.Get("/svms/:id", middleware.AllowOnly("GET"), devsyncapi.EdgeSVMS)
		r.Get("/ata/:id", middleware.AllowOnly("GET"), devsyncapi.EdgeATA)
		r.Get("/iboc/:id", middleware.AllowOnly("GET"), devsyncapi.EdgeIBOC)
	})
}
