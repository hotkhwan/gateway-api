// internal/services/sourceprofilesvc/service.go
package sourceprofilesvc

import (
	"context"
	"time"

	"github.com/hotkhwan/gateway-api/internal/repo/cachesourceprofile"
	"github.com/hotkhwan/gateway-api/internal/repo/sourceprofilerepo"
	"github.com/hotkhwan/gateway-api/models/ingestmod"
	"github.com/hotkhwan/gateway-api/utils/traceutil"
	"go.mongodb.org/mongo-driver/bson"
)

type SourceProfileService struct {
	repo *sourceprofilerepo.SourceProfileRepo
}

func NewSourceProfileService(repo *sourceprofilerepo.SourceProfileRepo) *SourceProfileService {
	if repo == nil {
		panic("SourceProfileService: repo required")
	}
	return &SourceProfileService{repo: repo}
}

func (s *SourceProfileService) Create(ctx context.Context, p *ingestmod.SourceProfile) error {
	ctx, end, _ := traceutil.StartLite(ctx, "gateway.sourceprofilesvc", "Create", "sourceprofilesvc", "Create")
	defer end()

	now := time.Now().UTC()
	p.CreatedAt = now
	p.UpdatedAt = now
	return s.repo.Insert(ctx, p)
}

func (s *SourceProfileService) Get(ctx context.Context, sourceFamily string) (*ingestmod.SourceProfile, error) {
	ctx, end, _ := traceutil.StartLite(ctx, "gateway.sourceprofilesvc", "Get", "sourceprofilesvc", "Get")
	defer end()

	if cached := cachesourceprofile.Get(ctx, sourceFamily); cached != nil {
		return cached, nil
	}
	p, err := s.repo.FindByFamily(ctx, sourceFamily)
	if err != nil {
		return nil, err
	}
	cachesourceprofile.Set(ctx, p)
	return p, nil
}

func (s *SourceProfileService) List(ctx context.Context) ([]*ingestmod.SourceProfile, error) {
	ctx, end, _ := traceutil.StartLite(ctx, "gateway.sourceprofilesvc", "List", "sourceprofilesvc", "List")
	defer end()

	return s.repo.List(ctx)
}

func (s *SourceProfileService) Update(ctx context.Context, sourceFamily string, update bson.M) error {
	ctx, end, _ := traceutil.StartLite(ctx, "gateway.sourceprofilesvc", "Update", "sourceprofilesvc", "Update")
	defer end()

	update["updatedAt"] = time.Now().UTC()
	if err := s.repo.Update(ctx, sourceFamily, update); err != nil {
		return err
	}
	cachesourceprofile.Invalidate(ctx, sourceFamily)
	return nil
}

func (s *SourceProfileService) Delete(ctx context.Context, sourceFamily string) error {
	ctx, end, _ := traceutil.StartLite(ctx, "gateway.sourceprofilesvc", "Delete", "sourceprofilesvc", "Delete")
	defer end()

	if err := s.repo.Delete(ctx, sourceFamily); err != nil {
		return err
	}
	cachesourceprofile.Invalidate(ctx, sourceFamily)
	return nil
}
