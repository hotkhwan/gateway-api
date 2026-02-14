// controllers/rscapi/resource.go
package rscapi

import (
	"context"
	"strconv"
	"strings"
	"time"

	"klynx/internal/logger"
	"klynx/internal/services/rscsvc"
	"klynx/models/gmod"
	"klynx/models/rscmod"
	"klynx/utils/httputil"

	"github.com/gofiber/fiber/v2"
	"go.opentelemetry.io/otel"
)

// @Tags Resource
// @Summary List resources
// @Description List resources with pagination, search and filters.
// @Produce json
// @Param page query int false "Page number" default(1)
// @Param perPages query int false "Items per page" default(10)
// @Param provider query string false "Filter by provider (e.g. klynx, watchman)"
// @Param type query string false "Filter by resource type (e.g. menu, camera, warrant)"
// @Param search query string false "Free text search in displayName"
// @Param sortField query string false "Sort field" Enums(updatedAt,createdAt,displayName) default(updatedAt)
// @Param sortOrder query string false "Sort order" Enums(asc,desc) default(desc)
// @Success 200 {object} rscmod.ResourcePaginationResponse
// @Failure 500 {object} gmod.ErrorResponse
// @Router /resources [get]
// @Security BearerAuth
func ResourceList(c *fiber.Ctx) error {
	ctx := c.UserContext()
	tracer := otel.Tracer("klynx/rscapi")
	ctx, span := tracer.Start(ctx, "ResourceList")
	defer span.End()

	log := logger.FromCtx(ctx, "rscapi", "ResourceList")

	page, _ := strconv.Atoi(c.Query("page", "1"))
	perPages, _ := strconv.Atoi(c.Query("perPages", "10"))
	sortField := strings.TrimSpace(c.Query("sortField", "updatedAt"))
	sortOrder := strings.ToLower(strings.TrimSpace(c.Query("sortOrder", "desc")))

	filters := map[string]string{
		"provider": strings.TrimSpace(c.Query("provider")),
		"type":     strings.TrimSpace(c.Query("type")),
		"search":   strings.TrimSpace(c.Query("search")),
	}

	data, pag, err := rscsvc.ResourceList(ctx, page, perPages, filters, sortField, sortOrder)
	if err != nil {
		log.Error().Err(err).Msg("❌ Failed to list resources")
		return httputil.FailInternalReason(c, "internal server error", "FAILED_TO_LIST_RESOURCES")
	}

	// 🔁 Convert gmod.Pagination -> rscmod.Pagination
	rpag := rscmod.Pagination{
		Page:         pag.Page,
		PerPages:     pag.PerPages,
		TotalRecords: pag.TotalRecords,
		TotalPages:   pag.TotalPages,
		SortField:    pag.SortField,
		SortOrder:    pag.SortOrder,
	}

	return rscmod.SendPagination(c, data, rpag)
}

// @Tags Resource
// @Summary Create resource
// @Accept json
// @Produce json
// @Param body body rscmod.ResourceUpsert true "Resource payload"
// @Success 201 {object} gmod.SuccessMessageCreateResponse
// @Failure 400 {object} gmod.SuccessMessageCreateResponse
// @Failure 500 {object} gmod.SuccessMessageCreateResponse
// @Router /resources [post]
// @Security BearerAuth
func ResourceCreate(c *fiber.Ctx) error {
	var payload rscmod.ResourceUpsert
	if err := c.BodyParser(&payload); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(gmod.SuccessMessageCreateResponse{
			Code:    "BAD_REQUEST",
			Message: "invalid JSON body",
			Status:  false,
			ID:      "",
		})
	}

	// validate เบื้องต้น
	if payload.CanonicalId == "" || payload.Provider == "" || payload.Type == "" || payload.DisplayName == "" {
		return c.Status(fiber.StatusBadRequest).JSON(gmod.SuccessMessageCreateResponse{
			Code:    "VALIDATION_ERROR",
			Message: "canonicalId, provider, type, displayName are required",
			Status:  false,
			ID:      "",
		})
	}

	now := time.Now()
	doc := &rscmod.Resource{
		CanonicalId: payload.CanonicalId,
		Provider:    payload.Provider,
		Type:        payload.Type,
		Id:          payload.Id,
		ExternalId:  payload.ExternalId,
		DisplayName: payload.DisplayName,
		Icon:        payload.Icon,
		Path:        payload.Path,
		Tags:        payload.Tags,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	ctx, cancel := context.WithTimeout(c.Context(), 10*time.Second)
	defer cancel()

	insertedID, err := rscsvc.ResourceCreate(ctx, doc)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(gmod.SuccessMessageCreateResponse{
			Code:    "CREATE_FAILED",
			Message: err.Error(),
			Status:  false,
			ID:      "",
		})
	}

	return c.Status(fiber.StatusCreated).JSON(gmod.SuccessMessageCreateResponse{
		Code:    "SUCCESS",
		Message: "Resource created successfully",
		Status:  true,
		ID:      insertedID,
	})
}

