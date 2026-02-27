// controllers/authzapi/authzDebug.go
package authzapi

import (
	"github.com/gofiber/fiber/v2"
	"github.com/hotkhwan/gateway-api/internal/services/authzsvc"
	"github.com/hotkhwan/gateway-api/models/gmod"
)

// ✅ DI struct
type AuthzDebugController struct {
	debugService *authzsvc.AuthzDebugService
}

func NewAuthzDebugController(svc *authzsvc.AuthzDebugService) *AuthzDebugController {
	return &AuthzDebugController{debugService: svc}
}

func (ctrl *AuthzDebugController) ListPermifyTuples(c *fiber.Ctx) error {
	tenantId := c.Query("tenantId", "")
	filter := authzsvc.PermifyTupleFilter{
		EntityType:      c.Query("entityType", ""),
		EntityId:        c.Query("entityId", ""),
		Relation:        c.Query("relation", ""),
		SubjectType:     c.Query("subjectType", ""),
		SubjectId:       c.Query("subjectId", ""),
		ContinuousToken: c.Query("continuousToken", ""),
		PageSize:        c.QueryInt("pageSize", 50),
	}

	res, err := ctrl.debugService.ListPermifyTuples(c.Context(), tenantId, filter)
	if err != nil {
		return c.Status(500).JSON(gmod.ApiErrorResponse{Code: gmod.CodeInternalError, Message: err.Error(), Status: false})
	}

	return c.JSON(fiber.Map{
		"code": gmod.CodeSuccess, "message": "Permify tuples fetched successfully", "status": true,
		"details":    res.Tuples,
		"pagination": fiber.Map{"continuousToken": res.ContinuousToken, "pageSize": res.PageSize},
	})
}

func (ctrl *AuthzDebugController) FactoryResetPermifyTuples(c *fiber.Ctx) error {
	var body struct {
		TenantId   string `json:"tenantId"`
		EntityType string `json:"entityType"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(gmod.ApiErrorResponse{Code: gmod.CodeBadRequest, Message: "Invalid request body", Status: false})
	}
	if body.TenantId == "" {
		return c.Status(400).JSON(gmod.ApiErrorResponse{Code: gmod.CodeBadRequest, Message: "tenantId is required", Status: false})
	}

	deleted, err := ctrl.debugService.FactoryResetPermifyTuples(c.Context(), body.TenantId, body.EntityType)
	if err != nil {
		return c.Status(500).JSON(gmod.ApiErrorResponse{Code: gmod.CodeInternalError, Message: err.Error(), Status: false})
	}

	return c.JSON(fiber.Map{"code": gmod.CodeSuccess, "message": "Factory reset completed", "status": true,
		"details": fiber.Map{"deletedCount": deleted}})
}

func (ctrl *AuthzDebugController) ProveUserOrgsAgainstMongo(c *fiber.Ctx) error {
	tenantId := c.Query("tenantId", "")
	userId, _ := c.Locals("userId").(string)

	res, err := ctrl.debugService.ProveUserOrgsAgainstMongo(c.Context(), tenantId, userId)
	if err != nil {
		return c.Status(500).JSON(gmod.ApiErrorResponse{Code: gmod.CodeInternalError, Message: err.Error(), Status: false})
	}

	return c.JSON(fiber.Map{"code": gmod.CodeSuccess, "message": "Prove completed", "status": true, "details": res})
}

func (ctrl *AuthzDebugController) ResetAllTuples(c *fiber.Ctx) error {
	var body struct {
		TenantId   string `json:"tenantId"`
		EntityType string `json:"entityType"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(gmod.ApiErrorResponse{Code: gmod.CodeBadRequest, Message: "Invalid request body", Status: false})
	}

	res, err := ctrl.debugService.ResetPermifyTuplesAll(c.Context(), body.TenantId, body.EntityType)
	if err != nil {
		return c.Status(500).JSON(gmod.ApiErrorResponse{Code: gmod.CodeInternalError, Message: err.Error(), Status: false})
	}

	return c.JSON(fiber.Map{"code": gmod.CodeSuccess, "message": "Reset all tuples completed", "status": true, "details": res})
}

