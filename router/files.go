// router/image.go
package router

import (
	"github.com/hotkhwan/gateway-api/controllers/fileapi"
	"github.com/hotkhwan/gateway-api/internal/middleware"

	"github.com/gofiber/fiber/v3"
)

func RegisterImageProxy(r fiber.Router) {
	// /image/* → no auth, public buckets only
	r.Get("/image/*", fileapi.ProxyEventImage)

	// /files/* → auth required, all buckets
	r.Use("/files", middleware.AuthBearerOrCookie())
	r.Get("/files/*", fileapi.ProxyFiles)
}
