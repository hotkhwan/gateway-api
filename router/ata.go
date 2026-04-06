// router/ata.go
package router

import (
	"github.com/hotkhwan/gateway-api/controllers/atapi"
	ataWebhook "github.com/hotkhwan/gateway-api/controllers/webhooks/analytic/atapi"
	"github.com/hotkhwan/gateway-api/internal/middleware"

	"github.com/gofiber/fiber/v3"
)

func RegisterHookATA(r fiber.Router) {
	g := r.Group("/atahook")

	g.Post("/", ataWebhook.HandleATA)
	g.Post("", ataWebhook.HandleATA)
	g.Post("/pusher-event", ataWebhook.HandlePusherEvent)

	g.Get("/healthz", func(c fiber.Ctx) error { return c.SendStatus(fiber.StatusOK) })
}

func RegisterAPIATA(router fiber.Router) {
	router.Route("/atapi", func(r fiber.Router) {
		r.Use(middleware.AuthBearer())

		// ถ้าคุณมี DevicesList/DeviceGetByID/DeviceCreate/Update/Delete อยู่แล้ว
		// ก็ใช้ของเดิมต่อได้ ตรงนี้ผมคอมเมนต์ไว้เฉพาะส่วนใหม่
		// r.Get("", middleware.AllowOnly("GET"), ataapi.DevicesList)
		// r.Get("/:deviceId", middleware.AllowOnly("GET"), ataapi.DeviceGetByID)
		r.Post("/edge/sync", middleware.AllowOnly("POST"), atapi.DeviceSyncFromSVMS)
		// endpoint stream WSS
		r.Get("/stream/:channelId", middleware.AllowOnly("GET"), atapi.GetStreamWSS)
		// ✅ NEW: PeopleCounting summary + list
		r.Get("/peopleCounting/summary", middleware.AllowOnly("GET"), atapi.PeopleCountingSummary)
		r.Get("/notification/summary", middleware.AllowOnly("GET"), atapi.NotificationSummary)
		r.Get("/blacklist/summary", middleware.AllowOnly("GET"), atapi.BlacklistSummary)
		// r.Patch("/blacklsit/:id", middleware.AllowOnly("PATCH"), atapi.BlacklistSummaryUpdate)
		// r.Post("", middleware.AllowOnly("POST"), ataapi.DeviceCreate)
		// r.Patch("/:id", middleware.AllowOnly("PATCH"), ataapi.DeviceUpdate)
		// r.Delete("/:id", middleware.AllowOnly("DELETE"), ataapi.DeviceDelete)
	})
}
