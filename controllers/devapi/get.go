// controllers/devapi/get.go
package devapi

import (
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v4"

	"github.com/hotkhwan/gateway-api/internal/services/devsvc"
	"github.com/hotkhwan/gateway-api/models/devmod"
	"github.com/hotkhwan/gateway-api/utils/httputil"
	"github.com/hotkhwan/gateway-api/utils/traceutil"
)

// DevicesList godoc
// @Summary List all devices
// @Description Get devices with pagination and optional search
// @Tags Devices
// @Accept  json
// @Produce  json
// @Param page query int false "Page number"
// @Param perPages query int false "Items per page"
// @Param sortField query string false "Sort field name"
// @Param sortOrder query string false "asc or desc"
// @Param id query string false "Filter by ID"
// @Param code query int false "Filter by code"
// @Param search query string false "Search by name or code"
// @Success 200 {object} devmod.DeviceListResponse
// @Router /devices [get]
// @Security BearerAuth
func DevicesList(c *fiber.Ctx) error {
	ctx, end, log := traceutil.StartLite(c.UserContext(), "gateway.devapi", "DevicesList", "devapi", "DevicesList")
	defer end()

	// 👀 ดูว่าปัจจุบัน middleware ยัดอะไรไว้ใน Locals("user")
	rawUser := c.Locals("user")
	log.Debug().
		Interface("auth_user_locals", rawUser).
		Msg("🔐 [DevicesList] user from context")

	// แปลงเป็น map[string]interface{} ให้ devapi ใช้ง่าย
	var claims map[string]interface{}
	switch v := rawUser.(type) {
	case map[string]interface{}:
		claims = v
	case jwt.MapClaims:
		claims = map[string]interface{}(v)
	case nil:
		log.Warn().Msg("⚠️ [DevicesList] c.Locals(\"user\") is nil")
	default:
		log.Warn().
			Msgf("⚠️ [DevicesList] c.Locals(\"user\") is type %T, cannot cast", v)
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
	sortField := c.Query("sortField", "dateTimeCreate")

	filters := map[string]string{}
	if id := c.Query("id"); id != "" {
		filters["id"] = id
	}
	if code := c.Query("code"); code != "" {
		filters["code"] = code
	}
	if search := c.Query("search"); search != "" {
		filters["search"] = search
	}

	// 🔐 map JWT → subjectType/subjectId สำหรับ Permify
	if claims != nil {
		if role, ok := claims["role"].(string); ok && role != "" {
			// ใช้ role เป็น subjectId (เราทำ tuple เป็น role:user / role:mgt แล้ว)
			filters["subjectType"] = "role"
			filters["subjectId"] = role // "user", "mgt"
		}
		if sub, ok := claims["sub"].(string); ok && sub != "" {
			filters["userId"] = sub // เผื่ออยากใช้ filter per-user ภายหลัง
		}
	} else {
		log.Warn().Msg("⚠️ [DevicesList] claims is nil, permission filter will be skipped")
	}

	log.Debug().
		Int("page", page).
		Int("perPages", perPages).
		Str("sortField", sortField).
		Str("sortOrder", sortOrder).
		Interface("filters", filters).
		Msg("📦 [DevicesList] Fetching device list")

	// service คืน: []Device, gmod.Pagination, online, offline, error
	// devs, pag, online, offline, err := devsvc.DevicesList(
	// 	ctx, page, perPages, filters, sortField, sortOrder,
	// )
	devs, pag, online, offline, err := devsvc.DevicesList(
		ctx, page, perPages, filters, sortField, sortOrder,
	)
	if err != nil {
		log.Error().Err(err).Msg("❌ [DevicesList] failed to list devices")
		return httputil.FailInternal(c, "failed to list devices")
	}

	// แปลง gmod.Pagination -> devmod.Pagination
	devPagination := devmod.Pagination{
		Page:         pag.Page,
		PerPages:     pag.PerPages,
		TotalRecords: pag.TotalRecords,
		TotalPages:   pag.TotalPages,
		SortField:    pag.SortField,
		SortOrder:    pag.SortOrder,
	}

	log.Debug().
		Int("count", len(devs)).
		Int("online", online).
		Int("offline", offline).
		Msg("✅ [DevicesList] Device list fetched successfully")

	resp := devmod.DeviceListResponse{
		Details:    devs,
		Pagination: devPagination,
		Online:     online,
		Offline:    offline,
		Status:     true,
	}
	return c.JSON(resp)
}

func DevicesListWithPermission(c *fiber.Ctx) error {
	ctx, end, log := traceutil.StartLite(c.UserContext(), "gateway.devapi", "DevicesListWithPermission", "devapi", "DevicesListWithPermission")
	defer end()

	// 👀 ดูว่าปัจจุบัน middleware ยัดอะไรไว้ใน Locals("user")
	rawUser := c.Locals("user")
	log.Debug().
		Interface("auth_user_locals", rawUser).
		Msg("🔐 [DevicesListWithPermission] user from context")

	// แปลงเป็น map[string]interface{} ให้ devapi ใช้ง่าย
	var claims map[string]interface{}
	switch v := rawUser.(type) {
	case map[string]interface{}:
		claims = v
	case jwt.MapClaims:
		claims = map[string]interface{}(v)
	case nil:
		log.Warn().Msg("⚠️ [DevicesListWithPermission] c.Locals(\"user\") is nil")
	default:
		log.Warn().
			Msgf("⚠️ [DevicesListWithPermission] c.Locals(\"user\") is type %T, cannot cast", v)
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
	sortField := c.Query("sortField", "dateTimeCreate")

	filters := map[string]string{}
	if id := c.Query("id"); id != "" {
		filters["id"] = id
	}
	if code := c.Query("code"); code != "" {
		filters["code"] = code
	}
	if search := c.Query("search"); search != "" {
		filters["search"] = search
	}

	// 🔐 map JWT → subjectType/subjectId สำหรับ Permify
	if claims != nil {
		if role, ok := claims["role"].(string); ok && role != "" {
			// ใช้ role เป็น subjectId (เราทำ tuple เป็น role:user / role:mgt แล้ว)
			filters["subjectType"] = "role"
			filters["subjectId"] = role // "user", "mgt"
		}
		if sub, ok := claims["sub"].(string); ok && sub != "" {
			filters["userId"] = sub // เผื่ออยากใช้ filter per-user ภายหลัง
		}
	} else {
		log.Warn().Msg("⚠️ [DevicesListWithPermission] claims is nil, permission filter will be skipped")
	}

	log.Debug().
		Int("page", page).
		Int("perPages", perPages).
		Str("sortField", sortField).
		Str("sortOrder", sortOrder).
		Interface("filters", filters).
		Msg("📦 [DevicesListWithPermission] Fetching device list")

	// service คืน: []Device, gmod.Pagination, online, offline, error
	// devs, pag, online, offline, err := devsvc.DevicesList(
	// 	ctx, page, perPages, filters, sortField, sortOrder,
	// )

	// devs, pag, online, offline, err := devsvc.DevicesListCheckPermission(
	// 	ctx ,page, perPages, filters, sortField, sortOrder, claims["sub"].(string),
	// )
	devs, pag, online, offline, err := devsvc.DevicesListCheckPermission(
		ctx, "", page, perPages, filters, sortField, sortOrder,
	)
	if err != nil {
		log.Error().Err(err).Msg("❌ [DevicesListWithPermission] failed to list devices")
		return httputil.FailInternal(c, "failed to list devices")
	}

	// แปลง gmod.Pagination -> devmod.Pagination
	devPagination := devmod.Pagination{
		Page:         pag.Page,
		PerPages:     pag.PerPages,
		TotalRecords: pag.TotalRecords,
		TotalPages:   pag.TotalPages,
		SortField:    pag.SortField,
		SortOrder:    pag.SortOrder,
	}

	log.Debug().
		Int("count", len(devs)).
		Int("online", online).
		Int("offline", offline).
		Msg("✅ [DevicesList] Device list fetched successfully")

	resp := devmod.DeviceListResponse{
		Details:    devs,
		Pagination: devPagination,
		Online:     online,
		Offline:    offline,
		Status:     true,
	}
	return c.JSON(resp)
}
