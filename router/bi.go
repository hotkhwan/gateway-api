// router/authz.go
package router

import (
	"klynx/controllers/biapi"
	"klynx/internal/middleware"

	"github.com/gofiber/fiber/v2"
)

func RegisterBIRoutes(router fiber.Router) {
	router.Route("/bi", func(r fiber.Router) {
		r.Use(middleware.AuthBearer())
		r.All("/signUrl", middleware.AllowOnly("GET"), biapi.SignBIUrl)
	})
}
