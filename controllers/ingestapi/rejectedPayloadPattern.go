// controllers/ingestapi/rejectedPayloadPattern.go
package ingestapi

import (
	"errors"

	"github.com/gofiber/fiber/v3"
	"github.com/hotkhwan/gateway-api/internal/services/rejectedpayloadpatternsvc"
	"github.com/hotkhwan/gateway-api/models/gmod"
	"github.com/hotkhwan/gateway-api/utils/httputil"
	"github.com/hotkhwan/gateway-api/utils/traceutil"
)

type RejectedPayloadPatternController struct {
	svc *rejectedpayloadpatternsvc.RejectedPayloadPatternService
}

func NewRejectedPayloadPatternController(svc *rejectedpayloadpatternsvc.RejectedPayloadPatternService) *RejectedPayloadPatternController {
	if svc == nil {
		panic("RejectedPayloadPatternController: svc required")
	}
	return &RejectedPayloadPatternController{svc: svc}
}

func (h *RejectedPayloadPatternController) orgId(c fiber.Ctx) string {
	oid, _ := c.Locals("activeOrg").(string)
	return oid
}

// List godoc
// @Summary      List rejected payload patterns
// @Tags         ingest-rejected-payload-patterns
// @Security     BearerAuth
// @Param        X-Active-Org  header  string  true  "Active Org ID"
// @Param        page          query   int     false "Page" default(1)
// @Param        perPages      query   int     false "Per page" default(10)
// @Success      200  {object}  gmod.PaginationResponse
// @Failure      401  {object}  gmod.ApiErrorResponse
// @Router       /api/v1/ingest/rejectedPayloadPatterns [get]
func (h *RejectedPayloadPatternController) List(c fiber.Ctx) error {
	ctx, end, log := traceutil.StartLite(c, "gateway.ingestapi", "RejectedPayloadPatternController.List", "ingestapi", "ListRejectedPayloadPatterns")
	defer end()

	orgId := h.orgId(c)
	page := fiber.Query[int](c, "page", 1)
	perPage := fiber.Query[int](c, "perPages", 10)

	items, pag, err := h.svc.List(ctx, orgId, page, perPage)
	if err != nil {
		log.Error().Err(err).Msg("[ListRejectedPayloadPatterns] failed")
		return httputil.FailInternal(c, "failed to list rejected payload patterns")
	}
	return c.JSON(fiber.Map{
		"code":       gmod.CodeSuccess,
		"message":    "rejected payload patterns fetched",
		"status":     true,
		"details":    items,
		"pagination": pag,
	})
}

// Delete godoc
// @Summary      Delete rejected payload pattern
// @Tags         ingest-rejected-payload-patterns
// @Security     BearerAuth
// @Param        X-Active-Org  header  string  true  "Active Org ID"
// @Param        id            path    string  true  "Pattern ID"
// @Success      200  {object}  gmod.SuccessDataResponse
// @Failure      404  {object}  gmod.ApiErrorResponse
// @Router       /api/v1/ingest/rejectedPayloadPatterns/{id} [delete]
func (h *RejectedPayloadPatternController) Delete(c fiber.Ctx) error {
	ctx, end, log := traceutil.StartLite(c, "gateway.ingestapi", "RejectedPayloadPatternController.Delete", "ingestapi", "DeleteRejectedPayloadPattern")
	defer end()

	orgId := h.orgId(c)
	patternId := c.Params("id")

	if err := h.svc.Delete(ctx, orgId, patternId); err != nil {
		if errors.Is(err, rejectedpayloadpatternsvc.ErrNotFound) {
			return httputil.FailNotFound(c, "rejected payload pattern not found")
		}
		log.Error().Err(err).Msg("[DeleteRejectedPayloadPattern] failed")
		return httputil.FailInternal(c, "failed to delete rejected payload pattern")
	}

	log.Info().Str("patternId", patternId).Str("orgId", orgId).Msg("[DeleteRejectedPayloadPattern] deleted")
	return httputil.Ok(c, fiber.Map{"patternId": patternId}, "rejected payload pattern deleted")
}
