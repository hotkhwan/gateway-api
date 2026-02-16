// controllers/authznewapi/orgUnit.go
package authznewapi

import (
	"github.com/gofiber/fiber/v2"
	"github.com/hotkhwan/gateway-api/internal/services/authzsvc"
	"github.com/hotkhwan/gateway-api/models/gmod"
)

// POST /orgUnits
func CreateOrgUnit(c *fiber.Ctx) error {

	var body struct {
		Name     string  `json:"name"`
		ParentId *string `json:"parentId"`
	}

	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(gmod.ApiErrorResponse{
			Code:    gmod.CodeBadRequest,
			Message: "invalid body",
			Status:  false,
		})
	}

	tenantId := c.Locals("tenantId").(string)
	orgId := c.Locals("activeOrg").(string)
	userId := c.Locals("userId").(string)

	id, err := authzsvc.CreateOrgUnit(
		c.Context(),
		tenantId,
		orgId,
		body.Name,
		body.ParentId,
		userId,
	)

	if err != nil {
		return c.Status(500).JSON(gmod.ApiErrorResponse{
			Code:    gmod.CodeInternalError,
			Message: err.Error(),
			Status:  false,
		})
	}

	return c.JSON(gmod.SuccessMessageCreateResponse{
		Code:    gmod.CodeCreated,
		Message: "orgUnit created",
		Status:  true,
		ID:      id,
	})
}

// GET /orgUnits/tree
func GetOrgUnitTree(c *fiber.Ctx) error {

	tenantId := c.Locals("tenantId").(string)
	orgId := c.Locals("activeOrg").(string)

	tree, err := authzsvc.GetOrgUnitTree(
		c.Context(),
		tenantId,
		orgId,
	)

	if err != nil {
		return c.Status(500).JSON(gmod.ApiErrorResponse{
			Code:    gmod.CodeInternalError,
			Message: err.Error(),
			Status:  false,
		})
	}

	return c.JSON(fiber.Map{
		"code":    gmod.CodeSuccess,
		"message": "success",
		"status":  true,
		"details": tree,
	})
}

// PATCH /orgUnits/:id
func UpdateOrgUnit(c *fiber.Ctx) error {

	unitId := c.Params("id")

	var body struct {
		Name string `json:"name"`
	}

	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(gmod.ApiErrorResponse{
			Code:    gmod.CodeBadRequest,
			Message: "invalid body",
			Status:  false,
		})
	}

	if err := authzsvc.UpdateOrgUnit(c.Context(), unitId, body.Name); err != nil {
		return c.Status(500).JSON(gmod.ApiErrorResponse{
			Code:    gmod.CodeInternalError,
			Message: err.Error(),
			Status:  false,
		})
	}

	return c.JSON(gmod.SuccessMessageResponse{
		Code:    gmod.CodeSuccess,
		Message: "updated",
		Status:  true,
	})
}

// DELETE /orgUnits/:id
func DeleteOrgUnit(c *fiber.Ctx) error {

	unitId := c.Params("id")

	if err := authzsvc.DeleteOrgUnit(c.Context(), unitId); err != nil {
		return c.Status(500).JSON(gmod.ApiErrorResponse{
			Code:    gmod.CodeInternalError,
			Message: err.Error(),
			Status:  false,
		})
	}

	return c.JSON(gmod.SuccessMessageResponse{
		Code:    gmod.CodeSuccess,
		Message: "deleted",
		Status:  true,
	})
}
