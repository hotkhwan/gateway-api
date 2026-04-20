// internal/gateways/authzgw/grpc.go
package authzgw

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/hotkhwan/gateway-api/config"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	permify_payload "buf.build/gen/go/permifyco/permify/protocolbuffers/go/base/v1"
)

type GrpcClient struct{}

// toStringMap รองรับทั้ง map[string]string และ map[string]interface{}
func toStringMap(v interface{}) map[string]string {
	out := map[string]string{}
	switch m := v.(type) {
	case map[string]string:
		return m
	case map[string]interface{}:
		for k, val := range m {
			out[k], _ = val.(string)
		}
	}
	return out
}

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

	if tenantId == "" {
		return fmt.Errorf("tenantId required")
	}

	if len(tuples) == 0 {
		return nil
	}

	pbTuples := make([]*permify_payload.Tuple, 0, len(tuples))
	// internal/gateways/authzgw/grpc.go — WriteTuples
	for _, t := range tuples {
		// ✅ helper รองรับทั้ง map[string]string และ map[string]interface{}
		entity := toStringMap(t["entity"])
		subject := toStringMap(t["subject"])
		relation, _ := t["relation"].(string)

		pbTuples = append(pbTuples, &permify_payload.Tuple{
			Entity: &permify_payload.Entity{
				Type: entity["type"],
				Id:   entity["id"],
			},
			Relation: relation,
			Subject: &permify_payload.Subject{
				Type: subject["type"],
				Id:   subject["id"],
			},
		})
	}
	// for _, t := range tuples {

	// 	entity := t["entity"].(map[string]interface{})
	// 	subject := t["subject"].(map[string]interface{})
	// 	relation := t["relation"].(string)

	// 	pbTuples = append(pbTuples, &permify_payload.Tuple{
	// 		Entity: &permify_payload.Entity{
	// 			Type: entity["type"].(string),
	// 			Id:   entity["id"].(string),
	// 		},
	// 		Relation: relation,
	// 		Subject: &permify_payload.Subject{
	// 			Type: subject["type"].(string),
	// 			Id:   subject["id"].(string),
	// 		},
	// 	})
	// }

	_, err := config.PermifyClient.Data.WriteRelationships(
		ctx,
		&permify_payload.RelationshipWriteRequest{
			TenantId: tenantId,
			Metadata: &permify_payload.RelationshipWriteRequestMetadata{
				SchemaVersion: "", // empty = use latest schema
			},
			Tuples: pbTuples,
		},
	)

	return err
}

// LookupWorkspaces returns all workspace IDs the user has "view" access to (any role).
func (g *GrpcClient) LookupWorkspaces(ctx context.Context, tenantId string, userId string) ([]string, error) {
	subjectId := normalizeUserSubjectId(userId)

	stream, err := config.PermifyClient.Permission.LookupEntityStream(
		ctx,
		&permify_payload.PermissionLookupEntityRequest{
			TenantId: tenantId,
			Metadata: &permify_payload.PermissionLookupEntityRequestMetadata{
				SchemaVersion: config.CurrentSchemaVersion,
				Depth:         50,
			},
			EntityType: "workspace",
			Permission: "view",
			Subject: &permify_payload.Subject{
				Type: "user",
				Id:   subjectId,
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

// ListRelationshipsBySubject reads all tuples where subject = subjectType:subjectId.
func (g *GrpcClient) ListRelationshipsBySubject(
	ctx context.Context,
	tenantId string,
	subjectType string,
	subjectId string,
) ([]Relationship, error) {
	if config.PermifyClient == nil {
		return nil, fmt.Errorf("grpc client not initialized")
	}

	resp, err := config.PermifyClient.Data.ReadRelationships(
		ctx,
		&permify_payload.RelationshipReadRequest{
			TenantId: tenantId,
			Metadata: &permify_payload.RelationshipReadRequestMetadata{
				SnapToken: "",
			},
			Filter: &permify_payload.TupleFilter{
				Subject: &permify_payload.SubjectFilter{
					Type: subjectType,
					Ids:  []string{subjectId},
				},
			},
		},
	)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return []Relationship{}, nil
		}
		return nil, err
	}

	out := make([]Relationship, 0, len(resp.Tuples))
	for _, t := range resp.Tuples {
		out = append(out, Relationship{
			Entity:   EntityRef{Type: t.Entity.Type, ID: t.Entity.Id},
			Relation: t.Relation,
			Subject:  SubjectRef{Type: t.Subject.Type, ID: t.Subject.Id},
		})
	}
	return out, nil
}

func (g *GrpcClient) ListEntityRelationships(
	ctx context.Context,
	tenantId string,
	entityType string,
	entityId string,
) ([]*permify_payload.Tuple, error) {

	if config.PermifyClient == nil {
		return nil, fmt.Errorf("grpc client not initialized")
	}

	resp, err := config.PermifyClient.Data.ReadRelationships(
		ctx,
		&permify_payload.RelationshipReadRequest{
			TenantId: tenantId,
			// ✅ SnapToken เท่านั้น — ไม่มี SchemaVersion/Depth ใน Metadata นี้
			Metadata: &permify_payload.RelationshipReadRequestMetadata{
				SnapToken: "",
			},
			Filter: &permify_payload.TupleFilter{
				Entity: &permify_payload.EntityFilter{
					Type: entityType,
					Ids:  []string{entityId},
				},
			},
		},
	)

	if err != nil {
		if status.Code(err) == codes.NotFound {
			return []*permify_payload.Tuple{}, nil
		}
		return nil, err
	}

	return resp.Tuples, nil
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
				SchemaVersion: "", // empty = use latest schema in Permify
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

func (g *GrpcClient) DeleteSpecificTuple(
	ctx context.Context,
	tenantId, entityType, entityId, relation, subjectType, subjectId string,
) error {
	if config.PermifyClient == nil {
		return fmt.Errorf("grpc client not initialized")
	}

	_, err := config.PermifyClient.Data.Delete(
		ctx,
		&permify_payload.DataDeleteRequest{
			TenantId: tenantId,
			// ❌ ไม่มี Metadata field
			TupleFilter: &permify_payload.TupleFilter{
				Entity: &permify_payload.EntityFilter{
					Type: entityType,
					Ids:  []string{entityId},
				},
				Relation: relation,
				Subject: &permify_payload.SubjectFilter{
					Type: subjectType,
					Ids:  []string{subjectId}, // ✅ Ids ไม่ใช่ Id
				},
			},
		},
	)
	return err
}
