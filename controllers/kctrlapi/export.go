// controllers/kctrlapi/export.go
package kctrlapi

import (
	"github.com/hotkhwan/gateway-api/internal/services/kctrlsvc"
	"github.com/hotkhwan/gateway-api/utils/httputil"
	"github.com/hotkhwan/gateway-api/utils/traceutil"

	"github.com/gofiber/fiber/v2"
)

// ExportKcontrolAlarms godoc
// @Summary      Export Kcontrol alarms as CSV (events store)
// @Description  Download filtered alarm list as a CSV file from kcontrol_events (kind=alarms)
// @Tags         Kcontrol
// @Produce      text/csv
// @Param        dateTime  query  string  false  "Datetime range: start,end in RFC3339 format, e.g. 2025-06-13T03:14:54.660Z,2025-06-20T03:14:54.660Z"
// @Param        deviceId  query  string  false  "Device ID"
// @Success      200  {file}  csv
// @Failure      500  {object}  gmod.BadRequestResponse
// @Router       /kcontrol/alarms/export [get]
// @Security     BearerAuth
func ExportAlarms(c *fiber.Ctx) error {
	ctx, span, log := traceutil.Start(c.UserContext(), "github.com/hotkhwan/gateway-api/kctrlapi", "alarms.ExportAlarms", "kctrlapi", "ExportAlarms")
	defer span.End()

	format := c.Query("format", "csv")
	log.Info().Str("format", format).Msg("📤 [ExportAlarms] Export request received (events store)")

	if err := kctrlsvc.ExportAlarms(ctx, c); err != nil {
		log.Error().Err(err).Msg("❌ [ExportAlarms] Failed to export alarms")
		return httputil.FailInternal(c, "EXPORT_FAILED", err.Error())
	}

	log.Info().Str("format", format).Msg("✅ [ExportAlarms] Export completed successfully (events store)")
	return nil
}
