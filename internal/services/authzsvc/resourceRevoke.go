// internal/services/authzsvc/resourceRevoke.go
package authzsvc

import (
    "bytes"
    "context"
    "encoding/json"
    "fmt"
    "io"
    "net/http"
    "time"

    "klynx/config"
    "klynx/utils/traceutil"
)

// RevokeResource deletes all tuples and attributes related to a resource.
func RevokeResource(ctx context.Context, entityType string, entityIDs []string) error {
    ctx, end, log := traceutil.StartLite(
        ctx,
        "klynx/authzsvc",
        "authorization.RevokeResource",
        "authzsvc", "RevokeResource",
    )
    defer end()

    if entityType == "" || len(entityIDs) == 0 {
        return fmt.Errorf("entityType and entityIDs are required")
    }

    ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
    defer cancel()

    schemaVersion := config.CurrentSchemaVersion
    if schemaVersion == "" {
        schemaVersion = "latest"
    }

    url := fmt.Sprintf("%s/v1/tenants/%s/data/delete", config.PermifyBaseURL, config.PermifyTenantID)

    payload := map[string]interface{}{
        "metadata": map[string]interface{}{
            "schema_version": schemaVersion,
        },
        "tuple_filter": map[string]interface{}{
            "entity": map[string]interface{}{
                "type": entityType,
                "ids":  entityIDs,
            },
        },
        "attribute_filter": map[string]interface{}{
            "entity": map[string]interface{}{
                "type": entityType,
                "ids":  entityIDs,
            },
        },
    }

    data, _ := json.Marshal(payload)
    req, _ := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
    req.Header.Set("Content-Type", "application/json")

    resp, err := http.DefaultClient.Do(req)
    if err != nil {
        log.Error().Err(err).Msg("❌ REST delete call failed")
        return err
    }
    defer resp.Body.Close()

    body, _ := io.ReadAll(resp.Body)
    if resp.StatusCode != http.StatusOK {
        log.Error().
            Int("status", resp.StatusCode).
            RawJSON("response", body).
            Msg("❌ Delete failed")
        return fmt.Errorf("status=%d resp=%s", resp.StatusCode, string(body))
    }

    log.Debug().
        Int("status", resp.StatusCode).
        Str("entityType", entityType).
        Strs("entityIds", entityIDs).
        Msg("✅ Entities deleted successfully")
    return nil
}