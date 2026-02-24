// controllers/deviceapi/camera.go
package deviceapi

import (
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/hotkhwan/gateway-api/internal/logger"
	"github.com/hotkhwan/gateway-api/internal/services/devicesvc"
	"github.com/hotkhwan/gateway-api/models/devmod"
	"github.com/hotkhwan/gateway-api/models/gmod"
)

// UpdateGroupRequest — ใช้ใน deviceGroup.go Update handler
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

func (ctrl *CameraController) mustLocals(c *fiber.Ctx) (tenantId, orgId, callerUserId string) {
	tenantId, _ = c.Locals("tenantId").(string)
	orgId, _ = c.Locals("activeOrg").(string)
	callerUserId, _ = c.Locals("userId").(string)
	return
}

type CreateCameraRequest struct {
	Name                  string        `json:"name"`
	User                  string        `json:"user"`
	Password              string        `json:"password"`
	URL                   string        `json:"url"`
	District              string        `json:"district"`
	Lat                   float64       `json:"lat"`
	Lng                   float64       `json:"lng"`
	AtaWsFlvUrl           string        `json:"ataWsFlvUrl,omitempty"`
	Brand                 string        `json:"brand,omitempty"`
	Roi                   []interface{} `json:"roi,omitempty"`
	MapVisibility string        `json:"mapVisibility"`
}

func (ctrl *CameraController) Create(c *fiber.Ctx) error {
	tenantId, orgId, callerUserId := ctrl.mustLocals(c)
	log := logger.FromCtx(c.UserContext(), "deviceapi", "CameraController.Create")
	var body CreateCameraRequest
	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(gmod.ApiErrorResponse{Code: gmod.CodeBadRequest, Message: "invalid body"})
	}
	if body.Name == "" || body.URL == "" {
		return c.Status(400).JSON(gmod.ApiErrorResponse{Code: gmod.CodeBadRequest, Message: "name and url are required"})
	}
	log.Info().Str("orgId", orgId).Str("name", body.Name).Msg("📥 CreateCamera")
	cam, err := ctrl.service.Create(c.UserContext(), devmod.CreateCameraInput{
		TenantID: tenantId, OrgID: orgId, CallerID: callerUserId,
		Name: body.Name, User: body.User, Password: body.Password,
		URL: body.URL, District: body.District, Lat: body.Lat, Lng: body.Lng,
		AtaWsFlvUrl: body.AtaWsFlvUrl, Brand: body.Brand, Roi: body.Roi,
		MapVisibility: body.MapVisibility,
	})
	if err != nil {
		log.Error().Err(err).Msg("❌ CreateCamera failed")
		return handleErr(c, err)
	}
	log.Info().Str("deviceId", cam.ID).Msg("✅ Camera created")
	return c.Status(201).JSON(fiber.Map{"code": gmod.CodeSuccess, "message": "camera created successfully", "status": true, "details": cam})
}

func (ctrl *CameraController) List(c *fiber.Ctx) error {
	tenantId, orgId, _ := ctrl.mustLocals(c)
	input := devicesvc.ListCameraInput{
		TenantID: tenantId, OrgID: orgId,
		Search: c.Query("search"), GroupID: c.Query("groupId"),
		Page: c.QueryInt("page", 1), PerPages: c.QueryInt("perPages", 10),
		SortField: c.Query("sortField", "dateTimeCreate"), SortOrder: c.Query("sortOrder", "desc"),
	}
	result, err := ctrl.service.List(c.UserContext(), input)
	if err != nil {
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

func (ctrl *CameraController) GetByID(c *fiber.Ctx) error {
	tenantId, orgId, _ := ctrl.mustLocals(c)
	cam, err := ctrl.service.GetByID(c.UserContext(), tenantId, orgId, c.Params("id"))
	if err != nil {
		return handleErr(c, err)
	}
	return c.JSON(fiber.Map{"code": gmod.CodeSuccess, "message": "camera fetched", "status": true, "details": cam})
}

type UpdateCameraRequest struct {
	Name                  string        `json:"name"`
	User                  string        `json:"user"`
	Password              string        `json:"password"`
	URL                   string        `json:"url"`
	District              string        `json:"district"`
	Lat                   float64       `json:"lat"`
	Lng                   float64       `json:"lng"`
	Roi                   []interface{} `json:"roi,omitempty"`
	MapVisibility string        `json:"mapVisibility"`
}

func (ctrl *CameraController) Update(c *fiber.Ctx) error {
	tenantId, orgId, callerUserId := ctrl.mustLocals(c)
	deviceId := c.Params("id")
	var body UpdateCameraRequest
	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(gmod.ApiErrorResponse{Code: gmod.CodeBadRequest, Message: "invalid body"})
	}
	if err := ctrl.service.Update(c.UserContext(), tenantId, orgId, callerUserId, deviceId, devmod.UpdateCameraInput{
		Name: body.Name, User: body.User, Password: body.Password,
		URL: body.URL, District: body.District, Lat: body.Lat, Lng: body.Lng,
		Roi: body.Roi, MapVisibility: body.MapVisibility,
	}); err != nil {
		return handleErr(c, err)
	}
	return c.JSON(fiber.Map{"code": gmod.CodeSuccess, "message": "camera updated successfully", "status": true})
}

func (ctrl *CameraController) Delete(c *fiber.Ctx) error {
	tenantId, orgId, callerUserId := ctrl.mustLocals(c)
	if err := ctrl.service.Delete(c.UserContext(), tenantId, orgId, callerUserId, c.Params("id")); err != nil {
		return handleErr(c, err)
	}
	return c.JSON(fiber.Map{"code": gmod.CodeSuccess, "message": "camera deleted successfully", "status": true})
}

func (ctrl *CameraController) Import(c *fiber.Ctx) error {
	tenantId, orgId, callerUserId := ctrl.mustLocals(c)
	log := logger.FromCtx(c.UserContext(), "deviceapi", "CameraController.Import")
	fileHeader, err := c.FormFile("file")
	if err != nil {
		return c.Status(400).JSON(gmod.ApiErrorResponse{Code: gmod.CodeBadRequest, Message: "file is required"})
	}
	file, err := fileHeader.Open()
	if err != nil {
		return c.Status(400).JSON(gmod.ApiErrorResponse{Code: gmod.CodeBadRequest, Message: "cannot open file"})
	}
	defer file.Close()

	filename := strings.ToLower(fileHeader.Filename)
	var result *devicesvc.ImportResult
	if strings.HasSuffix(filename, ".xlsx") || strings.HasSuffix(filename, ".xls") {
		result, err = ctrl.service.ImportFromXLSX(c.UserContext(), tenantId, orgId, callerUserId, file)
	} else {
		result, err = ctrl.service.ImportFromCSV(c.UserContext(), tenantId, orgId, callerUserId, file)
	}
	if err != nil {
		log.Error().Err(err).Msg("❌ Import failed")
		return handleErr(c, err)
	}
	return c.JSON(fiber.Map{
		"code": gmod.CodeSuccess, "message": "import complete", "status": true,
		"details": fiber.Map{
			"totalRows": result.TotalRows, "inserted": result.Inserted,
			"invalidRows": result.InvalidRows, "duplicateIPInFile": result.DuplicateIPInFile,
			"duplicateIPInDB": result.DuplicateIPInDB, "permifySyncFailed": result.PermifySyncFailed,
			"results": result.Results,
		},
	})
}
