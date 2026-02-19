// controllers/authznewapi/orgUnit.go
package authznewapi

import (
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/hotkhwan/gateway-api/internal/services/authzsvc"
	"github.com/hotkhwan/gateway-api/models/gmod"
)

// CreateOrgUnitBody swagger model
type CreateOrgUnitBody struct {
	Name        string  `json:"name" example:"Operations"`
	Description string  `json:"description" example:"Details Operations"`
	ParentId    *string `json:"parentId" example:"unit_root_123"`
}

// UpdateOrgUnitBody swagger model
type UpdateOrgUnitBody struct {
	Name        string `json:"name" example:"Operations (North)"`
	Description string `json:"description" example:"Details Operations (North)"`
}

// OrgUnitNode swagger model
type OrgUnitNode struct {
	UnitId      string        `json:"unitId"`
	ParentId    *string       `json:"parentId,omitempty"`
	Name        string        `json:"name"`
	Description string        `json:"description"`
	IsRoot      bool          `json:"isRoot"`
	Children    []OrgUnitNode `json:"children"`
	Orphaned    bool          `json:"orphaned,omitempty"`
	CreatedAt   int64         `json:"createdAt,omitempty"`
}

// @Summary Create org unit
// @Description Create an org unit under active org (root is created by bootstrap only)
// @Tags OrgUnit
// @Accept json
// @Produce json
// @Param body body CreateOrgUnitBody true "payload"
// @Success 200 {object} gmod.SuccessMessageCreateResponse
// @Failure 400 {object} gmod.ApiErrorResponse
// @Failure 401 {object} gmod.ApiErrorResponse
// @Failure 409 {object} gmod.ApiErrorResponse
// @Failure 500 {object} gmod.ApiErrorResponse
// @Router /api/v1/orgs/units [post]
func CreateOrgUnit(c *fiber.Ctx) error {
	var body CreateOrgUnitBody
	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(gmod.ApiErrorResponse{
			Code: gmod.CodeBadRequest, Message: "invalid body", Status: false,
		})
	}

	body.Name = strings.TrimSpace(body.Name)
	if body.Name == "" {
		return c.Status(400).JSON(gmod.ApiErrorResponse{
			Code: gmod.CodeBadRequest, Message: "name is required", Status: false,
		})
	}

	if body.ParentId != nil {
		v := strings.TrimSpace(*body.ParentId)
		if v == "" {
			body.ParentId = nil
		} else {
			body.ParentId = &v
		}
	}

	tenantId, _ := c.Locals("tenantId").(string)
	orgId, _ := c.Locals("activeOrg").(string)
	userId, _ := c.Locals("userId").(string)

	if tenantId == "" || orgId == "" || userId == "" {
		return c.Status(401).JSON(gmod.ApiErrorResponse{
			Code: gmod.CodeUnauthorized, Message: "Unauthorized", Status: false,
		})
	}

	unitId, err := authzsvc.CreateOrgUnit(c.Context(), tenantId, orgId, body.Name, body.Description, body.ParentId, userId)
	if err != nil {
		status, code := authzsvc.MapSvcError(err)
		return c.Status(status).JSON(gmod.ApiErrorResponse{
			Code: code, Message: err.Error(), Status: false,
		})
	}

	return c.JSON(gmod.SuccessMessageCreateResponse{
		Code:    gmod.CodeCreated,
		Message: "orgUnit created",
		Status:  true,
		ID:      unitId,
	})
}

// @Summary Get org unit tree
// @Description Get org unit tree for active org
// @Tags OrgUnit
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Failure 401 {object} gmod.ApiErrorResponse
// @Failure 500 {object} gmod.ApiErrorResponse
// @Router /api/v1/orgs/units/tree [get]
func GetOrgUnitTree(c *fiber.Ctx) error {
	tenantId, _ := c.Locals("tenantId").(string)
	orgId, _ := c.Locals("activeOrg").(string)

	if tenantId == "" || orgId == "" {
		return c.Status(401).JSON(gmod.ApiErrorResponse{
			Code: gmod.CodeUnauthorized, Message: "Unauthorized", Status: false,
		})
	}

	tree, err := authzsvc.GetOrgUnitTree(c.Context(), tenantId, orgId)
	if err != nil {
		status, code := authzsvc.MapSvcError(err)
		return c.Status(status).JSON(gmod.ApiErrorResponse{
			Code: code, Message: err.Error(), Status: false,
		})
	}

	return c.JSON(fiber.Map{
		"code":    gmod.CodeSuccess,
		"message": "success",
		"status":  true,
		"details": tree,
	})
}

