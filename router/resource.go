// router/resources.go
package router

import (
	"github.com/hotkhwan/gateway-api/controllers/rscapi"
	"github.com/hotkhwan/gateway-api/internal/middleware"

	"github.com/gofiber/fiber/v2"
)

func RegisterResourceRoutes(router fiber.Router) {
	router.Route("/resources", func(r fiber.Router) {
		r.Use(middleware.AuthBearer())
		r.Use("", middleware.AllowMethods("GET", "POST", "PUT", "DELETE"))

		r.Get("", rscapi.ResourceList)
		r.Post("", rscapi.ResourceCreate)
		r.Post("/bulk", rscapi.BulkCreateResources)

		r.Put("/:id", rscapi.ResourceUpdate)
		r.Delete("/:id", rscapi.ResourceDelete)

	})
}
