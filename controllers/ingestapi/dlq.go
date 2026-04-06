// controllers/ingestapi/dlq.go
package ingestapi

import (
	"errors"

	"github.com/gofiber/fiber/v3"
	"github.com/hotkhwan/gateway-api/internal/services/dlqsvc"
	"github.com/hotkhwan/gateway-api/utils/httputil"
	"github.com/hotkhwan/gateway-api/utils/traceutil"
)

// DLQController handles DLQ management endpoints.
type DLQController struct {
	svc *dlqsvc.DLQService
}

func NewDLQController(svc *dlqsvc.DLQService) *DLQController {
	if svc == nil {
		panic("DLQController: svc is required")
	}
	return &DLQController{svc: svc}
}

func (h *DLQController) tenantId(c fiber.Ctx) string {
	tid, _ := c.Locals("tenantId").(string)
	return tid
}

func (h *DLQController) orgId(c fiber.Ctx) string {
	org, _ := c.Locals("activeOrg").(string)
	return org
}

// ListDLQ godoc
// @Summary      List DLQ messages
// @Tags         ingest-dlq
// @Security     BearerAuth
// @Param        X-Active-Org  header    string  true   "Active Org ID"
// @Param        stage         query     string  false  "Filter by stage (normalize|deliver|webhook)"
// @Param        status        query     string  false  "Filter by status (pending|retrying|resolved|abandoned)"
// @Param        page          query     int     false  "Page number"    default(1)
// @Param        perPage       query     int     false  "Items per page" default(20)
// @Success      200  {object}  gmod.PaginatedResponse
// @Failure      401  {object}  gmod.ApiErrorResponse
// @Router       /ingest/dlq [get]
func (h *DLQController) ListDLQ(c fiber.Ctx) error {
	ctx, end, log := traceutil.StartLite(c, "gateway.ingestapi", "DLQController.ListDLQ", "ingestapi", "ListDLQ")
	defer end()

	tenantId := h.tenantId(c)
	orgId := h.orgId(c)
	stage := c.Query("stage")
	status := c.Query("status")
	page := fiber.Query[int](c, "page", 1)
	perPage := fiber.Query[int](c, "perPage", 20)

	log.Debug().Str("tenantId", tenantId).Str("orgId", orgId).Str("stage", stage).Str("status", status).Msg("[ListDLQ] listing")

	items, pag, err := h.svc.List(ctx, tenantId, orgId, stage, status, page, perPage)
	if err != nil {
		log.Error().Err(err).Str("tenantId", tenantId).Msg("[ListDLQ] failed")
		return httputil.FailInternal(c, "failed to list dlq messages")
	}

	return c.JSON(fiber.Map{
		"code":       "SUCCESS",
		"message":    "dlq messages fetched",
		"status":     true,
		"details":    items,
		"pagination": pag,
	})
}

// GetDLQStats godoc
// @Summary      DLQ aggregate stats
// @Tags         ingest-dlq
// @Security     BearerAuth
// @Param        X-Active-Org  header    string  true  "Active Org ID"
// @Success      200  {object}  gmod.SuccessDataResponse
// @Failure      401  {object}  gmod.ApiErrorResponse
// @Router       /ingest/dlq/stats [get]
func (h *DLQController) GetDLQStats(c fiber.Ctx) error {
	ctx, end, log := traceutil.StartLite(c, "gateway.ingestapi", "DLQController.GetDLQStats", "ingestapi", "GetDLQStats")
	defer end()

	tenantId := h.tenantId(c)
	orgId := h.orgId(c)

	log.Debug().Str("tenantId", tenantId).Str("orgId", orgId).Msg("[GetDLQStats] fetching stats")

	stats, err := h.svc.Stats(ctx, tenantId, orgId)
	if err != nil {
		log.Error().Err(err).Str("tenantId", tenantId).Msg("[GetDLQStats] failed")
		return httputil.FailInternal(c, "failed to fetch dlq stats")
	}

	return httputil.Ok(c, stats, "dlq stats fetched")
}

// GetDLQMessage godoc
// @Summary      Get DLQ message by ID
// @Tags         ingest-dlq
// @Security     BearerAuth
// @Param        X-Active-Org  header    string  true  "Active Org ID"
// @Param        id            path      string  true  "Message ID"
// @Success      200  {object}  gmod.SuccessDataResponse
// @Failure      404  {object}  gmod.ApiErrorResponse
// @Router       /ingest/dlq/{id} [get]
func (h *DLQController) GetDLQMessage(c fiber.Ctx) error {
	ctx, end, log := traceutil.StartLite(c, "gateway.ingestapi", "DLQController.GetDLQMessage", "ingestapi", "GetDLQMessage")
	defer end()

	tenantId := h.tenantId(c)
	messageId := c.Params("id")

	log.Debug().Str("tenantId", tenantId).Str("messageId", messageId).Msg("[GetDLQMessage] fetching")

	msg, err := h.svc.Get(ctx, tenantId, messageId)
	if err != nil {
		if errors.Is(err, dlqsvc.ErrDLQMessageNotFound) {
			return httputil.FailNotFound(c, err.Error())
		}
		log.Error().Err(err).Str("messageId", messageId).Msg("[GetDLQMessage] failed")
		return httputil.FailInternal(c, "failed to get dlq message")
	}

	return httputil.Ok(c, msg, "dlq message fetched")
}

