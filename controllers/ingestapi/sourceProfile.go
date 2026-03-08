// controllers/ingestapi/sourceProfile.go
package ingestapi

import (
	"errors"

	"github.com/gofiber/fiber/v2"
	"github.com/hotkhwan/gateway-api/internal/services/sourceprofilesvc"
	"github.com/hotkhwan/gateway-api/models/ingestmod"
	"github.com/hotkhwan/gateway-api/utils/httputil"
	"github.com/hotkhwan/gateway-api/utils/traceutil"
	"go.mongodb.org/mongo-driver/bson"
)

type SourceProfileController struct {
	svc *sourceprofilesvc.SourceProfileService
}

func NewSourceProfileController(svc *sourceprofilesvc.SourceProfileService) *SourceProfileController {
	if svc == nil {
		panic("SourceProfileController: svc required")
	}
	return &SourceProfileController{svc: svc}
}

func (h *SourceProfileController) List(c *fiber.Ctx) error {
	ctx, end, log := traceutil.StartLite(c.UserContext(), "gateway.ingestapi", "SourceProfileController.List", "ingestapi", "ListSourceProfiles")
	defer end()

	items, err := h.svc.List(ctx)
	if err != nil {
		log.Error().Err(err).Msg("[ListSourceProfiles] failed")
		return httputil.FailInternal(c, "failed to list source profiles")
	}

	log.Debug().Int("count", len(items)).Msg("[ListSourceProfiles] fetched")
	return httputil.Ok(c, items, "source profiles fetched successfully")
}

func (h *SourceProfileController) Get(c *fiber.Ctx) error {
	ctx, end, log := traceutil.StartLite(c.UserContext(), "gateway.ingestapi", "SourceProfileController.Get", "ingestapi", "GetSourceProfile")
	defer end()

	sf := c.Params("sourceFamily")
	p, err := h.svc.Get(ctx, sf)
	if err != nil {
		if errors.Is(err, sourceprofilesvc.ErrNotFound) {
			return httputil.FailNotFound(c, "source profile not found")
		}
		log.Error().Err(err).Str("sourceFamily", sf).Msg("[GetSourceProfile] failed")
		return httputil.FailInternal(c, "failed to get source profile")
	}
	return httputil.Ok(c, p, "source profile fetched successfully")
}

func (h *SourceProfileController) Create(c *fiber.Ctx) error {
	ctx, end, log := traceutil.StartLite(c.UserContext(), "gateway.ingestapi", "SourceProfileController.Create", "ingestapi", "CreateSourceProfile")
	defer end()

	var in ingestmod.SourceProfile
	if err := c.BodyParser(&in); err != nil {
		return httputil.FailBadRequest(c, "invalid request body")
	}
	if in.SourceFamily == "" {
		return httputil.FailBadRequest(c, "sourceFamily is required")
	}
	if in.DisplayName == "" {
		return httputil.FailBadRequest(c, "displayName is required")
	}

	if err := h.svc.Create(ctx, &in); err != nil {
		log.Error().Err(err).Str("sourceFamily", in.SourceFamily).Msg("[CreateSourceProfile] failed")
		return httputil.FailInternal(c, "failed to create source profile")
	}

	log.Info().Str("sourceFamily", in.SourceFamily).Msg("[CreateSourceProfile] created")
	return httputil.Created(c, in, "source profile created successfully")
}

func (h *SourceProfileController) Update(c *fiber.Ctx) error {
	ctx, end, log := traceutil.StartLite(c.UserContext(), "gateway.ingestapi", "SourceProfileController.Update", "ingestapi", "UpdateSourceProfile")
	defer end()

	sf := c.Params("sourceFamily")

	var body map[string]any
	if err := c.BodyParser(&body); err != nil {
		return httputil.FailBadRequest(c, "invalid request body")
	}

	update := bson.M{}
	if v, ok := body["displayName"]; ok {
		update["displayName"] = v
	}
	if v, ok := body["mode"]; ok {
		update["mode"] = v
	}
	if len(update) == 0 {
		return httputil.FailBadRequest(c, "no fields to update")
	}

	if err := h.svc.Update(ctx, sf, update); err != nil {
		if errors.Is(err, sourceprofilesvc.ErrNotFound) {
			return httputil.FailNotFound(c, "source profile not found")
		}
		log.Error().Err(err).Str("sourceFamily", sf).Msg("[UpdateSourceProfile] failed")
		return httputil.FailInternal(c, "failed to update source profile")
	}

	log.Info().Str("sourceFamily", sf).Msg("[UpdateSourceProfile] updated")
	return httputil.MessageOK(c, "source profile updated successfully")
}
