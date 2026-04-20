// internal/middleware/activeWorkspace.go
package middleware

import (
	"context"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/hotkhwan/gateway-api/internal/gateways/authzgw"
	"github.com/hotkhwan/gateway-api/models/gmod"
	"github.com/hotkhwan/gateway-api/utils/traceutil"
)

const activeWorkspaceHeader = "X-Active-Workspace"

// ActiveWorkspace validates that the caller is a member of the workspace identified
// by the X-Active-Workspace header, using Permify workspace-scoped RBAC.
// Sets c.Locals("activeWorkspace", workspaceId) on success.
func ActiveWorkspace() fiber.Handler {
	return func(c fiber.Ctx) error {
		userId, _ := c.Locals("userId").(string)
		tenantId, _ := c.Locals("tenantId").(string)
		role, _ := c.Locals("role").(string)

		userId = strings.TrimSpace(userId)
		tenantId = strings.TrimSpace(tenantId)

		if userId == "" || tenantId == "" {
			return c.Status(fiber.StatusUnauthorized).JSON(gmod.ApiErrorResponse{
				Code:    gmod.CodeUnauthorized,
				Message: "Unauthorized",
				Status:  false,
			})
		}

		workspaceId := strings.TrimSpace(c.Get(activeWorkspaceHeader))
		if workspaceId == "" {
			return c.Status(fiber.StatusBadRequest).JSON(gmod.ApiErrorResponse{
				Code:    gmod.CodeBadRequest,
				Message: "X-Active-Workspace header required",
				Status:  false,
			})
		}

		// Platform admins (JWT role = administrator) bypass Permify check.
		if role == "administrator" {
			c.Locals("activeWorkspace", workspaceId)
			return c.Next()
		}

		ctx, cancel := context.WithTimeout(c, 3*time.Second)
		defer cancel()

		ctx, end, log := traceutil.StartLite(ctx,
			"github.com/hotkhwan/gateway-api/middleware",
			"activeWorkspace.check",
			"middleware", "ActiveWorkspace",
		)
		defer end()

		grpc := authzgw.NewGrpcClient()

		allowed, err := grpc.CheckPermission(
			ctx,
			tenantId,
			"workspace",
			workspaceId,
			"view",
			"user",
			userId,
		)

		log.Info().
			Str("tenantId", tenantId).
			Str("workspaceId", maskId(workspaceId)).
			Str("userId", maskId(userId)).
			Bool("allowed", allowed).
			Bool("hasErr", err != nil).
			Msg("permify active workspace check")

		if err != nil {
			// CheckPermission failed (e.g. schema version mismatch) — fall back to
			// direct tuple read: allow if user has any direct relation on this workspace.
			log.Warn().Err(err).Str("workspaceId", workspaceId).Msg("workspace permission check failed — falling back to tuple read")
			tuples, readErr := grpc.ListEntityRelationships(ctx, tenantId, "workspace", workspaceId)
			if readErr != nil {
				log.Error().Err(readErr).Str("workspaceId", workspaceId).Msg("workspace tuple fallback read failed")
				return c.Status(fiber.StatusInternalServerError).JSON(gmod.ApiErrorResponse{
					Code:    gmod.CodeInternalError,
					Message: "authz check failed",
					Status:  false,
				})
			}
			allowed = false
			for _, t := range tuples {
				if t.Subject.Type == "user" && t.Subject.Id == userId {
					allowed = true
					break
				}
			}
			log.Info().
				Str("workspaceId", maskId(workspaceId)).
				Str("userId", maskId(userId)).
				Bool("allowed", allowed).
				Msg("workspace access via tuple fallback")
		}

		if !allowed {
			return c.Status(fiber.StatusForbidden).JSON(gmod.ApiErrorResponse{
				Code:    gmod.CodeForbidden,
				Message: "Forbidden",
				Status:  false,
			})
		}

		c.Locals("activeWorkspace", workspaceId)
		return c.Next()
	}
}

// TryActiveWorkspace reads X-Active-Workspace and verifies membership if present.
// If the header is absent or auth is not set, it continues silently (for optional-auth routes).
func TryActiveWorkspace() fiber.Handler {
	return func(c fiber.Ctx) error {
		workspaceId := strings.TrimSpace(c.Get(activeWorkspaceHeader))
		if workspaceId == "" {
			return c.Next()
		}

		userId, _ := c.Locals("userId").(string)
		tenantId, _ := c.Locals("tenantId").(string)
		if userId == "" || tenantId == "" {
			return c.Next()
		}

		ctx, cancel := context.WithTimeout(c, 3*time.Second)
		defer cancel()

		grpc := authzgw.NewGrpcClient()
		allowed, err := grpc.CheckPermission(ctx, tenantId, "workspace", workspaceId, "view", "user", userId)
		if err == nil && allowed {
			c.Locals("activeWorkspace", workspaceId)
		}
		return c.Next()
	}
}
