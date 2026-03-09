// controllers/sysapi/edgeapi/create.go
package edgeapi

import (
	"strings"

	"github.com/hotkhwan/gateway-api/internal/services/systemsvc/edgesvc"
	"github.com/hotkhwan/gateway-api/models/systemmod"
	"github.com/hotkhwan/gateway-api/utils/httputil"
	"github.com/hotkhwan/gateway-api/utils/traceutil"

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
	ctx, end, log := traceutil.StartLite(c.UserContext(), "gateway.edgeapi", "EdgeApi.CreateEdge", "sysapi", "CreateEdge")
	defer end()

	edgeType := strings.TrimSpace(c.Params("edgeType"))

	var req systemmod.EdgeCreateReq
	if err := c.BodyParser(&req); err != nil {
		log.Error().Err(err).Msg("invalid body")
		return httputil.FailBadRequest(c, "invalid json body")
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
		return httputil.FailBadRequest(c, "Missing required fields: "+strings.Join(missing, ","))
	}

	oid, err := edgesvc.CreateEdge(ctx, edgeType, req)
	if err != nil {
		log.Error().Err(err).Msg("create edge failed")
		return httputil.FailInternalReason(c, "internal server error", "CREATE_FAILED")
	}

	return httputil.Created(c, fiber.Map{
		"id": oid.Hex(),
	}, "edge created")
}
