// controllers/rscapi/resourceBulk.go
package rscapi

import (
	"context"
	"time"

	"github.com/hotkhwan/gateway-api/internal/services/rscsvc"
	"github.com/hotkhwan/gateway-api/models/gmod"
	"github.com/hotkhwan/gateway-api/models/rscmod"
	"github.com/hotkhwan/gateway-api/utils/traceutil"

	"github.com/gofiber/fiber/v2"
)

// BulkCreateResources godoc
// @Summary     Bulk create resources
// @Description Create multiple resources in one request. The service validates required fields, removes in-payload duplicates by (provider,type,canonicalId), skips existing duplicates in DB, then inserts the rest.
// @Tags        Resources
// @Accept      json
// @Produce     json
// @Param       body  body     []rscmod.ResourceUpsert  true  "Array of resources to create"
// @Success     201   {object} gmod.SuccessMessageBulkCreateResponse "Bulk resources created"
// @Failure     400   {object} gmod.SuccessMessageBulkCreateResponse "Bad request / invalid body"
// @Failure     500   {object} gmod.SuccessMessageBulkCreateResponse "Internal error"
// @Router      /rsc/v1/resources/bulk [post]
func BulkCreateResources(c *fiber.Ctx) error {
	_, end, log := traceutil.StartLite(c.UserContext(), "gateway.rscapi", "BulkCreateResources", "rscapi", "BulkCreateResources")
	defer end()

	var payload []rscmod.ResourceUpsert
	if err := c.BodyParser(&payload); err != nil || len(payload) == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(gmod.SuccessMessageBulkCreateResponse{
			Code:     "BAD_REQUEST",
			Message:  "body must be a non-empty JSON array of ResourceUpsert",
			Status:   false,
			Inserted: 0,
			Skipped:  0,
			IDs:      []string{},
			Errors:   nil,
		})
	}

	ctx, cancel := context.WithTimeout(c.Context(), 20*time.Second)
	defer cancel()

	result, err := rscsvc.ResourceBulkCreate(ctx, payload)
	if err != nil {
		log.Error().Err(err).Msg("bulk create failed")
		return c.Status(fiber.StatusInternalServerError).JSON(gmod.SuccessMessageBulkCreateResponse{
			Code:     "BULK_CREATE_FAILED",
			Message:  err.Error(),
			Status:   false,
			Inserted: result.Inserted,
			Skipped:  result.Skipped,
			IDs:      result.IDs,
			Errors:   toBulkErrors(result.Errors),
		})
	}

	return c.Status(fiber.StatusCreated).JSON(gmod.SuccessMessageBulkCreateResponse{
		Code:     "SUCCESS",
		Message:  "Bulk resources created",
		Status:   true,
		Inserted: result.Inserted,
		Skipped:  result.Skipped,
		IDs:      result.IDs,
		Errors:   toBulkErrors(result.Errors),
	})
}

func toBulkErrors(in []struct {
	Index   int
	Reason  string
	Details string
}) []gmod.BulkError {
	if len(in) == 0 {
		return nil
	}
	out := make([]gmod.BulkError, 0, len(in))
	for _, e := range in {
		out = append(out, gmod.BulkError{
			Index:   e.Index,
			Reason:  e.Reason,
			Details: e.Details,
		})
	}
	return out
}
