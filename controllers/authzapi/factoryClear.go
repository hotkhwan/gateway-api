// controllers/authzapi/factoryResetTenant.go
package authzapi

import (
	"os"

	"github.com/hotkhwan/gateway-api/internal/services/authzsvc"
	"github.com/hotkhwan/gateway-api/models/gmod"

	"github.com/gofiber/fiber/v2"
	"go.opentelemetry.io/otel"
)

// FactoryResetTenantHandler godoc
// @Summary      Factory reset all Permify tuples for current tenant (DEV ONLY)
// @Description  Danger: delete ALL relationships (tuples) in this tenant. Intended for dev/staging only.
// @Tags         Authz
// @Accept       json
// @Produce      json
// @Param        body body authzmod.FactoryForceRequest true "Require {\"force\": true}"
// @Success      200 {object} map[string]interface{}
// @Failure      400 {object} gmod.ErrorMessageResponse
// @Failure      403 {object} gmod.ErrorMessageResponse
// @Failure      500 {object} gmod.ErrorMessageResponse
// @Router       /authz/tuples/factoryResetTenant [post]
// @Security     BearerAuth
func FactoryResetTenantHandler(c *fiber.Ctx) error {
	ctx := c.UserContext()
	tracer := otel.Tracer("github.com/hotkhwan/gateway-api/authzapi")
	ctx, span := tracer.Start(ctx, "Authz.FactoryResetTenantHandler")
	defer span.End()

	// 🔐 กันยิงใน production
	if env := os.Getenv("APP_ENV"); env == "prod" || env == "production" {
		return c.Status(fiber.StatusForbidden).JSON(gmod.ErrorMessageResponse{
			Code:    "FORBIDDEN",
			Message: "factory reset is disabled in production",
			Status:  false,
		})
	}

	var req struct {
		Force bool `json:"force"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(gmod.ErrorMessageResponse{
			Code:    "INVALID_REQUEST",
			Message: "invalid json body",
			Status:  false,
		})
	}
	if !req.Force {
		return c.Status(fiber.StatusBadRequest).JSON(gmod.ErrorMessageResponse{
			Code:    "BAD_REQUEST",
			Message: "force=true is required for factory reset",
			Status:  false,
		})
	}

	total, deleted, failed, err := authzsvc.FactoryResetTenant(ctx)
	if err != nil {
		// แยกเคส client not init เผื่อ debug ง่าย
		if err == authzsvc.ErrPermifyClientNotInit {
			return c.Status(fiber.StatusInternalServerError).JSON(gmod.ErrorMessageResponse{
				Code:    "ERROR",
				Message: "permify client not initialized",
				Status:  false,
			})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(gmod.ErrorMessageResponse{
			Code:    "ERROR",
			Message: err.Error(),
			Status:  false,
		})
	}

	// รูปแบบ response ให้เหมือน factory-clear ที่คุณใช้แล้ว
	return c.JSON(fiber.Map{
		"code":    "SUCCESS",
		"message": "factory reset tenant completed",
		"status":  true,
		"result": fiber.Map{
			"total":   total,
			"deleted": deleted,
			"failed":  failed,
		},
	})
}
