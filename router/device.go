// router/device.go
package router

import (
	"os"

	"github.com/gofiber/fiber/v2"
	"github.com/hotkhwan/gateway-api/config"
	appcontainer "github.com/hotkhwan/gateway-api/internal/app"
	"github.com/hotkhwan/gateway-api/internal/middleware"
	"github.com/hotkhwan/gateway-api/internal/repo/authzrepo"
)

// RegisterDevRegisterResourceRoutesceRoutes — device + camera + resourceGroup routes
func RegisterResourceRoutes(router fiber.Router, c *appcontainer.Container) {
	auditMiddleware := middleware.Audit(middleware.AuditConfig{
		AuditRepo: authzrepo.NewAuditLogRepo(config.DB),
		AuditChan: nil,
		BasePath:  os.Getenv("BASE_PATH"),
		Effective: middleware.EffectiveGetter(),
	})

	// ============================================================
	// /resources/cameras — Camera CRUD + import
	// resourceType = camera, BSON _id internal
	// ============================================================
	router.Route("/resources/camera", func(r fiber.Router) {
		r.Use(middleware.AuthBearer())
		r.Use(auditMiddleware)

		protected := r.Group("/", middleware.ActiveOrg())

		// single create
		protected.All("/", middleware.AllowMethods("GET", "POST"))
		protected.Get("/", c.CameraController.List)
		protected.Post("/", c.CameraController.Create)

		// bulk import (csv / xlsx)
		// ⚠️ ต้องอยู่ก่อน /:id เพื่อไม่ให้ Fiber match "import" เป็น :id
		protected.All("/import", middleware.AllowMethods("POST"))
		protected.Post("/import", c.CameraController.Import)

		// single by id
		protected.All("/:id", middleware.AllowMethods("GET", "PATCH", "DELETE"))
		protected.Get("/:id", c.CameraController.GetByID)
		protected.Patch("/:id", c.CameraController.Update)
		protected.Delete("/:id", c.CameraController.Delete)
	})

	// ============================================================
	// /resources/groups — ResourceGroup (UUID external)
	// ============================================================
	router.Route("/resources/groups", func(r fiber.Router) {
		r.Use(middleware.AuthBearer())
		r.Use(auditMiddleware)

		protected := r.Group("/", middleware.ActiveOrg())

		// CRUD
		protected.All("/", middleware.AllowMethods("GET", "POST"))
		protected.Get("/", c.ResourceGroupController.List)
		protected.Post("/", c.ResourceGroupController.Create)

		protected.All("/:id", middleware.AllowMethods("PATCH", "DELETE"))
		protected.Patch("/:id", c.ResourceGroupController.Update)
		protected.Delete("/:id", c.ResourceGroupController.Delete)

		// ---------- Devices in Group (bulk add/remove) ----------
		// POST  → bulk add cameras to group
		// PATCH → bulk remove cameras from group
		protected.All("/:groupId/devices", middleware.AllowMethods("POST", "PATCH"))
		protected.Post("/:groupId/devices", c.ResourceGroupController.AddDevices)
		protected.Patch("/:groupId/devices", c.ResourceGroupController.RemoveDevices)

		// ---------- Group ↔ OrgUnit assign ----------
		protected.All("/:groupId/assignOu", middleware.AllowMethods("POST", "DELETE"))
		protected.Post("/:groupId/assignOu", c.ResourceGroupController.AssignOU)
		protected.Delete("/:groupId/assignOu", c.ResourceGroupController.RemoveOU)
	})
}

// // router/device.go
// package router

// import (
// 	"github.com/hotkhwan/gateway-api/controllers/devapi"
// 	"github.com/hotkhwan/gateway-api/internal/middleware"

// 	"github.com/gofiber/fiber/v2"
// )

// func RegisterDeviceRoutes(router fiber.Router) {
// 	router.Route("/devices", func(r fiber.Router) {
// 		r.Use(middleware.AuthBearer())
// 		r.Get("", middleware.AllowOnly("GET"), devapi.DevicesList)
// 		r.Get("/permission", middleware.AllowOnly("GET"), devapi.DevicesListWithPermission)
// 		r.Get("/:deviceId", middleware.AllowOnly("GET"), devapi.DeviceGetByID)
// 		r.Post("/svms/sync", middleware.AllowOnly("POST"), devapi.DeviceSyncFromSVMS)
// 		r.Post("", middleware.AllowOnly("POST"), devapi.DeviceCreate)
// 		r.All("/import", middleware.AllowOnly("POST"), devapi.DeviceTemplate)
// 		r.Patch("/:id", middleware.AllowOnly("PATCH"), devapi.DeviceUpdate)
// 		r.Delete("/:id", middleware.AllowOnly("DELETE"), devapi.DeviceDelete)
// 	})
// }

// // func RegisterFacesCCTVRoutes(router fiber.Router) {
// // 	// ใช้ AuthBearer กับ route นี้
// // 	router.All("/upload", middleware.AllowOnly("POST"), middleware.AuthBearer(), controllers.UploadFaceData)
// // }
