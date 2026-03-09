// controllers/authzapi/authzDebug.go
package authzapi

import (
	"github.com/gofiber/fiber/v2"
	"github.com/hotkhwan/gateway-api/internal/services/authzsvc"
	"github.com/hotkhwan/gateway-api/models/gmod"
	"github.com/hotkhwan/gateway-api/utils/httputil"
	"github.com/hotkhwan/gateway-api/utils/traceutil"
)

// ✅ DI struct
type AuthzDebugController struct {
	debugService *authzsvc.AuthzDebugService
}

func NewAuthzDebugController(svc *authzsvc.AuthzDebugService) *AuthzDebugController {
	return &AuthzDebugController{debugService: svc}
}

func (ctrl *AuthzDebugController) ListPermifyTuples(c *fiber.Ctx) error {
	_, end, _ := traceutil.StartLite(c.UserContext(), "gateway.authzapi", "AuthzDebugController.ListPermifyTuples", "authzapi", "ListPermifyTuples")
	defer end()

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
		return httputil.FailInternal(c, err.Error())
	}

	return c.JSON(fiber.Map{
		"code": gmod.CodeSuccess, "message": "Permify tuples fetched successfully", "status": true,
		"details":    res.Tuples,
		"pagination": fiber.Map{"continuousToken": res.ContinuousToken, "pageSize": res.PageSize},
	})
}

func (ctrl *AuthzDebugController) FactoryResetPermifyTuples(c *fiber.Ctx) error {
	_, end, _ := traceutil.StartLite(c.UserContext(), "gateway.authzapi", "AuthzDebugController.FactoryResetPermifyTuples", "authzapi", "FactoryResetPermifyTuples")
	defer end()

	var body struct {
		TenantId   string `json:"tenantId"`
		EntityType string `json:"entityType"`
	}
	if err := c.BodyParser(&body); err != nil {
		return httputil.FailBadRequest(c, "Invalid request body")
	}
	if body.TenantId == "" {
		return httputil.FailBadRequest(c, "tenantId is required")
	}

	deleted, err := ctrl.debugService.FactoryResetPermifyTuples(c.Context(), body.TenantId, body.EntityType)
	if err != nil {
		return httputil.FailInternal(c, err.Error())
	}

	return httputil.Ok(c, fiber.Map{"deletedCount": deleted}, "Factory reset completed")
}

func (ctrl *AuthzDebugController) ProveUserOrgsAgainstMongo(c *fiber.Ctx) error {
	_, end, _ := traceutil.StartLite(c.UserContext(), "gateway.authzapi", "AuthzDebugController.ProveUserOrgsAgainstMongo", "authzapi", "ProveUserOrgsAgainstMongo")
	defer end()

	tenantId := c.Query("tenantId", "")
	userId, _ := c.Locals("userId").(string)

	res, err := ctrl.debugService.ProveUserOrgsAgainstMongo(c.Context(), tenantId, userId)
	if err != nil {
		return httputil.FailInternal(c, err.Error())
	}

	return httputil.Ok(c, res, "Prove completed")
}

func (ctrl *AuthzDebugController) ResetAllTuples(c *fiber.Ctx) error {
	_, end, _ := traceutil.StartLite(c.UserContext(), "gateway.authzapi", "AuthzDebugController.ResetAllTuples", "authzapi", "ResetAllTuples")
	defer end()

	var body struct {
		TenantId   string `json:"tenantId"`
		EntityType string `json:"entityType"`
	}
	if err := c.BodyParser(&body); err != nil {
		return httputil.FailBadRequest(c, "Invalid request body")
	}

	res, err := ctrl.debugService.ResetPermifyTuplesAll(c.Context(), body.TenantId, body.EntityType)
	if err != nil {
		return httputil.FailInternal(c, err.Error())
	}

	return httputil.Ok(c, res, "Reset all tuples completed")
}

func (ctrl *AuthzDebugController) ResetTuplesByUser(c *fiber.Ctx) error {
	_, end, _ := traceutil.StartLite(c.UserContext(), "gateway.authzapi", "AuthzDebugController.ResetTuplesByUser", "authzapi", "ResetTuplesByUser")
	defer end()

	var body struct {
		TenantId string `json:"tenantId"`
		UserId   string `json:"userId"`
	}
	if err := c.BodyParser(&body); err != nil {
		return httputil.FailBadRequest(c, "Invalid request body")
	}

	res, err := ctrl.debugService.ResetPermifyTuplesByUser(c.Context(), body.TenantId, body.UserId)
	if err != nil {
		return httputil.FailInternal(c, err.Error())
	}

	return httputil.Ok(c, res, "Reset tuples by user completed")
}

