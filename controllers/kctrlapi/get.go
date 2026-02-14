// controllers/kctrlapi/get.go
package kctrlapi

import (
	"strconv"
	"strings"

	"klynx/internal/services/kctrlsvc"
	"klynx/models/gmod"
	"klynx/utils"
	"klynx/utils/httputil"
	"klynx/utils/traceutil"

	"github.com/gofiber/fiber/v2"
)

// ListKcontrolDevices godoc
// @Summary      Receive list of Kcontrol devices
// @Description  Returns paginated Kcontrol device list
// @Tags         Kcontrol
// @Accept       json
// @Produce      json
// @Param        page       query     int     false  "Page number"
// @Param        perPages   query     int     false  "Items per page"
// @Param        sortOrder  query     string  false  "asc or desc"
// @Param        id         query     string  false  "Device ID"
// @Param        hwid       query     string  false  "Device HWID"
// @Param        code       query     string  false  "Device Code"
// @Param        search     query     string  false  "Search by name or Devices"
// @Success      200   {object}  kctrlmod.PaginationKctrlResponse
// @Failure      400   {object}  gmod.BadRequestResponse
// @Failure      500   {object}  gmod.ErrorResponse
// @Security     BearerAuth
// @Router       /kcontrol [get]
func ListDevices(c *fiber.Ctx) error {
	ctx, span, log := traceutil.Start(c.UserContext(), "klynx/kctrlapi", "devices.ListDevices", "kctrlapi", "ListDevices")
	defer span.End()

	page, _ := strconv.Atoi(c.Query("page", "1"))
	perPages, _ := strconv.Atoi(c.Query("perPages", "10"))
	sortOrder := strings.ToLower(c.Query("sortOrder", "desc"))
	if sortOrder != "asc" && sortOrder != "desc" {
		sortOrder = "desc"
	}
	sortField := c.Query("sortField", "dateTimeCreate")

	filters := map[string]string{}
	if id := c.Query("id"); id != "" {
		filters["id"] = id
	}
	if hwid := c.Query("hwid"); hwid != "" {
		filters["hwid"] = hwid
	}
	if code := c.Query("code"); code != "" {
		filters["code"] = code
	}
	if search := c.Query("search"); search != "" {
		filters["search"] = search
	}

	log.Debug().
		Int("page", page).
		Int("perPages", perPages).
		Str("sortField", sortField).
		Str("sortOrder", sortOrder).
		Interface("filters", filters).
		Msg("📡 [ListKcontrolDevices] Listing devices")

	details, pagination, err := kctrlsvc.ListDevices(ctx, page, perPages, filters, sortField, sortOrder)
	if err != nil {
		log.Error().Err(err).Msg("❌ [ListKcontrolDevices] Failed to fetch device list")
		return httputil.FailInternal(c, "DEVICE_LIST_FAILED", err.Error())
	}

	log.Debug().
		Int("resultCount", len(details)).
		Interface("pagination", pagination).
		Msg("✅ [ListKcontrolDevices] Device list fetched successfully")

	return gmod.SendPagination(c, details, pagination)
}

// GetKcontrolDeviceByID godoc
// @Summary      Get Kcontrol device by ID
// @Description  Returns a single Kcontrol device by its ID
// @Tags         Kcontrol
// @Accept       json
// @Produce      json
// @Param        id   path      string  true  "Device ID"
// @Success      200  {object}  kctrlmod.DeviceResponse
// @Failure      400  {object}  gmod.BadRequestResponse
// @Failure      404  {object}  gmod.ErrorResponse
// @Failure      500  {object}  gmod.ErrorResponse
// @Security     BearerAuth
// @Router       /kcontrol/{id} [get]
func DeviceGetByID(c *fiber.Ctx) error {
	ctx, span, log := traceutil.Start(c.UserContext(), "klynx/kctrlapi", "devices.DeviceGetByID", "kctrlapi", "DeviceGetByID")
	defer span.End()

	id := c.Params("id")
	if id == "" {
		return httputil.FailBadRequest(c, "INVALID_ID", "Device ID is required")
	}

	log.Info().Str("id", id).Msg("📡 [DeviceGetByID] Request to get device by ID")

	detail, err := kctrlsvc.DeviceGetByID(ctx, id)
	if err != nil {
		log.Error().Err(err).Str("id", id).Msg("❌ [DeviceGetByID] Failed to fetch device")
		return httputil.FailNotFound(c, "DEVICE_NOT_FOUND", err.Error())
	}

	log.Info().Str("id", id).Msg("✅ [DeviceGetByID] Device fetched successfully")
	return c.Status(fiber.StatusOK).JSON(detail)
}

