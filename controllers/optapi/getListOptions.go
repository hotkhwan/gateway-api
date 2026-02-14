package optapi

import (
	"github.com/gofiber/fiber/v2"

	"klynx/internal/services/optsvc"
	"klynx/models/gmod"
	"klynx/utils/httputil"
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
func GetOptions(c *fiber.Ctx) error {
	kind := c.Query("kind", "list")
	ns := c.Query("ns", "")
	name := c.Query("name", "")
	qRegion := c.Query("policeRegion", "")
	qProv := c.Query("policeProvincial", "")
	qStation := c.Query("policeStation", "")
	path := c.Query("path", "")

	// ไม่มี ns => ดึงทุก options
	if ns == "" {
		out, err := optsvc.GetAllOptions(c.Context(), kind)
		if err != nil {
			return httputil.FailInternalReason(c, "internal server error", "FAILED_TO_GET_OPTIONS")
		}
		return gmod.SendSuccess(c, out)
	}

	// 🔹 Normalize: policeRegion (รับได้ทั้ง id / code) -> id ที่แท้จริง
	if qRegion != "" {
		if regID, err := optsvc.ResolveRegionCanonicalID(c.Context(), kind, ns, qRegion); err != nil {
			return httputil.FailInternalReason(c, "internal server error", "FAILED_TO_RESOLVE_REGION")
		} else if regID != "" {
			qRegion = regID
		}
	}

	// 🔹 Normalize: policeProvincial (รับได้ทั้ง id / code) -> id ที่แท้จริง
	if qProv != "" {
		if provID, err := optsvc.ResolveProvincialCanonicalID(c.Context(), kind, ns, qProv); err != nil {
			return httputil.FailInternalReason(c, "internal server error", "FAILED_TO_RESOLVE_PROVINCIAL")
		} else if provID != "" {
			qProv = provID
		}
	}

	// --- เคสฟิลเตอร์แบบ 2 ชั้น/3 ชั้น ---

	// 1) มีแค่ region -> คืนรายการ provincial ของ region นั้น
	if qRegion != "" && qProv == "" && qStation == "" {
		provList, err := optsvc.GetNamespaceOptionsPath(c.Context(), kind, ns, "policeProvincial", qRegion)
		if err != nil {
			return httputil.FailInternalReason(c, "internal server error", "FAILED_TO_GET_POLICE_PROVINCIAL")
		}
		// ส่งแบบแบน
		return gmod.SendSuccess(c, fiber.Map{
			"policeProvincial": provList[ns]["policeProvincial"],
		})
	}

	// 2) มี region + provincial (ยังไม่เลือก station) -> คืนรายการ station ของ provincial นั้น
	if qRegion != "" && qProv != "" && qStation == "" {
		// validate ว่า provincial นี้อยู่ใต้ region ที่ส่งมา
		if regID, err := optsvc.ResolveRegionByProvincial(c.Context(), kind, ns, qProv); err != nil {
			return httputil.FailInternalReason(c, "internal server error", "FAILED_TO_RESOLVE_REGION")
		} else if regID != "" && regID != qRegion {
			return httputil.FailBadRequest(c, "MISMATCH", "policeRegion does not match policeProvincial")
		}

		stationList, err := optsvc.GetNamespaceOptionsPath(c.Context(), kind, ns, "policeStation", qProv)
		if err != nil {
			return httputil.FailInternalReason(c, "internal server error", "FAILED_TO_GET_POLICE_STATION")
		}
		return gmod.SendSuccess(c, fiber.Map{
			"policeStation": stationList[ns]["policeStation"],
		})
	}

	// 3) มี station -> resolve ย้อนกลับเพื่อความสอดคล้อง แล้วคืนลิสต์ที่เกี่ยวข้อง
	if qStation != "" {
		provID, _, err := optsvc.ResolveProvincialByStation(c.Context(), kind, ns, qStation)
		if err != nil {
			return httputil.FailBadRequest(c, "INVALID_STATION", "Invalid policeStation")
		}
		if provID == "" {
			return httputil.FailNotFound(c, "NOT_FOUND", "policeStation not found")
		}
		regID, err := optsvc.ResolveRegionByProvincial(c.Context(), kind, ns, provID)
		if err != nil || regID == "" {
			return httputil.FailNotFound(c, "NOT_FOUND", "policeRegion of this provincial not found")
		}
		// ถ้ามี qRegion/qProv ที่ผู้ใช้ยิงมา ให้ตรวจความสอดคล้อง
		if qProv != "" && qProv != provID {
			return httputil.FailBadRequest(c, "MISMATCH", "policeProvincial does not match policeStation")
		}
		if qRegion != "" && qRegion != regID {
			return httputil.FailBadRequest(c, "MISMATCH", "policeRegion does not match policeStation")
		}

		// คืน options ให้หน้าบ้านเติม dropdown ได้ครบ
		provList, err := optsvc.GetNamespaceOptionsPath(c.Context(), kind, ns, "policeProvincial", regID)
		if err != nil {
			return httputil.FailInternalReason(c, "internal server error", "FAILED_TO_GET_POLICE_PROVINCIAL")
		}
		stationList, err := optsvc.GetNamespaceOptionsPath(c.Context(), kind, ns, "policeStation", provID)
		if err != nil {
			return httputil.FailInternalReason(c, "internal server error", "FAILED_TO_GET_POLICE_STATION")
		}

		return gmod.SendSuccess(c, fiber.Map{
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
		out, err := optsvc.GetNamespaceOptions(c.Context(), kind, ns, "")
		if err != nil {
			return httputil.FailInternalReason(c, "internal server error", "FAILED_TO_GET_NAMESPACE_OPTIONS")
		}
		return gmod.SendSuccess(c, out)
	}

	// ขอ field เดียว + sub-path
	if path != "" {
		out, err := optsvc.GetNamespaceOptionsPath(c.Context(), kind, ns, name, path)
		if err != nil {
			return httputil.FailInternalReason(c, "internal server error", "FAILED_TO_GET_NAMESPACE_OPTIONS_PATH")
		}
		return gmod.SendSuccess(c, out)
	}

	// ขอ field เดียว (ไม่มี path)
	out, err := optsvc.GetNamespaceOptions(c.Context(), kind, ns, name)
	if err != nil {
		return httputil.FailInternalReason(c, "internal server error", "FAILED_TO_GET_NAMESPACE_OPTIONS")
	}
	return gmod.SendSuccess(c, out)
}
