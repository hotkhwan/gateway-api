// internal/services/authzsvc/authzDebugService.go
package authzsvc

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/hotkhwan/gateway-api/config"
	"github.com/hotkhwan/gateway-api/internal/gateways/authzgw"
)

// ✅ DI struct — แยกออกมาจาก authzDebug.go
type AuthzDebugService struct {
	authzClient authzgw.Client
}

func NewAuthzDebugService(authzClient authzgw.Client) *AuthzDebugService {
	return &AuthzDebugService{authzClient: authzClient}
}

func (s *AuthzDebugService) ListPermifyTuples(ctx context.Context, tenantId string, filter PermifyTupleFilter) (*PermifyTupleListResult, error) {
	return ListPermifyTuples(ctx, tenantId, filter)
}

func (s *AuthzDebugService) FactoryResetPermifyTuples(ctx context.Context, tenantId string, entityType string) (int, error) {
	return FactoryResetPermifyTuples(ctx, tenantId, entityType)
}

func (s *AuthzDebugService) ProveUserOrgsAgainstMongo(ctx context.Context, tenantId string, userId string) (*ProveUserOrgsResult, error) {
	return ProveUserOrgsAgainstMongo(ctx, tenantId, userId)
}

func (s *AuthzDebugService) ResetPermifyTuplesAll(ctx context.Context, tenantId string, entityType string) (*ResetAllTuplesResult, error) {
	return ResetPermifyTuplesAll(ctx, tenantId, entityType)
}

func (s *AuthzDebugService) ResetPermifyTuplesByUser(ctx context.Context, tenantId string, userId string) (*ResetUserTuplesResult, error) {
	return ResetPermifyTuplesByUser(ctx, tenantId, userId)
}

func (s *AuthzDebugService) ListTuplesByUser(ctx context.Context, tenantId string, userId string) ([]PermifyTupleRow, error) {
	return ListTuplesByUser(ctx, tenantId, userId)
}

func (s *AuthzDebugService) ListTuplesByEntityType(ctx context.Context, tenantId string, entityType string) ([]PermifyTupleRow, error) {
	return ListTuplesByEntityType(ctx, tenantId, entityType)
}

func (s *AuthzDebugService) ResetTenant(ctx context.Context, tenantId string) (*ResetTenantResult, error) {
	return ResetTenant(ctx, tenantId)
}

func (s *AuthzDebugService) FactoryOrgBootstrap(
	ctx context.Context,
	tenantId string,
	orgId string,
	userId string,
) (*FactoryBootstrapResult, error) {

	if tenantId == "" || orgId == "" || userId == "" {
		return nil, fmt.Errorf("invalid args")
	}
	if config.CurrentSchemaVersion == "" {
		return nil, fmt.Errorf("schema version not initialized")
	}

	userId = strings.TrimSpace(userId)

	tuples := []map[string]interface{}{
		{
			"entity":   map[string]interface{}{"type": "organization", "id": orgId},
			"relation": "admin",
			"subject":  map[string]interface{}{"type": "user", "id": userId},
		},
		{
			"entity":   map[string]interface{}{"type": "organization", "id": orgId},
			"relation": "member",
			"subject":  map[string]interface{}{"type": "user", "id": userId},
		},
	}

	if err := s.authzClient.WriteTuples(ctx, tenantId, tuples); err != nil {
		return nil, err
	}

	return &FactoryBootstrapResult{
		OrgId:         orgId,
		TupleCount:    len(tuples),
		SchemaVersion: config.CurrentSchemaVersion,
	}, nil
}

// internal/services/authzsvc/authzDebugService.go

var allEntityTypes = []string{
	"organization", "orgUnit", "deviceGroup", "device", "menu", "serviceAccount",
}

func (s *AuthzDebugService) ListAllTuples(ctx context.Context, tenantId string) ([]PermifyTupleRow, error) {
	if tenantId == "" {
		tenantId = config.PermifyTenantID
	}

	out := []PermifyTupleRow{}
	for _, et := range allEntityTypes {
		rows, err := ListTuplesByEntityType(ctx, tenantId, et)
		if err != nil {
			continue // skip entityType ที่ error ไม่ fail ทั้งหมด
		}
		out = append(out, rows...)
	}
	return out, nil
}

func (s *AuthzDebugService) ListTuplesByOrgId(ctx context.Context, tenantId string, orgId string) ([]PermifyTupleRow, error) {
	if tenantId == "" {
		tenantId = config.PermifyTenantID
	}

	client := authzgw.NewPermifyRestDebugClient()
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	out := []PermifyTupleRow{}
	continuousToken := ""

	for {
		readRes, err := client.ReadTuples(ctx, tenantId, authzgw.ReadTuplesRequest{
			EntityType:      "organization",
			EntityId:        orgId,
			PageSize:        200,
			ContinuousToken: continuousToken,
			SchemaVersion:   "latest",
		})
		if err != nil {
			return nil, err
		}
		for _, t := range readRes.Tuples {
			out = append(out, PermifyTupleRow{
				EntityType: t.Entity.Type, EntityId: t.Entity.Id,
				Relation:    t.Relation,
				SubjectType: t.Subject.Type, SubjectId: t.Subject.Id,
			})
		}
		if readRes.ContinuousToken == "" || readRes.ContinuousToken == continuousToken {
			break
		}
		continuousToken = readRes.ContinuousToken
	}
	return out, nil
}

func (s *AuthzDebugService) ClearOrgTuples(ctx context.Context, tenantId string, orgId string) error {
	client := authzgw.NewPermifyRestDebugClient()
	return client.DeleteOrgTuples(ctx, tenantId, orgId)
}
