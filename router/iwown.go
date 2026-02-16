// router/watchapi.go
package router

import (
	"github.com/hotkhwan/gateway-api/controllers/webhooks/iot/iwownapi"

	"github.com/gofiber/fiber/v2"
)

// RegisterHookIwownAPI ลงที่ main เหมือนเดิม
// เช่น router.RegisterHookIwownAPI(app.Group("/webhooks")) แล้วได้ path = /webhooks/4g/...
func RegisterHookIwownAPI(r fiber.Router) {
	g := r.Group("/api")
	// ====== iwown API ตาม spec ======
	// POST /4g/pb/upload
	g.Post("pb/upload", iwownapi.HandleIwownPbUpload)

	// POST /4g/alarm/upload
	g.Post("alarm/upload", iwownapi.HandleIwownAlarmUpload)

	// POST /4g/call_log/upload
	g.Post("call_log/upload", iwownapi.HandleIwownCallLogUpload)

	// POST /4g/deviceinfo/upload
	g.Post("deviceinfo/upload", iwownapi.HandleIwownDeviceInfoUpload)

	// POST /4g/status/notify
	g.Post("status/notify", iwownapi.HandleIwownStatusNotify)

	// GET /4g/health/sleep?deviceid=...&sleep_date=YYYY-MM-DD
	g.Get("health/sleep", iwownapi.HandleIwownHealthSleep)

	// health check
	g.Get("healthz", func(c *fiber.Ctx) error {
		return c.SendStatus(fiber.StatusOK)
	})
}
