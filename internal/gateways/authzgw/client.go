// internal/gateways/authzgw/client.go
package authzgw

import "context"

type Client interface {
	WriteTuples(ctx context.Context, tenantId string, tuples []map[string]any) error
	DeleteOrgTuples(ctx context.Context, tenantId string, orgId string) error
}

type HybridClient struct {
	rest *RestClient
	grpc *GrpcClient
}

func NewClient() *HybridClient {
	return &HybridClient{
		rest: NewRestClient(),
		grpc: NewGrpcClient(),
	}
}

func (c *HybridClient) WriteTuples(ctx context.Context, tenantId string, tuples []map[string]any) error {
	// today you still write via REST for schemaVersion metadata simplicity
	return c.rest.WriteTuples(ctx, tenantId, tuples)
}

func (c *HybridClient) DeleteOrgTuples(ctx context.Context, tenantId string, orgId string) error {
	return c.rest.DeleteOrgTuples(ctx, tenantId, orgId)
}

func (c *HybridClient) LookupOrganizations(ctx context.Context, tenantId string, userId string) ([]string, error) {
	ids, err := c.grpc.LookupOrganizations(ctx, tenantId, userId)
	if err == nil {
		return ids, nil
	}
	return c.rest.LookupOrganizations(ctx, tenantId, userId)
}

func (c *HybridClient) CheckPermission(
	ctx context.Context,
	tenantId string,
	entityType string,
	entityId string,
	permission string,
	subjectType string,
	subjectId string,
) (bool, error) {

	if c.grpc != nil {
		if ok, err := c.grpc.CheckPermission(ctx, tenantId, entityType, entityId, permission, subjectType, subjectId); err == nil {
			return ok, nil
		}
	}
	return c.rest.CheckPermission(ctx, tenantId, entityType, entityId, permission, subjectType, subjectId)
}
