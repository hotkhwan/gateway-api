// controllers/ingestapi/deviceManagement.go
package ingestapi

import (
	"errors"

	"github.com/gofiber/fiber/v2"
	"github.com/hotkhwan/gateway-api/internal/services/devicemgmtsvc"
	"github.com/hotkhwan/gateway-api/models/ingestmod"
	"github.com/hotkhwan/gateway-api/utils/httputil"
	"github.com/hotkhwan/gateway-api/utils/traceutil"
	"go.mongodb.org/mongo-driver/bson"
)

type DeviceManagementController struct {
	svc *devicemgmtsvc.DeviceManagementService
}

func NewDeviceManagementController(svc *devicemgmtsvc.DeviceManagementService) *DeviceManagementController {
	if svc == nil {
		panic("DeviceManagementController: svc required")
	}
	return &DeviceManagementController{svc: svc}
}

func (h *DeviceManagementController) tenantId(c *fiber.Ctx) string {
	tid, _ := c.Locals("tenantId").(string)
	return tid
}

func (h *DeviceManagementController) orgId(c *fiber.Ctx) string {
	oid, _ := c.Locals("activeOrg").(string)
	return oid
}

func (h *DeviceManagementController) List(c *fiber.Ctx) error {
	ctx, end, log := traceutil.StartLite(c.UserContext(), "gateway.ingestapi", "DeviceMgmtController.List", "ingestapi", "ListDeviceMgmt")
	defer end()

	tenantId := h.tenantId(c)
	orgId := h.orgId(c)
	page := c.QueryInt("page", 1)
	perPage := c.QueryInt("perPages", 20)

	items, err := h.svc.List(ctx, tenantId, orgId, page, perPage)
	if err != nil {
		log.Error().Err(err).Msg("[ListDeviceMgmt] failed")
		return httputil.FailInternal(c, "failed to list device management records")
	}
	return httputil.Ok(c, items, "device management records fetched")
}

func (h *DeviceManagementController) Get(c *fiber.Ctx) error {
	ctx, end, log := traceutil.StartLite(c.UserContext(), "gateway.ingestapi", "DeviceMgmtController.Get", "ingestapi", "GetDeviceMgmt")
	defer end()

	tenantId := h.tenantId(c)
	orgId := h.orgId(c)
	id := c.Params("id")

	d, err := h.svc.Get(ctx, tenantId, orgId, id)
	if err != nil {
		if errors.Is(err, devicemgmtsvc.ErrNotFound) {
			return httputil.FailNotFound(c, "device management record not found")
		}
		log.Error().Err(err).Msg("[GetDeviceMgmt] failed")
		return httputil.FailInternal(c, "failed to get device management record")
	}
	return httputil.Ok(c, d, "device management record fetched")
}

func (h *DeviceManagementController) Create(c *fiber.Ctx) error {
	ctx, end, log := traceutil.StartLite(c.UserContext(), "gateway.ingestapi", "DeviceMgmtController.Create", "ingestapi", "CreateDeviceMgmt")
	defer end()

	tenantId := h.tenantId(c)
	orgId := h.orgId(c)

	var in ingestmod.DeviceManagement
	if err := c.BodyParser(&in); err != nil {
		return httputil.FailBadRequest(c, "invalid request body")
	}
	if in.SourceFamily == "" || in.EntityType == "" || in.EntityId == "" {
		return httputil.FailBadRequest(c, "sourceFamily, entityType, and entityId are required")
	}

	in.TenantId = tenantId
	in.OrgId = orgId

	if err := h.svc.Create(ctx, &in); err != nil {
		log.Error().Err(err).Msg("[CreateDeviceMgmt] failed")
		return httputil.FailInternal(c, "failed to create device management record")
	}

	log.Info().Str("deviceMgmtId", in.DeviceMgmtId).Msg("[CreateDeviceMgmt] created")
	return httputil.Created(c, in, "device management record created")
}

func (h *DeviceManagementController) Update(c *fiber.Ctx) error {
	ctx, end, log := traceutil.StartLite(c.UserContext(), "gateway.ingestapi", "DeviceMgmtController.Update", "ingestapi", "UpdateDeviceMgmt")
	defer end()

	tenantId := h.tenantId(c)
	orgId := h.orgId(c)
	id := c.Params("id")

	var body map[string]any
	if err := c.BodyParser(&body); err != nil {
		return httputil.FailBadRequest(c, "invalid request body")
	}

	update := bson.M{}
	for _, field := range []string{"deviceId", "lat", "lng", "site", "zone"} {
		if v, ok := body[field]; ok {
			update[field] = v
		}
	}
	if len(update) == 0 {
		return httputil.FailBadRequest(c, "no fields to update")
	}

	if err := h.svc.Update(ctx, tenantId, orgId, id, update); err != nil {
		if errors.Is(err, devicemgmtsvc.ErrNotFound) {
			return httputil.FailNotFound(c, "device management record not found")
		}
		log.Error().Err(err).Msg("[UpdateDeviceMgmt] failed")
		return httputil.FailInternal(c, "failed to update device management record")
	}

	log.Info().Str("deviceMgmtId", id).Msg("[UpdateDeviceMgmt] updated")
	return httputil.MessageOK(c, "device management record updated")
}
