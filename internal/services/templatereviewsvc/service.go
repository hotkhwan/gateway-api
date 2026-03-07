// internal/services/templatereviewsvc/service.go
package templatereviewsvc

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/hotkhwan/gateway-api/internal/repo/cachereview"
	"github.com/hotkhwan/gateway-api/internal/repo/templatereviewrepo"
	"github.com/hotkhwan/gateway-api/models/gmod"
	"github.com/hotkhwan/gateway-api/models/ingestmod"
	"github.com/hotkhwan/gateway-api/utils/traceutil"
)

type TemplateReviewService struct {
	repo *templatereviewrepo.TemplateReviewRepo
}

func NewTemplateReviewService(repo *templatereviewrepo.TemplateReviewRepo) *TemplateReviewService {
	if repo == nil {
		panic("TemplateReviewService: repo required")
	}
	return &TemplateReviewService{repo: repo}
}

// CreateIfNotExists creates a review sample only if no pending review exists for this fingerprint.
// Returns the reviewId (existing or new).
func (s *TemplateReviewService) CreateIfNotExists(
	ctx context.Context,
	tenantId, orgId, sourceFamily, fingerprint string,
	samplePayload map[string]any,
	suggestedMatchFields []string,
) string {
	ctx, end, _ := traceutil.StartLite(ctx, "gateway.templatereviewsvc", "CreateIfNotExists", "templatereviewsvc", "CreateIfNotExists")
	defer end()

	// Check Redis lock first
	if cachereview.IsPending(ctx, tenantId, orgId, sourceFamily, fingerprint) {
		return "" // already pending
	}

	reviewId := uuid.NewString()
	now := time.Now().UTC()
	rev := &ingestmod.TemplateReview{
		ReviewId:             reviewId,
		TenantId:             tenantId,
		OrgId:                orgId,
		SourceFamily:         sourceFamily,
		Fingerprint:          fingerprint,
		SamplePayload:        samplePayload,
		SuggestedMatchFields: suggestedMatchFields,
		Status:               "pending",
		CreatedAt:            now,
		UpdatedAt:            now,
	}

	if err := s.repo.Insert(ctx, rev); err != nil {
		// Duplicate index hit = review already exists, not an error
		return ""
	}

	cachereview.SetPending(ctx, tenantId, orgId, sourceFamily, fingerprint, reviewId)
	return reviewId
}

func (s *TemplateReviewService) Get(ctx context.Context, tenantId, orgId, reviewId string) (*ingestmod.TemplateReview, error) {
	ctx, end, _ := traceutil.StartLite(ctx, "gateway.templatereviewsvc", "Get", "templatereviewsvc", "Get")
	defer end()

	return s.repo.FindById(ctx, tenantId, orgId, reviewId)
}

func (s *TemplateReviewService) List(ctx context.Context, tenantId, orgId string, page, perPage int) ([]*ingestmod.TemplateReview, *gmod.Pagination, error) {
	ctx, end, _ := traceutil.StartLite(ctx, "gateway.templatereviewsvc", "List", "templatereviewsvc", "List")
	defer end()

	return s.repo.List(ctx, tenantId, orgId, page, perPage)
}

func (s *TemplateReviewService) Archive(ctx context.Context, tenantId, orgId, reviewId string) error {
	ctx, end, _ := traceutil.StartLite(ctx, "gateway.templatereviewsvc", "Archive", "templatereviewsvc", "Archive")
	defer end()

	// Get the review to clear cache
	rev, err := s.repo.FindById(ctx, tenantId, orgId, reviewId)
	if err != nil {
		return err
	}

	if err := s.repo.Archive(ctx, tenantId, orgId, reviewId); err != nil {
		return err
	}

	cachereview.ClearPending(ctx, tenantId, orgId, rev.SourceFamily, rev.Fingerprint)
	return nil
}
