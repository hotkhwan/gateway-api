// internal/middleware/activeorg.go
package middleware

import (
	"context"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/hotkhwan/gateway-api/internal/gateways/authzgw"
)

const activeOrgHeader = "X-Active-Org"

func ActiveOrg() fiber.Handler {

	return func(c *fiber.Ctx) error {

		userId, ok := c.Locals("userId").(string)
		if !ok || userId == "" {
			return fiber.ErrUnauthorized
		}

		tenantId, ok := c.Locals("tenantId").(string)
		if !ok || tenantId == "" {
			return fiber.ErrUnauthorized
		}

		orgId := strings.TrimSpace(c.Get(activeOrgHeader))
		if orgId == "" {
			return fiber.NewError(
				fiber.StatusBadRequest,
				"X-Active-Org header required",
			)
		}

		// 🔥 check membership via Permify
		ctx, cancel := context.WithTimeout(c.UserContext(), 3*time.Second)
		defer cancel()

		client := authzgw.NewClient()

		allowed, err := client.CheckPermission(
			ctx,
			tenantId,
			"organization",
			orgId,
			"view",
			"user",
			userId,
		)

		if err != nil {
			return fiber.NewError(
				fiber.StatusInternalServerError,
				"authz check failed",
			)
		}

		if !allowed {
			return fiber.ErrForbidden
		}

		// inject active org
		c.Locals("activeOrgId", orgId)

		return c.Next()
	}
}
