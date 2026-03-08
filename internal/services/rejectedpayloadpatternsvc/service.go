// internal/services/rejectedpayloadpatternsvc/service.go
package rejectedpayloadpatternsvc

import (
	"context"

	"github.com/hotkhwan/gateway-api/internal/repo/cacherejected"
	"github.com/hotkhwan/gateway-api/internal/repo/rejectedpayloadpatternrepo"
	"github.com/hotkhwan/gateway-api/models/gmod"
	"github.com/hotkhwan/gateway-api/models/ingestmod"
	"github.com/hotkhwan/gateway-api/utils/traceutil"
)

type RejectedPayloadPatternService struct {
	repo *rejectedpayloadpatternrepo.RejectedPayloadPatternRepo
}

func NewRejectedPayloadPatternService(repo *rejectedpayloadpatternrepo.RejectedPayloadPatternRepo) *RejectedPayloadPatternService {
	if repo == nil {
		panic("RejectedPayloadPatternService: repo required")
	}
	return &RejectedPayloadPatternService{repo: repo}
}

// IsRejected returns true if the given fingerprint has been rejected for this org+sourceFamily.
// Hot-path: Redis cache first, then DB fallback.
func (s *RejectedPayloadPatternService) IsRejected(ctx context.Context, orgId, sourceFamily, fingerprint string) bool {
	ctx, end, _ := traceutil.StartLite(ctx, "gateway.rejectedpayloadpatternsvc", "IsRejected", "rejectedpayloadpatternsvc", "IsRejected")
	defer end()

	// Hot-path: Redis cache
	if cacherejected.IsRejected(ctx, orgId, sourceFamily, fingerprint) {
		return true
	}

	// DB fallback (warm cache on miss)
	exists, err := s.repo.Exists(ctx, orgId, sourceFamily, fingerprint)
	if err != nil || !exists {
		return false
	}
	cacherejected.SetRejected(ctx, orgId, sourceFamily, fingerprint)
	return true
}

// Create creates a new rejected pattern. Idempotent — returns existing patternId if already exists.
func (s *RejectedPayloadPatternService) Create(
	ctx context.Context,
	orgId, sourceFamily, fingerprint, reason, createdBy string,
) (string, error) {
	ctx, end, _ := traceutil.StartLite(ctx, "gateway.rejectedpayloadpatternsvc", "Create", "rejectedpayloadpatternsvc", "Create")
	defer end()

	patternId, err := s.repo.Create(ctx, orgId, sourceFamily, fingerprint, reason, createdBy)
	if err != nil {
		if err == rejectedpayloadpatternrepo.ErrAlreadyExists {
			return "", ErrAlreadyExists
		}
		return "", err
	}

	cacherejected.SetRejected(ctx, orgId, sourceFamily, fingerprint)
	return patternId, nil
}

// List returns paginated rejected patterns for an org.
func (s *RejectedPayloadPatternService) List(ctx context.Context, orgId string, page, perPage int) ([]*ingestmod.RejectedPayloadPattern, *gmod.Pagination, error) {
	ctx, end, _ := traceutil.StartLite(ctx, "gateway.rejectedpayloadpatternsvc", "List", "rejectedpayloadpatternsvc", "List")
	defer end()

	return s.repo.List(ctx, orgId, page, perPage)
}

// Delete removes a rejected pattern and clears its cache entry.
func (s *RejectedPayloadPatternService) Delete(ctx context.Context, orgId, patternId string) error {
	ctx, end, _ := traceutil.StartLite(ctx, "gateway.rejectedpayloadpatternsvc", "Delete", "rejectedpayloadpatternsvc", "Delete")
	defer end()

	pattern, err := s.repo.FindById(ctx, orgId, patternId)
	if err != nil {
		return ErrNotFound
	}

	if err := s.repo.Delete(ctx, orgId, patternId); err != nil {
		return err
	}

	cacherejected.ClearRejected(ctx, orgId, pattern.SourceFamily, pattern.Fingerprint)
	return nil
}