func (ctrl *AuthzDebugController) ResetTuplesByUser(c *fiber.Ctx) error {
	var body struct {
		TenantId string `json:"tenantId"`
		UserId   string `json:"userId"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(gmod.ApiErrorResponse{Code: gmod.CodeBadRequest, Message: "Invalid request body", Status: false})
	}

	res, err := ctrl.debugService.ResetPermifyTuplesByUser(c.Context(), body.TenantId, body.UserId)
	if err != nil {
		return c.Status(500).JSON(gmod.ApiErrorResponse{Code: gmod.CodeInternalError, Message: err.Error(), Status: false})
	}

	return c.JSON(fiber.Map{"code": gmod.CodeSuccess, "message": "Reset tuples by user completed", "status": true, "details": res})
}

func (ctrl *AuthzDebugController) ListTuplesByUser(c *fiber.Ctx) error {
	tenantId := c.Query("tenantId", "")
	userId := c.Query("userId", "")
	if userId == "" {
		return c.Status(400).JSON(gmod.ApiErrorResponse{Code: gmod.CodeBadRequest, Message: "userId is required", Status: false})
	}

	res, err := ctrl.debugService.ListTuplesByUser(c.Context(), tenantId, userId)
	if err != nil {
		return c.Status(500).JSON(gmod.ApiErrorResponse{Code: gmod.CodeInternalError, Message: err.Error(), Status: false})
	}

	return c.JSON(fiber.Map{"code": gmod.CodeSuccess, "message": "User tuples fetched", "status": true, "details": res})
}

func (ctrl *AuthzDebugController) ListOrgUnitTuples(c *fiber.Ctx) error {
	tenantId := c.Query("tenantId", "")

	res, err := ctrl.debugService.ListTuplesByEntityType(c.Context(), tenantId, "orgUnit")
	if err != nil {
		return c.Status(500).JSON(gmod.ApiErrorResponse{Code: gmod.CodeInternalError, Message: err.Error(), Status: false})
	}

	return c.JSON(fiber.Map{"code": gmod.CodeSuccess, "message": "OU tuples fetched", "status": true, "details": res})
}

func (ctrl *AuthzDebugController) ResetTenant(c *fiber.Ctx) error {
	var body struct {
		TenantId string `json:"tenantId"`
		Confirm  string `json:"confirm"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(gmod.ApiErrorResponse{Code: gmod.CodeBadRequest, Message: "Invalid request body", Status: false})
	}
	if body.TenantId == "" {
		return c.Status(400).JSON(gmod.ApiErrorResponse{Code: gmod.CodeBadRequest, Message: "tenantId is required", Status: false})
	}
	if body.Confirm != "RESET_TENANT" {
		return c.Status(400).JSON(gmod.ApiErrorResponse{Code: gmod.CodeBadRequest, Message: "confirmation required: send confirm=RESET_TENANT", Status: false})
	}

	res, err := ctrl.debugService.ResetTenant(c.Context(), body.TenantId)
	if err != nil {
		return c.Status(500).JSON(gmod.ApiErrorResponse{Code: gmod.CodeInternalError, Message: err.Error(), Status: false})
	}

	return c.JSON(fiber.Map{"code": gmod.CodeSuccess, "message": "Tenant hard reset completed", "status": true, "details": res})
}

