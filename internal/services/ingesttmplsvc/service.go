// internal/services/ingesttmplsvc/service.go
package ingesttmplsvc

import (
	"context"
	"errors"
	"strings"

	"github.com/hotkhwan/gateway-api/internal/logger"
	"github.com/hotkhwan/gateway-api/internal/repo/ingesttmplrepo"
	"github.com/hotkhwan/gateway-api/models/gmod"
	"github.com/hotkhwan/gateway-api/models/ingestmod"
	"go.mongodb.org/mongo-driver/bson"
)

// ingestTemplateRepoI is the repo subset used by IngestTemplateService.
// *ingesttmplrepo.IngestTemplateRepo satisfies this interface.
type ingestTemplateRepoI interface {
	ExistsByName(ctx context.Context, workspaceId, name, excludeID string) (bool, error)
	Insert(ctx context.Context, t *ingestmod.IngestTemplate) error
	FindByID(ctx context.Context, workspaceId, id string) (*ingestmod.IngestTemplate, error)
	List(ctx context.Context, workspaceId string, page, perPage int) ([]ingestmod.IngestTemplate, *gmod.Pagination, error)
	Update(ctx context.Context, workspaceId, id string, fields bson.M) error
	Delete(ctx context.Context, workspaceId, id string) error
}

// ============================================================
// Input types
// ============================================================

type CreateIngestTemplateInput struct {
	WorkspaceID  string
	Name         string
	SourceFamily string
	MatchRules   []ingestmod.MatchRule
	FieldMapping map[string]any
	Enabled      bool
}

type UpdateIngestTemplateInput struct {
	WorkspaceID  string
	ID           string
	Name         *string
	SourceFamily *string
	MatchRules   []ingestmod.MatchRule
	FieldMapping map[string]any
	Enabled      *bool
}

// ============================================================
// IngestTemplateService
// ============================================================

type IngestTemplateService struct {
	repo ingestTemplateRepoI
}

func NewIngestTemplateService(repo ingestTemplateRepoI) *IngestTemplateService {
	if repo == nil {
		panic("IngestTemplateService: repo required")
	}
	return &IngestTemplateService{repo: repo}
}

// ============================================================
// Create
// ============================================================

func (s *IngestTemplateService) Create(ctx context.Context, input CreateIngestTemplateInput) (*ingestmod.IngestTemplate, error) {
	log := logger.FromCtx(ctx, "ingesttmplsvc", "IngestTemplateService.Create")

	input.Name = strings.TrimSpace(input.Name)
	input.SourceFamily = strings.TrimSpace(input.SourceFamily)
	if input.WorkspaceID == "" || input.Name == "" || input.SourceFamily == "" {
		return nil, ErrBadRequest
	}

	exists, err := s.repo.ExistsByName(ctx, input.WorkspaceID, input.Name, "")
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, ErrConflict
	}

	t := &ingestmod.IngestTemplate{
		WorkspaceID:  input.WorkspaceID,
		Name:         input.Name,
		SourceFamily: input.SourceFamily,
		MatchRules:   input.MatchRules,
		FieldMapping: input.FieldMapping,
		Enabled:      input.Enabled,
	}

	if err := s.repo.Insert(ctx, t); err != nil {
		log.Error().Err(err).Msg("ingest template insert failed")
		return nil, err
	}

	log.Info().Str("id", t.ID).Str("workspaceId", input.WorkspaceID).Msg("ingest template created")
	return t, nil
}

// ============================================================
// List
// ============================================================

func (s *IngestTemplateService) List(ctx context.Context, workspaceId string, page, perPage int) ([]ingestmod.IngestTemplate, *gmod.Pagination, error) {
	if workspaceId == "" {
		return nil, nil, ErrBadRequest
	}
	return s.repo.List(ctx, workspaceId, page, perPage)
}

// ============================================================
// GetOne
// ============================================================

func (s *IngestTemplateService) GetOne(ctx context.Context, workspaceId, id string) (*ingestmod.IngestTemplate, error) {
	if workspaceId == "" || id == "" {
		return nil, ErrBadRequest
	}
	t, err := s.repo.FindByID(ctx, workspaceId, id)
	if err != nil {
		if errors.Is(err, ingesttmplrepo.ErrTemplateNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return t, nil
}

// ============================================================
// Update
// ============================================================

func (s *IngestTemplateService) Update(ctx context.Context, input UpdateIngestTemplateInput) (*ingestmod.IngestTemplate, error) {
	log := logger.FromCtx(ctx, "ingesttmplsvc", "IngestTemplateService.Update")

	if input.WorkspaceID == "" || input.ID == "" {
		return nil, ErrBadRequest
	}

	if _, err := s.repo.FindByID(ctx, input.WorkspaceID, input.ID); err != nil {
		if errors.Is(err, ingesttmplrepo.ErrTemplateNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	fields := bson.M{}

	if input.Name != nil {
		name := strings.TrimSpace(*input.Name)
		if name == "" {
			return nil, ErrBadRequest
		}
		exists, err := s.repo.ExistsByName(ctx, input.WorkspaceID, name, input.ID)
		if err != nil {
			return nil, err
		}
		if exists {
			return nil, ErrConflict
		}
		fields["name"] = name
	}
	if input.SourceFamily != nil {
		sf := strings.TrimSpace(*input.SourceFamily)
		if sf == "" {
			return nil, ErrBadRequest
		}
		fields["sourceFamily"] = sf
	}
	if input.MatchRules != nil {
		fields["matchRules"] = input.MatchRules
	}
	if input.FieldMapping != nil {
		fields["fieldMapping"] = input.FieldMapping
	}
	if input.Enabled != nil {
		fields["enabled"] = *input.Enabled
	}

	if len(fields) == 0 {
		return nil, ErrBadRequest
	}

	if err := s.repo.Update(ctx, input.WorkspaceID, input.ID, fields); err != nil {
		return nil, err
	}

	log.Info().Str("id", input.ID).Str("workspaceId", input.WorkspaceID).Msg("ingest template updated")
	return s.repo.FindByID(ctx, input.WorkspaceID, input.ID)
}

// ============================================================
// Delete
// ============================================================

func (s *IngestTemplateService) Delete(ctx context.Context, workspaceId, id string) error {
	log := logger.FromCtx(ctx, "ingesttmplsvc", "IngestTemplateService.Delete")

	if workspaceId == "" || id == "" {
		return ErrBadRequest
	}

	if _, err := s.repo.FindByID(ctx, workspaceId, id); err != nil {
		if errors.Is(err, ingesttmplrepo.ErrTemplateNotFound) {
			return ErrNotFound
		}
		return err
	}

	if err := s.repo.Delete(ctx, workspaceId, id); err != nil {
		return err
	}

	log.Info().Str("id", id).Str("workspaceId", workspaceId).Msg("ingest template deleted")
	return nil
}
