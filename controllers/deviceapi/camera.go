// controllers/deviceapi/camera.go
package deviceapi

import (
	"strings"

	"github.com/gofiber/fiber/v3"
	"github.com/hotkhwan/gateway-api/internal/services/devicesvc"
	"github.com/hotkhwan/gateway-api/models/devmod"
	"github.com/hotkhwan/gateway-api/models/gmod"
	"github.com/hotkhwan/gateway-api/utils/httputil"
	"github.com/hotkhwan/gateway-api/utils/traceutil"
)

// UpdateGroupRequest — used in deviceGroup.go Update handler
type UpdateGroupRequest struct {
	Name          string `json:"name"`
	Description   string `json:"description"`
	MapVisibility string `json:"mapVisibility"` // public | private
}

// ============================================================
// CameraController
// ============================================================

type CameraController struct {
	service *devicesvc.CameraService
}

func NewCameraController(service *devicesvc.CameraService) *CameraController {
	if service == nil {
		panic("CameraService required")
	}
	return &CameraController{service: service}
}

func (ctrl *CameraController) mustLocals(c fiber.Ctx) (tenantId, orgId, callerUserId string) {
	tenantId, _ = c.Locals("tenantId").(string)
	orgId, _ = c.Locals("activeWorkspace").(string)
	callerUserId, _ = c.Locals("userId").(string)
	return
}

type CreateCameraRequest struct {
	Name          string        `json:"name"`
	User          string        `json:"user"`
	Password      string        `json:"password"`
	URL           string        `json:"url"`
	District      string        `json:"district"`
	Lat           float64       `json:"lat"`
	Lng           float64       `json:"lng"`
	AtaWsFlvUrl   string        `json:"ataWsFlvUrl,omitempty"`
	Brand         string        `json:"brand,omitempty"`
	Roi           []interface{} `json:"roi,omitempty"`
	MapVisibility string        `json:"mapVisibility"`
}

func (ctrl *CameraController) Create(c fiber.Ctx) error {
	_, end, log := traceutil.StartLite(c, "gateway.deviceapi", "CameraController.Create", "deviceapi", "Create")
	defer end()

	tenantId, orgId, callerUserId := ctrl.mustLocals(c)
	var body CreateCameraRequest
	if err := c.Bind().Body(&body); err != nil {
		return httputil.FailBadRequest(c, "invalid body")
	}
	if body.Name == "" || body.URL == "" {
		return httputil.FailBadRequest(c, "name and url are required")
	}
	log.Info().Str("orgId", orgId).Str("name", body.Name).Msg("CreateCamera")
	cam, err := ctrl.service.Create(c, devmod.CreateCameraInput{
		TenantID: tenantId, OrgID: orgId, CallerID: callerUserId,
		Name: body.Name, User: body.User, Password: body.Password,
		URL: body.URL, District: body.District, Lat: body.Lat, Lng: body.Lng,
		AtaWsFlvUrl: body.AtaWsFlvUrl, Brand: body.Brand, Roi: body.Roi,
		MapVisibility: body.MapVisibility,
	})
	if err != nil {
		log.Error().Err(err).Msg("CreateCamera failed")
		return handleErr(c, err)
	}
	log.Info().Str("deviceId", cam.ID).Msg("Camera created")
	return httputil.Created(c, cam, "camera created successfully")
}

func (ctrl *CameraController) List(c fiber.Ctx) error {
	_, end, log := traceutil.StartLite(c, "gateway.deviceapi", "CameraController.List", "deviceapi", "List")
	defer end()

	tenantId, orgId, _ := ctrl.mustLocals(c)
	input := devicesvc.ListCameraInput{
		TenantID: tenantId, OrgID: orgId,
		Search: c.Query("search"), GroupID: c.Query("groupId"),
		Page: fiber.Query[int](c, "page", 1), PerPages: fiber.Query[int](c, "perPages", 10),
		SortField: c.Query("sortField", "dateTimeCreate"), SortOrder: c.Query("sortOrder", "desc"),
	}
	result, err := ctrl.service.List(c, input)
	if err != nil {
		log.Error().Err(err).Msg("List cameras failed")
		return handleErr(c, err)
	}
	totalPages := (int(result.Total) + input.PerPages - 1) / input.PerPages
	return c.JSON(fiber.Map{
		"code": gmod.CodeSuccess, "message": "cameras fetched successfully", "status": true,
		"details": result.Items,
		"summary": fiber.Map{"online": result.Online, "offline": result.Offline},
		"pagination": fiber.Map{
			"page": input.Page, "perPages": input.PerPages,
			"totalRecords": result.Total, "totalPages": totalPages,
			"sortField": input.SortField, "sortOrder": input.SortOrder,
		},
	})
}