func (ctrl *AuthzDebugController) FactoryOrgBootstrap(c *fiber.Ctx) error {
	var body struct {
		TenantId string `json:"tenantId"`
		OrgId    string `json:"orgId"`
		UserId   string `json:"userId"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(fiber.Map{"code": "BAD_REQUEST", "message": "invalid body", "status": false})
	}

	res, err := ctrl.debugService.FactoryOrgBootstrap(c.Context(), body.TenantId, body.OrgId, body.UserId)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"code": "INTERNAL_ERROR", "message": err.Error(), "status": false})
	}

	return c.JSON(fiber.Map{"code": "SUCCESS", "message": "factory bootstrap done", "status": true, "details": res})
}

// controllers/authzapi/authzDebug.go

func (ctrl *AuthzDebugController) ListAllTuples(c *fiber.Ctx) error {
	tenantId := c.Query("tenantId", "")
	entityType := c.Query("entityType", "") // optional filter

	var res []authzsvc.PermifyTupleRow
	var err error

	if entityType != "" {
		res, err = ctrl.debugService.ListTuplesByEntityType(c.Context(), tenantId, entityType)
	} else {
		res, err = ctrl.debugService.ListAllTuples(c.Context(), tenantId)
	}

	if err != nil {
		return c.Status(500).JSON(gmod.ApiErrorResponse{Code: gmod.CodeInternalError, Message: err.Error(), Status: false})
	}

	return c.JSON(fiber.Map{
		"code": gmod.CodeSuccess, "status": true,
		"total":   len(res),
		"details": res,
	})
}

func (ctrl *AuthzDebugController) ClearAllTuples(c *fiber.Ctx) error {
	var body struct {
		TenantId string `json:"tenantId"`
		Confirm  string `json:"confirm"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(gmod.ApiErrorResponse{Code: gmod.CodeBadRequest, Message: "invalid body", Status: false})
	}
	if body.Confirm != "CLEAR_ALL" {
		return c.Status(400).JSON(gmod.ApiErrorResponse{Code: gmod.CodeBadRequest, Message: "send confirm=CLEAR_ALL", Status: false})
	}

	res, err := ctrl.debugService.ResetTenant(c.Context(), body.TenantId)
	if err != nil {
		return c.Status(500).JSON(gmod.ApiErrorResponse{Code: gmod.CodeInternalError, Message: err.Error(), Status: false})
	}

	return c.JSON(fiber.Map{"code": gmod.CodeSuccess, "status": true, "details": res})
}

func (ctrl *AuthzDebugController) ListOrgTuples(c *fiber.Ctx) error {
	tenantId := c.Query("tenantId", "")
	orgId := c.Params("orgId")

	res, err := ctrl.debugService.ListTuplesByOrgId(c.Context(), tenantId, orgId)
	if err != nil {
		return c.Status(500).JSON(gmod.ApiErrorResponse{Code: gmod.CodeInternalError, Message: err.Error(), Status: false})
	}

	return c.JSON(fiber.Map{"code": gmod.CodeSuccess, "status": true, "total": len(res), "details": res})
}

