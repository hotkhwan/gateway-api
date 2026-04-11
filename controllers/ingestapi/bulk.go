// controllers/ingestapi/bulk.go
package ingestapi

import (
	"github.com/gofiber/fiber/v3"
	"github.com/hotkhwan/gateway-api/internal/services/ingestsvc"
	"github.com/hotkhwan/gateway-api/utils/httputil"
	"github.com/hotkhwan/gateway-api/utils/traceutil"
)

// BulkController handles bulk operations on pending events.
type BulkController struct {
	service *ingestsvc.BulkService
}

// NewBulkController creates a BulkController.
func NewBulkController(service *ingestsvc.BulkService) *BulkController {
	if service == nil {
		panic("BulkService required")
	}
	return &BulkController{service: service}
}

// bulkRequest is the request body for bulk approve / reject / delete.
type bulkRequest struct {
	EventIds []string `json:"eventIds"`
}

// bulkApplyTemplateRequest is the request body for bulk applyTemplate.
type bulkApplyTemplateRequest struct {
	TemplateId string   `json:"templateId"`
	EventIds   []string `json:"eventIds"`
}

func mustBulkLocals(c fiber.Ctx) (tenantId, orgId, callerUserId string) {
	tenantId, _ = c.Locals("tenantId").(string)
	orgId, _ = c.Locals("activeWorkspace").(string)
	callerUserId, _ = c.Locals("userId").(string)
	return
}

// BulkApprove godoc
// @Summary      Bulk approve pending events
// @Description  Approves up to 100 pending events in a single request. Returns per-event success/failure detail.
// @Tags         ingest-bulk
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        X-Active-Org  header    string       true  "Active Org ID"
// @Param        body          body      bulkRequest  true  "List of event IDs to approve (max 100)"
// @Success      200  {object}  ingestsvc.BulkResult
// @Failure      400  {object}  gmod.ApiErrorResponse
// @Failure      401  {object}  gmod.ApiErrorResponse
// @Failure      500  {object}  gmod.ApiErrorResponse
// @Router       /ingest/management/bulk/approve [post]
func (ctrl *BulkController) BulkApprove(c fiber.Ctx) error {
	ctx, end, log := traceutil.StartLite(c, "gateway.ingestapi", "BulkController.BulkApprove", "ingestapi", "BulkApprove")
	defer end()

	tenantId, orgId, callerUserId := mustBulkLocals(c)
	log.Info().Str("orgId", orgId).Msg("📥 [BulkApprove] request received")

	var req bulkRequest
	if err := c.Bind().Body(&req); err != nil || len(req.EventIds) == 0 {
		log.Warn().Str("orgId", orgId).Msg("❌ [BulkApprove] invalid or empty eventIds")
		return httputil.FailBadRequest(c, "eventIds is required and must not be empty")
	}
	if len(req.EventIds) > 100 {
		log.Warn().Str("orgId", orgId).Int("count", len(req.EventIds)).Msg("❌ [BulkApprove] batch too large")
		return httputil.FailBadRequest(c, "eventIds exceeds maximum batch size of 100")
	}

	result := ctrl.service.BulkApprove(ctx, tenantId, orgId, callerUserId, req.EventIds)
	log.Info().Str("orgId", orgId).Int("succeeded", len(result.Succeeded)).Int("failed", len(result.Failed)).Msg("✅ [BulkApprove] done")
	return httputil.Ok(c, result)
}

// BulkReject godoc
// @Summary      Bulk reject pending events
// @Description  Rejects up to 100 pending events in a single request. Returns per-event success/failure detail.
// @Tags         ingest-bulk
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        X-Active-Org  header    string       true  "Active Org ID"
// @Param        body          body      bulkRequest  true  "List of event IDs to reject (max 100)"
// @Success      200  {object}  ingestsvc.BulkResult
// @Failure      400  {object}  gmod.ApiErrorResponse
// @Failure      401  {object}  gmod.ApiErrorResponse
// @Failure      500  {object}  gmod.ApiErrorResponse
// @Router       /ingest/management/bulk/reject [post]
func (ctrl *BulkController) BulkReject(c fiber.Ctx) error {
	ctx, end, log := traceutil.StartLite(c, "gateway.ingestapi", "BulkController.BulkReject", "ingestapi", "BulkReject")
	defer end()

	tenantId, orgId, callerUserId := mustBulkLocals(c)
	log.Info().Str("orgId", orgId).Msg("📥 [BulkReject] request received")

	var req bulkRequest
	if err := c.Bind().Body(&req); err != nil || len(req.EventIds) == 0 {
		log.Warn().Str("orgId", orgId).Msg("❌ [BulkReject] invalid or empty eventIds")
		return httputil.FailBadRequest(c, "eventIds is required and must not be empty")
	}
	if len(req.EventIds) > 100 {
		log.Warn().Str("orgId", orgId).Int("count", len(req.EventIds)).Msg("❌ [BulkReject] batch too large")
		return httputil.FailBadRequest(c, "eventIds exceeds maximum batch size of 100")
	}

	result := ctrl.service.BulkReject(ctx, tenantId, orgId, callerUserId, req.EventIds)
	log.Info().Str("orgId", orgId).Int("succeeded", len(result.Succeeded)).Int("failed", len(result.Failed)).Msg("✅ [BulkReject] done")
	return httputil.Ok(c, result)
}

