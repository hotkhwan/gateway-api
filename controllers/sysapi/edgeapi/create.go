package edgeapi

import (
	"strings"

	"klynx/internal/logger"
	"klynx/internal/services/systemsvc/edgesvc"
	"klynx/models/systemmod"
	"klynx/utils/httputil"
	"klynx/utils/traceutil"

	"github.com/gofiber/fiber/v2"
)

// CreateEdge godoc
// @Summary Create edge by type (encrypt secrets)
// @Tags System Edge
// @Accept json
// @Produce json
// @Param edgeType path string true "edge type" Enums(ata,svms,iboc)
// @Param body body systemmod.EdgeCreateReq true "payload"
// @Success 201 {object} systemmod.EdgeCreateSuccessResponse
// @Failure 400 {object} gmod.BadRequestResponse
// @Failure 401 {object} gmod.UnauthorizedResponse
// @Failure 500 {object} gmod.InternalErrorResponse
// @Router /system/edge/type/{edgeType} [post]
// @Security BearerAuth
func CreateEdge(c *fiber.Ctx) error {
	ctx, span, log := traceutil.Start(c.UserContext(), "klynx/edgeapi", "system.edge.Create", "edgeapi", "CreateEdge")
	defer span.End()
	_ = logger.FromCtx(ctx, "edgeapi", "CreateEdge")

	edgeType := strings.TrimSpace(c.Params("edgeType"))

	var req systemmod.EdgeCreateReq
	if err := c.BodyParser(&req); err != nil {
		log.Error().Err(err).Msg("invalid body")
		return httputil.FailBadRequest(c, "INVALID_BODY", "invalid json body")
	}

	// basic validate
	missing := []string{}
	if strings.TrimSpace(req.Username) == "" {
		missing = append(missing, "username")
	}
	if strings.TrimSpace(req.Password) == "" {
		missing = append(missing, "password")
	}
	if strings.TrimSpace(req.Name) == "" {
		missing = append(missing, "name")
	}
	if strings.TrimSpace(req.URL) == "" {
		missing = append(missing, "url")
	}
	if len(missing) > 0 {
		return httputil.FailBadRequest(c, "MISSING_FIELDS", "Missing required fields: "+strings.Join(missing, ","))
	}

	oid, err := edgesvc.CreateEdge(ctx, edgeType, req)
	if err != nil {
		log.Error().Err(err).Msg("create edge failed")
		return httputil.FailInternalReason(c, "internal server error", "CREATE_FAILED")
	}

	return c.Status(fiber.StatusCreated).JSON(systemmod.EdgeCreateSuccessResponse{
		Code:    "SUCCESS",
		ID:      oid.Hex(), // ✅ id root
		Message: "edge created",
		Status:  true,
	})
}
