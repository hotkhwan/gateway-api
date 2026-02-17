// router/authzDebug.go
package router

import (
	"os"

	"github.com/gofiber/fiber/v2"
	"github.com/hotkhwan/gateway-api/config"
	"github.com/hotkhwan/gateway-api/controllers/authznewapi"
	"github.com/hotkhwan/gateway-api/internal/middleware"
	"github.com/hotkhwan/gateway-api/internal/repo/authzrepo"
)

func RegisterAuthzDebugRoutes(router fiber.Router) {
	router.Route("/authzDebugs", func(r fiber.Router) {
		// ✅ Audit ก่อนเสมอ ตาม pattern
		r.Use(middleware.Audit(middleware.AuditConfig{
			AuditRepo: authzrepo.NewAuditLogRepo(config.DB),
			AuditChan: nil,
			BasePath:  os.Getenv("BASE_PATH"),
			Effective: middleware.EffectiveGetter(),
		}))

		protected := r.Group("", middleware.AuthBearer())

		// GET /api/v1/authz/tuples?tenantId=aisom&entityType=organization&entityId=...&subjectType=user&subjectId=...&relation=member&pageSize=50&continuousToken=...
		protected.Get("/tuples", authznewapi.ListPermifyTuples)

		// POST /api/v1/authz/tuples/factoryReset  (danger)
		protected.Post("/tuples/factoryReset", authznewapi.FactoryResetPermifyTuples)
		protected.Post("/tuples/resetAll", authznewapi.ResetAllTuples)
		protected.Post("/tuples/resetByUser", authznewapi.ResetTuplesByUser)

		// GET /api/v1/authz/prove/orgs?tenantId=aisom
		protected.Get("/prove/orgs", authznewapi.ProveUserOrgsAgainstMongo)
		
	})
}