// ListKcontrolAlarms godoc
// @Summary      Receive list of Kcontrol alarms (from events store)
// @Description  Returns paginated alarm events (kind=alarms) with device details, backed by kcontrol_events
// @Tags         Kcontrol
// @Accept       json
// @Produce      json
// @Param        page       query     int     false  "Page number"
// @Param        perPages   query     int     false  "Items per page"
// @Param        sortOrder  query     string  false  "asc or desc"
// @Param        search     query     string  false  "Search by message or name"
// @Param        hwid       query     string  false  "Filter by hardware id"
// @Param        datetime   query     string  false  "Filter by datetime range (RFC3339 format, e.g., 2023-01-02T15:04:05Z/2023-01-03T15:04:05Z)"
// @Param        unacked    query     string  false  "Filter unacknowledged alarms only (true|false)"
// @Success      200   {object}  kctrlmod.PaginationAlarmsResponse
// @Failure      400   {object}  gmod.BadRequestResponse
// @Router       /kcontrol/alarms [get]
func ListAlarms(c *fiber.Ctx) error {
	ctx, span, log := traceutil.Start(c.UserContext(), "klynx/kctrlapi", "alarms.ListAlarms", "kctrlapi", "ListAlarms")
	defer span.End()

	page, _ := strconv.Atoi(c.Query("page", "1"))
	perPages, _ := strconv.Atoi(c.Query("perPages", "10"))
	sortOrder := strings.ToLower(c.Query("sortOrder", "desc"))
	if sortOrder != "asc" && sortOrder != "desc" {
		sortOrder = "desc"
	}
	sortField := c.Query("sortField", "datetime")

	filters := map[string]string{
		"kind":    "alarms",
		"search":  c.Query("search"),
		"hwId":    c.Query("hwid"), // ✅ normalize เป็น hwId ส่งให้ service
		"unacked": c.Query("unacked"),
	}

	// ✅ dateTime range
	if from, to, ok := utils.ParseDateTimeRangeQuery(c.Query("dateTime")); ok {
		filters["from"] = from
		filters["to"] = to
	} else {
		// default: today 00:00 (BKK) -> now (UTC)
		fromUtc, toUtc := utils.DefaultDateTimeRangeBangkokToNowUTC()
		filters["from"] = fromUtc
		filters["to"] = toUtc
	}

	log.Debug().
		Int("page", page).
		Int("perPages", perPages).
		Str("sortOrder", sortOrder).
		Str("sortField", sortField).
		Interface("filters", filters).
		Msg("📡 [ListAlarms] Request to list alarm records")

	details, pagination, err := kctrlsvc.ListAlarms(ctx, page, perPages, filters, sortField, sortOrder)
	if err != nil {
		log.Error().Err(err).Msg("❌ [ListAlarms] Failed to fetch alarms")
		return httputil.FailInternal(c, "ALARM_LIST_FAILED", err.Error())
	}

	return gmod.SendPagination(c, details, pagination)
}

