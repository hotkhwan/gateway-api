// controllers/ingestapi/templateReview.go
package ingestapi

import (
	"errors"

	"github.com/gofiber/fiber/v2"
	"github.com/hotkhwan/gateway-api/internal/services/templatereviewsvc"
	"github.com/hotkhwan/gateway-api/models/gmod"
	"github.com/hotkhwan/gateway-api/utils/httputil"
	"github.com/hotkhwan/gateway-api/utils/traceutil"
)

type TemplateReviewController struct {
	svc *templatereviewsvc.TemplateReviewService
}

func NewTemplateReviewController(svc *templatereviewsvc.TemplateReviewService) *TemplateReviewController {
	if svc == nil {
		panic("TemplateReviewController: svc required")
	}
	return &TemplateReviewController{svc: svc}
}

func (h *TemplateReviewController) tenantId(c *fiber.Ctx) string {
	tid, _ := c.Locals("tenantId").(string)
	return tid
}

func (h *TemplateReviewController) orgId(c *fiber.Ctx) string {
	oid, _ := c.Locals("activeOrg").(string)
	return oid
}

func (h *TemplateReviewController) List(c *fiber.Ctx) error {
	ctx, end, log := traceutil.StartLite(c.UserContext(), "gateway.ingestapi", "TemplateReviewController.List", "ingestapi", "ListTemplateReviews")
	defer end()

	tenantId := h.tenantId(c)
	orgId := h.orgId(c)
	page := c.QueryInt("page", 1)
	perPage := c.QueryInt("perPages", 10)

	items, pag, err := h.svc.List(ctx, tenantId, orgId, page, perPage)
	if err != nil {
		log.Error().Err(err).Msg("[ListTemplateReviews] failed")
		return httputil.FailInternal(c, "failed to list template reviews")
	}
	return c.JSON(fiber.Map{
		"code":       gmod.CodeSuccess,
		"message":    "template reviews fetched",
		"status":     true,
		"details":    items,
		"pagination": pag,
	})
}

func (h *TemplateReviewController) Get(c *fiber.Ctx) error {
	ctx, end, log := traceutil.StartLite(c.UserContext(), "gateway.ingestapi", "TemplateReviewController.Get", "ingestapi", "GetTemplateReview")
	defer end()

	tenantId := h.tenantId(c)
	orgId := h.orgId(c)
	reviewId := c.Params("id")

	rev, err := h.svc.Get(ctx, tenantId, orgId, reviewId)
	if err != nil {
		if errors.Is(err, templatereviewsvc.ErrNotFound) {
			return httputil.FailNotFound(c, "template review not found")
		}
		log.Error().Err(err).Msg("[GetTemplateReview] failed")
		return httputil.FailInternal(c, "failed to get template review")
	}
	return httputil.Ok(c, rev, "template review fetched")
}

func (h *TemplateReviewController) Archive(c *fiber.Ctx) error {
	ctx, end, log := traceutil.StartLite(c.UserContext(), "gateway.ingestapi", "TemplateReviewController.Archive", "ingestapi", "ArchiveTemplateReview")
	defer end()

	tenantId := h.tenantId(c)
	orgId := h.orgId(c)
	reviewId := c.Params("id")

	if err := h.svc.Archive(ctx, tenantId, orgId, reviewId); err != nil {
		if errors.Is(err, templatereviewsvc.ErrNotFound) {
			return httputil.FailNotFound(c, "template review not found")
		}
		log.Error().Err(err).Msg("[ArchiveTemplateReview] failed")
		return httputil.FailInternal(c, "failed to archive template review")
	}

	log.Info().Str("reviewId", reviewId).Msg("[ArchiveTemplateReview] archived")
	return httputil.MessageOK(c, "template review archived")
}
