// router/device.go
package router

import (
	"github.com/hotkhwan/gateway-api/controllers/devapi"
	"github.com/hotkhwan/gateway-api/internal/middleware"

	"github.com/gofiber/fiber/v2"
)

func RegisterDeviceRoutes(router fiber.Router) {
	router.Route("/devices", func(r fiber.Router) {
		r.Use(middleware.AuthBearer())
		r.Get("", middleware.AllowOnly("GET"), devapi.DevicesList)
		r.Get("/permission", middleware.AllowOnly("GET"), devapi.DevicesListWithPermission)
		r.Get("/:deviceId", middleware.AllowOnly("GET"), devapi.DeviceGetByID)
		r.Post("/svms/sync", middleware.AllowOnly("POST"), devapi.DeviceSyncFromSVMS)
		r.Post("", middleware.AllowOnly("POST"), devapi.DeviceCreate)
		r.All("/import", middleware.AllowOnly("POST"), devapi.DeviceTemplate)
		r.Patch("/:id", middleware.AllowOnly("PATCH"), devapi.DeviceUpdate)
		r.Delete("/:id", middleware.AllowOnly("DELETE"), devapi.DeviceDelete)
	})
}

// func RegisterFacesCCTVRoutes(router fiber.Router) {
// 	// ใช้ AuthBearer กับ route นี้
// 	router.All("/upload", middleware.AllowOnly("POST"), middleware.AuthBearer(), controllers.UploadFaceData)
// }
