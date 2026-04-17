// controllers/rscapi/resourceList.go
package rscapi

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/hotkhwan/gateway-api/internal/services/rscsvc"
	"github.com/hotkhwan/gateway-api/models/rscmod"
	"github.com/hotkhwan/gateway-api/utils/httputil"
	"github.com/hotkhwan/gateway-api/utils/traceutil"

	"github.com/gofiber/fiber/v3"
)

// @Tags Resource
// @Summary List resources
// @Description List resources with pagination, search and filters.
// @Produce json
// @Param page query int false "Page number" default(1)
// @Param perPage query int false "Items per page" default(10)
// @Param provider query string false "Filter by provider (e.g. klynx, watchman)"
// @Param type query string false "Filter by resource type (e.g. menu, camera, warrant)"
// @Param search query string false "Free text search in displayName"
// @Param sortField query string false "Sort field" Enums(updatedAt,createdAt,displayName) default(updatedAt)
// @Param sortOrder query string false "Sort order" Enums(asc,desc) default(desc)
// @Success 200 {object} rscmod.ResourcePaginationResponse
// @Failure 500 {object} gmod.ErrorResponse
// @Router /resources [get]
// @Security BearerAuth
func ResourceList(c fiber.Ctx) error {
	ctx, end, log := traceutil.StartLite(c, "gateway.rscapi", "ResourceList", "rscapi", "ResourceList")
	defer end()

	page, _ := strconv.Atoi(c.Query("page", "1"))
	perPages, _ := strconv.Atoi(c.Query("perPage", "10"))
	sortField := strings.TrimSpace(c.Query("sortField", "updatedAt"))
	sortOrder := strings.ToLower(strings.TrimSpace(c.Query("sortOrder", "desc")))

	filters := map[string]string{
		"provider": strings.TrimSpace(c.Query("provider")),
		"type":     strings.TrimSpace(c.Query("type")),
		"search":   strings.TrimSpace(c.Query("search")),
	}

	data, pag, err := rscsvc.ResourceList(ctx, page, perPages, filters, sortField, sortOrder)
	if err != nil {
		log.Error().Err(err).Msg("Failed to list resources")
		return httputil.FailInternal(c, "failed to list resources")
	}

	// 🔁 Convert gmod.Pagination -> rscmod.Pagination
	rpag := rscmod.Pagination{
		Page:         pag.Page,
		PerPage:     pag.PerPage,
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
func ResourceCreate(c fiber.Ctx) error {
	_, end, log := traceutil.StartLite(c, "gateway.rscapi", "ResourceCreate", "rscapi", "ResourceCreate")
	defer end()

	var payload rscmod.ResourceUpsert
	if err := c.Bind().Body(&payload); err != nil {
		return httputil.FailBadRequest(c, "invalid JSON body")
	}

	// validate เบื้องต้น
	if payload.CanonicalId == "" || payload.Provider == "" || payload.Type == "" || payload.DisplayName == "" {
		return httputil.FailBadRequest(c, "canonicalId, provider, type, displayName are required")
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

	ctx, cancel := context.WithTimeout(c, 10*time.Second)
	defer cancel()

	insertedID, err := rscsvc.ResourceCreate(ctx, doc)
	if err != nil {
		log.Error().Err(err).Msg("create resource failed")
		return httputil.FailInternal(c, "failed to create resource")
	}

	return httputil.Created(c, fiber.Map{
		"id": insertedID,
	}, "Resource created successfully")
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
func ResourceUpdate(c fiber.Ctx) error {
	ctx, end, log := traceutil.StartLite(c, "gateway.rscapi", "ResourceUpdate", "rscapi", "ResourceUpdate")
	defer end()

	id := strings.TrimSpace(c.Params("id"))
	var body rscmod.ResourceUpsert
	if err := c.Bind().Body(&body); err != nil {
		return httputil.FailBadRequest(c, "Invalid request body")
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
		log.Error().Err(err).Str("id", id).Msg("Update failed")
		return httputil.FailInternal(c, "update failed")
	}
	return httputil.Ok(c, doc)
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
func ResourceDelete(c fiber.Ctx) error {
	ctx, end, log := traceutil.StartLite(c, "gateway.rscapi", "ResourceDelete", "rscapi", "ResourceDelete")
	defer end()

	id := strings.TrimSpace(c.Params("id"))
	if err := rscsvc.ResourceDelete(ctx, id); err != nil {
		log.Error().Err(err).Str("id", id).Msg("Delete failed")
		return httputil.FailInternal(c, "delete failed")
	}
	return c.SendStatus(fiber.StatusNoContent)
}
