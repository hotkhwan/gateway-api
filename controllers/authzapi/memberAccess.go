// controllers/authzapi/memberAccess.go
package authzapi

import (
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/hotkhwan/gateway-api/internal/services/authzsvc"
	"github.com/hotkhwan/gateway-api/models/gmod"
	"go.opentelemetry.io/otel"
)

// MemberAccessController exposes read-only endpoints for authenticated members
// to list menus and resource groups they personally have access to,
// based on their OrgUnit membership and active permission profiles.
type MemberAccessController struct {
	svc *authzsvc.MemberAccessService
}

func NewMemberAccessController(svc *authzsvc.MemberAccessService) *MemberAccessController {
	return &MemberAccessController{svc: svc}
}

// MyMenuAccess godoc
// @Summary      List menus accessible to the current user
// @Description  Returns menu IDs the caller can access via their OrgUnit membership and active MenuPermissionProfiles.
// @Tags         MemberAccess
// @Security     BearerAuth
// @Produce      json
// @Success      200 {object} gmod.SuccessDetailResponseMenuAccess
// @Failure      401 {object} gmod.ApiErrorResponse
// @Failure      500 {object} gmod.ApiErrorResponse
// @Router       /api/v1/orgs/menu/access [get]
func (ctrl *MemberAccessController) MyMenuAccess(c *fiber.Ctx) error {
	ctx := c.UserContext()
	tracer := otel.Tracer("authzapi")
	ctx, span := tracer.Start(ctx, "MemberAccess.MyMenuAccess")
	defer span.End()

	tenantId, _ := c.Locals("tenantId").(string)
	orgId, _ := c.Locals("activeOrg").(string)
	userId, _ := c.Locals("userId").(string)

	menus, err := ctrl.svc.GetMemberMenuAccess(ctx, tenantId, orgId, userId)
	if err != nil {
		return c.Status(500).JSON(gmod.ApiErrorResponse{
			Code:    gmod.CodeInternalError,
			Message: "failed to resolve menu access",
			Status:  false,
		})
	}

	return c.JSON(gmod.SuccessDetailResponseMenuAccess{
		Code:    gmod.CodeSuccess,
		Message: "accessible menus fetched",
		Status:  true,
		Detail:  menus,
	})
}

// MyResourceAccess godoc
// @Summary      List cameras accessible to the current user
// @Description  Returns paginated cameras the caller can access via OrgUnit membership and active ResourcePermissionProfiles, enriched with the union of Permify relations (viewer, editor, …).
// @Tags         MemberAccess
// @Security     BearerAuth
// @Produce      json
// @Param        resourceType query string false "Filter by resource type (e.g. camera)"
// @Param        search       query string false "Search camera name"
// @Param        page         query int    false "Page number (default 1)"
// @Param        perPages     query int    false "Items per page (default 10)"
// @Param        sortField    query string false "Sort field (default createAt)"
// @Param        sortOrder    query string false "Sort order: asc | desc (default desc)"
// @Success      200 {object} gmod.SuccessDetailResponseResourceAccess
// @Failure      401 {object} gmod.ApiErrorResponse
// @Failure      500 {object} gmod.ApiErrorResponse
// @Router       /api/v1/orgs/resource/access [get]
func (ctrl *MemberAccessController) MyResourceAccess(c *fiber.Ctx) error {
	ctx := c.UserContext()
	tracer := otel.Tracer("authzapi")
	ctx, span := tracer.Start(ctx, "MemberAccess.MyResourceAccess")
	defer span.End()

	tenantId, _ := c.Locals("tenantId").(string)
	orgId, _ := c.Locals("activeOrg").(string)
	userId, _ := c.Locals("userId").(string)

	page, _ := strconv.Atoi(c.Query("page", "1"))
	perPage, _ := strconv.Atoi(c.Query("perPages", "10"))

	params := authzsvc.MemberCameraAccessParams{
		ResourceType: c.Query("resourceType"),
		Search:       c.Query("search"),
		Page:         page,
		PerPage:      perPage,
		SortField:    c.Query("sortField"),
		SortOrder:    c.Query("sortOrder"),
	}

	result, err := ctrl.svc.GetMemberCameraAccess(ctx, tenantId, orgId, userId, params)
	if err != nil {
		return c.Status(500).JSON(gmod.ApiErrorResponse{
			Code:    gmod.CodeInternalError,
			Message: "failed to resolve resource access",
			Status:  false,
		})
	}

	return c.JSON(gmod.SuccessDetailResponseResourceAccess{
		Code:    gmod.CodeSuccess,
		Message: "accessible cameras fetched",
		Status:  true,
		Detail: fiber.Map{
			"cameras":      result.Cameras,
			"totalRecords": result.TotalRecords,
			"totalPages":   result.TotalPages,
			"page":         result.Page,
			"perPage":      result.PerPage,
		},
	})
}
