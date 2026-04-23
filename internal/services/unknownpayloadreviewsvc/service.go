// internal/services/unknownpayloadreviewsvc/service.go
package unknownpayloadreviewsvc

import (
	"context"

	"github.com/google/uuid"
	"github.com/hotkhwan/gateway-api/internal/repo/cachereview"
	"github.com/hotkhwan/gateway-api/internal/repo/unknownpayloadreviewrepo"
	"github.com/hotkhwan/gateway-api/models/gmod"
	"github.com/hotkhwan/gateway-api/models/ingestmod"
	"github.com/hotkhwan/gateway-api/utils/traceutil"
)

type UnknownPayloadReviewService struct {
	repo *unknownpayloadreviewrepo.UnknownPayloadReviewRepo
}

func NewUnknownPayloadReviewService(repo *unknownpayloadreviewrepo.UnknownPayloadReviewRepo) *UnknownPayloadReviewService {
	if repo == nil {
		panic("UnknownPayloadReviewService: repo required")
	}
	return &UnknownPayloadReviewService{repo: repo}
}

// UpsertIfNotExists creates a review on first occurrence or updates seenCount+lastSeenAt on repeat.
// Returns the reviewId (new or existing). Returns "" if already cached as pending (no DB hit).
func (s *UnknownPayloadReviewService) UpsertIfNotExists(
	ctx context.Context,
	orgId, sourceFamily, fingerprint string,
	samplePayload map[string]any,
	candidateSuggestionIds []string,
) string {
	ctx, end, _ := traceutil.StartLite(ctx, "gateway.unknownpayloadreviewsvc", "UpsertIfNotExists", "unknownpayloadreviewsvc", "UpsertIfNotExists")
	defer end()

	// Hot-path: skip DB if already pending in cache
	if cachereview.IsPending(ctx, orgId, sourceFamily, fingerprint) {
		return ""
	}

	reviewId := uuid.NewString()
	id, err := s.repo.Upsert(ctx, reviewId, orgId, sourceFamily, fingerprint, samplePayload, candidateSuggestionIds)
	if err != nil {
		return ""
	}

	cachereview.SetPending(ctx, orgId, sourceFamily, fingerprint, id)
	return id
}

func (s *UnknownPayloadReviewService) Get(ctx context.Context, orgId, reviewId string) (*ingestmod.UnknownPayloadReview, error) {
	ctx, end, _ := traceutil.StartLite(ctx, "gateway.unknownpayloadreviewsvc", "Get", "unknownpayloadreviewsvc", "Get")
	defer end()

	return s.repo.FindById(ctx, orgId, reviewId)
}

func (s *UnknownPayloadReviewService) List(
	ctx context.Context,
	orgId string,
	f unknownpayloadreviewrepo.ListFilter,
	page, perPage int,
) ([]*ingestmod.UnknownPayloadReview, *gmod.Pagination, error) {
	ctx, end, _ := traceutil.StartLite(ctx, "gateway.unknownpayloadreviewsvc", "List", "unknownpayloadreviewsvc", "List")
	defer end()

	return s.repo.List(ctx, orgId, f, page, perPage)
}

// Reject marks a review as "rejected" and clears its cache entry.
// Caller is responsible for creating rejectedPayloadPattern (Step 4).
func (s *UnknownPayloadReviewService) Reject(ctx context.Context, orgId, reviewId string) (*ingestmod.UnknownPayloadReview, error) {
	ctx, end, _ := traceutil.StartLite(ctx, "gateway.unknownpayloadreviewsvc", "Reject", "unknownpayloadreviewsvc", "Reject")
	defer end()

	rev, err := s.repo.FindById(ctx, orgId, reviewId)
	if err != nil {
		return nil, err
	}

	if err := s.repo.MarkRejected(ctx, orgId, reviewId); err != nil {
		return nil, err
	}

	cachereview.ClearPending(ctx, orgId, rev.SourceFamily, rev.Fingerprint)
	return rev, nil
}

// Delete hard-deletes a review (called after template is created successfully).
func (s *UnknownPayloadReviewService) Delete(ctx context.Context, orgId, reviewId string) error {
	ctx, end, _ := traceutil.StartLite(ctx, "gateway.unknownpayloadreviewsvc", "Delete", "unknownpayloadreviewsvc", "Delete")
	defer end()

	rev, err := s.repo.FindById(ctx, orgId, reviewId)
	if err != nil {
		return err
	}

	if err := s.repo.Delete(ctx, orgId, reviewId); err != nil {
		return err
	}

	cachereview.ClearPending(ctx, orgId, rev.SourceFamily, rev.Fingerprint)
	return nil
}
