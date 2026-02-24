// internal/services/devicesvc/deviceGroupUpdate.go
package devicesvc

import (
	"context"
	"strings"

	"github.com/hotkhwan/gateway-api/internal/logger"
)

// UpdateGroupInput — service input for updating a resource group
type UpdateGroupInput struct {
	TenantID      string
	OrgID         string
	CallerID      string
	GroupID       string
	Name          string
	Description   string
	MapVisibility string // public | private
}

// Update — แก้ name / description / mapVisibility
func (s *ResourceGroupService) Update(ctx context.Context, input UpdateGroupInput) error {
	log := logger.FromCtx(ctx, "devicesvc", "ResourceGroupService.Update")

	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" {
		return ErrInvalidArgs
	}

	if err := s.guardManageOrg(ctx, input.TenantID, input.OrgID, input.CallerID); err != nil {
		return err
	}

	// cross-org: group ต้องอยู่ใน org นี้
	if _, err := s.groupRepo.FindByIDAndOrg(ctx, input.GroupID, input.TenantID, input.OrgID); err != nil {
		return err
	}

	if err := s.groupRepo.Update(ctx, input.GroupID, input.Name, input.Description, input.MapVisibility); err != nil {
		log.Error().Err(err).Str("groupId", input.GroupID).Msg("❌ Update resource group failed")
		return err
	}

	log.Info().Str("groupId", input.GroupID).Msg("✅ ResourceGroup updated")
	return nil
}
