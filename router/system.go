// router/system.go
package router

import (
	"context"
	"os"

	"klynx/config"
	"klynx/controllers/mediapi"
	"klynx/controllers/sysapi"
	"klynx/controllers/sysapi/edgeapi"
	"klynx/controllers/sysapi/setapi"
	"klynx/internal/middleware"
	"klynx/internal/repo/authzrepo"
	"klynx/internal/repo/optionsrepo"

	"github.com/gofiber/fiber/v2"
)

func RegisterSystemRoutes(api fiber.Router) {
	api.Route("/system", func(r fiber.Router) {
		r.Use(middleware.AuthBearer())
		r.Use(middleware.Audit(middleware.AuditConfig{
			AuditRepo: authzrepo.NewAuditLogRepo(config.DB),
			AuditChan: nil,
			BasePath:  os.Getenv("BASE_PATH"),
			Effective: middleware.EffectiveGetter(),
		}))

		// ===== options (/system/options) =====
		optsRepo := optionsrepo.New(config.DB)
		optsCtl := sysapi.NewOptionsController(optsRepo, func(ctx context.Context, days int) error {
			return authzrepo.EnsureAuditIndexes(ctx, config.DB, days)
		})

		r.All("/options", middleware.AllowMethods("GET", "PATCH"))
		r.Get("/options", optsCtl.GetEffective)
		r.Patch("/options", optsCtl.Patch)

		// ===== edge (/system/edge/...) =====
		er := r.Group("/edge")

		// Root list: รองรับทั้ง /edge และ /edge/ (กัน StrictRouting)
		er.All("", middleware.AllowMethods("GET"))
		er.All("/", middleware.AllowMethods("GET"))
		er.Get("", edgeapi.ListEdges)
		er.Get("/", edgeapi.ListEdges)

		// Create by type
		// Create (by type)
		er.All("/type/:edgeType", middleware.AllowMethods("POST"))
		er.Post("/type/:edgeType", edgeapi.CreateEdge)
		er.Get("/:edgeType/sso/:id", middleware.AllowOnly("GET"), edgeapi.GetEdgeSSOURL)
		// Update / Delete (by id)
		er.All("/:id", middleware.AllowMethods("PATCH", "DELETE"))
		er.Patch("/:id", edgeapi.UpdateEdge)
		er.Delete("/:id", edgeapi.DeleteEdge)

		// ===== stream (/system/setting/...) =====
		str := r.Group("/mapLocation")

		// Root list: รองรับทั้ง /stream และ /stream/ (กัน StrictRouting)
		str.All("", middleware.AllowMethods("GET"))
		str.All("/", middleware.AllowMethods("GET"))
		str.Get("", setapi.ListConfig)
		str.Get("/", setapi.ListConfig)

		// Update/Delete by id
		str.All("/:id", middleware.AllowMethods("PATCH"))
		str.Patch("/:id", setapi.UpdateConfig)

		// ===== stream (/system/stream/...) =====
		sr := r.Group("/stream")

		// Root list: รองรับทั้ง /stream และ /stream/ (กัน StrictRouting)
		sr.All("", middleware.AllowMethods("GET"))
		sr.All("/", middleware.AllowMethods("GET"))
		sr.Get("", mediapi.ListConfig)
		sr.Get("/", mediapi.ListConfig)

		// Update/Delete by id
		sr.All("/:id", middleware.AllowMethods("PATCH"))
		sr.Patch("/:id", mediapi.UpdateConfig)
	})
}
