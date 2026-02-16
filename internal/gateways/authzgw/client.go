// internal/gateways/authzgw/client.go
package authzgw

import "context"

type Client interface {
	WriteTuples(ctx context.Context, tenantId string, tuples []map[string]interface{}) error
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

func (c *HybridClient) WriteTuples(ctx context.Context, tenantId string, tuples []map[string]interface{}) error {
	// ✅ write path ใช้ REST (ครบ/ชัวร์)
	return c.rest.WriteTuples(ctx, tenantId, tuples)
}
