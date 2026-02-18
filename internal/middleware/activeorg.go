// internal/middleware/activeorg.go
package middleware

import (
	"context"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/hotkhwan/gateway-api/config"
	"github.com/hotkhwan/gateway-api/internal/gateways/authzgw"
	"github.com/hotkhwan/gateway-api/models/gmod"
	"github.com/hotkhwan/gateway-api/utils/traceutil"
)

const activeOrgHeader = "X-Active-Org"

func maskId(v string) string {
	v = strings.TrimSpace(v)
	if len(v) <= 8 {
		return v
	}
	return v[:4] + "..." + v[len(v)-4:]
}

func ActiveOrg() fiber.Handler {
	return func(c *fiber.Ctx) error {
		userId, _ := c.Locals("userId").(string)
		tenantId, _ := c.Locals("tenantId").(string)

		userId = strings.TrimSpace(userId)
		tenantId = strings.TrimSpace(tenantId)

		if userId == "" || tenantId == "" {
			return c.Status(fiber.StatusUnauthorized).JSON(gmod.ApiErrorResponse{
				Code:    gmod.CodeUnauthorized,
				Message: "Unauthorized",
				Status:  false,
			})
		}

		orgId := strings.TrimSpace(c.Get(activeOrgHeader))
		if orgId == "" {
			return c.Status(fiber.StatusBadRequest).JSON(gmod.ApiErrorResponse{
				Code:    gmod.CodeBadRequest,
				Message: "X-Active-Org header required",
				Status:  false,
			})
		}

		schemaVersion := strings.TrimSpace(config.CurrentSchemaVersion)
		if schemaVersion == "" {
			return c.Status(fiber.StatusInternalServerError).JSON(gmod.ApiErrorResponse{
				Code:    gmod.CodeInternalError,
				Message: "permify schema version not configured",
				Status:  false,
			})
		}

		ctx, cancel := context.WithTimeout(c.UserContext(), 3*time.Second)
		defer cancel()

		ctx, end, log := traceutil.StartLite(ctx, "github.com/hotkhwan/gateway-api/middleware", "activeOrg.check", "middleware", "ActiveOrg")
		defer end()

		// ✅ Use gRPC check only (single source of truth)
		grpc := authzgw.NewGrpcClient()

		allowed, err := grpc.CheckPermission(
			ctx,
			tenantId,
			"organization",
			orgId,
			"view",
			"user",
			"user:"+userId, // gRPC path in your code expects user:<uuid>
		)

		log.Info().
			Str("tenantId", tenantId).
			Str("orgId", orgId).
			Str("schemaVersion", schemaVersion).
			Str("userId", maskId(userId)).
			Bool("allowed", allowed).
			Bool("err", err != nil).
			Msg("permify active org check (grpc)")

		if err != nil {
			log.Info().
				Err(err).
				Str("tenantId", tenantId).
				Str("orgId", orgId).
				Str("schemaVersion", schemaVersion).
				Msg("authz check failed (grpc)")
			return c.Status(fiber.StatusInternalServerError).JSON(gmod.ApiErrorResponse{
				Code:    gmod.CodeInternalError,
				Message: "authz check failed",
				Status:  false,
			})
		}

		if !allowed {
			return c.Status(fiber.StatusForbidden).JSON(gmod.ApiErrorResponse{
				Code:    gmod.CodeForbidden,
				Message: "Forbidden",
				Status:  false,
			})
		}

		c.Locals("activeOrg", orgId)
		return c.Next()
	}
}
