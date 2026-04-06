// controllers/optapi/seedPoliceStation.go
package optapi

import (
	"strconv"

	"github.com/gofiber/fiber/v3"

	"github.com/hotkhwan/gateway-api/internal/services/optsvc"
	"github.com/hotkhwan/gateway-api/models/optmod"
	"github.com/hotkhwan/gateway-api/utils/httputil"
	"github.com/hotkhwan/gateway-api/utils/traceutil"
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
func SeedPoliceStation(c fiber.Ctx) error {
	_, end, log := traceutil.StartLite(c, "gateway.optapi", "SeedPoliceStation", "optapi", "SeedPoliceStation")
	defer end()

	ns := c.Query("ns", "kwatch")
	writeStr := c.Query("write", "false")
	write, _ := strconv.ParseBool(writeStr)

	var body []optmod.StationRaw
	if err := c.Bind().Body(&body); err != nil {
		return httputil.FailBadRequest(c, "invalid json body")
	}
	if len(body) == 0 {
		return httputil.FailBadRequest(c, "payload must be non-empty array")
	}

	ps, err := optsvc.SeedPoliceStation(c, ns, body, write)
	if err != nil {
		log.Error().Err(err).Msg("failed to seed police station")
		return httputil.FailInternal(c, "failed to seed police station")
	}

	// ให้รูปแบบผลลัพธ์เหมือน options endpoint เดิม
	out := map[string]map[string]any{
		ns: {
			"policeStation": ps,
		},
	}
	return httputil.Ok(c, out)
}