func (ctrl *AuthzDebugController) ListTuplesByUser(c *fiber.Ctx) error {
	_, end, _ := traceutil.StartLite(c.UserContext(), "gateway.authzapi", "AuthzDebugController.ListTuplesByUser", "authzapi", "ListTuplesByUser")
	defer end()

	tenantId := c.Query("tenantId", "")
	userId := c.Query("userId", "")
	if userId == "" {
		return httputil.FailBadRequest(c, "userId is required")
	}

	res, err := ctrl.debugService.ListTuplesByUser(c.Context(), tenantId, userId)
	if err != nil {
		return httputil.FailInternal(c, err.Error())
	}

	return httputil.Ok(c, res, "User tuples fetched")
}

func (ctrl *AuthzDebugController) ListOrgUnitTuples(c *fiber.Ctx) error {
	_, end, _ := traceutil.StartLite(c.UserContext(), "gateway.authzapi", "AuthzDebugController.ListOrgUnitTuples", "authzapi", "ListOrgUnitTuples")
	defer end()

	tenantId := c.Query("tenantId", "")

	res, err := ctrl.debugService.ListTuplesByEntityType(c.Context(), tenantId, "orgUnit")
	if err != nil {
		return httputil.FailInternal(c, err.Error())
	}

	return httputil.Ok(c, res, "OU tuples fetched")
}

func (ctrl *AuthzDebugController) ResetTenant(c *fiber.Ctx) error {
	_, end, _ := traceutil.StartLite(c.UserContext(), "gateway.authzapi", "AuthzDebugController.ResetTenant", "authzapi", "ResetTenant")
	defer end()

	var body struct {
		TenantId string `json:"tenantId"`
		Confirm  string `json:"confirm"`
	}
	if err := c.BodyParser(&body); err != nil {
		return httputil.FailBadRequest(c, "Invalid request body")
	}
	if body.TenantId == "" {
		return httputil.FailBadRequest(c, "tenantId is required")
	}
	if body.Confirm != "RESET_TENANT" {
		return httputil.FailBadRequest(c, "confirmation required: send confirm=RESET_TENANT")
	}

	res, err := ctrl.debugService.ResetTenant(c.Context(), body.TenantId)
	if err != nil {
		return httputil.FailInternal(c, err.Error())
	}

	return httputil.Ok(c, res, "Tenant hard reset completed")
}

func (ctrl *AuthzDebugController) FactoryOrgBootstrap(c *fiber.Ctx) error {
	_, end, _ := traceutil.StartLite(c.UserContext(), "gateway.authzapi", "AuthzDebugController.FactoryOrgBootstrap", "authzapi", "FactoryOrgBootstrap")
	defer end()

	var body struct {
		TenantId string `json:"tenantId"`
		OrgId    string `json:"orgId"`
		UserId   string `json:"userId"`
	}
	if err := c.BodyParser(&body); err != nil {
		return httputil.FailBadRequest(c, "invalid body")
	}

	res, err := ctrl.debugService.FactoryOrgBootstrap(c.Context(), body.TenantId, body.OrgId, body.UserId)
	if err != nil {
		return httputil.FailInternal(c, err.Error())
	}

	return httputil.Ok(c, res, "factory bootstrap done")
}

// controllers/authzapi/authzDebug.go

func (ctrl *AuthzDebugController) ListAllTuples(c *fiber.Ctx) error {
	_, end, _ := traceutil.StartLite(c.UserContext(), "gateway.authzapi", "AuthzDebugController.ListAllTuples", "authzapi", "ListAllTuples")
	defer end()

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
		return httputil.FailInternal(c, err.Error())
	}

	return c.JSON(fiber.Map{
		"code": gmod.CodeSuccess, "status": true,
		"total":   len(res),
		"details": res,
	})
}

func (ctrl *AuthzDebugController) ClearAllTuples(c *fiber.Ctx) error {
	_, end, _ := traceutil.StartLite(c.UserContext(), "gateway.authzapi", "AuthzDebugController.ClearAllTuples", "authzapi", "ClearAllTuples")
	defer end()

	var body struct {
		TenantId string `json:"tenantId"`
		Confirm  string `json:"confirm"`
	}
	if err := c.BodyParser(&body); err != nil {
		return httputil.FailBadRequest(c, "invalid body")
	}
	if body.Confirm != "CLEAR_ALL" {
		return httputil.FailBadRequest(c, "send confirm=CLEAR_ALL")
	}

	res, err := ctrl.debugService.ResetTenant(c.Context(), body.TenantId)
	if err != nil {
		return httputil.FailInternal(c, err.Error())
	}

	return httputil.Ok(c, res)
}

