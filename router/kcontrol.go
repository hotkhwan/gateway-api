// router/kcontrol.go
package router

import (
	"github.com/hotkhwan/gateway-api/config"
	"github.com/hotkhwan/gateway-api/controllers/kctrlapi"
	"github.com/hotkhwan/gateway-api/internal/middleware"
	"github.com/hotkhwan/gateway-api/internal/repo/authzrepo"
	"os"

	"github.com/gofiber/fiber/v2"
)

func RegisterKcontrolRoutes(router fiber.Router) {
	router.Route("/kcontrol", func(r fiber.Router) {
		r.Use(middleware.AuthBearer())
		r.Use(middleware.Audit(middleware.AuditConfig{
			AuditRepo: authzrepo.NewAuditLogRepo(config.DB),
			AuditChan: nil,
			BasePath:  os.Getenv("BASE_PATH"), // หรือ os.Getenv("BASE_PATH")
			Effective: middleware.EffectiveGetter(),
		}))

		r.Get("", middleware.AllowOnly("GET"), kctrlapi.ListDevices)
		r.Get("/alarms", middleware.AllowOnly("GET"), kctrlapi.ListAlarms)
		r.Get("/alarms/:id", middleware.AllowOnly("GET"), kctrlapi.ListAlarmById)
		// r.Patch("/alarms/:id", middleware.AllowOnly("PATCH"), kctrlapi.AckAlarm)
		r.Patch("/alarms/:id/ack", middleware.AllowOnly("PATCH"), kctrlapi.AckAlarm)
		r.Patch("/alarms/:id/resolved", middleware.AllowOnly("PATCH"), kctrlapi.ResolvedAlarm)
		r.Patch("/alarms/:id/sop", middleware.AllowOnly("PATCH"), kctrlapi.AppendAlarmSop)
		r.Delete("/alarms/:id", middleware.AllowOnly("DELETE"), kctrlapi.DeleteAlarm)
		r.Post("/alarms/bulk-delete", middleware.AllowOnly("POST"), kctrlapi.DeleteAlarmBulk)
		r.Get("/events/export", middleware.AllowOnly("GET"), kctrlapi.ExportAlarms)
		r.Get("/events/logs", middleware.AllowOnly("GET"), kctrlapi.ListAlarmsLogs)
		r.Get("/events", middleware.AllowOnly("GET"), kctrlapi.ListEvents)
		
		r.Post("", middleware.AllowOnly("POST"), kctrlapi.SendMessage)
		r.Get("/:id", middleware.AllowOnly("GET"), kctrlapi.DeviceGetByID)
		r.Patch("/:id", middleware.AllowOnly("PATCH"), kctrlapi.UpdateDevice)
		r.Patch("/:id/ack", middleware.AllowOnly("PATCH"), kctrlapi.AckDeviceAlarm)
		r.Patch("/:id/reset-stats", middleware.AllowOnly("PATCH"), kctrlapi.ResetStats)
		// r.Delete("/:id", middleware.AllowOnly("DELETE"), controllers.DeleteKcontrolDevice)
	})
}
