// controllers/adminapi/cameraOverlayInbound.go
package adminapi

import (
	"context"
	"errors"

	"github.com/gofiber/fiber/v3"
	"github.com/hotkhwan/gateway-api/internal/services/devicemgmtsvc"
	"github.com/hotkhwan/gateway-api/models/ingestmod"
	"github.com/hotkhwan/gateway-api/utils/httputil"
	"github.com/hotkhwan/gateway-api/utils/traceutil"
)

// cameraOverlaySvc is the narrow interface CameraOverlayInboundController uses
// so handler tests can swap a fake without standing up Mongo / Kafka.
type cameraOverlaySvc interface {
	ApplyKlynxOverlay(
		ctx context.Context,
		tenantId, workspaceId, deviceMgmtId string,
		body map[string]any,
		ifMatch string,
	) (*ingestmod.DeviceManagement, devicemgmtsvc.IfMatchStatus, error)
}

// CameraOverlayInboundController serves PATCH /admin/device-management/cameras/{gwDeviceMgmtId}
// per klynx-api/docs/contracts/camera-gw-managed-overlay.md §8.
//
// Auth (v1, locked by Codex): forwarded operator Bearer JWT + X-Active-Workspace.
// Idempotency: optional If-Match → replay-only, surfaced via X-If-Match-Status; no 409 in v1.
type CameraOverlayInboundController struct {
	svc cameraOverlaySvc
}

// NewCameraOverlayInboundController wires the controller with the service.
func NewCameraOverlayInboundController(svc cameraOverlaySvc) *CameraOverlayInboundController {
	if svc == nil {
		panic("CameraOverlayInboundController: svc required")
	}
	return &CameraOverlayInboundController{svc: svc}
}

// Apply godoc
// @Summary      Klynx-initiated camera overlay PATCH
// @Description  Accept a klynx-side operator edit on a gw-managed camera and persist into device_management. Whitelist-restricted: only name/description/lat/lng/site/zone/serialNo accepted. Other fields return 400. If-Match is replay-only in v1 (no 409); X-If-Match-Status response header reports drift. Companion to klynx-api/docs/contracts/camera-gw-managed-overlay.md §8.
// @Tags         Admin.DeviceManagement
// @Accept       json
// @Produce      json
// @Param        gwDeviceMgmtId path string true "device_management.deviceMgmtId UUID"
// @Param        X-Active-Workspace header string true "gw workspace id"
// @Param        If-Match header string false "previous lastOutboundHash; replay-only — never gates the write"
// @Param        request body object true "accepted fields whitelist: name, description, lat, lng, site, zone, serialNo"
// @Success      200 {object} gmod.SuccessDataResponse
// @Failure      400 {object} gmod.ApiErrorResponse
// @Failure      401 {object} gmod.ApiErrorResponse
// @Failure      403 {object} gmod.ApiErrorResponse
// @Failure      404 {object} gmod.ApiErrorResponse
// @Failure      500 {object} gmod.ApiErrorResponse
// @Router       /admin/device-management/cameras/{gwDeviceMgmtId} [patch]
// @Security     BearerAuth
func (ctrl *CameraOverlayInboundController) Apply(c fiber.Ctx) error {
	ctx, end, log := traceutil.StartLite(c, "gateway.adminapi", "CameraOverlayInbound.Apply", "adminapi", "Apply")
	defer end()

	tenantId, _ := c.Locals("tenantId").(string)
	workspaceId, _ := c.Locals("activeWorkspace").(string)
	if workspaceId == "" {
		workspaceId = string(c.Request().Header.Peek("X-Active-Workspace"))
	}
	gwDeviceMgmtId := c.Params("gwDeviceMgmtId")
	ifMatch := string(c.Request().Header.Peek("If-Match"))

	// Body must be a JSON object. Anything else (array, number, "string",
	// null) is a caller bug — reject before the service sees it.
	var body map[string]any
	if len(c.Body()) == 0 {
		return httputil.FailBadRequest(c, "request body is required")
	}
	if err := c.Bind().Body(&body); err != nil {
		return httputil.FailBadRequest(c, "invalid JSON body")
	}
	if body == nil {
		return httputil.FailBadRequest(c, "request body must be a JSON object")
	}

	updated, status, err := ctrl.svc.ApplyKlynxOverlay(ctx, tenantId, workspaceId, gwDeviceMgmtId, body, ifMatch)
	if err != nil {
		// Validation errors carry the offending field list.
		var verr *devicemgmtsvc.OverlayValidationError
		if errors.As(err, &verr) {
			log.Warn().Str("code", verr.Code).Strs("fields", verr.Fields).Msg("overlay validation rejected")
			return httputil.Fail(c, fiber.StatusBadRequest, verr.Code, verr.Code, httputil.ErrorDetails{"fields": verr.Fields})
		}
		if errors.Is(err, devicemgmtsvc.ErrNotFound) {
			return httputil.Fail(c, fiber.StatusNotFound, "DEVICE_NOT_FOUND", "device not found")
		}
		log.Error().Err(err).Msg("ApplyKlynxOverlay failed")
		return httputil.FailInternal(c, "internal error")
	}

	// Per §8.4: response header surfaces drift to klynx without gating the
	// write. Always set, even when If-Match was absent.
	c.Set("X-If-Match-Status", string(status))

	return httputil.Ok(c, updated, "device updated")
}
