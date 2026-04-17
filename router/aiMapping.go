// router/aiMapping.go
package router

import (
	"github.com/gofiber/fiber/v3"
	"github.com/hotkhwan/gateway-api/internal/app"
	"github.com/hotkhwan/gateway-api/internal/middleware"
)

// RegisterAIMappingRoutes wires AI mapping suggestion + config draft endpoints
// under the existing /ingest group (requires AuthBearer + ActiveWorkspace).
//
// Endpoints added:
//
//	POST   /ingest/mappingTemplates/ai-suggest
//	GET    /ingest/ai-config
//	PUT    /ingest/ai-config
//	DELETE /ingest/ai-config/key
//	POST   /ingest/ai-config/validate
//	POST   /ingest/config-drafts/from-prompt
//	POST   /ingest/config-drafts/:draftId/refine
//	POST   /ingest/config-drafts/:draftId/dry-run
//	POST   /ingest/config-drafts/:draftId/save
func RegisterAIMappingRoutes(ingestRouter fiber.Router, c *app.Container) {
	// ---------- AI Suggest ----------
	ingestRouter.All("/mappingTemplates/ai-suggest", middleware.AllowMethods("POST"))
	ingestRouter.Post("/mappingTemplates/ai-suggest", c.AIMappingController.AISuggest)

	// ---------- AI Config ----------
	ingestRouter.All("/ai-config", middleware.AllowMethods("GET", "PUT"))
	ingestRouter.Get("/ai-config", c.AIMappingController.GetAIConfig)
	ingestRouter.Put("/ai-config", c.AIMappingController.UpsertAIConfig)

	ingestRouter.All("/ai-config/key", middleware.AllowMethods("DELETE"))
	ingestRouter.Delete("/ai-config/key", c.AIMappingController.ClearApiKey)

	ingestRouter.All("/ai-config/validate", middleware.AllowMethods("POST"))
	ingestRouter.Post("/ai-config/validate", c.AIMappingController.ValidateAIConfig)

	// ---------- Config Drafts (Feature B) ----------
	ingestRouter.All("/config-drafts/from-prompt", middleware.AllowMethods("POST"))
	ingestRouter.Post("/config-drafts/from-prompt", c.ConfigDraftController.FromPrompt)

	ingestRouter.All("/config-drafts/:draftId/refine", middleware.AllowMethods("POST"))
	ingestRouter.Post("/config-drafts/:draftId/refine", c.ConfigDraftController.Refine)

	ingestRouter.All("/config-drafts/:draftId/dry-run", middleware.AllowMethods("POST"))
	ingestRouter.Post("/config-drafts/:draftId/dry-run", c.ConfigDraftController.DryRun)

	ingestRouter.All("/config-drafts/:draftId/save", middleware.AllowMethods("POST"))
	ingestRouter.Post("/config-drafts/:draftId/save", c.ConfigDraftController.Save)
}