func (ctrl *AuthzDebugController) ListOrgTuples(c *fiber.Ctx) error {
	_, end, _ := traceutil.StartLite(c.UserContext(), "gateway.authzapi", "AuthzDebugController.ListOrgTuples", "authzapi", "ListOrgTuples")
	defer end()

	tenantId := c.Query("tenantId", "")
	orgId := c.Params("orgId")

	res, err := ctrl.debugService.ListTuplesByOrgId(c.Context(), tenantId, orgId)
	if err != nil {
		return httputil.FailInternal(c, err.Error())
	}

	return c.JSON(fiber.Map{"code": gmod.CodeSuccess, "status": true, "total": len(res), "details": res})
}

func (ctrl *AuthzDebugController) ClearOrgTuples(c *fiber.Ctx) error {
	_, end, _ := traceutil.StartLite(c.UserContext(), "gateway.authzapi", "AuthzDebugController.ClearOrgTuples", "authzapi", "ClearOrgTuples")
	defer end()

	var body struct {
		TenantId string `json:"tenantId"`
		OrgId    string `json:"orgId"`
	}
	if err := c.BodyParser(&body); err != nil {
		return httputil.FailBadRequest(c, "invalid body")
	}
	if body.TenantId == "" || body.OrgId == "" {
		return httputil.FailBadRequest(c, "tenantId and orgId required")
	}

	err := ctrl.debugService.ClearOrgTuples(c.Context(), body.TenantId, body.OrgId)
	if err != nil {
		return httputil.FailInternal(c, err.Error())
	}

	return httputil.MessageOK(c, "org tuples cleared")
}

// ========== Debug/Tuples methods (migrated from DebugController) ==========

func (ctrl *AuthzDebugController) DebugReadTuples(c *fiber.Ctx) error {
	_, end, _ := traceutil.StartLite(c.UserContext(), "gateway.authzapi", "AuthzDebugController.DebugReadTuples", "authzapi", "DebugReadTuples")
	defer end()

	tenantId, _ := c.Locals("tenantId").(string)

	if tenantId == "" {
		return httputil.FailUnauthorized(c, "tenantId required")
	}

	entityType := c.Query("entityType")
	entityId := c.Query("entityId")
	relation := c.Query("relation")
	subjectType := c.Query("subjectType")
	subjectId := c.Query("subjectId")

	// Require at least one filter - Permify requires filter in request
	if entityType == "" && entityId == "" && relation == "" && subjectType == "" && subjectId == "" {
		return httputil.FailBadRequest(c, "at least one filter required (entityType, entityId, relation, subjectType, or subjectId)")
	}

	res, err := ctrl.debugService.DebugReadTuples(c.UserContext(), tenantId, authzsvc.ReadTuplesRequest{
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
		return httputil.FailInternal(c, err.Error())
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
	_, end, _ := traceutil.StartLite(c.UserContext(), "gateway.authzapi", "AuthzDebugController.DebugDeleteTuples", "authzapi", "DebugDeleteTuples")
	defer end()

	tenantId, _ := c.Locals("tenantId").(string)

	if tenantId == "" {
		return httputil.FailUnauthorized(c, "tenantId required")
	}

	var body map[string]any
	if err := c.BodyParser(&body); err != nil {
		return httputil.FailBadRequest(c, "invalid body")
	}

	entityType, _ := body["entityType"].(string)
	entityId, _ := body["entityId"].(string)
	relation, _ := body["relation"].(string)
	subjectType, _ := body["subjectType"].(string)
	subjectId, _ := body["subjectId"].(string)

	if entityType == "" {
		return httputil.FailBadRequest(c, "entityType required")
	}

	err := ctrl.debugService.DebugDeleteTuples(c.UserContext(), tenantId, authzsvc.DeleteTuplesRequest{
		EntityType:    entityType,
		EntityId:      entityId,
		Relation:      relation,
		SubjectType:   subjectType,
		SubjectId:     subjectId,
		SchemaVersion: "latest",
		Depth:         50,
	})
	if err != nil {
		return httputil.FailInternal(c, err.Error())
	}

	return httputil.MessageOK(c, "tuples deleted")
}

func (ctrl *AuthzDebugController) DebugDeleteOrgTuples(c *fiber.Ctx) error {
	_, end, _ := traceutil.StartLite(c.UserContext(), "gateway.authzapi", "AuthzDebugController.DebugDeleteOrgTuples", "authzapi", "DebugDeleteOrgTuples")
	defer end()

	tenantId, _ := c.Locals("tenantId").(string)
	orgId := c.Params("orgId")

	if tenantId == "" {
		return httputil.FailUnauthorized(c, "tenantId required")
	}

	if orgId == "" {
		return httputil.FailBadRequest(c, "orgId required")
	}

	err := ctrl.debugService.DebugDeleteOrgTuples(c.UserContext(), tenantId, orgId)
	if err != nil {
		return httputil.FailInternal(c, err.Error())
	}

	return httputil.MessageOK(c, "org tuples deleted")
}
