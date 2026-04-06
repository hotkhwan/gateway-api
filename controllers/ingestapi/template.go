// controllers/ingestapi/template.go
package ingestapi

import (
	"errors"

	"github.com/gofiber/fiber/v3"
	"github.com/hotkhwan/gateway-api/internal/services/templatesvc"
	"github.com/hotkhwan/gateway-api/models/gmod"
	"github.com/hotkhwan/gateway-api/utils/httputil"
	"github.com/hotkhwan/gateway-api/utils/traceutil"
)

// TemplateController handles mapping template CRUD endpoints.
type TemplateController struct {
	svc *templatesvc.TemplateService
}

func NewTemplateController(svc *templatesvc.TemplateService) *TemplateController {
	if svc == nil {
		panic("TemplateController: svc is required")
	}
	return &TemplateController{svc: svc}
}

func (h *TemplateController) orgId(c fiber.Ctx) string {
	orgId, _ := c.Locals("activeOrg").(string)
	return orgId
}

// List godoc
// @Summary      List mapping templates
// @Tags         ingest-mapping-templates
// @Security     BearerAuth
// @Param        X-Active-Org  header    string  true   "Active Org ID"
// @Param        page          query     int     false  "Page number"    default(1)
// @Param        perPages      query     int     false  "Items per page" default(10)
// @Param        search        query     string  false  "Search by name"
// @Param        sortField     query     string  false  "Sort field"     default(createdAt)
// @Param        sortOrder     query     string  false  "Sort order"     default(desc)
// @Success      200  {object}  gmod.PaginatedResponse
// @Failure      401  {object}  gmod.ApiErrorResponse
// @Router       /ingest/mappingTemplates [get]
func (h *TemplateController) List(c fiber.Ctx) error {
	ctx, end, log := traceutil.StartLite(c, "gateway.ingestapi", "TemplateController.List", "ingestapi", "ListTemplates")
	defer end()

	in := &templatesvc.ListTemplatesInput{
		OrgId:     h.orgId(c),
		Search:    c.Query("search"),
		Page:      fiber.Query[int](c, "page", 1),
		PerPage:   fiber.Query[int](c, "perPages", 10),
		SortField: c.Query("sortField", "createdAt"),
		SortOrder: c.Query("sortOrder", "desc"),
	}

	// Debug — read-only list query
	log.Debug().
		Str("orgId", in.OrgId).
		Str("search", in.Search).
		Int("page", in.Page).
		Msg("📥 [ListTemplates] listing templates")

	items, pag, err := h.svc.List(ctx, in)
	if err != nil {
		log.Error().Err(err).Str("orgId", in.OrgId).Msg("❌ [ListTemplates] failed to list templates")
		return httputil.FailInternal(c, "failed to list templates")
	}

	// Debug — read-only result
	log.Debug().Str("orgId", in.OrgId).Msg("✅ [ListTemplates] templates fetched")

	return c.JSON(fiber.Map{
		"code":       gmod.CodeSuccess,
		"message":    "templates fetched successfully",
		"status":     true,
		"details":    items,
		"pagination": pag,
	})
}

// Get godoc
// @Summary      Get mapping template
// @Tags         ingest-mapping-templates
// @Security     BearerAuth
// @Param        X-Active-Org  header    string  true  "Active Org ID"
// @Param        templateId    path      string  true  "Template ID"
// @Success      200  {object}  gmod.SuccessDataResponse
// @Failure      404  {object}  gmod.ApiErrorResponse
// @Router       /ingest/mappingTemplates/{templateId} [get]
func (h *TemplateController) Get(c fiber.Ctx) error {
	ctx, end, log := traceutil.StartLite(c, "gateway.ingestapi", "TemplateController.Get", "ingestapi", "GetTemplate")
	defer end()

	orgId := h.orgId(c)
	templateId := c.Params("templateId")

	// Debug — read-only single fetch
	log.Debug().Str("orgId", orgId).Str("templateId", templateId).Msg("📥 [GetTemplate] get template")

	t, err := h.svc.Get(ctx, orgId, templateId)
	if err != nil {
		if errors.Is(err, templatesvc.ErrTemplateNotFound) {
			log.Warn().Str("orgId", orgId).Str("templateId", templateId).Msg("❌ [GetTemplate] not found")
			return httputil.FailNotFound(c, err.Error())
		}
		log.Error().Err(err).Str("orgId", orgId).Str("templateId", templateId).Msg("❌ [GetTemplate] internal error")
		return httputil.FailInternal(c, "failed to get template")
	}

	// Debug — read-only result
	log.Debug().Str("orgId", orgId).Str("templateId", templateId).Msg("✅ [GetTemplate] template fetched")
	return httputil.Ok(c, t, "template fetched successfully")
}

