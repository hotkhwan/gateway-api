// controllers/adminapi/license.go
package adminapi

import (
	"context"
	"errors"

	"github.com/gofiber/fiber/v3"
	"github.com/hotkhwan/gateway-api/internal/repo/subscriprepo"
	"github.com/hotkhwan/gateway-api/internal/services/licensesvc"
	"github.com/hotkhwan/gateway-api/models/subscripmod"
	"github.com/hotkhwan/gateway-api/utils/httputil"
	"github.com/hotkhwan/gateway-api/utils/traceutil"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// licenseAdminSvc is the minimal surface LicenseAdminController needs, kept
// as an interface so tests can swap in mocks without instantiating the real
// service against MongoDB.
type licenseAdminSvc interface {
	Issue(ctx context.Context, opts licensesvc.IssueOptions) (*subscripmod.LicenseKey, error)
	List(ctx context.Context) ([]*subscripmod.LicenseKey, error)
	Get(ctx context.Context, id primitive.ObjectID) (*subscripmod.LicenseKey, error)
	Revoke(ctx context.Context, id primitive.ObjectID) error
}

// LicenseAdminController exposes the minimal HTTP surface that replaces the
// old cmd/license.go CLI for issuing and revoking enterprise license keys.
// Mounted only when LICENSE_ADMIN_ENABLED=true so customer deployments do not
// ship issuance routes.
type LicenseAdminController struct {
	svc licenseAdminSvc
}

func NewLicenseAdminController(svc *licensesvc.Service) *LicenseAdminController {
	if svc == nil {
		panic("LicenseAdminController: svc required")
	}
	return &LicenseAdminController{svc: svc}
}

type issueRequest struct {
	PlanId string                          `json:"planId"`
	Notes  *string                         `json:"notes,omitempty"`
	Limits *subscripmod.SubscriptionLimits `json:"limits,omitempty"`
}

// Issue godoc
// @Summary      Issue a new enterprise license key
// @Description  Generates a new license key and stores it in available state. Requires LICENSE_ADMIN_ENABLED=true.
// @Tags         Admin.License
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        request body issueRequest false "Issue options"
// @Success      200 {object} gmod.SuccessDataResponse
// @Failure      500 {object} gmod.ApiErrorResponse
// @Router       /admin/licenses [post]
func (ctrl *LicenseAdminController) Issue(c fiber.Ctx) error {
	ctx, end, log := traceutil.StartLite(c, "gateway.adminapi", "LicenseAdminController.Issue", "adminapi", "Issue")
	defer end()

	// Empty body is legitimate — issuance has sensible defaults. A body that
	// is present but malformed (wrong JSON, unexpected type) is a caller bug
	// we should surface with 400 instead of silently minting a default key.
	var req issueRequest
	if len(c.Body()) > 0 {
		if err := c.Bind().Body(&req); err != nil {
			return httputil.FailBadRequest(c, "invalid request body")
		}
	}

	lic, err := ctrl.svc.Issue(ctx, licensesvc.IssueOptions{
		PlanID: req.PlanId,
		Notes:  req.Notes,
		Limits: req.Limits,
	})
	if err != nil {
		if errors.Is(err, licensesvc.ErrSecretRequired) {
			return httputil.FailInternal(c, "LIC_SEC_KEY is not configured")
		}
		log.Error().Err(err).Msg("license issue failed")
		return httputil.FailInternal(c, "failed to issue license")
	}
	return httputil.Ok(c, lic)
}

// List godoc
// @Summary      List license keys
// @Description  Returns all license keys ordered by createdAt desc.
// @Tags         Admin.License
// @Security     BearerAuth
// @Produce      json
// @Success      200 {object} gmod.SuccessDataResponse
// @Failure      500 {object} gmod.ApiErrorResponse
// @Router       /admin/licenses [get]
func (ctrl *LicenseAdminController) List(c fiber.Ctx) error {
	ctx, end, log := traceutil.StartLite(c, "gateway.adminapi", "LicenseAdminController.List", "adminapi", "List")
	defer end()

	licenses, err := ctrl.svc.List(ctx)
	if err != nil {
		log.Error().Err(err).Msg("license list failed")
		return httputil.FailInternal(c, "failed to list licenses")
	}
	return httputil.Ok(c, licenses)
}

// Get godoc
// @Summary      Get one license key
// @Tags         Admin.License
// @Security     BearerAuth
// @Produce      json
// @Param        id path string true "License id (ObjectID hex)"
// @Success      200 {object} gmod.SuccessDataResponse
// @Failure      404 {object} gmod.ApiErrorResponse
// @Router       /admin/licenses/{id} [get]
func (ctrl *LicenseAdminController) Get(c fiber.Ctx) error {
	ctx, end, log := traceutil.StartLite(c, "gateway.adminapi", "LicenseAdminController.Get", "adminapi", "Get")
	defer end()

	id, err := primitive.ObjectIDFromHex(c.Params("id"))
	if err != nil {
		return httputil.FailBadRequest(c, "invalid license id")
	}
	lic, err := ctrl.svc.Get(ctx, id)
	if err != nil {
		if errors.Is(err, subscriprepo.ErrLicenseNotFound) {
			return httputil.FailNotFound(c, "license not found")
		}
		log.Error().Err(err).Msg("license get failed")
		return httputil.FailInternal(c, "failed to get license")
	}
	return httputil.Ok(c, lic)
}

// Revoke godoc
// @Summary      Revoke a license key
// @Tags         Admin.License
// @Security     BearerAuth
// @Produce      json
// @Param        id path string true "License id (ObjectID hex)"
// @Success      200 {object} gmod.SuccessDataResponse
// @Router       /admin/licenses/{id}/revoke [post]
func (ctrl *LicenseAdminController) Revoke(c fiber.Ctx) error {
	ctx, end, log := traceutil.StartLite(c, "gateway.adminapi", "LicenseAdminController.Revoke", "adminapi", "Revoke")
	defer end()

	id, err := primitive.ObjectIDFromHex(c.Params("id"))
	if err != nil {
		return httputil.FailBadRequest(c, "invalid license id")
	}
	if err := ctrl.svc.Revoke(ctx, id); err != nil {
		log.Error().Err(err).Msg("license revoke failed")
		return httputil.FailInternal(c, "failed to revoke license")
	}
	return httputil.Ok(c, fiber.Map{"id": id.Hex(), "revoked": true})
}
