// router/authzDebug.go
package router

import (
	"os"

	"github.com/gofiber/fiber/v2"
	"github.com/hotkhwan/gateway-api/config"
	"github.com/hotkhwan/gateway-api/controllers/authznewapi"
	"github.com/hotkhwan/gateway-api/internal/gateways/authzgw"
	"github.com/hotkhwan/gateway-api/internal/middleware"
	"github.com/hotkhwan/gateway-api/internal/repo/authzrepo"
	"github.com/hotkhwan/gateway-api/internal/services/authzsvc"
)

func RegisterAuthzDebugRoutes(router fiber.Router) {
	// ✅ DI
	authzClient := authzgw.NewClient()
	debugService := authzsvc.NewAuthzDebugService(authzClient)
	ctrl := authznewapi.NewAuthzDebugController(debugService)

	router.Route("/authzDebugs", func(r fiber.Router) {
		r.Use(middleware.Audit(middleware.AuditConfig{
			AuditRepo: authzrepo.NewAuditLogRepo(config.DB),
			AuditChan: nil,
			BasePath:  os.Getenv("BASE_PATH"),
			Effective: middleware.EffectiveGetter(),
		}))

		protected := r.Group("", middleware.AuthBearer())

		protected.Get("/tuples", ctrl.ListPermifyTuples)
		protected.Get("/tuples/all", ctrl.ListAllTuples)        // list ทุก entityType
		protected.Post("/tuples/clearAll", ctrl.ClearAllTuples) // clear ทั้ง tenant
		protected.Get("/tuples/org/:orgId", ctrl.ListOrgTuples) // list tuple ของ org นั้น
		protected.Post("/tuples/clearOrg", ctrl.ClearOrgTuples) // clear org เดียว

		protected.Post("/tuples/factoryOrgBootstrap", ctrl.FactoryOrgBootstrap)
		protected.Post("/tuples/factoryReset", ctrl.FactoryResetPermifyTuples)
		protected.Post("/tuples/resetAll", ctrl.ResetAllTuples)
		protected.Post("/tuples/resetByUser", ctrl.ResetTuplesByUser)
		protected.Get("/prove/orgs", ctrl.ProveUserOrgsAgainstMongo)
		protected.Get("/tuples/byUser", ctrl.ListTuplesByUser)
		protected.Get("/tuples/ou", ctrl.ListOrgUnitTuples)
		protected.Post("/tuples/resetTenant", ctrl.ResetTenant)
	})
}
