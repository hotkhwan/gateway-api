// internal/services/authzsvc/org.go
package authzsvc

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hotkhwan/gateway-api/config"
	"github.com/hotkhwan/gateway-api/internal/gateways/authgw"
	"github.com/hotkhwan/gateway-api/internal/gateways/authzgw"
	"github.com/hotkhwan/gateway-api/internal/repo/authzrepo"
	"github.com/hotkhwan/gateway-api/models/authzmod"
	"github.com/hotkhwan/gateway-api/utils/traceutil"
	"go.mongodb.org/mongo-driver/bson"
)

// generateIngestKey สร้าง random 32-byte hex string สำหรับ HMAC signing
func generateIngestKey() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// maskSecret แสดงเฉพาะ prefix + suffix (สำหรับ UI)
func maskSecret(s string) string {
	if len(s) <= 8 {
		return s
	}
	return s[:4] + "..." + s[len(s)-4:]
}

// IngestConfigView คือ response ที่ส่งให้ FE (ไม่เปิดเผย raw key)
type IngestConfigView struct {
	IngestEndpoint     string          `json:"ingestEndpoint"`
	IngestSecretMasked string          `json:"ingestSecretMasked"`
	SignatureRequired  bool            `json:"signatureRequired"`
	RateLimit          RateLimitConfig `json:"rateLimit"`
}

type RateLimitConfig struct {
	PerSecond int `json:"perSecond"`
	Burst     int `json:"burst"`
}

const (
	defaultRateLimitPerSec = 10
	defaultRateLimitBurst  = 20
)

