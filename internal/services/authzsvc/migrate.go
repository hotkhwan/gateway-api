// internal/services/authzsvc/migrate.go
package authzsvc

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/hotkhwan/gateway-api/config"
	"github.com/hotkhwan/gateway-api/internal/gateways/authzgw"
	"github.com/hotkhwan/gateway-api/internal/logger"
	"github.com/hotkhwan/gateway-api/utils/traceutil"
)

func ApplySchema(ctx context.Context) (string, error) {
	ctx, end, log := traceutil.StartLite(
		ctx,
		"github.com/hotkhwan/gateway-api/authzsvc",
		"authorization.ApplySchema",
		"authzsvc", "ApplySchema",
	)
	defer end()

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	schemaBytes, err := os.ReadFile("internal/services/authzsvc/schema.perm")
	if err != nil {
		log.Error().Err(err).Msg("❌ Failed to read schema.perm")
		return "", err
	}

	url := fmt.Sprintf("%s/v1/tenants/%s/schemas/write", config.PermifyBaseURL, config.PermifyTenantID)
	payload := map[string]interface{}{
		"tenant_id": config.PermifyTenantID,
		"schema":    string(schemaBytes),
	}
	log.Debug().
		Str("tenant", config.PermifyTenantID).
		Str("url", url).
		Msg("📄 Applying Permify schema (REST)")
	log.Debug().
		Str("schema", string(schemaBytes)).
		Msg("📄 Schema Content")

	data, _ := json.Marshal(payload)

	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Error().Err(err).Msg("❌ REST Schema Write Failed")
		return "", err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		log.Error().
			Int("status", resp.StatusCode).
			RawJSON("response", body).
			Msg("❌ Apply schema failed (REST)")
		return "", fmt.Errorf("status=%d resp=%s", resp.StatusCode, string(body))
	}

	var result struct {
		SchemaVersion string `json:"schema_version"`
	}
	_ = json.Unmarshal(body, &result)
	config.CurrentSchemaVersion = result.SchemaVersion

	log.Info().
		Str("schemaVersion", result.SchemaVersion).
		Msg("✅ Permify Schema Applied Successfully (REST)")
	return result.SchemaVersion, nil
}

// BootstrapPlatformAdmins grants platform-level admin to users listed in
// PLATFORM_ADMIN_USER_IDS (comma-separated). Safe to call every startup —
// Permify WriteTuples is idempotent. Non-fatal on failure.
func BootstrapPlatformAdmins(ctx context.Context) {
	raw := strings.TrimSpace(os.Getenv("PLATFORM_ADMIN_USER_IDS"))
	if raw == "" {
		return
	}

	log := logger.Boot("authzsvc", "BootstrapPlatformAdmins")

	tenantID := config.PermifyTenantID
	grpc := authzgw.NewGrpcClient()

	var tuples []map[string]interface{}
	for _, uid := range strings.Split(raw, ",") {
		uid = strings.TrimSpace(uid)
		if uid == "" {
			continue
		}
		tuples = append(tuples, map[string]interface{}{
			"entity":   map[string]interface{}{"type": "platform", "id": tenantID},
			"relation": "admin",
			"subject":  map[string]interface{}{"type": "user", "id": uid},
		})
	}

	if len(tuples) == 0 {
		return
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := grpc.WriteTuples(ctx, tenantID, tuples); err != nil {
		log.Warn().Err(err).Msg("⚠️ BootstrapPlatformAdmins: write tuples failed (non-fatal)")
		return
	}

	log.Info().Int("count", len(tuples)).Str("tenant", tenantID).Msg("✅ platform admins bootstrapped")
}

// BackfillWorkspaceTuples queries all workspaces from MongoDB and writes the
// missing Permify tuples (platform link + owner). Safe to call every startup —
// WriteTuples is idempotent. Non-fatal on failure.
func BackfillWorkspaceTuples(ctx context.Context) {
	log := logger.Boot("authzsvc", "BackfillWorkspaceTuples")
	tenantID := config.PermifyTenantID
	grpc := authzgw.NewGrpcClient()

	fetchCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	col := config.DB.Collection("workspaces")
	cursor, err := col.Find(fetchCtx, bson.M{})
	if err != nil {
		log.Warn().Err(err).Msg("⚠️ BackfillWorkspaceTuples: find workspaces failed (non-fatal)")
		return
	}
	defer cursor.Close(fetchCtx)

	var workspaces []struct {
		WorkspaceID string `bson:"workspaceId"`
		OwnerUserID string `bson:"ownerUserId"`
	}
	if err := cursor.All(fetchCtx, &workspaces); err != nil {
		log.Warn().Err(err).Msg("⚠️ BackfillWorkspaceTuples: decode failed (non-fatal)")
		return
	}

	if len(workspaces) == 0 {
		return
	}

	var tuples []map[string]interface{}
	for _, ws := range workspaces {
		if ws.WorkspaceID == "" {
			continue
		}
		tuples = append(tuples,
			map[string]interface{}{
				"entity":   map[string]interface{}{"type": "workspace", "id": ws.WorkspaceID},
				"relation": "platform",
				"subject":  map[string]interface{}{"type": "platform", "id": tenantID},
			},
		)
		if ws.OwnerUserID != "" {
			tuples = append(tuples,
				map[string]interface{}{
					"entity":   map[string]interface{}{"type": "workspace", "id": ws.WorkspaceID},
					"relation": "owner",
					"subject":  map[string]interface{}{"type": "user", "id": ws.OwnerUserID},
				},
			)
		}
	}

	writeCtx, writeCancel := context.WithTimeout(ctx, 10*time.Second)
	defer writeCancel()

	if err := grpc.WriteTuples(writeCtx, tenantID, tuples); err != nil {
		log.Warn().Err(err).Int("workspaces", len(workspaces)).Msg("⚠️ BackfillWorkspaceTuples: write tuples failed (non-fatal)")
		return
	}

	log.Info().Int("workspaces", len(workspaces)).Int("tuples", len(tuples)).Str("tenant", tenantID).Msg("✅ workspace tuples backfilled")
}

func EnsureAuthzIndexes(ctx context.Context) error {
	col := config.DB.Collection("organizations")

	_, err := col.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{
			{Key: "tenantId", Value: 1},
			{Key: "name", Value: 1},
		},
		Options: options.Index().
			SetName("ux_org_tenant_name").
			SetUnique(true),
	})
	return err
}
