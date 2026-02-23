// controllers/devapi/create.go
package devapi

import (
	"github.com/hotkhwan/gateway-api/internal/services/devsvc"
	"github.com/hotkhwan/gateway-api/models/devmod"
	"github.com/hotkhwan/gateway-api/models/gmod"
	"github.com/hotkhwan/gateway-api/utils"
	"github.com/hotkhwan/gateway-api/utils/authutil"
	"github.com/hotkhwan/gateway-api/utils/httputil"
	"github.com/hotkhwan/gateway-api/utils/traceutil"

	"github.com/gofiber/fiber/v2"
)

// DeviceCreate godoc
// @Summary Create a new device
// @Description Insert a new camera device to the database
// @Tags Devices
// @Accept  json
// @Produce  json
// @Param device body devmod.DeviceRequest true "Device object"
// @Success 201 {object} gmod.SuccessMessageResponse
// @Failure 400 {object} gmod.BadRequestResponse
// @Failure 401 {object} gmod.UnauthorizedResponse
// @Failure 500 {object} gmod.InternalErrorResponse
// @Router /devices [post]
// @Security BearerAuth
func DeviceCreate(c *fiber.Ctx) error {
	ctx, span, log := traceutil.Start(c.UserContext(), "github.com/hotkhwan/gateway-api/devapi", "devices.DeviceCreate", "devapi", "DeviceCreate")
	defer span.End()
	var req devmod.DeviceRequest

	if err := c.BodyParser(&req); err != nil {
		log.Error().
			Err(err).Msg("❌ [DeviceCreate] Invalid request body")
		return httputil.FailBadRequest(c, "INVALID_BODY", err.Error())
	}

	log.Info().
		Interface("payload", req).
		Msg("📥 [DeviceCreate] Incoming request to create device")

	// ตรวจสอบฟิลด์ที่จำเป็น
	missing := []string{}
	if req.Name == "" {
		missing = append(missing, "name")
	}
	if req.District == "" {
		missing = append(missing, "district")
	}
	if req.User == "" {
		missing = append(missing, "user")
	}
	if req.Password == "" {
		missing = append(missing, "password")
	}
	if req.URL == "" {
		missing = append(missing, "url")
	}
	if req.Brand == "" {
		missing = append(missing, "brand")
	}
	if req.Lat == nil {
		missing = append(missing, "lat")
	}
	if req.Lng == nil {
		missing = append(missing, "lng")
	}

	if len(missing) > 0 {
		log.Error().
			Strs("missingFields", missing).
			Msg("❌ [DeviceCreate] Missing required fields")
		return httputil.FailBadRequest(c, "MISSING_FIELDS", "Missing required fields: "+authutil.JoinFields(missing))
	}

	device := devmod.Device{
		Name:     req.Name,
		User:     req.User,
		Password: req.Password,
		URL:      req.URL,
		District: req.District,
		Lat:      *req.Lat,
		Lng:      *req.Lng,
		Brand:    req.Brand,
		Status:   true,
		State:    "create",
		CreateAt: utils.CurrentDateTime(),
	}

	_, err := devsvc.DeviceCreate(ctx, device)
	if err != nil {
		log.Error().
			Err(err).Msg("❌ [DeviceCreate] Failed to create device")
		return httputil.FailInternal(c, "CREATE_FAILED", err.Error())
	}

	log.Info().
		Str("name", device.Name).Msg("✅ [DeviceCreate] Device created successfully")
	return gmod.SendCreatedMessage(c, "DEVICE_CREATE_SUCCESS", "Device created successfully")
}
