// controllers/sysapi/backup.go
package sysapi

import (
	"github.com/hotkhwan/gateway-api/utils/httputil"
	"github.com/hotkhwan/gateway-api/utils/traceutil"

	"github.com/gofiber/fiber/v3"
)

// GetBackupStatus returns the current backup status (placeholder).
//
//	@Summary      Get backup status
//	@Description  Returns the current backup status
//	@Tags         system
//	@Security     BearerAuth
//	@Success      200  {object}  gmod.SuccessDataResponse
//	@Failure      500  {object}  gmod.ErrorResponse
//	@Router       /system/backup/status [get]
func GetBackupStatus(c fiber.Ctx) error {
	_, end, _ := traceutil.StartLite(
		c,
		"gateway.sysapi", "GetBackupStatus",
		"sysapi", "GetBackupStatus",
	)
	defer end()

	return httputil.Ok(c, fiber.Map{
		"backupTime":   "",
		"backupStatus": "SUCCESS",
	})
}
