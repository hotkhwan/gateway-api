// router/thirdapi.go
package router

import (
	"github.com/hotkhwan/gateway-api/config"
	"github.com/hotkhwan/gateway-api/controllers/thirdapi"
	"github.com/hotkhwan/gateway-api/internal/middleware"
	"github.com/hotkhwan/gateway-api/internal/repo/authzrepo"
	"os"

	"github.com/gofiber/fiber/v2"
)

func RegisterThirdAPIRoutes(router fiber.Router) {
	router.Route("/token", func(r fiber.Router) {
		// 🔸 เปิด Audit สำหรับ trace & log ได้
		r.Use(middleware.Audit(middleware.AuditConfig{
			AuditRepo: authzrepo.NewAuditLogRepo(config.DB),
			AuditChan: nil,
			BasePath:  os.Getenv("BASE_PATH"),
			Effective: middleware.EffectiveGetter(),
		}))

		// ---------- Guards ----------
		r.All("/api/serviceAccount", middleware.AllowMethods("POST"))
		r.All("/api/clientCredentials", middleware.AllowMethods("POST"))
		// ---------- End Guards ----------

		// ---------- Public Handlers ----------
		// 1️⃣ Create new service account client (ไม่ต้องมี Bearer)
		r.Post("/api/serviceAccount", thirdapi.CreateServiceAccountClient)

		// 2️⃣ Get access token via client_credentials (ไม่ต้องมี Bearer)
		r.Post("/api/clientCredentials", thirdapi.ClientCredentialsToken)
	})
}
