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
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/hotkhwan/gateway-api/config"
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
