// internal/gateways/authzgw/grpc.go
package authzgw

import (
	"context"
	"fmt"

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