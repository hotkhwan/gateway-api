// internal/services/authzsvc/org.go
package authzsvc

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hotkhwan/gateway-api/internal/gateways/authgw"
	"github.com/hotkhwan/gateway-api/internal/gateways/authzgw"
	"github.com/hotkhwan/gateway-api/internal/repo/authzrepo"
	"github.com/hotkhwan/gateway-api/models/authzmod"
	"github.com/hotkhwan/gateway-api/utils/traceutil"
	"go.mongodb.org/mongo-driver/bson"
)

type OrganizationService struct {
	orgRepo     *authzrepo.OrgRepo
	orgUnitRepo *authzrepo.OrgUnitRepo
	authzClient authzgw.Client
	idClient    *authgw.Client
}

type OrgSummary struct {
	OrgId       string `json:"orgId"`
	TenantId    string `json:"tenantId"`
	Name        string `json:"name"`
	Description string `json:"description"`
	CreatedAt   string `json:"createdAt"`
	UpdatedAt   string `json:"updatedAt"`
}

func NewOrganizationService(
	orgRepo *authzrepo.OrgRepo,
	orgUnitRepo *authzrepo.OrgUnitRepo,
	authzClient authzgw.Client,
	idClient *authgw.Client,
) *OrganizationService {

	if orgRepo == nil || orgUnitRepo == nil || authzClient == nil {
		panic("OrganizationService dependencies required")
	}

	return &OrganizationService{
		orgRepo:     orgRepo,
		orgUnitRepo: orgUnitRepo,
		authzClient: authzClient,
		idClient:    idClient,
	}
}

func (s *OrganizationService) List(
	ctx context.Context,
	tenantId string,
	userId string,
) ([]OrgSummary, error) {

	ctx, end, log := traceutil.StartLite(
		ctx,
		"authzsvc",
		"org.list",
		"authzsvc",
		"List",
	)
	defer end()

	tenantId = strings.TrimSpace(tenantId)
	userId = strings.TrimSpace(userId)

	if tenantId == "" || userId == "" {
		return nil, ErrUnauthorized
	}

	orgIds, err := s.authzClient.LookupOrganizations(ctx, tenantId, userId)
	if err != nil {
		return nil, err
	}

	if len(orgIds) == 0 {
		return []OrgSummary{}, nil
	}

	orgs, err := s.orgRepo.FindByIds(ctx, tenantId, orgIds)
	if err != nil {
		return nil, err
	}

	result := make([]OrgSummary, 0, len(orgs))
	for _, o := range orgs {
		result = append(result, OrgSummary{
			OrgId:       o.OrgId,
			TenantId:    o.TenantId,
			Name:        o.Name,
			Description: o.Description,
			CreatedAt:   o.CreatedAt.UTC().Format(time.RFC3339Nano),
			UpdatedAt:   o.UpdatedAt.UTC().Format(time.RFC3339Nano),
		})
	}

	log.Info().Int("count", len(result)).Msg("organizations listed")
	return result, nil
}

func (s *OrganizationService) Create(
	ctx context.Context,
	tenantId,
	userId,
	name string,
	description *string,
) (string, error) {

	ctx, end, log := traceutil.StartLite(
		ctx,
		"authzsvc",
		"org.create",
		"authzsvc",
		"Create",
	)
	defer end()

	name = strings.TrimSpace(name)
	if name == "" {
		return "", ErrBadRequest
	}

	orgId := uuid.NewString()
	now := time.Now().UTC()

	desc := ""
	if description != nil {
		desc = strings.TrimSpace(*description)
	}

	org := &authzmod.Organization{
		OrgId:       orgId,
		TenantId:    tenantId,
		Name:        name,
		Description: desc,
		CreatedBy:   userId,
		CreatedAt:   now,
		UpdatedBy:   userId,
		UpdatedAt:   now,
		SyncStatus:  "pending",
	}

	if err := s.orgRepo.Insert(ctx, org); err != nil {
		return "", err
	}

	tuples := TupleFactoryOrgBootstrap(orgId, userId)

	if err := s.authzClient.WriteTuples(ctx, tenantId, tuples); err != nil {
		_ = s.orgRepo.MarkSyncError(ctx, orgId)
		return "", err
	}

	_ = s.orgRepo.MarkSyncOK(ctx, orgId)

	log.Info().Str("orgId", orgId).Msg("organization created")
	return orgId, nil
}

func (s *OrganizationService) Update(
	ctx context.Context,
	tenantId,
	userId,
	orgId,
	name string,
	description *string,
) error {

	ctx, end, log := traceutil.StartLite(
		ctx,
		"authzsvc",
		"org.update",
		"authzsvc",
		"Update",
	)
	defer end()

	name = strings.TrimSpace(name)
	if name == "" {
		return ErrBadRequest
	}

	update := bson.M{
		"$set": bson.M{
			"name":      name,
			"updatedBy": userId,
			"updatedAt": time.Now().UTC(),
		},
	}

	if description != nil {
		update["$set"].(bson.M)["description"] = strings.TrimSpace(*description)
	}

	if err := s.orgRepo.Update(ctx, orgId, update); err != nil {
		if err == authzrepo.ErrNotFound {
			return ErrNotFound
		}
		log.Error().Err(err).Str("orgId", orgId).Msg("update failed")
		return err
	}

	return nil
}

func (s *OrganizationService) Delete(
	ctx context.Context,
	tenantId,
	userId,
	orgId string,
) error {

	ctx, end, log := traceutil.StartLite(
		ctx,
		"authzsvc",
		"org.delete",
		"authzsvc",
		"Delete",
	)
	defer end()

	// 1️⃣ ensure org exists
	if _, err := s.orgRepo.FindById(ctx, orgId); err != nil {
		if err == authzrepo.ErrNotFound {
			return ErrNotFound
		}
		return err
	}

	// 2️⃣ guard: has children?
	count, err := s.orgUnitRepo.CountByOrg(ctx, tenantId, orgId)
	if err != nil {
		return err
	}

	if count > 0 {
		return ErrHasChildren
	}

	// 3️⃣ delete permify relationships
	if err := s.authzClient.DeleteOrgRelationships(ctx, tenantId, orgId); err != nil {
		return err
	}

	// 4️⃣ delete mongo
	if err := s.orgRepo.Delete(ctx, orgId); err != nil {
		return err
	}

	log.Info().Str("orgId", orgId).Msg("organization deleted")
	return nil
}
