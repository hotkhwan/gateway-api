// controllers/authznewapi/authzDebug.go
package authznewapi

import (
	"github.com/gofiber/fiber/v2"
	"github.com/hotkhwan/gateway-api/internal/services/authzsvc"
	"github.com/hotkhwan/gateway-api/models/gmod"
)

// ListPermifyTuples godoc
// @Summary List permify tuples (debug)
// @Tags 2.authorization
// @Security BearerAuth
// @Produce json
// @Param tenantId query string false "tenant id (default from config)"
// @Param entityType query string false "entity type"
// @Param entityId query string false "entity id"
// @Param relation query string false "relation"
// @Param subjectType query string false "subject type"
// @Param subjectId query string false "subject id"
// @Param pageSize query int false "page size (default 50)"
// @Param continuousToken query string false "continuous token"
// @Router /authz/tuples [get]
func ListPermifyTuples(c *fiber.Ctx) error {
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

	res, err := authzsvc.ListPermifyTuples(c.Context(), tenantId, filter)
	if err != nil {
		return c.Status(500).JSON(gmod.ApiErrorResponse{
			Code:    gmod.CodeInternalError,
			Message: err.Error(),
			Status:  false,
		})
	}

	return c.JSON(fiber.Map{
		"code":    gmod.CodeSuccess,
		"message": "Permify tuples fetched successfully",
		"status":  true,
		"details": res.Tuples,
		"pagination": fiber.Map{
			"continuousToken": res.ContinuousToken,
			"pageSize":        res.PageSize,
		},
	})
}

// FactoryResetPermifyTuples godoc
// @Summary Factory reset permify tuples for tenant (debug)
// @Tags 2.authorization
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param body body object true "request body"
// @Router /authz/tuples/factoryReset [post]
func FactoryResetPermifyTuples(c *fiber.Ctx) error {
	var body struct {
		TenantId string `json:"tenantId"`
		// optional scope
		EntityType string `json:"entityType"`
	}

	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(gmod.ApiErrorResponse{
			Code:    gmod.CodeBadRequest,
			Message: "Invalid request body",
			Status:  false,
		})
	}

	// ⚠️ safety: tenantId required
	if body.TenantId == "" {
		return c.Status(400).JSON(gmod.ApiErrorResponse{
			Code:    gmod.CodeBadRequest,
			Message: "tenantId is required",
			Status:  false,
		})
	}

	deleted, err := authzsvc.FactoryResetPermifyTuples(c.Context(), body.TenantId, body.EntityType)
	if err != nil {
		return c.Status(500).JSON(gmod.ApiErrorResponse{
			Code:    gmod.CodeInternalError,
			Message: err.Error(),
			Status:  false,
		})
	}

	return c.JSON(fiber.Map{
		"code":    gmod.CodeSuccess,
		"message": "Factory reset completed",
		"status":  true,
		"details": fiber.Map{
			"deletedCount": deleted,
		},
	})
}

// ProveUserOrgsAgainstMongo godoc
// @Summary Prove org ids from permify vs mongo (debug)
// @Tags 2.authorization
// @Security BearerAuth
// @Produce json
// @Param tenantId query string false "tenant id (default from config)"
// @Router /authz/prove/orgs [get]
func ProveUserOrgsAgainstMongo(c *fiber.Ctx) error {
	tenantId := c.Query("tenantId", "")

	userIdAny := c.Locals("userId")
	userId, _ := userIdAny.(string)
	if userId == "" {
		// ถ้าคุณใช้ locals key อื่น ให้เปลี่ยนเอง
		userAny := c.Locals("user")
		if u, ok := userAny.(map[string]any); ok {
			if v, ok := u["sub"].(string); ok {
				userId = v
			}
		}
	}

	res, err := authzsvc.ProveUserOrgsAgainstMongo(c.Context(), tenantId, userId)
	if err != nil {
		return c.Status(500).JSON(gmod.ApiErrorResponse{
			Code:    gmod.CodeInternalError,
			Message: err.Error(),
			Status:  false,
		})
	}

	return c.JSON(fiber.Map{
		"code":    gmod.CodeSuccess,
		"message": "Prove completed",
		"status":  true,
		"details": res,
	})
}

