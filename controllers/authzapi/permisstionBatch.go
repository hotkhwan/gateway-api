// controllers/authzapi/permisstionBatch.go
package authzapi

import (
	"github.com/hotkhwan/gateway-api/internal/logger"
	"github.com/hotkhwan/gateway-api/internal/services/authzsvc"
	"github.com/hotkhwan/gateway-api/models/authzmod"
	"github.com/hotkhwan/gateway-api/models/gmod"
	"github.com/hotkhwan/gateway-api/utils/traceutil"

	"github.com/gofiber/fiber/v2"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
)

// PermissionCheckBatchHandler handles batch permission checks
// @Summary      Batch permission check
// @Description  Check multiple permissions in a single request
// @Tags         Authz
// @Accept       json
// @Produce      json
// @Param        body body []authzmod.PermissionCheckRequest true "Batch permission check requests"
// @Success      200 {array} map[string]interface{} "Batch permission check results"
// @Failure      400 {object} gmod.SuccessMessageResponse "Invalid request body"
// @Failure      500 {object} gmod.SuccessMessageResponse "Internal server error"
// @Router       /authz/permission/check/batch [post]
// @Security     BearerAuth
func PermissionCheckBatchHandler(c *fiber.Ctx) error {
	ctx := c.UserContext()
	tracer := otel.Tracer("github.com/hotkhwan/gateway-api/authzapi")
	ctx, span := tracer.Start(ctx, "Authz.PermissionCheckBatchHandler")
	defer span.End()

	// attach traceID to logger
	if sc := trace.SpanContextFromContext(ctx); sc.IsValid() {
		ctx = traceutil.WithTraceID(ctx, sc.TraceID().String())
	}
	log := logger.FromCtx(ctx, "authzapi", "PermissionCheckBatchHandler")

	var reqs []authzmod.PermissionCheckRequest
	if err := c.BodyParser(&reqs); err != nil {
		log.Error().Err(err).Msg("invalid request body")
		return c.Status(fiber.StatusBadRequest).JSON(gmod.SuccessMessageResponse{
			Code:    "INVALID_REQUEST",
			Message: "invalid request body",
			Status:  false,
		})
	}

	// แปลงเป็น inputs สำหรับ batch API (ไม่มี Depth ใน struct)
	inputs := make([]authzmod.PermissionCheckInput, 0, len(reqs))
	for _, r := range reqs {
		inputs = append(inputs, authzmod.PermissionCheckInput{
			EntityType:  r.Entity.Type,
			EntityID:    r.Entity.ID,
			Permission:  r.Permission,
			SubjectType: r.Subject.Type,
			SubjectID:   r.Subject.ID,
		})
	}

	// ใช้ batch matrix service → จะได้ผลลัพธ์ [entityID][permission] = allowed
	matrix, err := authzsvc.CheckPermissionsBatchMatrix(ctx, inputs)
	if err != nil {
		log.Error().Err(err).Msg("batch permission check failed")
		return c.Status(fiber.StatusInternalServerError).JSON(gmod.SuccessMessageResponse{
			Code:    "ERROR",
			Message: "batch permission check failed",
			Status:  false,
		})
	}

	// map กลับตาม index ของ reqs
	results := make([]map[string]interface{}, 0, len(reqs))
	for idx, r := range reqs {
		entID := r.Entity.ID
		perm := r.Permission
		allowed := false

		if perMap, ok := matrix[entID]; ok {
			if v, ok2 := perMap[perm]; ok2 {
				allowed = v
			}
		}

		results = append(results, map[string]interface{}{
			"idx":     idx,
			"allowed": allowed,
			"entity": map[string]string{
				"type": r.Entity.Type,
				"id":   r.Entity.ID,
			},
			"permission": r.Permission,
			"subject": map[string]string{
				"type": r.Subject.Type,
				"id":   r.Subject.ID,
			},
		})
	}

	log.Info().Int("count", len(results)).Msg("batch permission check completed")
	return c.JSON(results)
}
