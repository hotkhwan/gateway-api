// controllers/adminapi/kctrlRegistry.go
//
// HTTP handlers for the kctrl-registry inbound surface. The endpoint receives
// PATCH/DELETE calls from klynx-api's outbound adapter (Phase B 4.60.0+) and
// publishes the gateway-side projection of klynx-api's kcontrol approval
// state. Canonical contract: klynx-api/docs/contracts/kcontrol-gw-managed-
// registry.md v0.1.
package adminapi

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/hotkhwan/gateway-api/internal/services/kctrlregistrysvc"
	"github.com/hotkhwan/gateway-api/models/kctrlmod"
	"github.com/hotkhwan/gateway-api/utils/httputil"
	"github.com/hotkhwan/gateway-api/utils/traceutil"
)

// kctrlRegistrySvc is the narrow interface this controller depends on.
type kctrlRegistrySvc interface {
	Upsert(ctx context.Context, in kctrlregistrysvc.UpsertInput) (*kctrlmod.KctrlRegistry, error)
	Delete(ctx context.Context, hwId string) error
	ListDrift(ctx context.Context, staleAfter time.Duration) (*kctrlregistrysvc.DriftReport, error)
}

// KctrlRegistryController serves the admin-scoped REST surfaces for the
// kctrl-registry projection per contract §4.
type KctrlRegistryController struct {
	svc kctrlRegistrySvc
}

// NewKctrlRegistryController wires the controller against the service.
func NewKctrlRegistryController(svc kctrlRegistrySvc) *KctrlRegistryController {
	if svc == nil {
		panic("KctrlRegistryController: svc required")
	}
	return &KctrlRegistryController{svc: svc}
}

// acceptedRegistryFields enumerates the contract §4.1 whitelist.
var acceptedRegistryFields = map[string]struct{}{
	"orgId":      {},
	"approved":   {},
	"approvedAt": {},
	"approvedBy": {},
}

// Upsert godoc
// @Summary      Upsert kctrl-registry row (klynx → gw)
// @Description  klynx-api Phase B outbound calls this on ApproveDevice. Whitelists orgId/approved/approvedAt/approvedBy; rejects others with 400 FIELD_NOT_ACCEPTED. Idempotent by hash — repeated PATCH with the same body returns 200 without rewriting fields. Companion to klynx-api/docs/contracts/kcontrol-gw-managed-registry.md §4.1.
// @Tags         Admin.KctrlRegistry
// @Accept       json
// @Produce      json
// @Param        hwId path string true "device hwId (mac-style or vendor serial)"
// @Param        X-Active-Workspace header string true "gw workspace id"
// @Param        request body object true "accepted fields whitelist: orgId, approved, approvedAt, approvedBy"
// @Success      200 {object} gmod.SuccessDataResponse
// @Failure      400 {object} gmod.ApiErrorResponse
// @Failure      401 {object} gmod.ApiErrorResponse
// @Failure      403 {object} gmod.ApiErrorResponse
// @Failure      500 {object} gmod.ApiErrorResponse
// @Router       /admin/kctrl-registry/{hwId} [patch]
// @Security     BearerAuth
func (ctrl *KctrlRegistryController) Upsert(c fiber.Ctx) error {
	ctx, end, log := traceutil.StartLite(c, "gateway.adminapi", "KctrlRegistry.Upsert", "adminapi", "Upsert")
	defer end()

	hwId := c.Params("hwId")
	if hwId == "" {
		return httputil.FailBadRequest(c, "hwId path param required")
	}
	workspaceId, _ := c.Locals("activeWorkspace").(string)
	if workspaceId == "" {
		workspaceId = string(c.Request().Header.Peek("X-Active-Workspace"))
	}

	if len(c.Body()) == 0 {
		return httputil.FailBadRequest(c, "request body required")
	}
	var body map[string]any
	if err := c.Bind().Body(&body); err != nil {
		return httputil.FailBadRequest(c, "invalid JSON body")
	}
	if body == nil {
		return httputil.FailBadRequest(c, "request body must be a JSON object")
	}

	// Whitelist enforcement — collect every offending field in one pass per
	// the same UX as cameraOverlayInbound.
	var rejected []string
	for k := range body {
		if _, ok := acceptedRegistryFields[k]; !ok {
			rejected = append(rejected, k)
		}
	}
	if len(rejected) > 0 {
		log.Warn().Strs("fields", rejected).Msg("kctrl registry: rejected non-whitelisted fields")
		return httputil.Fail(c, fiber.StatusBadRequest, "FIELD_NOT_ACCEPTED", "request includes fields outside the contract whitelist", httputil.ErrorDetails{"fields": rejected})
	}

	in := kctrlregistrysvc.UpsertInput{
		HwId:        hwId,
		WorkspaceId: workspaceId,
	}
	if v, ok := body["orgId"].(string); ok {
		in.OrgId = v
	}
	if v, ok := body["approved"].(bool); ok {
		in.Approved = v
	}
	if v, ok := body["approvedAt"].(string); ok && v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			return httputil.Fail(c, fiber.StatusBadRequest, "VALIDATION_FAILED", "approvedAt must be RFC3339", httputil.ErrorDetails{"fields": []string{"approvedAt"}})
		}
		in.ApprovedAt = t
	}
	if v, ok := body["approvedBy"].(string); ok {
		in.ApprovedBy = v
	}

	updated, err := ctrl.svc.Upsert(ctx, in)
	if err != nil {
		log.Error().Err(err).Str("hwId", hwId).Msg("kctrl registry upsert failed")
		return httputil.FailInternal(c, "internal error")
	}

	// Contract §4.1 response header — lets klynx-api mark its local push-state
	// as confirmed without inspecting the body.
	c.Set("X-Sync-State-Echo", "synced")
	return httputil.Ok(c, updated, "kctrl registry upserted")
}

