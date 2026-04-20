// internal/services/msgtmplsvc/service.go
package msgtmplsvc

import (
	"context"
	"errors"
	"strings"

	"github.com/hotkhwan/gateway-api/internal/logger"
	"github.com/hotkhwan/gateway-api/internal/repo/msgtmplrepo"
	"github.com/hotkhwan/gateway-api/models/gmod"
	"github.com/hotkhwan/gateway-api/models/ingestmod"
	"go.mongodb.org/mongo-driver/bson"
)

// msgTemplateRepoI is the repo subset used by MsgTemplateService.
// *msgtmplrepo.MsgTemplateRepo satisfies this interface.
type msgTemplateRepoI interface {
	ExistsByName(ctx context.Context, workspaceId, name, excludeID string) (bool, error)
	Insert(ctx context.Context, t *ingestmod.WorkspaceMessageTemplate) error
	FindByID(ctx context.Context, workspaceId, id string) (*ingestmod.WorkspaceMessageTemplate, error)
	List(ctx context.Context, workspaceId string, page, perPage int) ([]ingestmod.WorkspaceMessageTemplate, *gmod.Pagination, error)
	Update(ctx context.Context, workspaceId, id string, fields bson.M) error
	Delete(ctx context.Context, workspaceId, id string) error
}

// validChannels lists accepted channel types.
var validChannels = map[string]bool{
	"line":     true,
	"webhook":  true,
	"telegram": true,
	"discord":  true,
}

// ============================================================
// Input types
// ============================================================

type CreateMsgTemplateInput struct {
	WorkspaceID string
	Name        string
	Channel     string // line|webhook|telegram|discord
	Body        string
	Locale      string
}

type UpdateMsgTemplateInput struct {
	WorkspaceID string
	ID          string
	Name        *string
	Channel     *string
	Body        *string
	Locale      *string
}

// ============================================================
// MsgTemplateService
// ============================================================

type MsgTemplateService struct {
	repo msgTemplateRepoI
}

func NewMsgTemplateService(repo msgTemplateRepoI) *MsgTemplateService {
	if repo == nil {
		panic("MsgTemplateService: repo required")
	}
	return &MsgTemplateService{repo: repo}
}

// ============================================================
// Create
// ============================================================

func (s *MsgTemplateService) Create(ctx context.Context, input CreateMsgTemplateInput) (*ingestmod.WorkspaceMessageTemplate, error) {
	log := logger.FromCtx(ctx, "msgtmplsvc", "MsgTemplateService.Create")

	input.Name = strings.TrimSpace(input.Name)
	input.Channel = strings.TrimSpace(input.Channel)
	if input.WorkspaceID == "" || input.Name == "" {
		return nil, ErrBadRequest
	}
	if input.Channel != "" && !validChannels[input.Channel] {
		return nil, ErrBadRequest
	}

	exists, err := s.repo.ExistsByName(ctx, input.WorkspaceID, input.Name, "")
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, ErrConflict
	}

	t := &ingestmod.WorkspaceMessageTemplate{
		WorkspaceID: input.WorkspaceID,
		Name:        input.Name,
		Channel:     input.Channel,
		Body:        input.Body,
		Locale:      input.Locale,
	}

	if err := s.repo.Insert(ctx, t); err != nil {
		log.Error().Err(err).Msg("msg template insert failed")
		return nil, err
	}

	log.Info().Str("id", t.ID).Str("workspaceId", input.WorkspaceID).Msg("message template created")
	return t, nil
}

// ============================================================
// List
// ============================================================

func (s *MsgTemplateService) List(ctx context.Context, workspaceId string, page, perPage int) ([]ingestmod.WorkspaceMessageTemplate, *gmod.Pagination, error) {
	if workspaceId == "" {
		return nil, nil, ErrBadRequest
	}
	return s.repo.List(ctx, workspaceId, page, perPage)
}

// ============================================================
// GetOne
// ============================================================

func (s *MsgTemplateService) GetOne(ctx context.Context, workspaceId, id string) (*ingestmod.WorkspaceMessageTemplate, error) {
	if workspaceId == "" || id == "" {
		return nil, ErrBadRequest
	}
	t, err := s.repo.FindByID(ctx, workspaceId, id)
	if err != nil {
		if errors.Is(err, msgtmplrepo.ErrMsgTemplateNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return t, nil
}

// ============================================================
// Update
// ============================================================

func (s *MsgTemplateService) Update(ctx context.Context, input UpdateMsgTemplateInput) (*ingestmod.WorkspaceMessageTemplate, error) {
	log := logger.FromCtx(ctx, "msgtmplsvc", "MsgTemplateService.Update")

	if input.WorkspaceID == "" || input.ID == "" {
		return nil, ErrBadRequest
	}

	if _, err := s.repo.FindByID(ctx, input.WorkspaceID, input.ID); err != nil {
		if errors.Is(err, msgtmplrepo.ErrMsgTemplateNotFound) {
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
	if input.Channel != nil {
		ch := strings.TrimSpace(*input.Channel)
		if ch != "" && !validChannels[ch] {
			return nil, ErrBadRequest
		}
		fields["channel"] = ch
	}
	if input.Body != nil {
		fields["body"] = *input.Body
	}
	if input.Locale != nil {
		fields["locale"] = *input.Locale
	}

	if len(fields) == 0 {
		return nil, ErrBadRequest
	}

	if err := s.repo.Update(ctx, input.WorkspaceID, input.ID, fields); err != nil {
		return nil, err
	}

	log.Info().Str("id", input.ID).Str("workspaceId", input.WorkspaceID).Msg("message template updated")
	return s.repo.FindByID(ctx, input.WorkspaceID, input.ID)
}

// ============================================================
// Delete
// ============================================================

func (s *MsgTemplateService) Delete(ctx context.Context, workspaceId, id string) error {
	log := logger.FromCtx(ctx, "msgtmplsvc", "MsgTemplateService.Delete")

	if workspaceId == "" || id == "" {
		return ErrBadRequest
	}

	if _, err := s.repo.FindByID(ctx, workspaceId, id); err != nil {
		if errors.Is(err, msgtmplrepo.ErrMsgTemplateNotFound) {
			return ErrNotFound
		}
		return err
	}

	if err := s.repo.Delete(ctx, workspaceId, id); err != nil {
		return err
	}

	log.Info().Str("id", id).Str("workspaceId", workspaceId).Msg("message template deleted")
	return nil
}
