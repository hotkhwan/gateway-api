// internal/services/kwatsvc/Watchlist3partyWatchmanUpdate.go
package watchgw

import (
	"bytes"
	"context"
	"fmt"
	"klynx/utils/traceutil"
	"mime/multipart"
	"net/http"
	"os"
	"time"
)

// Update (multipart) ไปยัง Watchman
// ใช้ WATCHMAN_API เป็น base, เช่น: https://aliza.kudsonmoo.co/WatchmanData
func Watchlist3partyWatchmanUpdate(
	ctx context.Context,
	fields map[string]string, // ต้องมี "id"
	photoName string,
	photoData []byte,
) (string, int, error) {
	ctx, end, log := traceutil.StartLite(
		ctx,
		"klynx/watchgw",
		"watchman.Watchlist3partyWatchmanUpdate",
		"watchgw", "Watchlist3partyWatchmanUpdate",
	)
	defer end()
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	base := os.Getenv("WATCHMAN_API")
	url := fmt.Sprintf("%s/api_update_person.php", base)

	var body bytes.Buffer
	w := multipart.NewWriter(&body)

	// เขียน fields (ข้ามค่าว่าง)
	for k, v := range fields {
		if v == "" {
			continue
		}
		if err := w.WriteField(k, v); err != nil {
			return "", 0, err
		}
	}
	// แนบรูปถ้ามี
	if len(photoData) > 0 && photoName != "" {
		fw, err := w.CreateFormFile("photo", photoName)
		if err != nil {
			return "", 0, err
		}
		if _, err := fw.Write(photoData); err != nil {
			return "", 0, err
		}
	}
	if err := w.Close(); err != nil {
		return "", 0, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, &body)
	if err != nil {
		return "", 0, err
	}
	req.Header.Set("Content-Type", w.FormDataContentType())

	client := &http.Client{Timeout: 20 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		return "", 0, err
	}
	defer res.Body.Close()

	buf := new(bytes.Buffer)
	if _, err := buf.ReadFrom(res.Body); err != nil {
		return "", res.StatusCode, err
	}

	resp := buf.String()
	log.Debug().Int("status", res.StatusCode).Msg("Watchman update response")

	return resp, res.StatusCode, nil
}
