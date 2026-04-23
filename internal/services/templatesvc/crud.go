// internal/services/templatesvc/crud.go
package templatesvc

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/hotkhwan/gateway-api/internal/repo/cachetemplate"
	"github.com/hotkhwan/gateway-api/internal/repo/ingestrepo"
	"github.com/hotkhwan/gateway-api/models/gmod"
	"github.com/hotkhwan/gateway-api/models/ingestmod"
	"github.com/hotkhwan/gateway-api/utils/traceutil"
	"go.mongodb.org/mongo-driver/bson"
)

type TemplateService struct {
	repo *ingestrepo.MappingTemplateRepo
}

func NewTemplateService(repo *ingestrepo.MappingTemplateRepo) *TemplateService {
	if repo == nil {
		panic("TemplateService: repo is required")
	}
	return &TemplateService{repo: repo}
}

// Create inserts a new mapping template for the org.
func (s *TemplateService) Create(ctx context.Context, orgId string, in *CreateTemplateInput) (*ingestmod.MappingTemplate, error) {
	ctx, end, log := traceutil.StartLite(ctx, "gateway.templatesvc", "TemplateService.Create", "templatesvc", "CreateTemplate")
	defer end()

	log.Info().Str("orgId", orgId).Str("name", in.Name).Msg("📥 [CreateTemplate] creating template")

	if err := validateDeliveryRules(in.DeliveryMatchAll, in.DeliveryMatchAny); err != nil {
		log.Warn().Err(err).Str("orgId", orgId).Str("name", in.Name).Msg("❌ [CreateTemplate] invalid delivery rule")
		return nil, err
	}

	now := time.Now().UTC()
	enabled := true
	if in.Enabled != nil {
		enabled = *in.Enabled
	}
	t := &ingestmod.MappingTemplate{
		TemplateId:          uuid.NewString(),
		WorkspaceId:         orgId,
		Enabled:             enabled,
		SourceFamily:        in.SourceFamily,
		FinalEventType:      in.FinalEventType,
		Priority:            in.Priority,
		Name:                in.Name,
		Match:               in.Match,
		MatchAll:            in.MatchAll,
		MatchAny:            in.MatchAny,
		DeliveryMatchAll:    in.DeliveryMatchAll,
		DeliveryMatchAny:    in.DeliveryMatchAny,
		Mappings:            in.Mappings,
		DefaultLocale:       in.DefaultLocale,
		MessageTemplates:    in.MessageTemplates,
		ClassificationRules: in.ClassificationRules,
		DeliveryTargets:     in.DeliveryTargets,
		CreatedAt:           now,
		UpdatedAt:           now,
	}
	if in.DLQ != nil {
		t.DLQ = *in.DLQ
	}
	if t.Mappings == nil {
		t.Mappings = []ingestmod.FieldMapping{}
	}

	if err := s.repo.Insert(ctx, t); err != nil {
		log.Error().Err(err).Str("orgId", orgId).Str("name", in.Name).Msg("❌ [CreateTemplate] insert failed")
		return nil, err
	}

	// Invalidate fingerprint cache so the new template is picked up on next ingest
	_ = cachetemplate.InvalidateByOrg(ctx, orgId)
	cachetemplate.InvalidateMatchV2ByOrg(ctx, orgId)

	log.Info().Str("orgId", orgId).Str("templateId", t.TemplateId).Str("name", t.Name).Msg("✅ [CreateTemplate] template created")
	return t, nil
}

// Get returns a single template by orgId + templateId.
func (s *TemplateService) Get(ctx context.Context, orgId, templateId string) (*ingestmod.MappingTemplate, error) {
	ctx, end, log := traceutil.StartLite(ctx, "gateway.templatesvc", "TemplateService.Get", "templatesvc", "GetTemplate")
	defer end()

	log.Debug().Str("orgId", orgId).Str("templateId", templateId).Msg("📥 [GetTemplate] fetching template")

	t, err := s.repo.FindById(ctx, orgId, templateId)
	if err != nil {
		if err == ingestrepo.ErrTemplateNotFound {
			log.Warn().Str("orgId", orgId).Str("templateId", templateId).Msg("❌ [GetTemplate] not found")
			return nil, ErrTemplateNotFound
		}
		log.Error().Err(err).Str("orgId", orgId).Str("templateId", templateId).Msg("❌ [GetTemplate] db error")
		return nil, err
	}

	log.Debug().Str("orgId", orgId).Str("templateId", templateId).Msg("✅ [GetTemplate] template fetched")
	return t, nil
}

// List returns paginated templates for an org.
func (s *TemplateService) List(ctx context.Context, in *ListTemplatesInput) ([]*ingestmod.MappingTemplate, *gmod.Pagination, error) {
	ctx, end, log := traceutil.StartLite(ctx, "gateway.templatesvc", "TemplateService.List", "templatesvc", "ListTemplates")
	defer end()

	if in.Page < 1 {
		in.Page = 1
	}
	if in.PerPage <= 0 {
		in.PerPage = 10
	}
	if in.PerPage > 250 {
		in.PerPage = 250
	}

	log.Debug().Str("orgId", in.OrgId).Int("page", in.Page).Str("search", in.Search).Str("sourceFamily", in.SourceFamily).Msg("📥 [ListTemplates] listing templates")

	items, pag, err := s.repo.List(ctx, in.OrgId, ingestrepo.ListFilter{
		Search:       in.Search,
		SourceFamily: in.SourceFamily,
		Enabled:      in.Enabled,
	}, in.Page, in.PerPage, in.SortField, in.SortOrder)
	if err != nil {
		log.Error().Err(err).Str("orgId", in.OrgId).Msg("❌ [ListTemplates] db error")
		return nil, nil, err
	}

	log.Debug().Str("orgId", in.OrgId).Msg("✅ [ListTemplates] templates fetched")
	return items, pag, nil
}

