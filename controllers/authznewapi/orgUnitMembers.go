// controllers/authznewapi/orgUnitMembers.go
package authznewapi

import (
	"fmt"
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/hotkhwan/gateway-api/internal/services/authzsvc"
)

type AssignUsersRequest struct {
	Users []authzsvc.OUAssignUser `json:"users"`
}

type RemoveUsersRequest struct {
	Users []authzsvc.OURemoveUser `json:"users"`
}

type OUBulkResponse struct {
	Code    string                  `json:"code"`
	Message string                  `json:"message"`
	Details []authzsvc.OUBulkResult `json:"details"`
	Status  bool                    `json:"status"`
}

// ---------- HELPER ----------
// ✅ Fix signature + format string
func buildBulkMessage(inserted int, removed int, duplicates int, errors int) string {
	return fmt.Sprintf("insert %d, remove %d, duplicate %d, error %d", inserted, removed, duplicates, errors)
}

// ==============================
// LIST MEMBERS
// ==============================
//
// ListMembers godoc
// @Summary List OrgUnit members
// @Tags Authorization
// @Security BearerAuth
// @Produce json
// @Param X-Active-Org header string true "Active Organization ID"
// @Param id path string true "OrgUnit ID"
// @Router /api/v1/orgs/units/{id}/members [get]
// controllers/authznewapi/orgUnitMembers.go

func (ctrl *OrgUnitController) ListMembers(c *fiber.Ctx) error {
	ctx := c.UserContext()
	tenantId, _ := c.Locals("tenantId").(string)
	orgId, _ := c.Locals("activeOrg").(string)
	ouId := c.Params("id")

	// ✅ query params
	page, _ := strconv.Atoi(c.Query("page", "1"))
	sizeStr := c.Query("perPages", "")
	if sizeStr == "" {
		sizeStr = c.Query("size", "10")
	}
	size, _ := strconv.Atoi(sizeStr)
	search := c.Query("search", "")
	sortField := c.Query("sortField", "firstName")
	sortOrder := c.Query("sortOrder", "asc")

	res, err := ctrl.service.ListMembersOfOU(ctx, tenantId, orgId, ouId, authzsvc.OUListMembersParams{
		Page:      page,
		Size:      size,
		Search:    search,
		SortField: sortField,
		SortOrder: sortOrder,
	})
	if err != nil {
		return c.Status(403).JSON(fiber.Map{"code": "FORBIDDEN", "message": err.Error(), "status": false})
	}

	return c.JSON(fiber.Map{
		"code":    "SUCCESS",
		"message": "OU members fetched",
		"status":  true,
		"details": res.Members,
		"pagination": fiber.Map{
			"page":         res.Page,
			"perPages":     res.Size,
			"totalRecords": res.TotalRecords,
			"totalPages":   res.TotalPages,
			"sortField":    res.SortField,
			"sortOrder":    res.SortOrder,
		},
	})
}

//
// ==============================
// ASSIGN MEMBERS
// ==============================
//

// AssignMembers godoc
// @Summary Assign users to OrgUnit
// @Description Bulk assign users into an OrgUnit (organization admin only)
// @Tags Authorization
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param X-Active-Org header string true "Active Organization ID"
// @Param id path string true "OrgUnit ID"
// @Param body body AssignUsersRequest true "Assign users payload"
// @Success 200 {object} OUBulkResponse
// @Failure 400 {object} fiber.Map
// @Failure 403 {object} fiber.Map
// @Router /api/v1/orgs/units/{id}/members [post]
func (ctrl *OrgUnitController) AssignMembers(c *fiber.Ctx) error {

	ctx := c.UserContext()
	tenantId, _ := c.Locals("tenantId").(string)
	callerUserId, _ := c.Locals("userId").(string)
	orgId, _ := c.Locals("activeOrg").(string)
	ouId := c.Params("id")

	var req AssignUsersRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"code":    "INVALID_REQUEST",
			"message": "invalid body",
			"status":  false,
		})
	}

	results, inserted, duplicates, err := ctrl.service.AssignUsersToOU(
		ctx, tenantId, orgId, ouId, callerUserId, req.Users,
	)
	if err != nil {
		return c.Status(403).JSON(fiber.Map{"code": "FORBIDDEN", "message": err.Error(), "status": false})
	}

	// ✅ นับ error = failed ที่ไม่ใช่ duplicate
	errorCount := 0
	for _, r := range results {
		if !r.Success && r.Error != "user already in orgUnit" && r.Error != "" {
			errorCount++
		}
	}

	return c.JSON(OUBulkResponse{
		Code:    "SUCCESS",
		Message: buildBulkMessage(inserted, 0, duplicates, errorCount),
		Details: results,
		Status:  true,
	})
}

//
// ==============================
// REMOVE MEMBERS
// ==============================
//

// RemoveMembers godoc
// @Summary Remove users from OrgUnit
// @Description Bulk remove users from an OrgUnit (organization admin only)
// @Tags Authorization
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param X-Active-Org header string true "Active Organization ID"
// @Param id path string true "OrgUnit ID"
// @Param body body RemoveUsersRequest true "Remove users payload"
// @Success 200 {object} OUBulkResponse
// @Failure 400 {object} fiber.Map
// @Failure 403 {object} fiber.Map
// @Router /api/v1/orgs/units/{id}/members [patch]
func (ctrl *OrgUnitController) RemoveMembers(c *fiber.Ctx) error {

	ctx := c.UserContext()
	tenantId, _ := c.Locals("tenantId").(string)
	callerUserId, _ := c.Locals("userId").(string)
	orgId, _ := c.Locals("activeOrg").(string)
	ouId := c.Params("id")

	var req RemoveUsersRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"code":    "INVALID_REQUEST",
			"message": "invalid body",
			"status":  false,
		})
	}

	results, removed, err := ctrl.service.RemoveUsersFromOU(
		ctx, tenantId, orgId, ouId, callerUserId, req.Users,
	)
	if err != nil {
		return c.Status(403).JSON(fiber.Map{"code": "FORBIDDEN", "message": err.Error(), "status": false})
	}

	// ✅ นับ error = failed ที่ไม่ใช่ "user not in orgUnit"
	errorCount := 0
	for _, r := range results {
		if !r.Success && r.Error != "user not in orgUnit" && r.Error != "" {
			errorCount++
		}
	}

	return c.JSON(OUBulkResponse{
		Code:    "SUCCESS",
		Message: buildBulkMessage(0, removed, 0, errorCount),
		Details: results,
		Status:  true,
	})
}
