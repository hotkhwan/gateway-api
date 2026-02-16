// controllers/authznewapi/org.go
package authznewapi

import (
	"strings"

	"github.com/hotkhwan/gateway-api/internal/services/authzsvc"

	"github.com/gofiber/fiber/v2"
	"go.opentelemetry.io/otel"
)

// @Summary Create organization
// @Tags AuthzNew
// @Security BearerAuth
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{}
func CreateOrg(c *fiber.Ctx) error {

	ctx := c.UserContext()
	tracer := otel.Tracer("github.com/hotkhwan/gateway-api/authznewapi")
	ctx, span := tracer.Start(ctx, "AuthzNew.CreateOrg")
	defer span.End()

	userId, ok := c.Locals("userId").(string)
	if !ok || strings.TrimSpace(userId) == "" {
		return fiber.ErrUnauthorized
	}

	tenantId, ok := c.Locals("tenantId").(string)
	if !ok || strings.TrimSpace(tenantId) == "" {
		return fiber.ErrUnauthorized
	}

	var req struct {
		Name string `json:"name"`
	}

	if err := c.BodyParser(&req); err != nil {
		return fiber.ErrBadRequest
	}

	if req.Name == "" {
		return fiber.NewError(fiber.StatusBadRequest, "name required")
	}

	org, err := authzsvc.BootstrapOrganization(ctx, tenantId, userId, req.Name)
	if err != nil {
		return err
	}

	return c.JSON(fiber.Map{
		"status":  true,
		"orgId":   org.OrgId,
		"message": "organization created",
	})
}