// GetKcontrolAlarmByID godoc
// @Summary      Get Kcontrol alarm by ID
// @Description  Returns a single alarm document from kcontrol_alarms by its Mongo ObjectID (excluding soft-deleted)
// @Tags         Kcontrol
// @Accept       json
// @Produce      json
// @Param        id   path      string  true  "Alarm ID (Mongo ObjectID)"
// @Success      200  {object}  kctrlmod.AlarmResponseItem
// @Failure      400  {object}  gmod.BadRequestResponse
// @Failure      404  {object}  gmod.ErrorResponse
// @Failure      500  {object}  gmod.ErrorResponse
// @Security     BearerAuth
// @Router       /kcontrol/alarms/{id} [get]
func ListAlarmById(c *fiber.Ctx) error {
	ctx, span, log := traceutil.Start(c.UserContext(), "klynx/kctrlapi", "alarms.ListAlarmById", "kctrlapi", "ListAlarmById")
	defer span.End()

	id := strings.TrimSpace(c.Params("id"))
	if id == "" {
		return httputil.FailBadRequest(c, "INVALID_ID", "Alarm ID is required")
	}

	log.Info().Str("id", id).Msg("📡 [ListAlarmById] Request to get alarm by ID")

	detail, err := kctrlsvc.GetAlarmById(ctx, id)
	if err != nil {
		// ทำให้เป็น 404 แบบ predictable
		if strings.Contains(strings.ToLower(err.Error()), "not found") || strings.Contains(strings.ToLower(err.Error()), "no documents") {
			return httputil.FailNotFound(c, "ALARM_NOT_FOUND", err.Error())
		}
		if strings.Contains(strings.ToLower(err.Error()), "invalid objectid") {
			return httputil.FailBadRequest(c, "INVALID_ID", err.Error())
		}

		log.Error().Err(err).Str("id", id).Msg("❌ [ListAlarmById] Failed to fetch alarm")
		return httputil.FailInternal(c, "ALARM_GET_FAILED", err.Error())
	}

	return c.Status(fiber.StatusOK).JSON(detail)
}

// ListKcontrolEvents godoc
// @Summary      List Kcontrol events (alarms + sensor)
// @Description  Returns paginated events with kind filter (from kcontrol_events)
// @Tags         Kcontrol
// @Accept       json
// @Produce      json
// @Param        page       query     int     false  "Page number"
// @Param        perPages   query     int     false  "Items per page"
// @Param        sortOrder  query     string  false  "asc or desc"
// @Param        kind       query     string  false  "sensor|alarms (optional)"
// @Param        hwid       query     string  false  "Filter by hardware id"
// @Param        search     query     string  false  "Search by name/message"
// @Success      200   {object}  kctrlmod.PaginationAlarmsResponse
// @Failure      400   {object}  gmod.BadRequestResponse
// @Router       /kcontrol/events [get]
func ListEvents(c *fiber.Ctx) error {
	ctx, span, log := traceutil.Start(c.UserContext(), "klynx/kctrlapi", "events.ListEvents", "kctrlapi", "ListEvents")
	defer span.End()

	page, _ := strconv.Atoi(c.Query("page", "1"))
	perPages, _ := strconv.Atoi(c.Query("perPages", "10"))
	sortOrder := strings.ToLower(c.Query("sortOrder", "desc"))
	if sortOrder != "asc" && sortOrder != "desc" {
		sortOrder = "desc"
	}
	sortField := c.Query("sortField", "datetime")

	filters := map[string]string{
		"kind":   c.Query("kind"), // "sensor"|"alarms" หรือว่าง = ทั้งหมด
		"hwid":   c.Query("hwid"),
		"search": c.Query("search"),
	}
	// ✅ dateTime range
	if from, to, ok := utils.ParseDateTimeRangeQuery(c.Query("dateTime")); ok {
		filters["from"] = from
		filters["to"] = to
	} else {
		// default: today 00:00 (BKK) -> now (UTC)
		fromUtc, toUtc := utils.DefaultDateTimeRangeBangkokToNowUTC()
		filters["from"] = fromUtc
		filters["to"] = toUtc
	}
	details, pagination, err := kctrlsvc.ListEvents(ctx, page, perPages, filters, sortField, sortOrder)
	if err != nil {
		log.Error().Err(err).Msg("❌ [ListEvents] Failed")
		return httputil.FailInternal(c, "EVENT_LIST_FAILED", err.Error())
	}
	return gmod.SendPagination(c, details, pagination)
}
