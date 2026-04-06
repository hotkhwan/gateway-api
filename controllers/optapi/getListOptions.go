// controllers/optapi/getListOptions.go
package optapi

import (
	"github.com/gofiber/fiber/v3"

	"github.com/hotkhwan/gateway-api/internal/services/optsvc"
	"github.com/hotkhwan/gateway-api/utils/httputil"
	"github.com/hotkhwan/gateway-api/utils/traceutil"
)

// GET /options
// @Summary Get Options
// @Description options for selected with optional filters
// @Tags Options
// @Accept json
// @Produce json
// @Param ns query string false "namespace เช่น kwatch"
// @Param name query string false "field name ภายใต้ ns"
// @Param policeRegion query string false "เช่น 1"
// @Param policeProvincial query string false "เช่น 12"
// @Success 200 {object} gmod.Response
// @Router /options [get]
// @Security BearerAuth
func GetOptions(c fiber.Ctx) error {
	_, end, log := traceutil.StartLite(c, "gateway.optapi", "GetOptions", "optapi", "GetOptions")
	defer end()

	kind := c.Query("kind", "list")
	ns := c.Query("ns", "")
	name := c.Query("name", "")
	qRegion := c.Query("policeRegion", "")
	qProv := c.Query("policeProvincial", "")
	qStation := c.Query("policeStation", "")
	path := c.Query("path", "")

	// ไม่มี ns => ดึงทุก options
	if ns == "" {
		out, err := optsvc.GetAllOptions(c, kind)
		if err != nil {
			log.Error().Err(err).Msg("failed to get options")
			return httputil.FailInternal(c, "failed to get options")
		}
		return httputil.Ok(c, out)
	}

	// 🔹 Normalize: policeRegion (รับได้ทั้ง id / code) -> id ที่แท้จริง
	if qRegion != "" {
		if regID, err := optsvc.ResolveRegionCanonicalID(c, kind, ns, qRegion); err != nil {
			log.Error().Err(err).Msg("failed to resolve region")
			return httputil.FailInternal(c, "failed to resolve region")
		} else if regID != "" {
			qRegion = regID
		}
	}

	// 🔹 Normalize: policeProvincial (รับได้ทั้ง id / code) -> id ที่แท้จริง
	if qProv != "" {
		if provID, err := optsvc.ResolveProvincialCanonicalID(c, kind, ns, qProv); err != nil {
			log.Error().Err(err).Msg("failed to resolve provincial")
			return httputil.FailInternal(c, "failed to resolve provincial")
		} else if provID != "" {
			qProv = provID
		}
	}

	// --- เคสฟิลเตอร์แบบ 2 ชั้น/3 ชั้น ---

	// 1) มีแค่ region -> คืนรายการ provincial ของ region นั้น
	if qRegion != "" && qProv == "" && qStation == "" {
		provList, err := optsvc.GetNamespaceOptionsPath(c, kind, ns, "policeProvincial", qRegion)
		if err != nil {
			log.Error().Err(err).Msg("failed to get police provincial")
			return httputil.FailInternal(c, "failed to get police provincial")
		}
		// ส่งแบบแบน
		return httputil.Ok(c, fiber.Map{
			"policeProvincial": provList[ns]["policeProvincial"],
		})
	}

	// 2) มี region + provincial (ยังไม่เลือก station) -> คืนรายการ station ของ provincial นั้น
	if qRegion != "" && qProv != "" && qStation == "" {
		// validate ว่า provincial นี้อยู่ใต้ region ที่ส่งมา
		if regID, err := optsvc.ResolveRegionByProvincial(c, kind, ns, qProv); err != nil {
			log.Error().Err(err).Msg("failed to resolve region")
			return httputil.FailInternal(c, "failed to resolve region")
		} else if regID != "" && regID != qRegion {
			return httputil.FailBadRequest(c, "policeRegion does not match policeProvincial")
		}

		stationList, err := optsvc.GetNamespaceOptionsPath(c, kind, ns, "policeStation", qProv)
		if err != nil {
			log.Error().Err(err).Msg("failed to get police station")
			return httputil.FailInternal(c, "failed to get police station")
		}
		return httputil.Ok(c, fiber.Map{
			"policeStation": stationList[ns]["policeStation"],
		})
	}

	// 3) มี station -> resolve ย้อนกลับเพื่อความสอดคล้อง แล้วคืนลิสต์ที่เกี่ยวข้อง
	if qStation != "" {
		provID, _, err := optsvc.ResolveProvincialByStation(c, kind, ns, qStation)
		if err != nil {
			return httputil.FailBadRequest(c, "Invalid policeStation")
		}
		if provID == "" {
			return httputil.FailNotFound(c, "policeStation not found")
		}
		regID, err := optsvc.ResolveRegionByProvincial(c, kind, ns, provID)
		if err != nil || regID == "" {
			return httputil.FailNotFound(c, "policeRegion of this provincial not found")
		}
		// ถ้ามี qRegion/qProv ที่ผู้ใช้ยิงมา ให้ตรวจความสอดคล้อง
		if qProv != "" && qProv != provID {
			return httputil.FailBadRequest(c, "policeProvincial does not match policeStation")
		}
		if qRegion != "" && qRegion != regID {
			return httputil.FailBadRequest(c, "policeRegion does not match policeStation")
		}

		// คืน options ให้หน้าบ้านเติม dropdown ได้ครบ
		provList, err := optsvc.GetNamespaceOptionsPath(c, kind, ns, "policeProvincial", regID)
		if err != nil {
			log.Error().Err(err).Msg("failed to get police provincial")
			return httputil.FailInternal(c, "failed to get police provincial")
		}
		stationList, err := optsvc.GetNamespaceOptionsPath(c, kind, ns, "policeStation", provID)
		if err != nil {
			log.Error().Err(err).Msg("failed to get police station")
			return httputil.FailInternal(c, "failed to get police station")
		}

		return httputil.Ok(c, fiber.Map{
			"selected": fiber.Map{
				"policeRegion":     regID,
				"policeProvincial": provID,
				"policeStation":    qStation,
			},
			"policeProvincial": provList[ns]["policeProvincial"],
			"policeStation":    stationList[ns]["policeStation"],
		})
	}

	// ---- เคสเดิมๆ: name/path/general ----

	// ns อย่างเดียว (ไม่มี name/filters) => คืนทั้ง namespace (ตัดฟิลด์หนักแล้ว)
	if ns != "" && name == "" && qRegion == "" && qProv == "" && qStation == "" {
		out, err := optsvc.GetNamespaceOptions(c, kind, ns, "")
		if err != nil {
			log.Error().Err(err).Msg("failed to get namespace options")
			return httputil.FailInternal(c, "failed to get namespace options")
		}
		return httputil.Ok(c, out)
	}

	// ขอ field เดียว + sub-path
	if path != "" {
		out, err := optsvc.GetNamespaceOptionsPath(c, kind, ns, name, path)
		if err != nil {
			log.Error().Err(err).Msg("failed to get namespace options path")
			return httputil.FailInternal(c, "failed to get namespace options path")
		}
		return httputil.Ok(c, out)
	}

	// ขอ field เดียว (ไม่มี path)
	out, err := optsvc.GetNamespaceOptions(c, kind, ns, name)
	if err != nil {
		log.Error().Err(err).Msg("failed to get namespace options")
		return httputil.FailInternal(c, "failed to get namespace options")
	}
	return httputil.Ok(c, out)
}
