// controllers/atapi/notification.go
package atapi

import (
	"strconv"
	"strings"

	"github.com/hotkhwan/gateway-api/internal/services/atasvc"
	"github.com/hotkhwan/gateway-api/models/aimodel"
	"github.com/hotkhwan/gateway-api/utils/httputil"
	"github.com/hotkhwan/gateway-api/utils/traceutil"

	"github.com/gofiber/fiber/v2"
)

// BlacklistNotificationAll godoc
// @Summary Notification list (ALL events)
// @Description List events with pagination. Filter by dateTime/sn/channelId and search by camera name(zone/address). No type filter. No isDeleted filter.
// @Tags ATA
// @Accept  json
// @Produce json
// @Param dateTime query string true "Date range: YYYY-MM-DD,YYYY-MM-DD"
// @Param sn query string false "Filter by device SN"
// @Param channelId query int false "Filter by channelId"
// @Param search query string false "Regex search by camera name (address/zone) OR person name"
// @Param page query int false "Page number"
// @Param perPages query int false "Items per page (max 200)"
// @Param sortOrder query string false "asc or desc"
// @Success 200 {object} aimodel.StandardResponse
// @Router /atapi/blacklist/notification [get]
// @Security BearerAuth
func NotificationSummary(c *fiber.Ctx) error {
	ctx, span, log := traceutil.Start(
		c.UserContext(),
		"github.com/hotkhwan/gateway-api/atapi",
		"ata.NotificationSummary",
		"atapi", "NotificationSummary",
	)
	defer span.End()

	dateTime := strings.TrimSpace(c.Query("dateTime", ""))
	sn := strings.TrimSpace(c.Query("sn", ""))
	search := strings.TrimSpace(c.Query("search", ""))

	sortOrder := strings.ToLower(strings.TrimSpace(c.Query("sortOrder", "desc")))
	if sortOrder != "asc" && sortOrder != "desc" {
		sortOrder = "desc"
	}

	page, _ := strconv.Atoi(c.Query("page", "1"))
	perPages, _ := strconv.Atoi(c.Query("perPages", "10"))
	if page <= 0 {
		page = 1
	}
	if perPages <= 0 {
		perPages = 10
	}
	if perPages > 200 {
		perPages = 200
	}

	channelIdStr := strings.TrimSpace(c.Query("channelId", ""))
	var channelId int64
	if channelIdStr != "" {
		v, err := strconv.ParseInt(channelIdStr, 10, 64)
		if err != nil {
			return httputil.FailBadRequest(c, "INVALID_CHANNEL_ID", "channelId must be int")
		}
		channelId = v
	}

	// ✅ log เฉพาะ metadata (ไม่ log payload ดิบ)
	log.Debug().
		Str("dateTime", dateTime).
		Str("sn", sn).
		Int64("channelId", channelId).
		Str("search", search).
		Int("page", page).
		Int("perPages", perPages).
		Str("sortOrder", sortOrder).
		Msg("📦 [NotificationAll] querying")

	data, err := atasvc.NotificationAllSummary(ctx, aimodel.NotificationAllReq{
		DateTime:  dateTime,
		SN:        sn,
		ChannelID: channelId,
		Search:    search,
		Page:      page,
		PerPages:  perPages,
		SortOrder: sortOrder,
	})
	if err != nil {
		log.Error().Err(err).Msg("❌ [NotificationAll] failed")
		return httputil.FailInternal(c, "NOTIFICATION_ALL_FAILED", "Failed to get notification summary")
	}

	return c.JSON(aimodel.StandardResponse{
		Code:    "SUCCESS",
		Message: "ok",
		Status:  true,
		Details: data,
	})
}