// Update patches a template's fields.
func (s *TemplateService) Update(ctx context.Context, orgId, templateId string, in *UpdateTemplateInput) error {
	ctx, end, log := traceutil.StartLite(ctx, "gateway.templatesvc", "TemplateService.Update", "templatesvc", "UpdateTemplate")
	defer end()

	log.Info().Str("orgId", orgId).Str("templateId", templateId).Msg("📥 [UpdateTemplate] updating template")

	if err := validateDeliveryRules(in.DeliveryMatchAll, in.DeliveryMatchAny); err != nil {
		log.Warn().Err(err).Str("orgId", orgId).Str("templateId", templateId).Msg("❌ [UpdateTemplate] invalid delivery rule")
		return err
	}

	set := bson.M{}
	if in.Name != nil {
		set["name"] = *in.Name
	}
	if in.Enabled != nil {
		set["enabled"] = *in.Enabled
	}
	if in.SourceFamily != nil {
		set["sourceFamily"] = *in.SourceFamily
	}
	if in.FinalEventType != nil {
		set["finalEventType"] = *in.FinalEventType
	}
	if in.Priority != nil {
		set["priority"] = *in.Priority
	}
	if in.Match != nil {
		set["match"] = *in.Match
	}
	if in.MatchAll != nil {
		set["matchAll"] = in.MatchAll
	}
	if in.MatchAny != nil {
		set["matchAny"] = in.MatchAny
	}
	if in.DeliveryMatchAll != nil {
		set["deliveryMatchAll"] = in.DeliveryMatchAll
	}
	if in.DeliveryMatchAny != nil {
		set["deliveryMatchAny"] = in.DeliveryMatchAny
	}
	if in.Mappings != nil {
		set["mappings"] = in.Mappings
	}
	if in.DLQ != nil {
		set["dlq"] = *in.DLQ
	}
	if in.DefaultLocale != nil {
		set["defaultLocale"] = *in.DefaultLocale
	}
	if in.MessageTemplates != nil {
		set["messageTemplates"] = in.MessageTemplates
	}
	if in.ClassificationRules != nil {
		set["classificationRules"] = in.ClassificationRules
	}
	if in.DeliveryTargets != nil {
		set["deliveryTargets"] = in.DeliveryTargets
	}
	if len(set) == 0 {
		return nil // nothing to update
	}

	if err := s.repo.Update(ctx, orgId, templateId, set); err != nil {
		if err == ingestrepo.ErrTemplateNotFound {
			log.Warn().Str("orgId", orgId).Str("templateId", templateId).Msg("❌ [UpdateTemplate] not found")
			return ErrTemplateNotFound
		}
		log.Error().Err(err).Str("orgId", orgId).Str("templateId", templateId).Msg("❌ [UpdateTemplate] db error")
		return err
	}

	// Invalidate fingerprint cache so updated match rules take effect immediately
	_ = cachetemplate.InvalidateByOrg(ctx, orgId)
	cachetemplate.InvalidateMatchV2ByOrg(ctx, orgId)

	log.Info().Str("orgId", orgId).Str("templateId", templateId).Msg("✅ [UpdateTemplate] template updated")
	return nil
}

// BackfillEnabledForDeliveringTemplates flips enabled=true on every mapping
// template that has at least one delivery target and is not already enabled.
// Idempotent. Intended to run from bootstrap before the delivery consumer
// starts, so legacy documents whose enabled field was missing / false do not
// silently lose delivery when the template-level gate is enforced
// (plan decision D10).
func (s *TemplateService) BackfillEnabledForDeliveringTemplates(ctx context.Context) (int64, int64, error) {
	ctx, end, log := traceutil.StartLite(ctx, "gateway.templatesvc", "TemplateService.BackfillEnabled", "templatesvc", "BackfillEnabled")
	defer end()

	matched, modified, err := s.repo.BackfillEnabledForTemplatesWithDeliveryTargets(ctx)
	if err != nil {
		log.Error().Err(err).Msg("[templatesvc] backfill enabled failed")
		return 0, 0, err
	}
	log.Info().
		Int64("matched", matched).
		Int64("modified", modified).
		Msg("[templatesvc] backfill enabled complete")
	return matched, modified, nil
}

// Delete removes a template.
func (s *TemplateService) Delete(ctx context.Context, orgId, templateId string) error {
	ctx, end, log := traceutil.StartLite(ctx, "gateway.templatesvc", "TemplateService.Delete", "templatesvc", "DeleteTemplate")
	defer end()

	log.Info().Str("orgId", orgId).Str("templateId", templateId).Msg("📥 [DeleteTemplate] deleting template")

	if err := s.repo.Delete(ctx, orgId, templateId); err != nil {
		if err == ingestrepo.ErrTemplateNotFound {
			log.Warn().Str("orgId", orgId).Str("templateId", templateId).Msg("❌ [DeleteTemplate] not found")
			return ErrTemplateNotFound
		}
		log.Error().Err(err).Str("orgId", orgId).Str("templateId", templateId).Msg("❌ [DeleteTemplate] db error")
		return err
	}

	// Invalidate fingerprint cache so deleted template is no longer matched
	_ = cachetemplate.InvalidateByOrg(ctx, orgId)
	cachetemplate.InvalidateMatchV2ByOrg(ctx, orgId)

	log.Info().Str("orgId", orgId).Str("templateId", templateId).Msg("✅ [DeleteTemplate] template deleted")
	return nil
}
