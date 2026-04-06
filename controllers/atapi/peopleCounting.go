// controllers/atapi/peopleCounting.go
package atapi

import (
	"errors"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/golang-jwt/jwt/v4"

	"github.com/hotkhwan/gateway-api/internal/services/atasvc"
	"github.com/hotkhwan/gateway-api/models/aimodel"
	"github.com/hotkhwan/gateway-api/utils/httputil"
	"github.com/hotkhwan/gateway-api/utils/traceutil"
)

// queryCSV: CSV-only: supports camera=cam1,cam2 and direction=in,out
func queryCSV(c fiber.Ctx, key string) []string {
	val := strings.TrimSpace(c.Query(key, ""))
	if val == "" {
		return nil
	}

	if v, err := url.QueryUnescape(val); err == nil {
		val = strings.TrimSpace(v)
	}

	parts := strings.Split(val, ",")
	out := make([]string, 0, len(parts))
	seen := map[string]struct{}{}

	for _, p := range parts {
		s := strings.TrimSpace(p)
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

// defaultDateTimeRangeBangkokToNowUTC: default dateTime: today 00:00:00 (Asia/Bangkok) -> now, both in UTC RFC3339
func defaultDateTimeRangeBangkokToNowUTC() string {
	loc, err := time.LoadLocation("Asia/Bangkok")
	if err != nil {
		loc = time.FixedZone("Asia/Bangkok", 7*3600)
	}
	nowLocal := time.Now().In(loc)
	startLocal := time.Date(nowLocal.Year(), nowLocal.Month(), nowLocal.Day(), 0, 0, 0, 0, loc)

	startUTC := startLocal.UTC().Format(time.RFC3339)
	endUTC := time.Now().UTC().Format(time.RFC3339)
	return startUTC + "," + endUTC
}

// PeopleCountingSummary godoc
// @Summary PeopleCounting summary + list
// @Description Summary (in/out/total) + list events with pagination. Filter by camera/dateTime/direction.
// @Tags ATA
// @Accept  json
// @Produce json
// @Param dateTime query string false "Date range: RFC3339,RFC3339 or YYYY-MM-DD,YYYY-MM-DD (if omitted: today 00:00:00 (Asia/Bangkok) to now)"
// @Param camera query string false "CSV only: camera=cam1,cam2"
// @Param direction query string false "CSV only: direction=in,out"
// @Param sn query string false "Filter by device SN"
// @Param channelId query int false "Filter by channelId"
// @Param search query string false "Regex search by camera name (address) / zone"
// @Param directionCode query int false "Filter by directionCode 1=in 2=out (overrides direction)"
// @Param page query int false "Page number"
// @Param perPages query int false "Items per page (max 200)"
// @Param sortOrder query string false "asc or desc"
// @Success 200 {object} aimodel.PeopleCountingSummaryResponse
// @Failure 400 {object} gmod.BadRequestResponse
// @Failure 500 {object} gmod.InternalErrorResponse
// @Router /atapi/peopleCounting/summary [get]
// @Security BearerAuth
func PeopleCountingSummary(c fiber.Ctx) error {
	ctx, end, log := traceutil.StartLite(c, "gateway.atapi", "ata.PeopleCountingSummary", "atapi", "PeopleCountingSummary")
	defer end()

	rawUser := c.Locals("user")
	log.Debug().Interface("auth_user_locals", rawUser).Msg("[PeopleCountingSummary] user from context")

	var claims map[string]interface{}
	switch v := rawUser.(type) {
	case map[string]interface{}:
		claims = v
	case jwt.MapClaims:
		claims = map[string]interface{}(v)
	case nil:
		log.Warn().Msg("[PeopleCountingSummary] c.Locals(\"user\") is nil")
	default:
		log.Warn().Msgf("[PeopleCountingSummary] c.Locals(\"user\") is %T, cannot cast", v)
	}

	dateTime := strings.TrimSpace(c.Query("dateTime", ""))
	if dateTime == "" {
		dateTime = defaultDateTimeRangeBangkokToNowUTC()
	} else {
		if v, err := url.QueryUnescape(dateTime); err == nil {
			dateTime = strings.TrimSpace(v)
		}
	}

	cameras := queryCSV(c, "camera")
	directions := queryCSV(c, "direction")

	directionCodeStr := strings.TrimSpace(c.Query("directionCode", ""))
	var directionCode int64 = 0
	if directionCodeStr != "" {
		v, err := strconv.ParseInt(directionCodeStr, 10, 64)
		if err != nil {
			return httputil.FailBadRequest(c, "directionCode must be int")
		}
		if v != 1 && v != 2 {
			return httputil.FailBadRequest(c, "directionCode must be 1 (in) or 2 (out)")
		}
		directionCode = v
	}

	page, _ := strconv.Atoi(c.Query("page", "1"))
	perPages, _ := strconv.Atoi(c.Query("perPages", "10"))
	if perPages <= 0 {
		perPages = 10
	}
	if perPages > 200 {
		perPages = 200
	}

	sortOrder := strings.ToLower(c.Query("sortOrder", "desc"))
	if sortOrder != "asc" && sortOrder != "desc" {
		sortOrder = "desc"
	}

	sn := strings.TrimSpace(c.Query("sn", ""))
	channelIdStr := strings.TrimSpace(c.Query("channelId", ""))
	search := strings.TrimSpace(c.Query("search", ""))

	var channelId int64 = 0
	if channelIdStr != "" {
		v, err := strconv.ParseInt(channelIdStr, 10, 64)
		if err != nil {
			return httputil.FailBadRequest(c, "channelId must be int")
		}
		channelId = v
	}

	if claims != nil {
		if role, ok := claims["role"].(string); ok && role != "" {
			log.Debug().Str("role", role).Msg("[PeopleCountingSummary] role from claims")
		}
	}

	log.Debug().
		Str("dateTime", dateTime).
		Strs("camera", cameras).
		Strs("direction", directions).
		Int64("directionCode", directionCode).
		Msg("[PeopleCountingSummary] filters")

	// legacy single direction string (keep)
	directionLegacy := strings.TrimSpace(c.Query("direction", ""))

	resp, err := atasvc.PeopleCountingSummary(ctx, aimodel.PeopleCountingSummaryReq{
		DateTime:  dateTime,
		SN:        sn,
		ChannelID: channelId,
		Search:    search,

		Direction:     directionLegacy,
		DirectionCode: directionCode,

		Cameras:    cameras,
		Directions: directions,

		Page:      page,
		PerPages:  perPages,
		SortOrder: sortOrder,
	})
	if err != nil {
		log.Error().Err(err).Msg("[PeopleCountingSummary] failed")

		if errors.Is(err, atasvc.ErrBadRequest) {
			return httputil.FailBadRequest(c, err.Error())
		}

		return httputil.FailInternal(c, err.Error())
	}

	return httputil.Ok(c, resp)
}
