// controllers/kctrlapi/update.go
package kctrlapi

import (
	"strings"

	"github.com/hotkhwan/gateway-api/internal/services/kctrlsvc"
	"github.com/hotkhwan/gateway-api/models/kctrlmod"
	"github.com/hotkhwan/gateway-api/utils/httputil"
	"github.com/hotkhwan/gateway-api/utils/traceutil"

	"github.com/gofiber/fiber/v3"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// UpdateKcontrolDevice godoc
// @Summary      Update a Kcontrol device
// @Description  Sends update instructions to a device
// @Tags         Kcontrol
// @Accept       json
// @Produce      json
// @Param        id    path      string                  true  "Device ID"
// @Param        body  body      kctrlmod.EventMessage       true  "Update Payload"
// @Success      200   {object}  gmod.SuccessMessageResponse
// @Failure      400   {object}  gmod.BadRequestResponse
// @Failure      500   {object}  gmod.InternalErrorResponse
// @Security     BearerAuth
// @Router       /kcontrol/{id} [patch]
func UpdateDevice(c fiber.Ctx) error {
	ctx, end, log := traceutil.StartLite(c, "gateway.kctrlapi", "devices.UpdateDevice", "kctrlapi", "UpdateDevice")
	defer end()

	idStr := c.Params("id")
	if idStr == "" {
		log.Warn().
			Msg("❌ [UpdateDevice] Missing ID in path")
		return httputil.FailBadRequest(c, "MISSING_ID", "Missing id in path")
	}

	objID, err := primitive.ObjectIDFromHex(idStr)
	if err != nil {
		log.Warn().
			Str("id", idStr).
			Msg("❌ [UpdateDevice] Invalid object ID")
		return httputil.FailBadRequest(c, "INVALID_ID", "Invalid object ID")
	}

	var payload kctrlmod.UpdatePayload
	if err := c.Bind().Body(&payload); err != nil {
		log.Warn().
			Err(err).
			Str("id", idStr).
			Msg("❌ [UpdateDevice] Invalid payload")
		return httputil.FailBadRequest(c, "INVALID_PAYLOAD", "Invalid payload")
	}

	if err := kctrlsvc.UpdateDevice(ctx, objID, payload); err != nil {
		log.Error().
			Err(err).
			Str("id", idStr).
			Msg("❌ [UpdateDevice] Update failed")
		return httputil.FailInternal(c, "UPDATE_FAILED", err.Error())
	}

	log.Info().
		Str("id", idStr).
		Msg("✅ [UpdateDevice] Device updated successfully")
	return httputil.MessageOK(c, "Device updated successfully", "KCONTROL_UPDATE_SUCCESS")
}

// AckDeviceAlarm godoc
// @Summary      Acknowledge Kcontrol alarm
// @Description  Updates MongoDB by device ID and publishes ACK to MQTT using hwId
// @Tags         Kcontrol
// @Accept       json
// @Produce      json
// @Param        id    path      string                  true  "Device ID"
// @Param        body  body      kctrlmod.EventMessage       true  "Acknowledge Payload"
// @Success      200   {object}  gmod.SuccessMessageResponse
// @Failure      400   {object}  gmod.BadRequestResponse
// @Failure      500   {object}  gmod.InternalErrorResponse
// @Security     BearerAuth
// @Router       /kcontrol/{id}/ack [patch]
func AckDeviceAlarm(c fiber.Ctx) error {
	ctx, end, log := traceutil.StartLite(c, "gateway.kctrlapi", "alarms.AckDeviceAlarm", "kctrlapi", "AckDeviceAlarm")
	defer end()

	deviceId := c.Params("id")
	var payload kctrlmod.EventMessage

	if err := c.Bind().Body(&payload); err != nil {
		log.Warn().
			Err(err).
			Str("id", deviceId).
			Msg("❌ [AckDeviceAlarm] Invalid payload")
		return httputil.FailBadRequest(c, "INVALID_BODY", "Invalid payload")
	}
	if payload.HwID == "" {
		log.Warn().
			Str("id", deviceId).
			Msg("❌ [AckDeviceAlarm] Missing hwId in payload")
		return httputil.FailBadRequest(c, "MISSING_HWID", "Missing hwId in payload")
	}

	if err := kctrlsvc.AckDeviceAlarm(ctx, deviceId, payload); err != nil {
		log.Error().
			Err(err).
			Str("id", deviceId).
			Str("hwId", payload.HwID).
			Msg("❌ [AckDeviceAlarm] Failed to process")
		return httputil.FailInternal(c, "ACK_FAILED", err.Error())
	}

	log.Info().
		Str("id", deviceId).
		Str("hwId", payload.HwID).
		Msg("✅ [AckDeviceAlarm] Alarm acknowledged")
	return httputil.MessageOK(c, "Alarm acknowledged", "KCONTROL_ACK_SUCCESS")
}

// AckAlarm godoc
// @Summary      Acknowledge Kcontrol alarm
// @Description  Updates MongoDB by device ID and publishes ACK to MQTT using hwId
// @Tags         Kcontrol
// @Accept       json
// @Produce      json
// @Param        id    path      string                  true  "Event ID"
// @Success      200   {object}  gmod.SuccessMessageResponse
// @Failure      400   {object}  gmod.BadRequestResponse
// @Failure      500   {object}  gmod.InternalErrorResponse
// @Security     BearerAuth
// @Router       /kcontrol/{id}/ack [patch]
func AckAlarm(c fiber.Ctx) error {
	ctx, end, log := traceutil.StartLite(c, "gateway.kctrlapi", "alarms.AckAlarm", "kctrlapi", "AckAlarm")
	defer end()

	alarmId := c.Params("id")

	var req kctrlmod.AckAlarmRequest
	if err := c.Bind().Body(&req); err != nil {
		return httputil.FailBadRequest(c, "BAD_REQUEST", "invalid body")
	}
	if strings.TrimSpace(req.HwID) == "" {
		return httputil.FailBadRequest(c, "INVALID_HWID", "hwId is required")
	}
	if !req.Acknowledged {
		// คุณจะ allow un-ack ก็ได้ แต่ส่วนใหญ่ไม่ควร
		return httputil.FailBadRequest(c, "INVALID_ACK", "acknowledged must be true")
	}

	if err := kctrlsvc.AckAlarm(ctx, alarmId, req, c); err != nil {
		log.Error().
			Err(err).
			Str("id", alarmId).
			Str("hwId", req.HwID).
			Msg("❌ [AckAlarm] Failed to process")
		return httputil.FailInternal(c, "ACK_FAILED", err.Error())
	}

	log.Info().
		Str("id", alarmId).
		Str("hwId", req.HwID).
		Msg("✅ [AckAlarm] Alarm acknowledged")

	return httputil.MessageOK(c, "Alarm acknowledged", "KCONTROL_ACK_SUCCESS")
}

// ResetKcontrolStats godoc
// @Summary      Reset stats of a Kcontrol device
// @Description  Reset stats (online, offline, warning, alarm) by device ID
// @Tags         Kcontrol
// @Accept       json
// @Produce      json
// @Param        id    path      string  true  "Device ID"
// @Success      200   {object}  gmod.SuccessMessageResponse
// @Failure      400   {object}  gmod.BadRequestResponse
// @Failure      500   {object}  gmod.InternalErrorResponse
// @Security     BearerAuth
// @Router       /kcontrol/{id}/reset-stats [patch]
func ResetStats(c fiber.Ctx) error {
	ctx, end, log := traceutil.StartLite(c, "gateway.kctrlapi", "alarms.ResetStats", "kctrlapi", "ResetStats")
	defer end()

	deviceId := c.Params("id")
	if deviceId == "" {
		log.Warn().
			Msg("❌ [ResetStats] Missing device ID")
		return httputil.FailBadRequest(c, "MISSING_ID", "Missing device id")
	}

	if err := kctrlsvc.ResetStats(ctx, deviceId); err != nil {
		log.Error().
			Err(err).
			Str("id", deviceId).
			Msg("❌ [ResetStats] Failed to reset stats")
		return httputil.FailInternal(c, "RESET_FAILED", err.Error())
	}

	log.Info().
		Str("id", deviceId).
		Msg("✅ [ResetStats] Device stats reset successfully")
	return httputil.MessageOK(c, "Device stats reset successfully", "KCONTROL_STATS_RESET")
}