// Delete godoc
// @Summary      Delete kctrl-registry row (klynx → gw)
// @Description  klynx-api Phase B outbound calls this on UnapproveDevice. Idempotent — missing row returns 204 (not 404) per contract §4.2.
// @Tags         Admin.KctrlRegistry
// @Produce      json
// @Param        hwId path string true "device hwId"
// @Param        X-Active-Workspace header string true "gw workspace id"
// @Success      204
// @Failure      401 {object} gmod.ApiErrorResponse
// @Failure      403 {object} gmod.ApiErrorResponse
// @Failure      500 {object} gmod.ApiErrorResponse
// @Router       /admin/kctrl-registry/{hwId} [delete]
// @Security     BearerAuth
func (ctrl *KctrlRegistryController) Delete(c fiber.Ctx) error {
	ctx, end, log := traceutil.StartLite(c, "gateway.adminapi", "KctrlRegistry.Delete", "adminapi", "Delete")
	defer end()

	hwId := c.Params("hwId")
	if hwId == "" {
		return httputil.FailBadRequest(c, "hwId path param required")
	}
	if err := ctrl.svc.Delete(ctx, hwId); err != nil {
		log.Error().Err(err).Str("hwId", hwId).Msg("kctrl registry delete failed")
		return httputil.FailInternal(c, "internal error")
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// Drift godoc
// @Summary      List drifted kctrl-registry rows (operator)
// @Description  Returns rows where lastSyncFromKlynxAt is older than the configured stale window (default 1h). Operator triage view per contract §4.3.
// @Tags         Admin.System
// @Produce      json
// @Param        staleSecs query int false "stale window in seconds (default 3600)"
// @Success      200 {object} gmod.SuccessDataResponse
// @Failure      500 {object} gmod.ApiErrorResponse
// @Router       /admin/system/kctrlRegistryDrift [get]
// @Security     BearerAuth
func (ctrl *KctrlRegistryController) Drift(c fiber.Ctx) error {
	ctx, end, log := traceutil.StartLite(c, "gateway.adminapi", "KctrlRegistry.Drift", "adminapi", "Drift")
	defer end()

	staleAfter := time.Hour
	if q := c.Query("staleSecs"); q != "" {
		if n, err := strconv.Atoi(q); err == nil && n > 0 {
			staleAfter = time.Duration(n) * time.Second
		}
	}

	rep, err := ctrl.svc.ListDrift(ctx, staleAfter)
	if err != nil {
		log.Error().Err(err).Msg("kctrl registry drift failed")
		return httputil.FailInternal(c, "internal error")
	}
	return httputil.Ok(c, rep, "kctrl registry drift")
}

// Retry godoc
// @Summary      Acknowledge operator retry for a stale kctrl-registry row
// @Description  Operator-driven nudge that tells klynx-api to retry the outbox row for hwId. gateway-api itself has no retry state; this endpoint exists so klynx-api's outbound worker can reset attemptCount and re-attempt. Returns 200 immediately — the actual retry is klynx-api's responsibility per contract §7.3.
// @Tags         Admin.System
// @Produce      json
// @Param        hwId path string true "device hwId"
// @Success      200 {object} gmod.SuccessMessageResponse
// @Failure      500 {object} gmod.ApiErrorResponse
// @Router       /admin/system/kctrlRegistryRetry/{hwId} [post]
// @Security     BearerAuth
func (ctrl *KctrlRegistryController) Retry(c fiber.Ctx) error {
	_, end, _ := traceutil.StartLite(c, "gateway.adminapi", "KctrlRegistry.Retry", "adminapi", "Retry")
	defer end()

	hwId := c.Params("hwId")
	if hwId == "" {
		return httputil.FailBadRequest(c, "hwId path param required")
	}
	// gateway-api has nothing to retry locally — the outbox + worker live on
	// the klynx-api side. We acknowledge so operators get an HTTP 200, then
	// they call the equivalent klynx-api endpoint to actually flip
	// attemptCount=0 per contract §7.3.
	return httputil.MessageOK(c, "acknowledged; trigger retry on klynx-api outbox for hwId="+hwId)
}

// Compile-time assertion that the service contract is satisfied.
var _ kctrlRegistrySvc = (*kctrlregistrysvc.Service)(nil)

// safeguard against accidental import-loop refactor: keep errors imported.
var _ = errors.New
