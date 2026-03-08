// controllers/ingestapi/mappingSuggestion.go
package ingestapi

import (
	"github.com/gofiber/fiber/v2"
	"github.com/hotkhwan/gateway-api/internal/services/mappingsuggestionsvc"
	"github.com/hotkhwan/gateway-api/models/gmod"
	"github.com/hotkhwan/gateway-api/utils/httputil"
	"github.com/hotkhwan/gateway-api/utils/traceutil"
)

type MappingSuggestionController struct {
	svc *mappingsuggestionsvc.MappingSuggestionService
}

func NewMappingSuggestionController(svc *mappingsuggestionsvc.MappingSuggestionService) *MappingSuggestionController {
	if svc == nil {
		panic("MappingSuggestionController: svc required")
	}
	return &MappingSuggestionController{svc: svc}
}

// List godoc
// @Summary      List mapping suggestions
// @Tags         ingest-mapping-suggestions
// @Security     BearerAuth
// @Param        X-Active-Org   header  string  true   "Active Org ID"
// @Param        sourceFamily   query   string  false  "Filter by sourceFamily"
// @Success      200  {object}  gmod.SuccessDataResponse
// @Failure      401  {object}  gmod.ApiErrorResponse
// @Router       /api/v1/ingest/mappingSuggestions [get]
func (h *MappingSuggestionController) List(c *fiber.Ctx) error {
	ctx, end, _ := traceutil.StartLite(c.UserContext(), "gateway.ingestapi", "MappingSuggestionController.List", "ingestapi", "ListMappingSuggestions")
	defer end()
	_ = ctx

	sourceFamily := c.Query("sourceFamily")
	if sourceFamily != "" {
		items := h.svc.GetByFamily(sourceFamily)
		return c.JSON(fiber.Map{
			"code":    gmod.CodeSuccess,
			"message": "mapping suggestions fetched",
			"status":  true,
			"details": items,
		})
	}

	items := h.svc.List()
	return c.JSON(fiber.Map{
		"code":    gmod.CodeSuccess,
		"message": "mapping suggestions fetched",
		"status":  true,
		"details": items,
	})
}

// Get godoc
// @Summary      Get mapping suggestion by ID
// @Tags         ingest-mapping-suggestions
// @Security     BearerAuth
// @Param        X-Active-Org  header  string  true  "Active Org ID"
// @Param        id            path    string  true  "Suggestion ID"
// @Success      200  {object}  gmod.SuccessDataResponse
// @Failure      404  {object}  gmod.ApiErrorResponse
// @Router       /api/v1/ingest/mappingSuggestions/{id} [get]
func (h *MappingSuggestionController) Get(c *fiber.Ctx) error {
	ctx, end, _ := traceutil.StartLite(c.UserContext(), "gateway.ingestapi", "MappingSuggestionController.Get", "ingestapi", "GetMappingSuggestion")
	defer end()
	_ = ctx

	id := c.Params("id")
	item := h.svc.GetByID(id)
	if item == nil {
		return httputil.FailNotFound(c, "mapping suggestion not found")
	}
	return httputil.Ok(c, item, "mapping suggestion fetched")
}
