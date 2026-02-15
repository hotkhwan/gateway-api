// router/authz.go
package router

import (
	"klynx/config"
	"klynx/controllers/authznewapi"
	"klynx/internal/middleware"
	"klynx/internal/repo/authzrepo"
	"os"

	"github.com/gofiber/fiber/v2"
)

func RegisterAuthzNew(router fiber.Router) {
	router.Route("/orgs", func(r fiber.Router) {

		r.Use(middleware.AuthBearer())

		r.Use(middleware.Audit(middleware.AuditConfig{
			AuditRepo: authzrepo.NewAuditLogRepo(config.DB),
			AuditChan: nil,
			BasePath:  os.Getenv("BASE_PATH"),
			Effective: middleware.EffectiveGetter(),
		}))

		r.Post("/", authznewapi.CreateOrg)
	})
}