// @Tags Resource
// @Summary Update resource
// @Accept json
// @Produce json
// @Param id path string true "Resource id (canonicalId or id — you choose key in service)"
// @Param body body rscmod.ResourceUpsert true "Resource payload"
// @Success 200 {object} rscmod.Resource
// @Failure 400 {object} gmod.ErrorResponse
// @Failure 404 {object} gmod.ErrorResponse
// @Failure 500 {object} gmod.ErrorResponse
// @Router /resources/{id} [put]
// @Security BearerAuth
func ResourceUpdate(c *fiber.Ctx) error {
	ctx := c.UserContext()
	tracer := otel.Tracer("klynx/rscapi")
	ctx, span := tracer.Start(ctx, "ResourceUpdate")
	defer span.End()

	log := logger.FromCtx(ctx, "rscapi", "ResourceUpdate")

	id := strings.TrimSpace(c.Params("id"))
	var body rscmod.ResourceUpsert
	if err := c.BodyParser(&body); err != nil {
		return httputil.FailBadRequest(c, "INVALID_BODY", "Invalid request body")
	}
	doc := rscmod.Resource{
		CanonicalId: body.CanonicalId,
		Provider:    body.Provider,
		Type:        body.Type,
		Id:          body.Id,
		ExternalId:  body.ExternalId,
		DisplayName: body.DisplayName,
		Icon:        body.Icon,
		Path:        body.Path,
		Tags:        body.Tags,
		UpdatedAt:   time.Now(),
	}

	if err := rscsvc.ResourceUpdate(ctx, id, &doc); err != nil {
		log.Error().Err(err).Str("id", id).Msg("❌ Update failed")
		return httputil.FailInternalReason(c, "internal server error", "UPDATE_FAILED")
	}
	return c.JSON(doc)
}

// @Tags Resource
// @Summary Delete resource (soft-delete)
// @Produce json
// @Param id path string true "Resource id (canonicalId or id — you choose key in service)"
// @Success 204 {string} string "No Content"
// @Failure 404 {object} gmod.ErrorResponse
// @Failure 500 {object} gmod.ErrorResponse
// @Router /resources/{id} [delete]
// @Security BearerAuth
func ResourceDelete(c *fiber.Ctx) error {
	ctx := c.UserContext()
	tracer := otel.Tracer("klynx/rscapi")
	ctx, span := tracer.Start(ctx, "ResourceDelete")
	defer span.End()

	log := logger.FromCtx(ctx, "rscapi", "ResourceDelete")

	id := strings.TrimSpace(c.Params("id"))
	if err := rscsvc.ResourceDelete(ctx, id); err != nil {
		log.Error().Err(err).Str("id", id).Msg("❌ Delete failed")
		return httputil.FailInternalReason(c, "internal server error", "DELETE_FAILED")
	}
	return c.SendStatus(fiber.StatusNoContent)
}
