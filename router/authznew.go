// router/authz.go
package router

import (
	"os"

	"github.com/hotkhwan/gateway-api/config"
	"github.com/hotkhwan/gateway-api/controllers/authznewapi"
	"github.com/hotkhwan/gateway-api/internal/middleware"
	"github.com/hotkhwan/gateway-api/internal/repo/authzrepo"

	"github.com/gofiber/fiber/v2"
)

func RegisterAuthzNewRoutes(router fiber.Router) {
	router.Route("/orgs", func(r fiber.Router) {

		r.Use(middleware.AuthBearer())

		r.Use(middleware.Audit(middleware.AuditConfig{
			AuditRepo: authzrepo.NewAuditLogRepo(config.DB),
			AuditChan: nil,
			BasePath:  os.Getenv("BASE_PATH"),
			Effective: middleware.EffectiveGetter(),
		}))

		r.Post("/", authznewapi.CreateOrg)
		r.Post("", authznewapi.CreateOrg)
	})
}
