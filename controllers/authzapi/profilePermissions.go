// controllers/authzapi/profilePermissions.go
package authzapi

import (
	"errors"
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/hotkhwan/gateway-api/internal/repo/authzrepo"
	"github.com/hotkhwan/gateway-api/internal/services/authzsvc"
	"github.com/hotkhwan/gateway-api/models/gmod"
)

type ProfilePermissionsController struct {
	svc *authzsvc.PermissionProfileService
}

func NewProfilePermissionsController(svc *authzsvc.PermissionProfileService) *ProfilePermissionsController {
	return &ProfilePermissionsController{svc: svc}
}

// ============================================================
// Request types
// ============================================================

type createPermProfileBody struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Status      bool     `json:"status"`
	Relations   []string `json:"relations"`
}

type updatePermProfileBody struct {
	Name           string   `json:"name"`
	Description    string   `json:"description"`
	Status         bool     `json:"status"`
	Relations      []string `json:"relations"`
	// flat string IDs — nil = not provided (keep old), [] = clear, ["id"] = set
	OrgUnits       []string `json:"orgUnits"`
	ResourceGroups []string `json:"resourceGroups"`
}

// ============================================================
// POST /orgs/profile/permissions
// ============================================================

func (ctrl *ProfilePermissionsController) Create(c *fiber.Ctx) error {
	tenantId, _ := c.Locals("tenantId").(string)
	orgId, _ := c.Locals("activeOrg").(string)
	userId, _ := c.Locals("userId").(string)

	var body createPermProfileBody
	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(gmod.ApiErrorResponse{Code: gmod.CodeBadRequest, Message: "invalid body", Status: false})
	}
	if body.Name == "" {
		return c.Status(400).JSON(gmod.ApiErrorResponse{Code: gmod.CodeBadRequest, Message: "name is required", Status: false})
	}
	if len(body.Relations) == 0 {
		return c.Status(400).JSON(gmod.ApiErrorResponse{Code: gmod.CodeBadRequest, Message: "relations is required", Status: false})
	}

	profile, err := ctrl.svc.Create(c.UserContext(), authzsvc.CreatePermProfileInput{
		TenantID:    tenantId,
		OrgID:       orgId,
		CallerID:    userId,
		Name:        body.Name,
		Description: body.Description,
		Status:      body.Status,
		Relations:   body.Relations,
	})
	if err != nil {
		return handlePermProfileErr(c, err)
	}

	return c.Status(201).JSON(fiber.Map{
		"code":    gmod.CodeSuccess,
		"message": "permission profile created",
		"status":  true,
		"details": profile,
	})
}

// ============================================================
// PATCH /orgs/profile/permissions/:id
//
// Payload (all fields optional — omit to keep existing value):
//
//	{
//	  "name":           "...",
//	  "description":    "...",
//	  "status":         true,
//	  "relations":      ["viewer","editor"],
//	  "orgUnits":       ["uuid1","uuid2"],   // flat string IDs
//	  "resourceGroups": ["uuid1","uuid2"]    // flat string IDs
//	}
//
// status true  → write tuples (relations × orgUnits × resourceGroups)
// status false → delete existing tuples
// ============================================================

func (ctrl *ProfilePermissionsController) Update(c *fiber.Ctx) error {
	tenantId, _ := c.Locals("tenantId").(string)
	orgId, _ := c.Locals("activeOrg").(string)
	userId, _ := c.Locals("userId").(string)
	profileId := c.Params("id")

	var body updatePermProfileBody
	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(gmod.ApiErrorResponse{Code: gmod.CodeBadRequest, Message: "invalid body", Status: false})
	}

	// Preserve nil semantics:
	//   body.OrgUnits == nil      → field not in JSON → pass nil → service keeps old
	//   body.OrgUnits == []string{} → field was "orgUnits":[] → pass []string{} → service clears
	//   body.OrgUnits == ["id1"]  → pass ["id1"] → service validates + sets
	updated, err := ctrl.svc.Update(c.UserContext(), authzsvc.UpdatePermProfileInput{
		TenantID:         tenantId,
		OrgID:            orgId,
		ProfileID:        profileId,
		CallerID:         userId,
		Name:             body.Name,
		Description:      body.Description,
		Status:           body.Status,
		Relations:        body.Relations,
		OrgUnitIDs:       body.OrgUnits,       // nil when field absent in JSON
		ResourceGroupIDs: body.ResourceGroups, // nil when field absent in JSON
	})
	if err != nil {
		return handlePermProfileErr(c, err)
	}

	return c.JSON(fiber.Map{
		"code":    gmod.CodeSuccess,
		"message": "permission profile updated",
		"status":  true,
		"details": updated,
	})
}