// ResetAllTuples godoc
// @Summary Factory reset tuples by entityType (debug)
// @Tags authzDebug
// @Security BearerAuth
// @Accept json
// @Produce json
// @Router /authzDebugs/tuples/resetAll [post]
func ResetAllTuples(c *fiber.Ctx) error {
	var body struct {
		TenantId   string `json:"tenantId"`
		EntityType string `json:"entityType"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(gmod.ApiErrorResponse{
			Code:    gmod.CodeBadRequest,
			Message: "Invalid request body",
			Status:  false,
		})
	}

	res, err := authzsvc.ResetPermifyTuplesAll(c.Context(), body.TenantId, body.EntityType)
	if err != nil {
		return c.Status(500).JSON(gmod.ApiErrorResponse{
			Code:    gmod.CodeInternalError,
			Message: err.Error(),
			Status:  false,
		})
	}

	return c.JSON(fiber.Map{
		"code":    gmod.CodeSuccess,
		"message": "Reset all tuples completed",
		"status":  true,
		"details": res,
	})
}

// ResetTuplesByUser godoc
// @Summary Reset org membership tuples by user (debug)
// @Tags authzDebug
// @Security BearerAuth
// @Accept json
// @Produce json
// @Router /authzDebugs/tuples/resetByUser [post]
func ResetTuplesByUser(c *fiber.Ctx) error {
	var body struct {
		TenantId string `json:"tenantId"`
		UserId   string `json:"userId"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(gmod.ApiErrorResponse{
			Code:    gmod.CodeBadRequest,
			Message: "Invalid request body",
			Status:  false,
		})
	}

	res, err := authzsvc.ResetPermifyTuplesByUser(c.Context(), body.TenantId, body.UserId)
	if err != nil {
		return c.Status(500).JSON(gmod.ApiErrorResponse{
			Code:    gmod.CodeInternalError,
			Message: err.Error(),
			Status:  false,
		})
	}

	return c.JSON(fiber.Map{
		"code":    gmod.CodeSuccess,
		"message": "Reset tuples by user completed",
		"status":  true,
		"details": res,
	})
}

// ListTuplesByUser godoc
// @Summary List all tuples by user (debug)
// @Tags authzDebug
// @Security BearerAuth
// @Produce json
// @Param tenantId query string false "tenant id"
// @Param userId query string true "user id"
// @Router /authzDebugs/tuples/byUser [get]
func ListTuplesByUser(c *fiber.Ctx) error {

	tenantId := c.Query("tenantId", "")
	userId := c.Query("userId", "")

	if userId == "" {
		return c.Status(400).JSON(gmod.ApiErrorResponse{
			Code:    gmod.CodeBadRequest,
			Message: "userId is required",
			Status:  false,
		})
	}

	res, err := authzsvc.ListTuplesByUser(c.Context(), tenantId, userId)
	if err != nil {
		return c.Status(500).JSON(gmod.ApiErrorResponse{
			Code:    gmod.CodeInternalError,
			Message: err.Error(),
			Status:  false,
		})
	}

	return c.JSON(fiber.Map{
		"code":    gmod.CodeSuccess,
		"message": "User tuples fetched",
		"status":  true,
		"details": res,
	})
}

func ListOrgUnitTuples(c *fiber.Ctx) error {

	tenantId := c.Query("tenantId", "")

	res, err := authzsvc.ListTuplesByEntityType(c.Context(), tenantId, "orgUnit")
	if err != nil {
		return c.Status(500).JSON(gmod.ApiErrorResponse{
			Code:    gmod.CodeInternalError,
			Message: err.Error(),
			Status:  false,
		})
	}

	return c.JSON(fiber.Map{
		"code":    gmod.CodeSuccess,
		"message": "OU tuples fetched",
		"status":  true,
		"details": res,
	})
}

// ResetTenant godoc
// @Summary Hard reset ALL tuples in tenant (danger)
// @Tags authzDebug
// @Security BearerAuth
// @Accept json
// @Produce json
// @Router /authzDebugs/tuples/resetTenant [post]
func ResetTenant(c *fiber.Ctx) error {

	var body struct {
		TenantId string `json:"tenantId"`
		Confirm  string `json:"confirm"`
	}

	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(gmod.ApiErrorResponse{
			Code:    gmod.CodeBadRequest,
			Message: "Invalid request body",
			Status:  false,
		})
	}

	if body.TenantId == "" {
		return c.Status(400).JSON(gmod.ApiErrorResponse{
			Code:    gmod.CodeBadRequest,
			Message: "tenantId is required",
			Status:  false,
		})
	}

	// 🔥 SAFETY GUARD
	if body.Confirm != "RESET_TENANT" {
		return c.Status(400).JSON(gmod.ApiErrorResponse{
			Code:    gmod.CodeBadRequest,
			Message: "confirmation required: send confirm=RESET_TENANT",
			Status:  false,
		})
	}

	res, err := authzsvc.ResetTenant(c.Context(), body.TenantId)
	if err != nil {
		return c.Status(500).JSON(gmod.ApiErrorResponse{
			Code:    gmod.CodeInternalError,
			Message: err.Error(),
			Status:  false,
		})
	}

	return c.JSON(fiber.Map{
		"code":    gmod.CodeSuccess,
		"message": "Tenant hard reset completed",
		"status":  true,
		"details": res,
	})
}
