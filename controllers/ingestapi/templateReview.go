// controllers/ingestapi/unknownPayloadReview.go
package ingestapi

import (
	"errors"

	"github.com/gofiber/fiber/v3"
	"github.com/hotkhwan/gateway-api/internal/services/rejectedpayloadpatternsvc"
	"github.com/hotkhwan/gateway-api/internal/services/unknownpayloadreviewsvc"
	"github.com/hotkhwan/gateway-api/models/gmod"
	"github.com/hotkhwan/gateway-api/utils/httputil"
	"github.com/hotkhwan/gateway-api/utils/traceutil"
)

type UnknownPayloadReviewController struct {
	svc        *unknownpayloadreviewsvc.UnknownPayloadReviewService
	patternSvc *rejectedpayloadpatternsvc.RejectedPayloadPatternService
}

func NewUnknownPayloadReviewController(
	svc *unknownpayloadreviewsvc.UnknownPayloadReviewService,
	patternSvc *rejectedpayloadpatternsvc.RejectedPayloadPatternService,
) *UnknownPayloadReviewController {
	if svc == nil {
		panic("UnknownPayloadReviewController: svc required")
	}
	if patternSvc == nil {
		panic("UnknownPayloadReviewController: patternSvc required")
	}
	return &UnknownPayloadReviewController{svc: svc, patternSvc: patternSvc}
}

func (h *UnknownPayloadReviewController) orgId(c fiber.Ctx) string {
	oid, _ := c.Locals("activeOrg").(string)
	return oid
}

func (h *UnknownPayloadReviewController) userId(c fiber.Ctx) string {
	uid, _ := c.Locals("userId").(string)
	return uid
}

// List godoc
// @Summary      List unknown payload reviews
// @Tags         ingest-unknown-payload-reviews
// @Security     BearerAuth
// @Param        X-Active-Org  header  string  true  "Active Org ID"
// @Param        page          query   int     false "Page" default(1)
// @Param        perPages      query   int     false "Per page" default(10)
// @Success      200  {object}  gmod.PaginationResponse
// @Failure      401  {object}  gmod.ApiErrorResponse
// @Router       /ingest/unknownPayloadReviews [get]
func (h *UnknownPayloadReviewController) List(c fiber.Ctx) error {
	ctx, end, log := traceutil.StartLite(c, "gateway.ingestapi", "UnknownPayloadReviewController.List", "ingestapi", "ListUnknownPayloadReviews")
	defer end()

	orgId := h.orgId(c)
	page := fiber.Query[int](c, "page", 1)
	perPage := fiber.Query[int](c, "perPages", 10)

	items, pag, err := h.svc.List(ctx, orgId, page, perPage)
	if err != nil {
		log.Error().Err(err).Msg("[ListUnknownPayloadReviews] failed")
		return httputil.FailInternal(c, "failed to list unknown payload reviews")
	}
	return c.JSON(fiber.Map{
		"code":       gmod.CodeSuccess,
		"message":    "unknown payload reviews fetched",
		"status":     true,
		"details":    items,
		"pagination": pag,
	})
}

// Get godoc
// @Summary      Get unknown payload review
// @Tags         ingest-unknown-payload-reviews
// @Security     BearerAuth
// @Param        X-Active-Org  header  string  true  "Active Org ID"
// @Param        id            path    string  true  "Review ID"
// @Success      200  {object}  gmod.SuccessDataResponse
// @Failure      404  {object}  gmod.ApiErrorResponse
// @Router       /ingest/unknownPayloadReviews/{id} [get]
func (h *UnknownPayloadReviewController) Get(c fiber.Ctx) error {
	ctx, end, log := traceutil.StartLite(c, "gateway.ingestapi", "UnknownPayloadReviewController.Get", "ingestapi", "GetUnknownPayloadReview")
	defer end()

	orgId := h.orgId(c)
	reviewId := c.Params("id")

	rev, err := h.svc.Get(ctx, orgId, reviewId)
	if err != nil {
		if errors.Is(err, unknownpayloadreviewsvc.ErrNotFound) {
			return httputil.FailNotFound(c, "unknown payload review not found")
		}
		log.Error().Err(err).Msg("[GetUnknownPayloadReview] failed")
		return httputil.FailInternal(c, "failed to get unknown payload review")
	}
	return httputil.Ok(c, rev, "unknown payload review fetched")
}

// Reject godoc
// @Summary      Reject unknown payload review and create a rejected payload pattern
// @Tags         ingest-unknown-payload-reviews
// @Security     BearerAuth
// @Param        X-Active-Org  header  string  true  "Active Org ID"
// @Param        id            path    string  true  "Review ID"
// @Success      200  {object}  gmod.SuccessDataResponse
// @Failure      404  {object}  gmod.ApiErrorResponse
// @Router       /ingest/unknownPayloadReviews/{id}/reject [post]
func (h *UnknownPayloadReviewController) Reject(c fiber.Ctx) error {
	ctx, end, log := traceutil.StartLite(c, "gateway.ingestapi", "UnknownPayloadReviewController.Reject", "ingestapi", "RejectUnknownPayloadReview")
	defer end()

	orgId := h.orgId(c)
	reviewId := c.Params("id")
	createdBy := h.userId(c)

	rev, err := h.svc.Reject(ctx, orgId, reviewId)
	if err != nil {
		if errors.Is(err, unknownpayloadreviewsvc.ErrNotFound) {
			return httputil.FailNotFound(c, "unknown payload review not found")
		}
		log.Error().Err(err).Msg("[RejectUnknownPayloadReview] failed")
		return httputil.FailInternal(c, "failed to reject unknown payload review")
	}

	// Create rejectedPayloadPattern to silently drop future payloads with same shape
	patternId, patternErr := h.patternSvc.Create(ctx, orgId, rev.SourceFamily, rev.Fingerprint, "operatorRejected", createdBy)
	if patternErr != nil && !errors.Is(patternErr, rejectedpayloadpatternsvc.ErrAlreadyExists) {
		log.Error().Err(patternErr).Str("reviewId", reviewId).Msg("[RejectUnknownPayloadReview] failed to create rejected pattern (non-fatal)")
	}

	log.Info().
		Str("reviewId", reviewId).
		Str("orgId", orgId).
		Str("patternId", patternId).
		Msg("[RejectUnknownPayloadReview] rejected and pattern created")

	return httputil.Ok(c, fiber.Map{
		"reviewId":     rev.ReviewId,
		"sourceFamily": rev.SourceFamily,
		"fingerprint":  rev.Fingerprint,
		"patternId":    patternId,
	}, "unknown payload review rejected")
}
