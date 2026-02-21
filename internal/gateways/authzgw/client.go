// internal/gateways/authzgw/client.go
package authzgw

import (
	"context"

	"github.com/hotkhwan/gateway-api/internal/logger"
)

type Client interface {
	WriteTuples(ctx context.Context, tenantId string, tuples []map[string]interface{}) error
	DeleteOrgRelationships(ctx context.Context, tenantId string, orgId string) error
	DeleteEntityRelationships(ctx context.Context, tenantId string, entityType string, entityId string) error
	// Deprecated: debug only (test env) — delete all tuples under organization entityId via read+delete
	DeleteOrgTuples(ctx context.Context, tenantId string, orgId string) error

	// Permission
	CheckPermissionWithSchemaVersion(
		ctx context.Context,
		tenantId string,
		schemaVersion string,
		entityType string,
		entityId string,
		permission string,
		subjectType string,
		subjectId string,
	) (bool, error)

	LookupOrganizations(ctx context.Context, tenantId string, userId string) ([]string, error)

	ListEntityRelationships(
		ctx context.Context,
		tenantId string,
		entityType string,
		entityId string,
	) ([]Relationship, error)

	// internal/gateways/authzgw/client.go — interface
	DeleteSpecificTupleWithRelation(ctx context.Context, tenantId, entityType, entityId, relation, subjectType, subjectId string) error
}

type HybridClient struct {
	rest      *RestClient
	grpc      *GrpcClient
	restDebug *permifyRestDebugClient
}

func NewClient() Client {
	return &HybridClient{
		rest:      NewRestClient(),
		grpc:      NewGrpcClient(),
		restDebug: NewPermifyRestDebugClient(),
	}
}

func (c *HybridClient) WriteTuples(
	ctx context.Context,
	tenantId string,
	tuples []map[string]interface{},
) error {

	if c.grpc != nil {
		if err := c.grpc.WriteTuples(ctx, tenantId, tuples); err == nil {
			return nil
		}
	}

	if c.rest != nil {
		return c.rest.WriteTuples(ctx, tenantId, tuples)
	}

	return ErrNoAuthzClient
}
func (c *HybridClient) LookupOrganizations(
	ctx context.Context,
	tenantId string,
	userId string,
) ([]string, error) {

	if c.grpc != nil {
		ids, err := c.grpc.LookupOrganizations(ctx, tenantId, userId)
		if err == nil {
			return ids, nil
		}
	}

	if c.rest != nil {
		return c.rest.LookupOrganizations(ctx, tenantId, userId)
	}

	return nil, ErrNoAuthzClient
}

func (c *HybridClient) DeleteOrgRelationships(
	ctx context.Context,
	tenantId string,
	orgId string,
) error {

	if c.grpc != nil {
		if err := c.grpc.DeleteOrgRelationships(ctx, tenantId, orgId); err == nil {
			return nil
		}
	}

	if c.rest != nil {
		return c.rest.DeleteOrgRelationships(ctx, tenantId, orgId)
	}

	return ErrNoAuthzClient
}

//	func (c *HybridClient) DeleteOrgRelationships(ctx context.Context, tenantId string, orgId string) error {
//		return c.rest.DeleteOrgRelationships(ctx, tenantId, orgId)
//	}
func (c *HybridClient) DeleteEntityRelationships(
	ctx context.Context,
	tenantId string,
	entityType string,
	entityId string,
) error {

	if c.grpc != nil {
		if err := c.grpc.DeleteEntityRelationships(ctx, tenantId, entityType, entityId); err == nil {
			return nil
		}
	}

	if c.rest != nil {
		return c.rest.DeleteEntityRelationships(ctx, tenantId, entityType, entityId)
	}

	return ErrNoAuthzClient
}

// func (c *HybridClient) DeleteEntityRelationships(
// 	ctx context.Context,
// 	tenantId string,
// 	entityType string,
// 	entityId string,
// ) error {

// 	if c.grpc != nil {
// 		if err := c.grpc.DeleteEntityRelationships(ctx, tenantId, entityType, entityId); err == nil {
// 			return nil
// 		}
// 	}

// 	if c.rest != nil {
// 		return c.rest.DeleteEntityRelationships(ctx, tenantId, entityType, entityId)
// 	}

// 	return ErrNoAuthzClient
// }

// Deprecated: debug only
func (c *HybridClient) DeleteOrgTuples(
	ctx context.Context,
	tenantId string,
	orgId string,
) error {

	if c.restDebug != nil {
		return c.restDebug.DeleteOrgTuples(ctx, tenantId, orgId)
	}

	return nil
}

// func (c *HybridClient) DeleteOrgTuples(ctx context.Context, tenantId string, orgId string) error {
// 	if c.restDebug == nil {
// 		return nil
// 	}
// 	return c.restDebug.DeleteOrgTuples(ctx, tenantId, orgId)
// }

func (c *HybridClient) CheckPermissionWithSchemaVersion(
	ctx context.Context,
	tenantId string,
	schemaVersion string,
	entityType string,
	entityId string,
	permission string,
	subjectType string,
	subjectId string,
) (bool, error) {

	if c.grpc != nil {
		allowed, err := c.grpc.CheckPermission(
			ctx,
			tenantId,
			entityType,
			entityId,
			permission,
			subjectType,
			subjectId,
		)
		if err == nil {
			return allowed, nil
		}
	}

	if c.rest != nil {
		return c.rest.CheckPermissionWithSchemaVersion(
			ctx,
			tenantId,
			schemaVersion,
			entityType,
			entityId,
			permission,
			subjectType,
			subjectId,
		)
	}

	return false, ErrNoAuthzClient
}

// client.go — HybridClient.ListEntityRelationships
func (c *HybridClient) ListEntityRelationships(
	ctx context.Context,
	tenantId string,
	entityType string,
	entityId string,
) ([]Relationship, error) {

	// gRPC first
	log := logger.FromCtx(ctx, "authzsvc", "listMembers")
	if c.grpc != nil {
		tuples, err := c.grpc.ListEntityRelationships(ctx, tenantId, entityType, entityId)
		if err == nil {
			// ✅ เพิ่ม log ตรงนี้

			log.Info().
				Str("path", "grpc").
				Int("count", len(tuples)).
				Msg("list entity relationships")

			out := make([]Relationship, 0, len(tuples))
			for _, t := range tuples {
				out = append(out, Relationship{
					Entity:   EntityRef{Type: t.Entity.Type, ID: t.Entity.Id},
					Relation: t.Relation,
					Subject:  SubjectRef{Type: t.Subject.Type, ID: t.Subject.Id},
				})
			}
			return out, nil
		}

		// ✅ เพิ่ม log ตรงนี้ — รู้ว่า gRPC fail ด้วยอะไร
		log.Info().
			Err(err).
			Str("path", "grpc→rest fallback").
			Msg("grpc failed, falling back to rest")
	}

	if c.rest != nil {
		return c.rest.ListEntityRelationships(ctx, tenantId, entityType, entityId)
	}

	return nil, ErrNoAuthzClient
}

// internal/gateways/authzgw/client.go — HybridClient
func (c *HybridClient) DeleteSpecificTupleWithRelation(
	ctx context.Context,
	tenantId, entityType, entityId, relation, subjectType, subjectId string,
) error {
	if c.rest != nil {
		return c.rest.DeleteSpecificTupleWithRelation(ctx, tenantId, entityType, entityId, relation, subjectType, subjectId)
	}
	return ErrNoAuthzClient
}
