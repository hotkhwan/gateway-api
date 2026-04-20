// internal/services/targetsvc/registerKlynx.go
package targetsvc

import (
	"context"

	"github.com/hotkhwan/gateway-api/internal/logger"
	"github.com/hotkhwan/gateway-api/models/authzmod"
)

// RegisterKlynxTarget creates the mode=klynx system delivery target for a workspace.
// Idempotent: if a "klynx-platform" target already exists for this workspace, returns ("", nil).
// This is a system-level gRPC call — no Permify admin check, no quota enforcement.
func (s *TargetService) RegisterKlynxTarget(ctx context.Context, workspaceId, tenantId string) (targetId string, err error) {
	log := logger.FromCtx(ctx, "targetsvc", "RegisterKlynxTarget")

	if workspaceId == "" || tenantId == "" {
		return "", ErrBadRequest
	}

	// Idempotent: skip insert if "klynx-platform" already exists in this workspace.
	exists, err := s.repo.ExistsByNameInOrg(ctx, tenantId, workspaceId, "klynx-platform", "")
	if err != nil {
		return "", err
	}
	if exists {
		log.Info().Str("workspaceId", workspaceId).Msg("targetsvc: klynx delivery target already exists (idempotent)")
		return "", nil
	}

	target := &authzmod.DeliveryTarget{
		TenantId:    tenantId,
		WorkspaceId: workspaceId,
		Name:        "klynx-platform",
		Type:        "webhook",
		Mode:        "klynx",
		Enabled:     true,
		CreatedBy:   "system",
	}
	if err := s.repo.Insert(ctx, target); err != nil {
		log.Error().Err(err).Str("workspaceId", workspaceId).Msg("targetsvc: RegisterKlynxTarget insert failed")
		return "", err
	}

	log.Info().
		Str("workspaceId", workspaceId).
		Str("targetId", target.TargetId).
		Msg("targetsvc: klynx delivery target registered")
	return target.TargetId, nil
}