// @Summary Update org unit name
// @Tags OrgUnit
// @Accept json
// @Produce json
// @Param id path string true "unitId"
// @Param body body UpdateOrgUnitBody true "payload"
// @Success 200 {object} gmod.SuccessMessageResponse
// @Failure 400 {object} gmod.ApiErrorResponse
// @Failure 401 {object} gmod.ApiErrorResponse
// @Failure 404 {object} gmod.ApiErrorResponse
// @Failure 500 {object} gmod.ApiErrorResponse
// @Router /api/v1/orgs/units/{id} [patch]
func UpdateOrgUnit(c *fiber.Ctx) error {
	unitId := strings.TrimSpace(c.Params("id"))
	if unitId == "" {
		return c.Status(400).JSON(gmod.ApiErrorResponse{
			Code: gmod.CodeBadRequest, Message: "id is required", Status: false,
		})
	}

	var body UpdateOrgUnitBody
	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(gmod.ApiErrorResponse{
			Code: gmod.CodeBadRequest, Message: "invalid body", Status: false,
		})
	}

	body.Name = strings.TrimSpace(body.Name)
	if body.Name == "" {
		return c.Status(400).JSON(gmod.ApiErrorResponse{
			Code: gmod.CodeBadRequest, Message: "name is required", Status: false,
		})
	}

	tenantId, _ := c.Locals("tenantId").(string)
	orgId, _ := c.Locals("activeOrg").(string)
	userId, _ := c.Locals("userId").(string)

	if tenantId == "" || orgId == "" || userId == "" {
		return c.Status(401).JSON(gmod.ApiErrorResponse{
			Code: gmod.CodeUnauthorized, Message: "Unauthorized", Status: false,
		})
	}

	if err := authzsvc.UpdateOrgUnitName(c.Context(), tenantId, orgId, unitId, body.Name, body.Description, userId); err != nil {
		status, code := authzsvc.MapSvcError(err)
		return c.Status(status).JSON(gmod.ApiErrorResponse{
			Code: code, Message: err.Error(), Status: false,
		})
	}

	return c.JSON(gmod.SuccessMessageResponse{
		Code: gmod.CodeSuccess, Message: "updated", Status: true,
	})
}

// @Summary Delete org unit
// @Description Delete a unit only if it has no children (safe delete)
// @Tags OrgUnit
// @Produce json
// @Param id path string true "unitId"
// @Success 200 {object} gmod.SuccessMessageResponse
// @Failure 401 {object} gmod.ApiErrorResponse
// @Failure 404 {object} gmod.ApiErrorResponse
// @Failure 409 {object} gmod.ApiErrorResponse
// @Failure 500 {object} gmod.ApiErrorResponse
// @Router /api/v1/orgs/units/{id} [delete]
func DeleteOrgUnit(c *fiber.Ctx) error {
	unitId := strings.TrimSpace(c.Params("id"))
	if unitId == "" {
		return c.Status(400).JSON(gmod.ApiErrorResponse{
			Code: gmod.CodeBadRequest, Message: "id is required", Status: false,
		})
	}

	tenantId, _ := c.Locals("tenantId").(string)
	orgId, _ := c.Locals("activeOrg").(string)
	userId, _ := c.Locals("userId").(string)

	if tenantId == "" || orgId == "" || userId == "" {
		return c.Status(401).JSON(gmod.ApiErrorResponse{
			Code: gmod.CodeUnauthorized, Message: "Unauthorized", Status: false,
		})
	}

	if err := authzsvc.DeleteOrgUnit(c.Context(), tenantId, orgId, unitId, userId); err != nil {
		status, code := authzsvc.MapSvcError(err)
		return c.Status(status).JSON(gmod.ApiErrorResponse{
			Code: code, Message: err.Error(), Status: false,
		})
	}

	return c.JSON(gmod.SuccessMessageResponse{
		Code: gmod.CodeSuccess, Message: "deleted", Status: true,
	})
}
