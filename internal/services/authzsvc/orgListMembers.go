// internal/services/authzsvc/orgListMembers.go
package authzsvc

import (
	"context"

	"github.com/hotkhwan/gateway-api/config"
	"github.com/hotkhwan/gateway-api/internal/logger"
)

type OrgMember struct {
	UserId string `json:"userId"`
	Role   string `json:"role"`
}

func (s *OrganizationService) ListMembers(
	ctx context.Context,
	tenantId string,
	orgId string,
	callerUserId string,
) ([]OrgMember, error) {

	log := logger.FromCtx(ctx, "authzsvc", "listMembers")

	// ---- validate ----
	if tenantId == "" || orgId == "" || callerUserId == "" {
		log.Warn().Msg("invalid args")
		return nil, ErrInvalidArgs
	}

	// ---- permission check ----
	allowed, err := s.authzClient.CheckPermissionWithSchemaVersion(
		ctx,
		tenantId,
		config.CurrentSchemaVersion,
		"organization",
		orgId,
		"manage",
		"user",
		callerUserId,
	)
	if err != nil {
		log.Error().Err(err).Msg("permission check failed")
		return nil, err
	}

	if !allowed {
		log.Warn().
			Str("callerUserId", callerUserId).
			Msg("forbidden list members")
		return nil, ErrForbidden
	}

	// ---- read relationships ----
	relationships, err := s.authzClient.ListEntityRelationships(
		ctx,
		tenantId,
		"organization",
		orgId,
	)
	if err != nil {
		log.Error().Err(err).Msg("read relationships failed")
		return nil, err
	}

	members := make([]OrgMember, 0)

	for _, r := range relationships {

		if r.Relation != "member" && r.Relation != "admin" {
			continue
		}

		members = append(members, OrgMember{
			UserId: r.Subject.ID,
			Role:   r.Relation,
		})
	}

	log.Info().
		Int("count", len(members)).
		Msg("list members success")

	return members, nil
}
