// config/permifygRPC.go
package config

import (
	"context"
	"os"
	"time"

	permify_payload "buf.build/gen/go/permifyco/permify/protocolbuffers/go/base/v1"
	permify_grpc "github.com/Permify/permify-go/grpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/hotkhwan/gateway-api/internal/logger"
)

var (
	PermifyClient *permify_grpc.Client
)

// ✅ write MUST use the exact schema version you want
func GetSafeSchemaVersionForWrite() string {
	if CurrentSchemaVersion == "" {
		return "latest"
	}
	return CurrentSchemaVersion
}

// ✅ check MUST use the SAME schema version as write (not always latest)
func GetSafeSchemaVersionForCheck() string {
	if CurrentSchemaVersion == "" {
		return "latest"
	}
	return CurrentSchemaVersion
}

func InitPermifygRPC() {
	log := logger.Boot("permify", "config-InitPermify")

	PermifyTenantID = os.Getenv("KEYCLOAK_REALM")
	if PermifyTenantID == "" {
		PermifyTenantID = "klynx"
	}

	endpoint := os.Getenv("PERMIFY_GRPC_URI")
	if endpoint == "" {
		endpoint = "authz-permify.iam.svc.cluster.local:3478"
	}

	client, err := permify_grpc.NewClient(
		permify_grpc.Config{Endpoint: endpoint},
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		log.Fatal().Err(err).Msg("❌ Failed to connect to Permify")
	}

	PermifyClient = client
	log.Info().Str("endpoint", endpoint).Msg("✅ Permify client initialized")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := PermifyClient.Schema.List(ctx, &permify_payload.SchemaListRequest{
		TenantId:  PermifyTenantID,
		PageSize:  1,
	})
	if err != nil || len(resp.Schemas) == 0 {
		log.Warn().Err(err).Msg("⚠️ Failed to fetch schema version")
		CurrentSchemaVersion = "latest"
	} else {
		CurrentSchemaVersion = resp.Schemas[0].Version
		log.Info().Str("schema_version", CurrentSchemaVersion).Msg("✅ Schema version initialized")
	}
}
