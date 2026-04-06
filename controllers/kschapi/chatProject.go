// controllers/kschapi/chatProject.go
package kschapi

import (
	"errors"
	"strconv"
	"strings"

	"github.com/hotkhwan/gateway-api/internal/middleware"
	"github.com/hotkhwan/gateway-api/internal/services/kschsvc"
	"github.com/hotkhwan/gateway-api/models/gmod"
	"github.com/hotkhwan/gateway-api/models/kschmod"
	"github.com/hotkhwan/gateway-api/utils"
	"github.com/hotkhwan/gateway-api/utils/httputil"
	"github.com/hotkhwan/gateway-api/utils/traceutil"

	"github.com/gofiber/fiber/v3"
	"go.mongodb.org/mongo-driver/mongo"
)

// ProjectCreate godoc
// @Summary Create a new project
// @Description สร้าง Project ใหม่ (ใช้สำหรับจัดกลุ่ม Chat)
// @Tags Ksearch/Chats
// @Accept json
// @Produce json
// @Param body body kschmod.Project true "Project info"
// @Success 201 {object} gmod.SuccessMessageCreateResponse
// @Router /ksearch/projects [post]
// @Security BearerAuth
func ProjectCreate(c fiber.Ctx) error {
	ctx, end, log := traceutil.StartLite(c, "gateway.kschapi", "ChatProject.ProjectCreate", "kschapi", "ProjectCreate")
	defer end()

	var req kschmod.ProjectRequest
	if err := c.Bind().Body(&req); err != nil {
		return httputil.FailBadRequest(c, "INVALID_BODY", "Invalid request body")
	}

	// ✅ ใช้ helper ดึง user
	userId, err := middleware.CurrentUserID(c)
	if err != nil {
		return httputil.FailUnauthorized(c, "UNAUTHORIZED", err.Error())
	}

	proj, err := kschsvc.ProjectCreate(ctx, userId, kschmod.Project{
		Name: req.Name,
	})
	if err != nil {
		log.Error().Err(err).Msg("❌ create project failed")
		return httputil.FailInternal(c, "PROJECT_CREATE_FAILED", "Failed to create project")
	}

	return c.Status(fiber.StatusCreated).JSON(gmod.SuccessMessageCreateResponse{
		Code:    "SUCCESS",
		Message: "Project created successfully",
		Status:  true,
		ID:      proj.ID.Hex(),
	})
}

// ProjectList godoc
// @Summary List all projects (paginated)
// @Description ดึงรายการ Project ทั้งหมดแบบแบ่งหน้า
// @Tags Ksearch/Chats
// @Accept json
// @Produce json
// @Param page query int false "Page number" default(1)
// @Param perPage query int false "Items per page" default(10)
// @Param sortField query string false "Sort field" default("createdAt")
// @Param sortOrder query string false "Sort order" Enums(asc,desc) default(desc)
// @Success 200 {object} gmod.PaginationResponse
// @Router /ksearch/projects [get]
// @Security BearerAuth
func ProjectList(c fiber.Ctx) error {
	ctx, end, log := traceutil.StartLite(c, "gateway.kschapi", "ChatProject.ProjectList", "kschapi", "ProjectList")
	defer end()

	userId, err := middleware.CurrentUserID(c)
	if err != nil {
		return httputil.FailUnauthorized(c, "UNAUTHORIZED", err.Error())
	}

	page, _ := strconv.Atoi(c.Query("page", "1"))
	perPage, _ := strconv.Atoi(c.Query("perPage", "10"))
	sortField := c.Query("sortField", "createdAt")
	sortOrder := c.Query("sortOrder", "desc")

	projs, pagination, err := kschsvc.ProjectList(ctx, userId, page, perPage, sortField, sortOrder)
	if err != nil {
		log.Error().Err(err).Msg("❌ failed to list projects")
		return httputil.FailInternal(c, "INTERNAL_ERROR", "Failed to list projects")
	}

	return c.JSON(kschmod.ProjectListResponse{
		BaseResponse: gmod.BaseResponse{
			Code:    "SUCCESS",
			Message: "Projects retrieved successfully",
			Status:  true,
		},
		Details:    projs,
		Pagination: pagination,
	})
}

