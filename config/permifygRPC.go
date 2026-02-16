// config/permifygRPC.go
package config

import (
	"context"
	"os"
	"sort"
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

func GetSafeSchemaVersionForWrite() string {
	if CurrentSchemaVersion == "" {
		return "latest" // ครั้งแรกหลัง start
	}
	return CurrentSchemaVersion
}

func GetSafeSchemaVersionForCheck() string {
	return "latest"
}

// func GetSafeSchemaVersion() string {
// 	if CurrentSchemaVersion == "" {
// 		return "latest"
// 	}
// 	return CurrentSchemaVersion
// }

func InitPermifygRPC() {
	log := logger.Boot("permify", "config-InitPermify")

	PermifyTenantID = os.Getenv("KEYCLOAK_REALM")
	if PermifyTenantID == "" {
		PermifyTenantID = "klynx"
	}

	endpoint := os.Getenv("PERMIFY_URI")
	client, err := permify_grpc.NewClient(
		permify_grpc.Config{Endpoint: endpoint},
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		log.Fatal().Err(err).Msg("❌ Failed to connect to Permify")
	}

	PermifyClient = client
	log.Info().Str("endpoint", endpoint).Msg("✅ Permify client initialized")

	// ✅ ดึง Schema Version ล่าสุด (Async เพื่อ HA/High TPS)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		resp, err := PermifyClient.Schema.List(ctx, &permify_payload.SchemaListRequest{
			TenantId: PermifyTenantID,
			PageSize: 10,
		})
		if err != nil || resp == nil || len(resp.Schemas) == 0 {
			log.Warn().Err(err).Msg("⚠️ Failed to list schema versions, fallback to latest")
			CurrentSchemaVersion = "" // fallback ใช้ latest
			return
		}

		// ✅ แปลง CreatedAt (string → time.Time) แล้ว sort
		sort.Slice(resp.Schemas, func(i, j int) bool {
			t1, _ := time.Parse(time.RFC3339, resp.Schemas[i].CreatedAt)
			t2, _ := time.Parse(time.RFC3339, resp.Schemas[j].CreatedAt)
			return t1.After(t2)
		})

		CurrentSchemaVersion = resp.Schemas[0].Version
		log.Info().
			Str("schema_version", CurrentSchemaVersion).
			Msg("✅ Latest schema version fetched successfully")
	}()
}
