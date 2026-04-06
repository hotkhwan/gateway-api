// router/auth.go
package router

import (
	"os"

	"github.com/hotkhwan/gateway-api/config"
	"github.com/hotkhwan/gateway-api/controllers/authapi"
	"github.com/hotkhwan/gateway-api/internal/middleware"
	"github.com/hotkhwan/gateway-api/internal/repo/authzrepo"

	"github.com/gofiber/fiber/v3"
)

func RegisterAuthRoutes(router fiber.Router) {
	router.Route("/auth", func(r fiber.Router) {

		// ✅ ใส่ Audit ก่อนเสมอ (จะ audit เฉพาะ POST/PUT/PATCH/DELETE ตาม middleware ของคุณ)
		r.Use(middleware.Audit(middleware.AuditConfig{
			AuditRepo: authzrepo.NewAuditLogRepo(config.DB),
			AuditChan: nil,
			BasePath:  os.Getenv("BASE_PATH"),
			Effective: middleware.EffectiveGetter(),
		}))

		// ---------- Guards (405 สำหรับเมธอดที่ไม่รองรับ) ----------
		r.All("/signin", middleware.AllowMethods("POST"))
		r.All("/refreshToken", middleware.AllowMethods("POST"))
		r.All("/resetPassword", middleware.AllowMethods("POST"))
		r.All("/signout", middleware.AllowMethods("POST"))
		r.All("/introspect", middleware.AllowMethods("GET"))
		r.All("/oauth", middleware.AllowMethods("POST"))

		// ---------- แบ่งกลุ่ม ----------
		// 1) Public (ไม่ต้องมี JWT) — ปกติ signin, refreshToken(ขึ้นกับดีไซน์), resetPassword
		public := r.Group("")
		// ถ้าอยาก “พยายาม” อ่าน user จาก JWT ถ้ามี (แต่ไม่บังคับ) ใช้ Optional ด้านล่างแทน public := r.Group("", middleware.AuthBearerOptional())
		public.Post("/signin", authapi.Signin)
		public.Post("/refreshToken", authapi.RefreshToken)   // ถ้า refresh token อยู่ใน cookie/body มักไม่ใช้ AuthBearer
		public.Post("/resetPassword", authapi.ResetPassword) // ถ้าเป็นลิงก์ email token ก็มักไม่ใช้ AuthBearer
		public.Post("/oauth", authapi.OAuthCallback)
		// 2) Protected (ต้องมี JWT) — introspect, signout ฯลฯ
		protected := r.Group("", middleware.AuthBearer())
		protected.Get("/introspect", authapi.Introspect)
		protected.Post("/signout", authapi.Signout)
	})
}