// BulkDelete godoc
// @Summary      Bulk delete pending events
// @Description  Deletes up to 100 pending events in a single request. Returns per-event success/failure detail.
// @Tags         ingest-bulk
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        X-Active-Org  header    string       true  "Active Org ID"
// @Param        body          body      bulkRequest  true  "List of event IDs to delete (max 100)"
// @Success      200  {object}  ingestsvc.BulkResult
// @Failure      400  {object}  gmod.ApiErrorResponse
// @Failure      401  {object}  gmod.ApiErrorResponse
// @Failure      500  {object}  gmod.ApiErrorResponse
// @Router       /ingest/management/bulk/delete [post]
func (ctrl *BulkController) BulkDelete(c fiber.Ctx) error {
	ctx, end, log := traceutil.StartLite(c, "gateway.ingestapi", "BulkController.BulkDelete", "ingestapi", "BulkDelete")
	defer end()

	tenantId, orgId, callerUserId := mustBulkLocals(c)
	log.Info().Str("orgId", orgId).Msg("📥 [BulkDelete] request received")

	var req bulkRequest
	if err := c.Bind().Body(&req); err != nil || len(req.EventIds) == 0 {
		log.Warn().Str("orgId", orgId).Msg("❌ [BulkDelete] invalid or empty eventIds")
		return httputil.FailBadRequest(c, "eventIds is required and must not be empty")
	}
	if len(req.EventIds) > 100 {
		log.Warn().Str("orgId", orgId).Int("count", len(req.EventIds)).Msg("❌ [BulkDelete] batch too large")
		return httputil.FailBadRequest(c, "eventIds exceeds maximum batch size of 100")
	}

	result := ctrl.service.BulkDelete(ctx, tenantId, orgId, callerUserId, req.EventIds)
	log.Info().Str("orgId", orgId).Int("succeeded", len(result.Succeeded)).Int("failed", len(result.Failed)).Msg("✅ [BulkDelete] done")
	return httputil.Ok(c, result)
}

// BulkApplyTemplate godoc
// @Summary      Bulk apply template and auto-approve events
// @Description  Applies a mapping template to up to 100 pending events and auto-approves each one. Returns per-event success/failure detail.
// @Tags         ingest-bulk
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        X-Active-Org  header    string                    true  "Active Org ID"
// @Param        body          body      bulkApplyTemplateRequest  true  "Template ID and list of event IDs (max 100)"
// @Success      200  {object}  ingestsvc.BulkResult
// @Failure      400  {object}  gmod.ApiErrorResponse
// @Failure      401  {object}  gmod.ApiErrorResponse
// @Failure      500  {object}  gmod.ApiErrorResponse
// @Router       /ingest/management/bulk/applyTemplate [post]
func (ctrl *BulkController) BulkApplyTemplate(c fiber.Ctx) error {
	ctx, end, log := traceutil.StartLite(c, "gateway.ingestapi", "BulkController.BulkApplyTemplate", "ingestapi", "BulkApplyTemplate")
	defer end()

	tenantId, orgId, callerUserId := mustBulkLocals(c)
	log.Info().Str("orgId", orgId).Msg("📥 [BulkApplyTemplate] request received")

	var req bulkApplyTemplateRequest
	if err := c.Bind().Body(&req); err != nil {
		log.Warn().Str("orgId", orgId).Err(err).Msg("❌ [BulkApplyTemplate] body parse error")
		return httputil.FailBadRequest(c, "invalid request body")
	}
	if req.TemplateId == "" {
		log.Warn().Str("orgId", orgId).Msg("❌ [BulkApplyTemplate] missing templateId")
		return httputil.FailBadRequest(c, "templateId is required")
	}
	if len(req.EventIds) == 0 {
		log.Warn().Str("orgId", orgId).Msg("❌ [BulkApplyTemplate] empty eventIds")
		return httputil.FailBadRequest(c, "eventIds is required and must not be empty")
	}
	if len(req.EventIds) > 100 {
		log.Warn().Str("orgId", orgId).Int("count", len(req.EventIds)).Msg("❌ [BulkApplyTemplate] batch too large")
		return httputil.FailBadRequest(c, "eventIds exceeds maximum batch size of 100")
	}

	result := ctrl.service.BulkApplyTemplate(ctx, tenantId, orgId, callerUserId, req.TemplateId, req.EventIds)
	log.Info().
		Str("orgId", orgId).
		Str("templateId", req.TemplateId).
		Int("succeeded", len(result.Succeeded)).
		Int("failed", len(result.Failed)).
		Msg("✅ [BulkApplyTemplate] done")
	return httputil.Ok(c, result)
}
