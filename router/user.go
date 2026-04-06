// router/user.go
package router

import (
	"github.com/hotkhwan/gateway-api/config"
	"github.com/hotkhwan/gateway-api/controllers/usrapi"
	"github.com/hotkhwan/gateway-api/internal/middleware"
	"github.com/hotkhwan/gateway-api/internal/repo/authzrepo"
	"os"

	"github.com/gofiber/fiber/v3"
)

func RegisterUserRoutes(router fiber.Router) {
	router.Route("/users", func(r fiber.Router) {
		r.Use(middleware.AuthBearer())
		r.Use(middleware.Audit(middleware.AuditConfig{
			AuditRepo: authzrepo.NewAuditLogRepo(config.DB),
			AuditChan: nil,
			BasePath:  os.Getenv("BASE_PATH"),
			Effective: middleware.EffectiveGetter(),
		}))
		r.All("", middleware.AllowMethods("GET", "POST"))
		r.Get("", usrapi.ListUsers)
		r.Post("", usrapi.CreateUser)

		r.All("/", middleware.AllowMethods("GET", "POST", "PATCH", "DELETE"))
		r.Get("/", usrapi.ListUsers)
		r.Post("/", usrapi.CreateUser)
		r.Patch("/:id", usrapi.UpdateUser)
		r.Patch("/:id/disable", usrapi.DisableUser)
		r.Patch("/:id/enable", middleware.AllowOnly("PATCH"), usrapi.EnableUser)
		r.Patch("/:id/password", usrapi.ChangePassword)
		r.Delete("/:id", usrapi.DeleteUser)

		r.Post("/batch", usrapi.BatchGetUsers)
	})
}
