// router/authz.go
package router

import (
	"github.com/hotkhwan/gateway-api/controllers/biapi"
	"github.com/hotkhwan/gateway-api/internal/middleware"

	"github.com/gofiber/fiber/v3"
)

func RegisterBIRoutes(router fiber.Router) {
	router.Route("/bi", func(r fiber.Router) {
		r.Use(middleware.AuthBearer())
		r.All("/signUrl", middleware.AllowOnly("GET"), biapi.SignBIUrl)
	})
}