// resolveRateLimit ใช้ค่า default เมื่อ org เก่าที่ยังไม่มี ingestConfig ใน Mongo
func resolveRateLimit(cfg authzmod.OrgIngestConfig) RateLimitConfig {
	perSec := cfg.RateLimitPerSec
	if perSec <= 0 {
		perSec = defaultRateLimitPerSec
	}
	burst := cfg.RateLimitBurst
	if burst <= 0 {
		burst = defaultRateLimitBurst
	}
	return RateLimitConfig{PerSecond: perSec, Burst: burst}
}

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
	IsActive    bool   `json:"isActive"`
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
	activeOrgId string,
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
			IsActive:    o.OrgId == activeOrgId,
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

	ingestKey, err := generateIngestKey()
	if err != nil {
		return "", err
	}

	org := &authzmod.Organization{
		OrgId:       orgId,
		TenantId:    tenantId,
		Name:        name,
		Description: desc,
		IngestConfig: authzmod.OrgIngestConfig{
			IngestKey:         ingestKey,
			SignatureRequired: false,
			RateLimitPerSec:   defaultRateLimitPerSec,
			RateLimitBurst:    defaultRateLimitBurst,
		},
		CreatedBy:         userId,
		CreatedAt:         now,
		UpdatedBy:         userId,
		UpdatedAt:         now,
		BillingOwnerId:    userId, // NEW: set creator as billing owner
		MembershipVersion: 1,      // NEW: initialize version for race protection
		SyncStatus:        "pending",
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
	tenantId string,
	userId string,
	orgId string,
	name *string,
	description *string,
	isActive *bool,
) error {

	ctx, end, log := traceutil.StartLite(
		ctx,
		"authzsvc",
		"org.update",
		"authzsvc",
		"Update",
	)
	defer end()

	// Validate at least one field is provided
	if name == nil && description == nil && isActive == nil {
		return ErrBadRequest
	}

	update := bson.M{
		"$set": bson.M{
			"updatedBy": userId,
			"updatedAt": time.Now().UTC(),
		},
	}

	if name != nil {
		trimmed := strings.TrimSpace(*name)
		if trimmed == "" {
			return ErrBadRequest
		}
		update["$set"].(bson.M)["name"] = trimmed
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

	// Update user's active org if isActive is true
	if isActive != nil && *isActive {
		if err := s.idClient.SetUserActiveOrg(ctx, tenantId, userId, orgId); err != nil {
			log.Error().Err(err).Str("userId", userId).Str("orgId", orgId).Msg("failed to set user's active org")
			// Don't fail the update, just log the error
		}
	}

	return nil
}

// GetIngestConfig ดึง ingest config ของ org (admin only)
func (s *OrganizationService) GetIngestConfig(
	ctx context.Context,
	tenantId string,
	userId string,
	orgId string,
) (*IngestConfigView, error) {

	ctx, end, _ := traceutil.StartLite(ctx, "authzsvc", "org.getIngestConfig", "authzsvc", "GetIngestConfig")
	defer end()

	orgId = strings.TrimSpace(orgId)
	if orgId == "" {
		return nil, ErrBadRequest
	}

	// ตรวจสิทธิ์: ต้องเป็น owner หรือ admin ของ org (manage permission)
	allowed, err := s.authzClient.CheckPermissionWithSchemaVersion(
		ctx, tenantId, config.CurrentSchemaVersion,
		"organization", orgId, "manage", "user", userId,
	)
	if err != nil || !allowed {
		return nil, ErrForbidden
	}

	org, err := s.orgRepo.FindById(ctx, orgId)
	if err != nil {
		if err == authzrepo.ErrNotFound {
			return nil, ErrNotFound
		}
		return nil, err
	}

	return &IngestConfigView{
		IngestEndpoint:     fmt.Sprintf("/events/%s", orgId),
		IngestSecretMasked: maskSecret(org.IngestConfig.IngestKey),
		SignatureRequired:  org.IngestConfig.SignatureRequired,
		RateLimit:          resolveRateLimit(org.IngestConfig),
	}, nil
}

// RotateIngestSecret สร้าง ingest key ใหม่และ save ลง Mongo (admin only)
func (s *OrganizationService) RotateIngestSecret(
	ctx context.Context,
	tenantId string,
	userId string,
	orgId string,
) (*IngestConfigView, error) {

	ctx, end, log := traceutil.StartLite(ctx, "authzsvc", "org.rotateIngestSecret", "authzsvc", "RotateIngestSecret")
	defer end()

	orgId = strings.TrimSpace(orgId)
	if orgId == "" {
		return nil, ErrBadRequest
	}

	// ตรวจสิทธิ์: ต้องเป็น owner หรือ admin ของ org (manage permission)
	allowed, err := s.authzClient.CheckPermissionWithSchemaVersion(
		ctx, tenantId, config.CurrentSchemaVersion,
		"organization", orgId, "manage", "user", userId,
	)
	if err != nil || !allowed {
		return nil, ErrForbidden
	}

	newKey, err := generateIngestKey()
	if err != nil {
		return nil, err
	}

	if err := s.orgRepo.Update(ctx, orgId, bson.M{
		"$set": bson.M{
			"ingestConfig.ingestKey": newKey,
			"updatedBy":              userId,
			"updatedAt":              time.Now().UTC(),
		},
	}); err != nil {
		if err == authzrepo.ErrNotFound {
			return nil, ErrNotFound
		}
		return nil, err
	}

	log.Info().Str("orgId", orgId).Msg("ingest secret rotated")

	// ดึง rate limit ปัจจุบันจาก Mongo เพื่อ return ครบ
	org, err := s.orgRepo.FindById(ctx, orgId)
	if err != nil {
		return nil, err
	}

	return &IngestConfigView{
		IngestEndpoint:     fmt.Sprintf("/events/%s", orgId),
		IngestSecretMasked: maskSecret(newKey),
		SignatureRequired:  org.IngestConfig.SignatureRequired,
		RateLimit:          resolveRateLimit(org.IngestConfig),
	}, nil
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

// GetOrganizationByOrgId retrieves an organization by orgId
func (s *OrganizationService) GetOrganizationByOrgId(
	ctx context.Context,
	orgId string,
) (*authzmod.Organization, error) {
	orgId = strings.TrimSpace(orgId)
	if orgId == "" {
		return nil, ErrNotFound
	}
	return s.orgRepo.GetByOrgId(ctx, orgId)
}
