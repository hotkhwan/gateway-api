// controllers/authznewapi/orgUnit.go
package authznewapi

import (
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/hotkhwan/gateway-api/internal/services/authzsvc"
	"github.com/hotkhwan/gateway-api/models/gmod"
)

// POST /orgUnits
func CreateOrgUnit(c *fiber.Ctx) error {
  var body struct {
    Name string `json:"name"`
    ParentId *string `json:"parentId"`
  }

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

  // ✅ forbid creating root via API
  if body.ParentId == nil || strings.TrimSpace(*body.ParentId) == "" {
    return c.Status(400).JSON(gmod.ApiErrorResponse{
      Code: gmod.CodeBadRequest, Message: "parentId is required (root is created by bootstrap only)", Status: false,
    })
  }

  tenantId, ok := c.Locals("tenantId").(string)
  if !ok || tenantId == "" {
    return c.Status(401).JSON(gmod.ApiErrorResponse{
      Code: gmod.CodeUnauthorized, Message: "Unauthorized", Status: false,
    })
  }

  orgId, ok := c.Locals("activeOrg").(string)
  if !ok || orgId == "" {
    return c.Status(401).JSON(gmod.ApiErrorResponse{
      Code: gmod.CodeUnauthorized, Message: "Active org required", Status: false,
    })
  }

  userId, ok := c.Locals("userId").(string)
  if !ok || userId == "" {
    return c.Status(401).JSON(gmod.ApiErrorResponse{
      Code: gmod.CodeUnauthorized, Message: "Unauthorized", Status: false,
    })
  }

  id, err := authzsvc.CreateOrgUnit(c.Context(), tenantId, orgId, body.Name, body.ParentId, userId)
  if err != nil {
    return c.Status(500).JSON(gmod.ApiErrorResponse{
      Code: gmod.CodeInternalError, Message: err.Error(), Status: false,
    })
  }

  return c.JSON(gmod.SuccessMessageCreateResponse{
    Code: gmod.CodeCreated,
    Message: "orgUnit created",
    Status: true,
    ID: id,
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
