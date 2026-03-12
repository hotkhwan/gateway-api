// controllers/sysapi/settings.go
package sysapi

import (
	"github.com/hotkhwan/gateway-api/config"
	"github.com/hotkhwan/gateway-api/utils/httputil"
	"github.com/hotkhwan/gateway-api/utils/traceutil"

	"github.com/gofiber/fiber/v2"
)

// GetSettings returns basic system settings (version, app name).
//
//	@Summary      Get system settings
//	@Description  Returns gateway version and basic system info
//	@Tags         system
//	@Security     BearerAuth
//	@Success      200  {object}  gmod.SuccessDataResponse
//	@Failure      500  {object}  gmod.ErrorResponse
//	@Router       /system/settings [get]
func GetSettings(c *fiber.Ctx) error {
	_, end, _ := traceutil.StartLite(
		c.UserContext(),
		"gateway.sysapi", "GetSettings",
		"sysapi", "GetSettings",
	)
	defer end()

	appCfg := config.LoadAppConfig()

	return httputil.Ok(c, fiber.Map{
		"appName":    appCfg.AppName,
		"appVersion": appCfg.AppVersion,
	})
}
