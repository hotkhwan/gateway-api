// router/opt.go
package router

import (
	"klynx/config"
	"klynx/controllers/optapi"
	"klynx/internal/middleware"
	"klynx/internal/repo/authzrepo"
	"os"

	"github.com/gofiber/fiber/v2"
)

func RegisterOptRoutes(router fiber.Router) {
	router.Route("/options", func(r fiber.Router) {
		// ✅ ใส่ Audit ก่อน
		r.Use(middleware.Audit(middleware.AuditConfig{
			AuditRepo: authzrepo.NewAuditLogRepo(config.DB),
			AuditChan: nil,
			BasePath:  os.Getenv("BASE_PATH"),
			Effective: middleware.EffectiveGetter(),
		}))

		// ---------- Guards (เพิ่ม OPTIONS เผื่อ CORS/preflight) ----------
		r.All("", middleware.AllowMethods("GET", "PUT", "POST", "OPTIONS"))
		r.All("/", middleware.AllowMethods("GET", "PUT", "POST", "OPTIONS"))
		r.All("/seed", middleware.AllowMethods("POST", "OPTIONS"))

		// ---------- Public ----------
		public := r.Group("")
		public.Post("/seed", optapi.SeedPoliceStation)

		// ✅ รองรับทั้ง POST และ PUT (ทั้งมีและไม่มี '/')
		public.Post("/", optapi.PutOptions)
		public.Post("", optapi.PutOptions)
		public.Put("/", optapi.PutOptions)
		public.Put("", optapi.PutOptions)

		// ---------- Protected (ต้องมี JWT) ----------
		protected := r.Group("", middleware.AuthBearer())
		protected.Get("/", optapi.GetOptions)
		protected.Get("", optapi.GetOptions)
	})
}
