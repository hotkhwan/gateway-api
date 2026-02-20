// internal/gateways/authzgw/grpc.go
package authzgw

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/hotkhwan/gateway-api/config"

	permify_payload "buf.build/gen/go/permifyco/permify/protocolbuffers/go/base/v1"
)

type GrpcClient struct{}

func NewGrpcClient() *GrpcClient { return &GrpcClient{} }

// ตัวอย่าง: WriteSchema ใช้ gRPC ได้
func (g *GrpcClient) WriteSchema(ctx context.Context, tenantId string, schema string) (string, error) {
	if config.PermifyClient == nil {
		return "", fmt.Errorf("permify grpc client not initialized")
	}

	resp, err := config.PermifyClient.Schema.Write(ctx, &permify_payload.SchemaWriteRequest{
		TenantId: tenantId,
		Schema:   schema,
	})
	if err != nil {
		return "", err
	}

	return resp.SchemaVersion, nil
}

func normalizeUserSubjectId(subjectId string) string {
	return strings.TrimSpace(subjectId)
}

// func normalizeUserSubjectId(userId string) string {
// 	userId = strings.TrimSpace(userId)
// 	if userId == "" {
// 		return ""
// 	}
// 	if strings.HasPrefix(userId, "user:") {
// 		return userId
// 	}
// 	return "user:" + userId
// }

func (g *GrpcClient) WriteTuples(
	ctx context.Context,
	tenantId string,
	tuples []map[string]interface{},
) error {

	if config.PermifyClient == nil {
		return fmt.Errorf("grpc client not initialized")
	}

	// convert tuples → relationship write request
	// (คุณต้อง map เองเหมือนที่ทำกับ REST)

	return fmt.Errorf("not implemented yet")
}

func (g *GrpcClient) LookupOrganizations(ctx context.Context, tenantId string, userId string) ([]string, error) {
	subjectId := normalizeUserSubjectId(userId)

	stream, err := config.PermifyClient.Permission.LookupEntityStream(
		ctx,
		&permify_payload.PermissionLookupEntityRequest{
			TenantId: tenantId,
			Metadata: &permify_payload.PermissionLookupEntityRequestMetadata{
				SchemaVersion: config.CurrentSchemaVersion,
				Depth:         50,
			},
			EntityType: "organization",
			Permission: "view",
			Subject: &permify_payload.Subject{
				Type: "user",
				Id:   subjectId, // ✅ FIX
			},
		},
	)
	if err != nil {
		return nil, err
	}

	var ids []string
	for {
		res, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		ids = append(ids, res.EntityId)
	}

	return ids, nil
}

func (g *GrpcClient) CheckPermission(
	ctx context.Context,
	tenantId string,
	entityType string,
	entityId string,
	permission string,
	subjectType string,
	subjectId string,
) (bool, error) {

	resp, err := config.PermifyClient.Permission.Check(
		ctx,
		&permify_payload.PermissionCheckRequest{
			TenantId: tenantId,
			Metadata: &permify_payload.PermissionCheckRequestMetadata{
				SchemaVersion: config.CurrentSchemaVersion,
				Depth:         50,
			},
			Entity: &permify_payload.Entity{
				Type: entityType,
				Id:   entityId,
			},
			Permission: permission,
			Subject: &permify_payload.Subject{
				Type: subjectType,
				Id:   subjectId,
			},
		},
	)

	if err != nil {
		return false, err
	}

	return resp.Can == permify_payload.CheckResult_CHECK_RESULT_ALLOWED, nil
}

func (g *GrpcClient) DeleteEntityRelationships(
	ctx context.Context,
	tenantId string,
	entityType string,
	entityId string,
) error {

	if config.PermifyClient == nil {
		return fmt.Errorf("grpc client not initialized")
	}

	// TODO: implement real grpc relationship delete
	return fmt.Errorf("grpc delete entity relationships not implemented")
}
func (g *GrpcClient) DeleteOrgRelationships(
	ctx context.Context,
	tenantId string,
	orgId string,
) error {

	return g.DeleteEntityRelationships(ctx, tenantId, "organization", orgId)
}
