// router/image.go
package router

import (
	"github.com/hotkhwan/gateway-api/controllers/fileapi"
	"github.com/hotkhwan/gateway-api/internal/middleware"

	"github.com/gofiber/fiber/v2"
)

func RegisterImageProxy(r fiber.Router) {
	// /api/v3/image/* → ไม่ต้อง auth (By pass)
	// g := r.Group("/image")
	// g.Get("/*", fileapi.ProxyFiles)

	// /api/v3/image/* → ต้อง auth ด้วย Bearer
	// r.Route("/image", func(rr fiber.Router) {
	// 	rr.Use(middleware.AuthBearer())
	// 	rr.Get("/*", fileapi.ProxyFiles)
	// })
	// /api/v3/files/* → ต้อง auth Cookie
	r.Route("/files", func(rr fiber.Router) {
		rr.Use(middleware.AuthBearerOrCookie()) // ใช้เฉพาะภาพ
		rr.Get("/*", fileapi.ProxyFiles)
	})
}
