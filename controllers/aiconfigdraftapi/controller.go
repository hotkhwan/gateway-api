// controllers/aiconfigdraftapi/controller.go
package aiconfigdraftapi

import "github.com/hotkhwan/gateway-api/internal/services/aiconfigdraftsvc"

// ConfigDraftController handles HTTP requests for AI config draft operations.
type ConfigDraftController struct {
	svc *aiconfigdraftsvc.ConfigDraftService
}

// NewConfigDraftController constructs a ConfigDraftController.
// Panics if svc is nil.
func NewConfigDraftController(svc *aiconfigdraftsvc.ConfigDraftService) *ConfigDraftController {
	if svc == nil {
		panic("ConfigDraftController: svc required")
	}
	return &ConfigDraftController{svc: svc}
}
