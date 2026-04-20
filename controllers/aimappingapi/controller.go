// controllers/aimappingapi/controller.go
package aimappingapi

import "github.com/hotkhwan/gateway-api/internal/services/aimappingsvc"

// AIMappingController handles HTTP requests for AI mapping operations.
type AIMappingController struct {
	svc *aimappingsvc.AIMappingService
}

// NewAIMappingController constructs an AIMappingController.
// Panics if svc is nil.
func NewAIMappingController(svc *aimappingsvc.AIMappingService) *AIMappingController {
	if svc == nil {
		panic("AIMappingController: svc required")
	}
	return &AIMappingController{svc: svc}
}
