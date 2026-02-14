// controllers/authzapi/getHealth.go
package authzapi

import (
	"klynx/models/gmod"

	"github.com/gofiber/fiber/v2"
)

// @Summary Health check for Permify
// @Tags Authz
// @Success 200 {object} gmod.SuccessMessageResponse
// @Failure 500 {object} gmod.InternalErrorResponse
// @Router /authz/healthz [get]
// @Security BearerAuth
func GetHealth(c *fiber.Ctx) error {
	return c.JSON(gmod.SuccessMessageResponse{
		Code:    "SUCCESS",
		Message: "Permify connection is healthy",
		Status:  true,
	})
}
