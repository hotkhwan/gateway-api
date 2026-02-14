// controllers/authzapi/postApplySchema.go
package authzapi

import (
	"fmt"

	"klynx/internal/services/authzsvc"
	"klynx/models/gmod"

	"github.com/gofiber/fiber/v2"
	"go.opentelemetry.io/otel"
)

// ApplySchemaHandler godoc
// @Summary      Apply Permify Schema & Sync Relationships
// @Description  Apply schema.perm to Permify and sync relationships from MongoDB
// @Tags         Authz
// @Accept       json
// @Produce      json
// @Success      200 {object} gmod.SuccessMessageResponse "Schema applied & relationships synced successfully"
// @Failure      500 {object} gmod.SuccessMessageResponse "Internal server error"
// @Router       /authz/schema/apply [post]
// @Security     BearerAuth
func ApplySchemaHandler(c *fiber.Ctx) error {
	ctx := c.UserContext()
	tracer := otel.Tracer("klynx/authzapi")
	ctx, span := tracer.Start(ctx, "Authz.ApplySchemaHandler")
	defer span.End()

	// Apply schema
	schemaVersion, err := authzsvc.ApplySchema(ctx)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(gmod.SuccessMessageResponse{
			Code:    "ERROR",
			Message: err.Error(),
			Status:  false,
		})
	}

	// Sync relationships
	if err := authzsvc.InitialSyncRelationships(ctx, schemaVersion); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(gmod.SuccessMessageResponse{
			Code:    "ERROR",
			Message: "schema applied but sync relationships failed: " + err.Error(),
			Status:  false,
		})
	}

	return c.JSON(gmod.SuccessMessageResponse{
		Code:    "SUCCESS",
		Message: fmt.Sprintf("Schema applied & relationships synced successfully version: %s", schemaVersion),
		Status:  true,
	})
}
