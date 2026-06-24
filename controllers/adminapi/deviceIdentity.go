// controllers/adminapi/deviceIdentity.go
package adminapi

import (
	"context"

	"github.com/gofiber/fiber/v3"
	"github.com/hotkhwan/gateway-api/models/ingestmod"
	"github.com/hotkhwan/gateway-api/utils/httputil"
	"github.com/hotkhwan/gateway-api/utils/traceutil"
)

// deviceProvisionSvc is the narrow interface DeviceIdentityController uses so
// handler tests can swap a fake without Mongo / Kafka.
type deviceProvisionSvc interface {
	ProvisionByCamID(
		ctx context.Context,
		tenantId, workspaceId, sourceFamily, camID string,
		hints ingestmod.AutoUpsertHints,
	) (*ingestmod.DeviceManagement, error)
}

// DeviceIdentityController serves POST /admin/device-management/identities — the
// proactive per-camera device-identity provisioning used by klynx-api before a
// camera starts uploading to /events/{org}/{family}/{camID}.
//
// Contract: klynx-api/docs/contracts/dahua-camera-event-ingest.md §5.2.
// Phase 1 transport: HTTP (operator JWT + X-Active-Workspace). The contract's
// primary transport is gRPC + shared secret — a follow-up.
type DeviceIdentityController struct {
	svc deviceProvisionSvc
}

// NewDeviceIdentityController wires the controller with the provisioning service.
func NewDeviceIdentityController(svc deviceProvisionSvc) *DeviceIdentityController {
	if svc == nil {
		panic("DeviceIdentityController: svc required")
	}
	return &DeviceIdentityController{svc: svc}
}

type provisionIdentityRequest struct {
	SourceFamily string  `json:"sourceFamily"`
	CamID        string  `json:"camId"`
	Name         string  `json:"name,omitempty"`
	Description  string  `json:"description,omitempty"`
	Lat          float64 `json:"lat,omitempty"`
	Lng          float64 `json:"lng,omitempty"`
	Site         string  `json:"site,omitempty"`
	Zone         string  `json:"zone,omitempty"`
	SerialNo     string  `json:"serialNo,omitempty"`
}

// Provision godoc
// @Summary      Provision a per-camera device identity (camID)
// @Description  Pre-create/ensure a device_management record keyed by (sourceFamily, entityType="camera", entityId=camId) with deviceId=camId so per-camera ingest attributes deterministically. Idempotent. Emits gw.devices.changed.v1. See klynx-api/docs/contracts/dahua-camera-event-ingest.md §5.2.
// @Tags         Admin.DeviceManagement
// @Accept       json
// @Produce      json
// @Param        X-Active-Workspace header string true "gw workspace id"
// @Param        request body provisionIdentityRequest true "sourceFamily + camId (+ optional name/site/lat/lng)"
// @Success      200 {object} gmod.SuccessDataResponse
// @Failure      400 {object} gmod.ApiErrorResponse
// @Failure      401 {object} gmod.ApiErrorResponse
// @Failure      500 {object} gmod.ApiErrorResponse
// @Router       /admin/device-management/identities [post]
// @Security     BearerAuth
func (ctrl *DeviceIdentityController) Provision(c fiber.Ctx) error {
	ctx, end, log := traceutil.StartLite(c, "gateway.adminapi", "DeviceIdentity.Provision", "adminapi", "Provision")
	defer end()

	tenantId, _ := c.Locals("tenantId").(string)
	workspaceId, _ := c.Locals("activeWorkspace").(string)
	if workspaceId == "" {
		workspaceId = string(c.Request().Header.Peek("X-Active-Workspace"))
	}

	var req provisionIdentityRequest
	if len(c.Body()) == 0 {
		return httputil.FailBadRequest(c, "request body is required")
	}
	if err := c.Bind().Body(&req); err != nil {
		return httputil.FailBadRequest(c, "invalid JSON body")
	}
	if req.SourceFamily == "" || req.CamID == "" {
		return httputil.FailBadRequest(c, "sourceFamily and camId are required")
	}

	rec, err := ctrl.svc.ProvisionByCamID(ctx, tenantId, workspaceId, req.SourceFamily, req.CamID, ingestmod.AutoUpsertHints{
		DeviceId:    req.CamID,
		SerialNo:    req.SerialNo,
		Name:        req.Name,
		Description: req.Description,
		Lat:         req.Lat,
		Lng:         req.Lng,
		Site:        req.Site,
		Zone:        req.Zone,
	})
	if err != nil {
		log.Error().Err(err).Str("camId", req.CamID).Str("sourceFamily", req.SourceFamily).Msg("ProvisionByCamID failed")
		return httputil.FailInternal(c, "failed to provision device identity")
	}

	return httputil.Ok(c, rec, "device identity provisioned")
}
