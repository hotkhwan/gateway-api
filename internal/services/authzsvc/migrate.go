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
	"go.mongodb.org/mongo-driver/mongo/options"

	"klynx/config"
	"klynx/utils/traceutil"
)

func ApplySchema(ctx context.Context) (string, error) {
	ctx, end, log := traceutil.StartLite(
		ctx,
		"klynx/authzsvc",
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

func InitialSyncRelationships(ctx context.Context, schemaVersion string) error {
	ctx, end, log := traceutil.StartLite(
		ctx,
		"klynx/authzsvc",
		"authorization.InitialSyncRelationships",
		"authzsvc", "InitialSyncRelationships",
	)
	defer end()

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	db := config.MongoClient.Database(os.Getenv("MONGO_DB"))

	// ใช้ gCtx สำหรับ Mongo find แยกจาก ctx ของ HTTP
	gCtx, gCancel := context.WithTimeout(ctx, 10*time.Second)
	defer gCancel()

	log.Debug().Msg("🔄 Syncing Groups & Sensors to Permify (REST)")

	groupsCol := db.Collection("groups")
	cursor, err := groupsCol.Find(gCtx, bson.M{}, options.Find())
	if err != nil {
		return err
	}
	defer cursor.Close(gCtx)

	for cursor.Next(gCtx) {
		var group struct {
			ID       string   `bson:"_id"`
			ParentID string   `bson:"parentId"`
			Members  []string `bson:"members"`
		}
		if err := cursor.Decode(&group); err != nil {
			continue
		}

		if group.ParentID != "" {
			tuples := []map[string]interface{}{
				{
					"entity":   map[string]string{"type": "group", "id": group.ID},
					"relation": "parent",
					"subject":  map[string]string{"type": "group", "id": group.ParentID},
				},
			}
			if err := writeDataREST(ctx, tuples, nil); err != nil {
				log.Error().Err(err).Str("groupId", group.ID).Msg("❌ Parent relation error (REST)")
			}
		}

		for _, memberID := range group.Members {
			if err := GrantUserToGroup(ctx, group.ID, memberID); err != nil {
				log.Error().Err(err).Str("groupId", group.ID).Str("userId", memberID).Msg("❌ Member relation error (REST)")
			}
		}
	}

	sensorCol := db.Collection("sensors")
	sCtx, sCancel := context.WithTimeout(ctx, 10*time.Second)
	defer sCancel()

	sCursor, _ := sensorCol.Find(sCtx, bson.M{}, options.Find())
	defer sCursor.Close(sCtx)

	for sCursor.Next(sCtx) {
		var sensor struct {
			ID      string `bson:"_id"`
			GroupID string `bson:"groupId"`
		}
		if err := sCursor.Decode(&sensor); err != nil {
			continue
		}

		tuples := []map[string]interface{}{
			{
				"entity":   map[string]string{"type": "sensor_device", "id": sensor.ID},
				"relation": "located_in_group",
				"subject":  map[string]string{"type": "group", "id": sensor.GroupID},
			},
		}
		if err := writeDataREST(ctx, tuples, nil); err != nil {
			log.Error().Err(err).Str("sensorId", sensor.ID).Msg("❌ Sensor relation error (REST)")
		}
	}

	log.Info().Msg("✅ Relationships Synced Successfully (REST)")
	return nil
}
