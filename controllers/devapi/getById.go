// controllers/devapi/getById.go
package devapi

import (
	"errors"
	"strings"

	"github.com/hotkhwan/gateway-api/internal/services/devsvc"
	"github.com/hotkhwan/gateway-api/models/devmod"
	"github.com/hotkhwan/gateway-api/utils/httputil"
	"github.com/hotkhwan/gateway-api/utils/traceutil"

	"github.com/gofiber/fiber/v2"
	"go.mongodb.org/mongo-driver/mongo"
)

// GetDeviceByID godoc
// @Summary Get device by ID
// @Description Return device detail by ID
// @Tags Devices
// @Produce  json
// @Param id path string true "Device ID"
// @Success 200 {object} devmod.DeviceDetailSwagger
// @Failure 404 {object} gmod.ErrorMessageResponse
// @Router /devices/{id} [get]
// @Security BearerAuth
func DeviceGetByID(c *fiber.Ctx) error {
	ctx, end, log := traceutil.StartLite(c.UserContext(), "github.com/hotkhwan/gateway-api/devapi", "devapi.DeviceGetByID", "devapi", "DeviceGetByID")
	defer end()

	deviceId := strings.TrimSpace(c.Params("deviceId"))
	if deviceId == "" {
		return httputil.FailBadRequest(c, "BAD_REQUEST", "Invalid deviceId")
	}

	dev, err := devsvc.DeviceGetByID(ctx, deviceId)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return httputil.FailNotFound(c, "NOT_FOUND", "Device not found")
		}
		log.Error().Err(err).Str("deviceId", deviceId).Msg("❌ DeviceGetByID failed")
		return httputil.FailInternal(c, "INTERNAL_ERROR", "Failed to get device")
	}

	return c.JSON(struct {
		Code    string         `json:"code"`
		Message string         `json:"message"`
		Status  bool           `json:"status"`
		Detail  *devmod.Device `json:"detail"`
	}{
		Code:    "SUCCESS",
		Message: "Device retrieved successfully",
		Status:  true,
		Detail:  dev,
	})
}