// ProjectGetByID godoc
// @Summary Get project by ID with chats
// @Description ดึง Project ตาม ID พร้อม chats ที่อยู่ใน project นั้น
// @Tags Ksearch/Chats
// @Accept json
// @Produce json
// @Param projectId path string true "Project ID"
// @Param page query int false "Page number" default(1)
// @Param perPage query int false "Items per page" default(10)
// @Success 200 {object} kschmod.ProjectChatsResponse
// @Failure 400 {object} gmod.BaseResponse "Invalid project ID format"
// @Failure 404 {object} gmod.BaseResponse "Project not found"
// @Failure 500 {object} gmod.BaseResponse "Internal server error"
// @Router /ksearch/projects/{projectId} [get]
// @Security BearerAuth
func ProjectGetByID(c fiber.Ctx) error {
	ctx, end, log := traceutil.StartLite(c, "gateway.kschapi", "ChatProject.ProjectGetByID", "kschapi", "ProjectGetByID")
	defer end()

	projectId := c.Params("projectId")
	page, _ := strconv.Atoi(c.Query("page", "1"))
	perPage, _ := strconv.Atoi(c.Query("perPage", "10"))

	proj, chats, pagination, err := kschsvc.ProjectGetByID(ctx, projectId, page, perPage)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return httputil.FailNotFound(c, "NOT_FOUND", "Project not found")
		}
		if strings.Contains(err.Error(), "invalid projectId format") {
			return httputil.FailBadRequest(c, "BAD_REQUEST", "Invalid project projectId format")
		}

		log.Error().Err(err).Str("projectId", projectId).Msg("❌ failed to get project")
		return httputil.FailInternal(c, "INTERNAL_ERROR", "Failed to get project")
	}

	return c.JSON(kschmod.ProjectChatsResponse{
		BaseResponse: gmod.BaseResponse{
			Code:    "SUCCESS",
			Message: "Project retrieved successfully",
			Status:  true,
		},
		Details: struct {
			Project    *kschmod.Project `json:"project"`
			Chats      []kschmod.Chat   `json:"chats"`
			Pagination gmod.Pagination  `json:"pagination"`
		}{
			Project:    proj,
			Chats:      utils.EmptyIfNil(chats),
			Pagination: pagination,
		},
	})
}

// ProjectUpdate godoc
// @Summary Update a project
// @Description อัปเดตข้อมูล Project
// @Tags Ksearch/Chats
// @Accept json
// @Produce json
// @Param id path string true "Project ID"
// @Param request body kschmod.ProjectUpdateRequest true "Project update data"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /ksearch/projects/{id} [put]
// @Security BearerAuth
func ProjectUpdate(c fiber.Ctx) error {
	ctx, end, log := traceutil.StartLite(c, "gateway.kschapi", "ChatProject.ProjectUpdate", "kschapi", "ProjectUpdate")
	defer end()
	id := c.Params("id")
	var req kschmod.ProjectRequest
	if err := c.Bind().Body(&req); err != nil {
		return httputil.FailBadRequest(c, "INVALID_BODY", "Invalid request body")
	}

	project, err := kschsvc.ProjectUpdate(ctx, id, kschmod.Project{
		Name: req.Name,
	})
	if err != nil {
		log.Info().Err(err).Str("project", project.ID.Hex()).Msg("❌ failed to get project")
		return httputil.FailInternal(c, "INTERNAL_ERROR", err.Error())
	}
	return c.Status(fiber.StatusOK).JSON(gmod.SuccessMessageResponse{
		Code:    "SUCCESS",
		Message: "Updated project successfully",
		Status:  true,
	})
	// return c.JSON(fiber.Map{
	// 	"status":  true,
	// 	"message": "Project updated successfully",
	// 	"data":    project,
	// })
}

// ProjectDelete godoc
// @Summary Delete a project
// @Description ลบ Project พร้อมกับ Chats ทั้งหมดที่อยู่ใน Project
// @Tags Ksearch/Chats
// @Produce json
// @Param id path string true "Project ID"
// @Success 200 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /ksearch/projects/{id} [delete]
// @Security BearerAuth
func ProjectDelete(c fiber.Ctx) error {
	ctx, end, log := traceutil.StartLite(c, "gateway.kschapi", "ChatProject.ProjectDelete", "kschapi", "ProjectDelete")
	defer end()

	id := c.Params("id")
	if err := kschsvc.ProjectDelete(ctx, id); err != nil {
		log.Error().Err(err).Str("projectId", id).Msg("💥 ProjectDelete failed")

		if strings.Contains(err.Error(), "invalid id format") {
			return httputil.FailBadRequest(c, "BAD_REQUEST", "invalid id format")
		}

		return httputil.FailInternal(c, "PROJECT_DELETE_FAILED", "Failed to delete project")
	}

	log.Info().Str("projectId", id).Msg("✅ ProjectDelete finished")
	return httputil.MessageOK(c, "Chat deleted successfully")
}