// Create godoc
// @Summary      Create mapping template
// @Tags         ingest-mapping-templates
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        X-Active-Org  header    string                         true  "Active Org ID"
// @Param        body          body      templatesvc.CreateTemplateInput true  "Template definition"
// @Success      201  {object}  gmod.SuccessDataResponse
// @Failure      400  {object}  gmod.ApiErrorResponse
// @Router       /ingest/mappingTemplates [post]
func (h *TemplateController) Create(c fiber.Ctx) error {
	ctx, end, log := traceutil.StartLite(c, "gateway.ingestapi", "TemplateController.Create", "ingestapi", "CreateTemplate")
	defer end()

	orgId := h.orgId(c)

	var in templatesvc.CreateTemplateInput
	if err := c.Bind().Body(&in); err != nil {
		log.Warn().Str("orgId", orgId).Msg("❌ [CreateTemplate] invalid request body")
		return httputil.FailBadRequest(c, "invalid request body")
	}
	if in.Name == "" {
		log.Warn().Str("orgId", orgId).Msg("❌ [CreateTemplate] name is required")
		return httputil.FailBadRequest(c, "name is required")
	}

	log.Info().Str("orgId", orgId).Str("name", in.Name).Msg("📥 [CreateTemplate] creating template")

	t, err := h.svc.Create(ctx, orgId, &in)
	if err != nil {
		log.Error().Err(err).Str("orgId", orgId).Str("name", in.Name).Msg("❌ [CreateTemplate] failed to create template")
		return httputil.FailInternal(c, "failed to create template")
	}

	log.Info().Str("orgId", orgId).Str("templateId", t.TemplateId).Str("name", t.Name).Msg("✅ [CreateTemplate] template created")
	return httputil.Created(c, t, "template created successfully")
}

// Update godoc
// @Summary      Update mapping template
// @Tags         ingest-mapping-templates
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        X-Active-Org  header    string                         true  "Active Org ID"
// @Param        templateId    path      string                         true  "Template ID"
// @Param        body          body      templatesvc.UpdateTemplateInput true  "Fields to update"
// @Success      200  {object}  gmod.SuccessMessageResponse
// @Failure      400  {object}  gmod.ApiErrorResponse
// @Failure      404  {object}  gmod.ApiErrorResponse
// @Router       /ingest/mappingTemplates/{templateId} [patch]
func (h *TemplateController) Update(c fiber.Ctx) error {
	ctx, end, log := traceutil.StartLite(c, "gateway.ingestapi", "TemplateController.Update", "ingestapi", "UpdateTemplate")
	defer end()

	orgId := h.orgId(c)
	templateId := c.Params("templateId")

	var in templatesvc.UpdateTemplateInput
	if err := c.Bind().Body(&in); err != nil {
		log.Warn().Str("orgId", orgId).Str("templateId", templateId).Msg("❌ [UpdateTemplate] invalid request body")
		return httputil.FailBadRequest(c, "invalid request body")
	}

	log.Info().Str("orgId", orgId).Str("templateId", templateId).Msg("✏️ [UpdateTemplate] updating template")

	if err := h.svc.Update(ctx, orgId, templateId, &in); err != nil {
		if errors.Is(err, templatesvc.ErrTemplateNotFound) {
			log.Warn().Str("orgId", orgId).Str("templateId", templateId).Msg("❌ [UpdateTemplate] not found")
			return httputil.FailNotFound(c, err.Error())
		}
		log.Error().Err(err).Str("orgId", orgId).Str("templateId", templateId).Msg("❌ [UpdateTemplate] internal error")
		return httputil.FailInternal(c, "failed to update template")
	}

	log.Info().Str("orgId", orgId).Str("templateId", templateId).Msg("✅ [UpdateTemplate] template updated")
	return httputil.MessageOK(c, "template updated successfully")
}

// Delete godoc
// @Summary      Delete mapping template
// @Tags         ingest-mapping-templates
// @Security     BearerAuth
// @Param        X-Active-Org  header    string  true  "Active Org ID"
// @Param        templateId    path      string  true  "Template ID"
// @Success      200  {object}  gmod.SuccessMessageResponse
// @Failure      404  {object}  gmod.ApiErrorResponse
// @Router       /ingest/mappingTemplates/{templateId} [delete]
func (h *TemplateController) Delete(c fiber.Ctx) error {
	ctx, end, log := traceutil.StartLite(c, "gateway.ingestapi", "TemplateController.Delete", "ingestapi", "DeleteTemplate")
	defer end()

	orgId := h.orgId(c)
	templateId := c.Params("templateId")

	log.Info().Str("orgId", orgId).Str("templateId", templateId).Msg("🗑️ [DeleteTemplate] deleting template")

	if err := h.svc.Delete(ctx, orgId, templateId); err != nil {
		if errors.Is(err, templatesvc.ErrTemplateNotFound) {
			log.Warn().Str("orgId", orgId).Str("templateId", templateId).Msg("❌ [DeleteTemplate] not found")
			return httputil.FailNotFound(c, err.Error())
		}
		log.Error().Err(err).Str("orgId", orgId).Str("templateId", templateId).Msg("❌ [DeleteTemplate] internal error")
		return httputil.FailInternal(c, "failed to delete template")
	}

	log.Info().Str("orgId", orgId).Str("templateId", templateId).Msg("✅ [DeleteTemplate] template deleted")
	return httputil.MessageOK(c, "template deleted successfully")
}