// RetryDLQ godoc
// @Summary      Schedule a retry for a DLQ message
// @Tags         ingest-dlq
// @Security     BearerAuth
// @Param        X-Active-Org  header    string  true  "Active Org ID"
// @Param        id            path      string  true  "Message ID"
// @Success      200  {object}  gmod.SuccessDataResponse
// @Failure      400  {object}  gmod.ApiErrorResponse
// @Failure      404  {object}  gmod.ApiErrorResponse
// @Router       /ingest/dlq/{id}/retry [post]
func (h *DLQController) RetryDLQ(c fiber.Ctx) error {
	ctx, end, log := traceutil.StartLite(c, "gateway.ingestapi", "DLQController.RetryDLQ", "ingestapi", "RetryDLQ")
	defer end()

	tenantId := h.tenantId(c)
	messageId := c.Params("id")

	log.Info().Str("tenantId", tenantId).Str("messageId", messageId).Msg("[RetryDLQ] scheduling retry")

	result, err := h.svc.Retry(ctx, tenantId, messageId)
	if err != nil {
		switch {
		case errors.Is(err, dlqsvc.ErrDLQMessageNotFound):
			return httputil.FailNotFound(c, err.Error())
		case errors.Is(err, dlqsvc.ErrAlreadyResolved),
			errors.Is(err, dlqsvc.ErrMaxRetriesExceeded),
			errors.Is(err, dlqsvc.ErrRetryTooSoon):
			return httputil.FailBadRequest(c, err.Error())
		}
		log.Error().Err(err).Str("messageId", messageId).Msg("[RetryDLQ] failed")
		return httputil.FailInternal(c, "failed to retry dlq message")
	}

	log.Info().Str("messageId", messageId).Int("retryCount", result.RetryCount).Msg("[RetryDLQ] retry scheduled")
	return httputil.Ok(c, result, "dlq message retry scheduled")
}

// ReplayDLQ godoc
// @Summary      Force-replay a DLQ message (bypasses retry limits)
// @Tags         ingest-dlq
// @Security     BearerAuth
// @Param        X-Active-Org  header    string  true  "Active Org ID"
// @Param        id            path      string  true  "Message ID"
// @Success      200  {object}  gmod.SuccessDataResponse
// @Failure      404  {object}  gmod.ApiErrorResponse
// @Router       /ingest/dlq/{id}/replay [post]
func (h *DLQController) ReplayDLQ(c fiber.Ctx) error {
	ctx, end, log := traceutil.StartLite(c, "gateway.ingestapi", "DLQController.ReplayDLQ", "ingestapi", "ReplayDLQ")
	defer end()

	tenantId := h.tenantId(c)
	messageId := c.Params("id")

	log.Info().Str("tenantId", tenantId).Str("messageId", messageId).Msg("[ReplayDLQ] force replay")

	result, err := h.svc.Replay(ctx, tenantId, messageId)
	if err != nil {
		if errors.Is(err, dlqsvc.ErrDLQMessageNotFound) {
			return httputil.FailNotFound(c, err.Error())
		}
		log.Error().Err(err).Str("messageId", messageId).Msg("[ReplayDLQ] failed")
		return httputil.FailInternal(c, "failed to replay dlq message")
	}

	log.Info().Str("messageId", messageId).Msg("[ReplayDLQ] replayed")
	return httputil.Ok(c, result, "dlq message replayed")
}

// AbandonDLQ godoc
// @Summary      Abandon a DLQ message (no further retries)
// @Tags         ingest-dlq
// @Security     BearerAuth
// @Param        X-Active-Org  header    string  true  "Active Org ID"
// @Param        id            path      string  true  "Message ID"
// @Success      200  {object}  gmod.SuccessMessageResponse
// @Failure      404  {object}  gmod.ApiErrorResponse
// @Router       /ingest/dlq/{id}/abandon [post]
func (h *DLQController) AbandonDLQ(c fiber.Ctx) error {
	ctx, end, log := traceutil.StartLite(c, "gateway.ingestapi", "DLQController.AbandonDLQ", "ingestapi", "AbandonDLQ")
	defer end()

	tenantId := h.tenantId(c)
	messageId := c.Params("id")

	log.Info().Str("tenantId", tenantId).Str("messageId", messageId).Msg("[AbandonDLQ] abandoning")

	if err := h.svc.Abandon(ctx, tenantId, messageId); err != nil {
		if errors.Is(err, dlqsvc.ErrDLQMessageNotFound) {
			return httputil.FailNotFound(c, err.Error())
		}
		log.Error().Err(err).Str("messageId", messageId).Msg("[AbandonDLQ] failed")
		return httputil.FailInternal(c, "failed to abandon dlq message")
	}

	log.Info().Str("messageId", messageId).Msg("[AbandonDLQ] abandoned")
	return httputil.MessageOK(c, "dlq message abandoned")
}
