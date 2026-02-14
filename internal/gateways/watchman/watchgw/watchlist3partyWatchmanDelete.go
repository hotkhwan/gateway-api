// internal/services/kwatsvc/Watchlist3partyWatchmanDelete.go
package watchgw

import (
	"context"
	"fmt"
	"io"
	"klynx/utils/traceutil"
	"net/http"
	"os"
	"time"
)

func Watchlist3partyWatchmanDelete(ctx context.Context, id string) (string, error) {
	ctx, end, log := traceutil.StartLite(
		ctx,
		"klynx/watchgw",
		"watchman.Watchlist3partyWatchmanDelete",
		"watchgw", "Watchlist3partyWatchmanDelete",
	)
	defer end()
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	url := os.Getenv("WATCHMAN_API") + "/api_delete_person.php?id=" + id

	client := &http.Client{Timeout: 15 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, url, nil)
	if err != nil {
		return "", err
	}

	log.Info().Str("url", url).Msg("🗑 Sending Watchlist3partyWatchmanDelete request to WatchmanData API")

	resp, err := client.Do(req)
	if err != nil {
		log.Error().Err(err).Msg("❌ Request failed")
		return "", err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	respStr := string(body)

	log.Info().Int("status", resp.StatusCode).Str("response", respStr).Msg("✅ Received response from WatchmanData API")

	if resp.StatusCode >= 400 {
		return respStr, fmt.Errorf("watchman API delete error: %s", respStr)
	}
	return respStr, nil
}
