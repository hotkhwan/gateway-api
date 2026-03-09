// controllers/ingestapi/deviceManagement.go
package ingestapi

import (
	"errors"
	"fmt"

	"github.com/gofiber/fiber/v2"
	"github.com/hotkhwan/gateway-api/internal/services/devicemgmtsvc"
	"github.com/hotkhwan/gateway-api/models/gmod"
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
	for _, field := range []string{"deviceId", "name", "description", "lat", "lng", "site", "zone"} {
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

// DownloadTemplate godoc
// @Summary      Download XLSX template
// @Description  Returns an empty XLSX template prefilled with auth context for device management bulk import.
// @Tags         ingest-deviceManagement
// @Security     BearerAuth
// @Produce      application/vnd.openxmlformats-officedocument.spreadsheetml.sheet
// @Param        X-Active-Org  header  string  true  "Active Org ID"
// @Success      200
// @Failure      500  {object}  gmod.ApiErrorResponse
// @Router       /api/v1/ingest/deviceManagement/template [get]
func (h *DeviceManagementController) DownloadTemplate(c *fiber.Ctx) error {
	ctx, end, log := traceutil.StartLite(c.UserContext(), "gateway.ingestapi", "DeviceMgmtController.DownloadTemplate", "ingestapi", "DownloadTemplate")
	defer end()

	tenantId := h.tenantId(c)
	orgId := h.orgId(c)

	buf, err := h.svc.DownloadTemplate(ctx, tenantId, orgId)
	if err != nil {
		log.Error().Err(err).Msg("[DownloadTemplate] failed")
		return httputil.FailInternal(c, "failed to generate template")
	}

	c.Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	c.Set("Content-Disposition", fmt.Sprintf("attachment; filename=device_management_template_%s.xlsx", orgId))
	return c.Send(buf.Bytes())
}

// ExportXlsx godoc
// @Summary      Export device management data
// @Description  Exports current device management records as XLSX file, scoped to active org.
// @Tags         ingest-deviceManagement
// @Security     BearerAuth
// @Produce      application/vnd.openxmlformats-officedocument.spreadsheetml.sheet
// @Param        X-Active-Org   header  string  true   "Active Org ID"
// @Param        sourceFamily   query   string  false  "Filter by source family"
// @Param        entityType     query   string  false  "Filter by entity type"
// @Success      200
// @Failure      500  {object}  gmod.ApiErrorResponse
// @Router       /api/v1/ingest/deviceManagement/export [get]
func (h *DeviceManagementController) ExportXlsx(c *fiber.Ctx) error {
	ctx, end, log := traceutil.StartLite(c.UserContext(), "gateway.ingestapi", "DeviceMgmtController.ExportXlsx", "ingestapi", "ExportXlsx")
	defer end()

	tenantId := h.tenantId(c)
	orgId := h.orgId(c)
	sourceFamily := c.Query("sourceFamily", "")
	entityType := c.Query("entityType", "")

	buf, err := h.svc.ExportXlsx(ctx, tenantId, orgId, sourceFamily, entityType)
	if err != nil {
		log.Error().Err(err).Msg("[ExportXlsx] failed")
		return httputil.FailInternal(c, "failed to export data")
	}

	c.Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	c.Set("Content-Disposition", fmt.Sprintf("attachment; filename=device_management_export_%s.xlsx", orgId))
	return c.Send(buf.Bytes())
}

// ImportXlsx godoc
// @Summary      Bulk import device management from XLSX
// @Description  Parses an XLSX file and upserts device management records by business key.
// @Tags         ingest-deviceManagement
// @Security     BearerAuth
// @Accept       multipart/form-data
// @Produce      json
// @Param        X-Active-Org  header    string  true   "Active Org ID"
// @Param        file          formData  file    true   "XLSX file"
// @Param        dryRun        formData  string  false  "Preview mode"  default(false)
// @Success      200  {object}  gmod.BulkImportResponse
// @Failure      400  {object}  gmod.ApiErrorResponse
// @Failure      500  {object}  gmod.ApiErrorResponse
// @Router       /api/v1/ingest/deviceManagement/import [post]
func (h *DeviceManagementController) ImportXlsx(c *fiber.Ctx) error {
	ctx, end, log := traceutil.StartLite(c.UserContext(), "gateway.ingestapi", "DeviceMgmtController.ImportXlsx", "ingestapi", "ImportXlsx")
	defer end()

	tenantId := h.tenantId(c)
	orgId := h.orgId(c)
	dryRun := c.FormValue("dryRun") == "true"

	fileHeader, err := c.FormFile("file")
	if err != nil {
		return httputil.FailBadRequest(c, "file is required")
	}

	// Validate file extension
	if len(fileHeader.Filename) < 5 || fileHeader.Filename[len(fileHeader.Filename)-5:] != ".xlsx" {
		return httputil.FailBadRequest(c, "only .xlsx files are supported")
	}

	// Validate file size (10MB max)
	if fileHeader.Size > 10*1024*1024 {
		return httputil.FailBadRequest(c, "file size exceeds 10MB limit")
	}

	file, err := fileHeader.Open()
	if err != nil {
		return httputil.FailBadRequest(c, "failed to open uploaded file")
	}
	defer file.Close()

	result, err := h.svc.ImportFromXlsx(ctx, tenantId, orgId, file, dryRun)
	if err != nil {
		log.Error().Err(err).Str("filename", fileHeader.Filename).Msg("[ImportXlsx] failed")
		return httputil.FailBadRequest(c, err.Error())
	}

	log.Info().
		Str("orgId", orgId).
		Str("filename", fileHeader.Filename).
		Bool("dryRun", dryRun).
		Int("inserted", result.Inserted).
		Int("updated", result.Updated).
		Int("skipped", result.Skipped).
		Int("errors", len(result.Errors)).
		Msg("[ImportXlsx] completed")

	msg := "bulk import completed"
	if dryRun {
		msg = "dry run completed (no changes applied)"
	}

	return c.JSON(gmod.BulkImportResponse{
		Code:     gmod.CodeSuccess,
		Message:  msg,
		Status:   true,
		Inserted: result.Inserted,
		Updated:  result.Updated,
		Skipped:  result.Skipped,
		IDs:      result.IDs,
		Errors:   result.Errors,
	})
}