func (ctrl *AuthzDebugController) ClearOrgTuples(c *fiber.Ctx) error {
	var body struct {
		TenantId string `json:"tenantId"`
		OrgId    string `json:"orgId"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(gmod.ApiErrorResponse{Code: gmod.CodeBadRequest, Message: "invalid body", Status: false})
	}
	if body.TenantId == "" || body.OrgId == "" {
		return c.Status(400).JSON(gmod.ApiErrorResponse{Code: gmod.CodeBadRequest, Message: "tenantId and orgId required", Status: false})
	}

	err := ctrl.debugService.ClearOrgTuples(c.Context(), body.TenantId, body.OrgId)
	if err != nil {
		return c.Status(500).JSON(gmod.ApiErrorResponse{Code: gmod.CodeInternalError, Message: err.Error(), Status: false})
	}

	return c.JSON(fiber.Map{"code": gmod.CodeSuccess, "status": true, "message": "org tuples cleared"})
}

// ========== Debug/Tuples methods (migrated from DebugController) ==========

func (ctrl *AuthzDebugController) DebugReadTuples(c *fiber.Ctx) error {
	ctx := c.UserContext()
	tenantId, _ := c.Locals("tenantId").(string)

	if tenantId == "" {
		return c.Status(401).JSON(gmod.ApiErrorResponse{
			Code:    gmod.CodeUnauthorized,
			Message: "tenantId required",
			Status:  false,
		})
	}

	entityType := c.Query("entityType")
	entityId := c.Query("entityId")
	relation := c.Query("relation")
	subjectType := c.Query("subjectType")
	subjectId := c.Query("subjectId")

	// Require at least one filter - Permify requires filter in request
	if entityType == "" && entityId == "" && relation == "" && subjectType == "" && subjectId == "" {
		return c.Status(400).JSON(gmod.ApiErrorResponse{
			Code:    gmod.CodeBadRequest,
			Message: "at least one filter required (entityType, entityId, relation, subjectType, or subjectId)",
			Status:  false,
		})
	}

	res, err := ctrl.debugService.DebugReadTuples(ctx, tenantId, authzsvc.ReadTuplesRequest{
		EntityType:    entityType,
		EntityId:      entityId,
		Relation:      relation,
		SubjectType:   subjectType,
		SubjectId:     subjectId,
		SchemaVersion: "latest",
		Depth:         50,
		PageSize:      c.QueryInt("pageSize", 200),
	})
	if err != nil {
		return c.Status(500).JSON(gmod.ApiErrorResponse{
			Code:    gmod.CodeInternalError,
			Message: err.Error(),
			Status:  false,
		})
	}

	details := make([]map[string]any, 0, len(res.Tuples))
	for _, t := range res.Tuples {
		details = append(details, map[string]any{
			"entityType":  t.EntityType,
			"entityId":    t.EntityId,
			"relation":    t.Relation,
			"subjectType": t.SubjectType,
			"subjectId":   t.SubjectId,
		})
	}

	return c.JSON(fiber.Map{
		"code":       gmod.CodeSuccess,
		"status":     true,
		"message":    "tuples fetched",
		"details":    details,
		"pagination": fiber.Map{"continuousToken": res.ContinuousToken, "pageSize": res.PageSize},
	})
}

func (ctrl *AuthzDebugController) DebugDeleteTuples(c *fiber.Ctx) error {
	ctx := c.UserContext()
	tenantId, _ := c.Locals("tenantId").(string)

	if tenantId == "" {
		return c.Status(401).JSON(gmod.ApiErrorResponse{
			Code:    gmod.CodeUnauthorized,
			Message: "tenantId required",
			Status:  false,
		})
	}

	var body map[string]any
	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(gmod.ApiErrorResponse{
			Code:    gmod.CodeBadRequest,
			Message: "invalid body",
			Status:  false,
		})
	}

	entityType, _ := body["entityType"].(string)
	entityId, _ := body["entityId"].(string)
	relation, _ := body["relation"].(string)
	subjectType, _ := body["subjectType"].(string)
	subjectId, _ := body["subjectId"].(string)

	if entityType == "" {
		return c.Status(400).JSON(gmod.ApiErrorResponse{
			Code:    gmod.CodeBadRequest,
			Message: "entityType required",
			Status:  false,
		})
	}

	err := ctrl.debugService.DebugDeleteTuples(ctx, tenantId, authzsvc.DeleteTuplesRequest{
		EntityType:    entityType,
		EntityId:      entityId,
		Relation:      relation,
		SubjectType:   subjectType,
		SubjectId:     subjectId,
		SchemaVersion: "latest",
		Depth:         50,
	})
	if err != nil {
		return c.Status(500).JSON(gmod.ApiErrorResponse{
			Code:    gmod.CodeInternalError,
			Message: err.Error(),
			Status:  false,
		})
	}

	return c.JSON(fiber.Map{"code": gmod.CodeSuccess, "status": true, "message": "tuples deleted"})
}

func (ctrl *AuthzDebugController) DebugDeleteOrgTuples(c *fiber.Ctx) error {
	ctx := c.UserContext()
	tenantId, _ := c.Locals("tenantId").(string)
	orgId := c.Params("orgId")

	if tenantId == "" {
		return c.Status(401).JSON(gmod.ApiErrorResponse{
			Code:    gmod.CodeUnauthorized,
			Message: "tenantId required",
			Status:  false,
		})
	}

	if orgId == "" {
		return c.Status(400).JSON(gmod.ApiErrorResponse{
			Code:    gmod.CodeBadRequest,
			Message: "orgId required",
			Status:  false,
		})
	}

	err := ctrl.debugService.DebugDeleteOrgTuples(ctx, tenantId, orgId)
	if err != nil {
		return c.Status(500).JSON(gmod.ApiErrorResponse{
			Code:    gmod.CodeInternalError,
			Message: err.Error(),
			Status:  false,
		})
	}

	return c.JSON(fiber.Map{"code": gmod.CodeSuccess, "status": true, "message": "org tuples deleted"})
}
