// controllers/atapi/blacklist.go
package atapi

import (
	"strconv"
	"strings"

	"github.com/hotkhwan/gateway-api/models/aimodel"
	"github.com/hotkhwan/gateway-api/utils/httputil"
	"github.com/hotkhwan/gateway-api/utils/traceutil"

	"github.com/hotkhwan/gateway-api/internal/services/atasvc"

	"github.com/gofiber/fiber/v2"
)

// BlacklistSummary godoc
// @Summary Blacklist summary + list
// @Description Summary + list events with pagination. Filter by dateTime/sn/channelId/listType and search by camera name (address).
// @Tags ATA
// @Accept  json
// @Produce json
// @Param dateTime query string true "Date range: YYYY-MM-DD,YYYY-MM-DD"
// @Param sn query string false "Filter by device SN"
// @Param channelId query int false "Filter by channelId"
// @Param search query string false "Regex search by camera name (address)"
// @Param listType query int false "eventAttribute.listType (omit = all; if provided filters exact int, including 0)"
// @Param listName query string false "list name: blacklist|whitelist|redlist|strangers (omit = all). If both listType and listName provided, listType wins."
// @Param page query int false "Page number"
// @Param perPages query int false "Items per page (max 200)"
// @Param sortOrder query string false "asc or desc"
// @Success 200 {object} aimodel.BlacklistSummaryResponse
// @Router /atapi/blacklist/summary [get]
// @Security BearerAuth
func BlacklistSummary(c *fiber.Ctx) error {
	ctx, span, log := traceutil.Start(
		c.UserContext(),
		"github.com/hotkhwan/gateway-api/atapi",
		"ata.BlacklistSummary",
		"atapi", "BlacklistSummary",
	)
	defer span.End()

	dateTime := strings.TrimSpace(c.Query("dateTime", ""))
	sn := strings.TrimSpace(c.Query("sn", ""))
	search := strings.TrimSpace(c.Query("search", ""))
	sortOrder := strings.ToLower(c.Query("sortOrder", "desc"))
	if sortOrder != "asc" && sortOrder != "desc" {
		sortOrder = "desc"
	}

	page, _ := strconv.Atoi(c.Query("page", "1"))
	perPages, _ := strconv.Atoi(c.Query("perPages", "10"))
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

	// ✅ default: ไม่ส่ง listType = เอาทั้งหมด (ไม่ filter)
	listType := -1 // ✅ default: no filter (all)

	// 1) ถ้าส่ง listType มา -> ใช้เลย (รวมถึง 0)
	listTypeStr := strings.TrimSpace(c.Query("listType", ""))
	if listTypeStr != "" {
		v, err := strconv.Atoi(listTypeStr)
		if err != nil {
			return httputil.FailBadRequest(c, "INVALID_LIST_TYPE", "listType must be int")
		}
		listType = v
	} else {
		// 2) ถ้าไม่ส่ง listType แต่ส่ง listName -> map เป็น code
		listName := strings.TrimSpace(c.Query("listName", ""))
		if listName != "" {
			code, ok := parseListNameToCode(listName)
			if !ok {
				return httputil.FailBadRequest(c, "INVALID_LIST_NAME", "listName must be one of: blacklist, whitelist, redlist, strangers")
			}
			listType = code
		}
	}

	log.Debug().
		Str("dateTime", dateTime).
		Str("sn", sn).
		Int64("channelId", channelId).
		Str("search", search).
		Int("listType", listType).
		Int("page", page).
		Int("perPages", perPages).
		Str("sortOrder", sortOrder).
		Msg("📦 [BlacklistSummary] querying")

	resp, err := atasvc.BlacklistSummary(ctx, aimodel.BlacklistSummaryReq{
		DateTime:  dateTime,
		SN:        sn,
		ChannelID: channelId,
		Search:    search,
		ListType:  listType,
		Page:      page,
		PerPages:  perPages,
		SortOrder: sortOrder,
	})
	if err != nil {
		log.Error().Err(err).Msg("❌ [BlacklistSummary] failed")
		return httputil.FailInternal(c, "BLACKLIST_SUMMARY_FAILED", "Failed to get blacklist summary")
	}

	return c.JSON(resp)
}

func parseListNameToCode(s string) (int, bool) {
	x := strings.ToLower(strings.TrimSpace(s))
	x = strings.ReplaceAll(x, " ", "")
	x = strings.ReplaceAll(x, "_", "")
	x = strings.ReplaceAll(x, "-", "")

	switch x {
	case "blacklist":
		return 3, true
	case "redlist", "red":
		return 2, true
	case "whitelist", "white":
		// ถ้าระบบคุณถือว่า 0 และ 1 คือ whitelist ทั้งคู่
		// ให้ default ไป 1 (หรือ 0 ตามที่คุณต้องการ)
		return 1, true
	case "strangers", "stranger", "unknown":
		// NOTE: ตรงนี้ขึ้นกับ vendor ของคุณว่า strangers = code อะไร
		// ถ้า strangers ไม่ได้มี code แน่นอน แนะนำให้ "ไม่รองรับ" หรือใช้ -2 แยก logic
		return 0, true
	default:
		return 0, false
	}
}
