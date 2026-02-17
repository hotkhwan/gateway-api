// controllers/authznewapi/authzDebugDelete.go
package authznewapi

import (
	"github.com/gofiber/fiber/v2"
	"github.com/hotkhwan/gateway-api/internal/services/authzsvc"
	"github.com/hotkhwan/gateway-api/models/gmod"
)

// ResetAllTuples godoc
// @Summary Factory reset tuples by entityType (debug)
// @Tags authzDebug
// @Security BearerAuth
// @Accept json
// @Produce json
// @Router /authzDebugs/tuples/resetAll [post]
func ResetAllTuples(c *fiber.Ctx) error {
	var body struct {
		TenantId   string `json:"tenantId"`
		EntityType string `json:"entityType"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(gmod.ApiErrorResponse{
			Code:    gmod.CodeBadRequest,
			Message: "Invalid request body",
			Status:  false,
		})
	}

	res, err := authzsvc.ResetPermifyTuplesAll(c.Context(), body.TenantId, body.EntityType)
	if err != nil {
		return c.Status(500).JSON(gmod.ApiErrorResponse{
			Code:    gmod.CodeInternalError,
			Message: err.Error(),
			Status:  false,
		})
	}

	return c.JSON(fiber.Map{
		"code":    gmod.CodeSuccess,
		"message": "Reset all tuples completed",
		"status":  true,
		"details": res,
	})
}

// ResetTuplesByUser godoc
// @Summary Reset org membership tuples by user (debug)
// @Tags authzDebug
// @Security BearerAuth
// @Accept json
// @Produce json
// @Router /authzDebugs/tuples/resetByUser [post]
func ResetTuplesByUser(c *fiber.Ctx) error {
	var body struct {
		TenantId string `json:"tenantId"`
		UserId   string `json:"userId"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(gmod.ApiErrorResponse{
			Code:    gmod.CodeBadRequest,
			Message: "Invalid request body",
			Status:  false,
		})
	}

	res, err := authzsvc.ResetPermifyTuplesByUser(c.Context(), body.TenantId, body.UserId)
	if err != nil {
		return c.Status(500).JSON(gmod.ApiErrorResponse{
			Code:    gmod.CodeInternalError,
			Message: err.Error(),
			Status:  false,
		})
	}

	return c.JSON(fiber.Map{
		"code":    gmod.CodeSuccess,
		"message": "Reset tuples by user completed",
		"status":  true,
		"details": res,
	})
}
