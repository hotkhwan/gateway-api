package optapi

import (
	"strconv"

	"github.com/gofiber/fiber/v2"

	"github.com/hotkhwan/gateway-api/internal/services/optsvc"
	"github.com/hotkhwan/gateway-api/models/gmod"
	"github.com/hotkhwan/gateway-api/models/optmod"
	"github.com/hotkhwan/gateway-api/utils/httputil"
)

// POST /options/seed/policeStation
// @Summary Seed policeStation options
// @Description options for policeStation {<parent>:[{id,title}]}; if write=true for upsert into _id="list.<ns>"
// @Tags Options
// @Accept json
// @Produce json
// @Param ns query string false "namespace (default: kwatch)"
// @Param write query bool false "เขียนเข้า Mongo หรือไม่ (default: false)"
// @Param payload body []optmod.StationRaw true "raw stations array"
// @Success 200 {object} gmod.Response
// @Router /options/seed/policeStation [post]
// @Security BearerAuth
func SeedPoliceStation(c *fiber.Ctx) error {
	ns := c.Query("ns", "kwatch")
	writeStr := c.Query("write", "false")
	write, _ := strconv.ParseBool(writeStr)

	var body []optmod.StationRaw
	if err := c.BodyParser(&body); err != nil {
		return httputil.FailBadRequest(c, "BAD_REQUEST", "invalid json body")
	}
	if len(body) == 0 {
		return httputil.FailBadRequest(c, "BAD_REQUEST", "payload must be non-empty array")
	}

	ps, err := optsvc.SeedPoliceStation(c.Context(), ns, body, write)
	if err != nil {
		return httputil.FailInternalReason(c, "internal server error", "FAILED_TO_SEED_POLICE_STATION")
	}

	// ให้รูปแบบผลลัพธ์เหมือน options endpoint เดิม
	out := map[string]map[string]any{
		ns: {
			"policeStation": ps,
		},
	}
	return gmod.SendSuccess(c, out)
}
