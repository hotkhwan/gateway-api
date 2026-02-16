// router/device.go
package router

import (
	"github.com/hotkhwan/gateway-api/controllers/mapapi"
	"github.com/hotkhwan/gateway-api/internal/middleware"

	"github.com/gofiber/fiber/v2"
)

func RegisterMapsRoutes(router fiber.Router) {
	router.Route("/map", func(r fiber.Router) {
		r.Use(middleware.AuthBearer())
		r.All("", middleware.AllowOnly("GET"), mapapi.GetMAPPosition)
		r.All("file", middleware.AllowOnly("GET"), mapapi.GetKMLFile)
		r.All("", middleware.AllowOnly("POST"), mapapi.UploadKML)
		r.All("/camera", middleware.AllowMethods("GET"), mapapi.DeviceMap)
		// r.Post("import", middleware.AllowOnly("POST"), mapapi.DeviceTemplate)
		// r.Patch("/:id", middleware.AllowOnly("PATCH"), mapapi.UpdateDevice)
		// r.Delete("/:id", middleware.AllowOnly("DELETE"), mapapi.DeleteDevice)
	})
}

// func RegisterFacesCCTVRoutes(router fiber.Router) {
// 	// ใช้ AuthBearer กับ route นี้
// 	router.All("/upload", middleware.AllowOnly("POST"), middleware.AuthBearer(), controllers.UploadFaceData)
// }