// ============================================================
// DELETE /orgs/profile/permissions/:id
// ============================================================

func (ctrl *ProfilePermissionsController) Delete(c *fiber.Ctx) error {
	tenantId, _ := c.Locals("tenantId").(string)
	orgId, _ := c.Locals("activeOrg").(string)
	userId, _ := c.Locals("userId").(string)
	profileId := c.Params("id")

	if err := ctrl.svc.Delete(c.UserContext(), tenantId, orgId, profileId, userId); err != nil {
		return handlePermProfileErr(c, err)
	}

	return c.JSON(fiber.Map{
		"code":    gmod.CodeSuccess,
		"message": "permission profile deleted",
		"status":  true,
	})
}

// ============================================================
// GET /orgs/profile/permissions
// ============================================================

func (ctrl *ProfilePermissionsController) List(c *fiber.Ctx) error {
	tenantId, _ := c.Locals("tenantId").(string)
	orgId, _ := c.Locals("activeOrg").(string)

	page, _ := strconv.Atoi(c.Query("page", "1"))
	perPages, _ := strconv.Atoi(c.Query("perPages", "10"))

	profiles, total, err := ctrl.svc.List(c.UserContext(), authzsvc.ListPermProfileInput{
		TenantID:  tenantId,
		OrgID:     orgId,
		Search:    c.Query("search"),
		Page:      page,
		PerPage:   perPages,
		SortField: c.Query("sortField", "createdAt"),
		SortOrder: c.Query("sortOrder", "desc"),
	})
	if err != nil {
		return handlePermProfileErr(c, err)
	}

	totalPages := int((total + int64(perPages) - 1) / int64(perPages))
	if totalPages == 0 {
		totalPages = 1
	}

	return c.JSON(fiber.Map{
		"code":    gmod.CodeSuccess,
		"message": "permission profiles fetched",
		"status":  true,
		"details": profiles,
		"pagination": fiber.Map{
			"page":         page,
			"perPages":     perPages,
			"totalRecords": total,
			"totalPages":   totalPages,
		},
	})
}

// ============================================================
// GET /orgs/profile/permissions/:id
// ============================================================

func (ctrl *ProfilePermissionsController) GetOne(c *fiber.Ctx) error {
	tenantId, _ := c.Locals("tenantId").(string)
	orgId, _ := c.Locals("activeOrg").(string)
	profileId := c.Params("id")

	profile, err := ctrl.svc.Get(c.UserContext(), tenantId, orgId, profileId)
	if err != nil {
		return handlePermProfileErr(c, err)
	}

	return c.JSON(fiber.Map{
		"code":    gmod.CodeSuccess,
		"message": "permission profile fetched",
		"status":  true,
		"details": profile,
	})
}

// ============================================================
// Error handler
// ============================================================

func handlePermProfileErr(c *fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, authzsvc.ErrForbidden):
		return c.Status(403).JSON(gmod.ApiErrorResponse{Code: gmod.CodeForbidden, Message: "forbidden", Status: false})
	case errors.Is(err, authzsvc.ErrNotFound),
		errors.Is(err, authzrepo.ErrPermProfileNotFound):
		return c.Status(404).JSON(gmod.ApiErrorResponse{Code: gmod.CodeNotFound, Message: err.Error(), Status: false})
	case errors.Is(err, authzrepo.ErrPermProfileNameExists):
		return c.Status(409).JSON(gmod.ApiErrorResponse{Code: gmod.CodeConflict, Message: err.Error(), Status: false})
	case errors.Is(err, authzsvc.ErrInvalidArgs):
		return c.Status(400).JSON(gmod.ApiErrorResponse{Code: gmod.CodeBadRequest, Message: err.Error(), Status: false})
	default:
		return c.Status(500).JSON(gmod.ApiErrorResponse{Code: gmod.CodeInternalError, Message: "internal server error", Status: false})
	}
}
