// controllers/kctrlapi/alarmsLogs.go
package kctrlapi

import (
	"strings"

	"github.com/hotkhwan/gateway-api/internal/services/kctrlsvc"
	"github.com/hotkhwan/gateway-api/models/gmod"
	"github.com/hotkhwan/gateway-api/utils"
	"github.com/hotkhwan/gateway-api/utils/httputil"
	"github.com/hotkhwan/gateway-api/utils/traceutil"

	"github.com/gofiber/fiber/v2"
)

// ListAlarmsLogs godoc
// @Summary      List Kcontrol alarm audit logs
// @Description  Read alarm-related audit logs from audit_logs collection
// @Tags         Kcontrol
// @Security     BearerAuth
// @Accept       json
// @Produce      json
//
// @Param        page       query     int     false  "Page number"                  default(1)
// @Param        perPages   query     int     false  "Items per page"               default(20)
// @Param        sortField  query     string  false  "Sort field"                   default(createdAt)
// @Param        sortOrder  query     string  false  "Sort order"                   Enums(asc,desc) default(desc)
//
// @Param        alarmId    query     string  false  "Alarm ID (ObjectID hex)"
// @Param        method     query     string  false  "HTTP method filter"           Enums(PATCH,DELETE,POST,PUT)
// @Param        actorId    query     string  false  "Actor ID (user sub or client)"
// @Param        datetime   query     string  false  "Filter by datetime range (RFC3339 format, e.g., 2023-01-02T15:04:05Z/2023-01-03T15:04:05Z)"
//
// @Success      200 {object} gmod.SuccessPaginationResponse
// @Failure      400 {object} gmod.BadRequestResponse
// @Failure      401 {object} gmod.UnauthorizedResponse
// @Failure      500 {object} gmod.InternalErrorResponse
// @Router       /kcontrol/alarms/logs [get]

func ListAlarmsLogs(c *fiber.Ctx) error {
	ctx, span, log := traceutil.Start(c.UserContext(), "github.com/hotkhwan/gateway-api/kctrlapi", "alarms.ListAlarmsLogs", "kctrlapi", "ListAlarmsLogs")
	defer span.End()

	page := c.QueryInt("page", 1)
	perPages := c.QueryInt("perPages", 20)

	sortOrder := strings.ToLower(strings.TrimSpace(c.Query("sortOrder", "desc")))
	sortField := strings.TrimSpace(c.Query("sortField", "createdAt"))

	filters := map[string]string{
		"alarmId":   strings.TrimSpace(c.Query("alarmId")),
		"method":    strings.ToUpper(strings.TrimSpace(c.Query("method"))),
		"actorId":   strings.TrimSpace(c.Query("actorId")),
		"sortField": sortField,
	}

	// datetime: "fromUtc,toUtc"
	dateTime := c.Query("datetime")
	if from, to, ok := utils.ParseDateTimeRangeQuery(dateTime); ok {
		filters["from"] = from
		filters["to"] = to
	} else {
		// default: today 00:00 (Asia/Bangkok) -> now (UTC)
		fromUtc, toUtc := utils.DefaultDateTimeRangeBangkokToNowUTC()
		filters["from"] = fromUtc
		filters["to"] = toUtc
	}

	rows, pagination, err := kctrlsvc.ListAlarmsLogs(ctx, page, perPages, filters, sortOrder)
	if err != nil {
		log.Error().Err(err).Msg("ListAlarmsLogs failed")
		return httputil.FailInternal(c, "LIST_FAILED", err.Error())
	}

	return gmod.SendPaginationOK(c, rows, pagination)
}