func (ctrl *CameraController) GetByID(c fiber.Ctx) error {
	_, end, log := traceutil.StartLite(c, "gateway.deviceapi", "CameraController.GetByID", "deviceapi", "GetByID")
	defer end()

	tenantId, orgId, _ := ctrl.mustLocals(c)
	cam, err := ctrl.service.GetByID(c, tenantId, orgId, c.Params("id"))
	if err != nil {
		log.Error().Err(err).Msg("GetByID failed")
		return handleErr(c, err)
	}
	return httputil.Ok(c, cam, "camera fetched")
}

type UpdateCameraRequest struct {
	Name          string        `json:"name"`
	User          string        `json:"user"`
	Password      string        `json:"password"`
	URL           string        `json:"url"`
	District      string        `json:"district"`
	Lat           float64       `json:"lat"`
	Lng           float64       `json:"lng"`
	Roi           []interface{} `json:"roi,omitempty"`
	MapVisibility string        `json:"mapVisibility"`
}

func (ctrl *CameraController) Update(c fiber.Ctx) error {
	_, end, log := traceutil.StartLite(c, "gateway.deviceapi", "CameraController.Update", "deviceapi", "Update")
	defer end()

	tenantId, orgId, callerUserId := ctrl.mustLocals(c)
	deviceId := c.Params("id")
	var body UpdateCameraRequest
	if err := c.Bind().Body(&body); err != nil {
		return httputil.FailBadRequest(c, "invalid body")
	}
	if err := ctrl.service.Update(c, tenantId, orgId, callerUserId, deviceId, devmod.UpdateCameraInput{
		Name: body.Name, User: body.User, Password: body.Password,
		URL: body.URL, District: body.District, Lat: body.Lat, Lng: body.Lng,
		Roi: body.Roi, MapVisibility: body.MapVisibility,
	}); err != nil {
		log.Error().Err(err).Msg("Update camera failed")
		return handleErr(c, err)
	}
	return httputil.MessageOK(c, "camera updated successfully")
}

func (ctrl *CameraController) Delete(c fiber.Ctx) error {
	_, end, log := traceutil.StartLite(c, "gateway.deviceapi", "CameraController.Delete", "deviceapi", "Delete")
	defer end()

	tenantId, orgId, callerUserId := ctrl.mustLocals(c)
	if err := ctrl.service.Delete(c, tenantId, orgId, callerUserId, c.Params("id")); err != nil {
		log.Error().Err(err).Msg("Delete camera failed")
		return handleErr(c, err)
	}
	return httputil.MessageOK(c, "camera deleted successfully")
}

func (ctrl *CameraController) Import(c fiber.Ctx) error {
	_, end, log := traceutil.StartLite(c, "gateway.deviceapi", "CameraController.Import", "deviceapi", "Import")
	defer end()

	tenantId, orgId, callerUserId := ctrl.mustLocals(c)
	fileHeader, err := c.FormFile("file")
	if err != nil {
		return httputil.FailBadRequest(c, "file is required")
	}
	file, err := fileHeader.Open()
	if err != nil {
		return httputil.FailBadRequest(c, "cannot open file")
	}
	defer file.Close()

	filename := strings.ToLower(fileHeader.Filename)
	var result *devicesvc.ImportResult
	if strings.HasSuffix(filename, ".xlsx") || strings.HasSuffix(filename, ".xls") {
		result, err = ctrl.service.ImportFromXLSX(c, tenantId, orgId, callerUserId, file)
	} else {
		result, err = ctrl.service.ImportFromCSV(c, tenantId, orgId, callerUserId, file)
	}
	if err != nil {
		log.Error().Err(err).Msg("Import failed")
		return handleErr(c, err)
	}
	return httputil.Ok(c, fiber.Map{
		"totalRows": result.TotalRows, "inserted": result.Inserted,
		"invalidRows": result.InvalidRows, "duplicateIPInFile": result.DuplicateIPInFile,
		"duplicateIPInDB": result.DuplicateIPInDB, "permifySyncFailed": result.PermifySyncFailed,
		"results": result.Results,
	}, "import complete")
}
