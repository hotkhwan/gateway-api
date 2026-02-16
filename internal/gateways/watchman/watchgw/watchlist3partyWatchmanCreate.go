// internal/services/kwatsvc/createWatchlist3partyWatchman.go
package watchgw

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"time"

	"github.com/hotkhwan/gateway-api/models/kwatmod"
	"github.com/hotkhwan/gateway-api/utils/httputil"
	"github.com/hotkhwan/gateway-api/utils/traceutil"
)

func CreateWatchlist3partyWatchman(ctx context.Context, req kwatmod.WatchlistCreateRequest) (string, error) {
	ctx, end, log := traceutil.StartLite(
		ctx,
		"github.com/hotkhwan/gateway-api/watchgw",
		"watchman.CreateWatchlist3partyWatchman",
		"watchgw", "CreateWatchlist3partyWatchman",
	)
	defer end()
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	// helper สำหรับเพิ่ม field
	addField := func(field, value string) {
		if value != "" {
			_ = writer.WriteField(field, value)
		}
	}

	// ใส่ฟิลด์ form
	addField("type", req.Type)
	addField("personalType", req.PersonalType)
	addField("crimesType", req.CrimesType)
	addField("idcard", req.IdCard)
	addField("passport", req.Passport)
	addField("titlename", req.TitleName)
	addField("subTitlename", req.SubTitleName)
	addField("firstname", req.FirstName)
	addField("lastname", req.LastName)
	addField("nickname", req.NickName)
	addField("sex", req.Sex)
	addField("birthday", req.Birthday)
	addField("age", req.Age)
	addField("fatherName", req.FatherName)
	addField("fatherIdcard", req.FatherIdCard)
	addField("motherName", req.MotherName)
	addField("motherIdcard", req.MotherIdCard)
	addField("maritalStatus", req.MaritalStatus)
	addField("deathStatus", req.DeathStatus)
	addField("dateOfDeath", req.DateOfDeath)
	addField("policeRegion", req.PoliceRegion)
	addField("policeProvincial", req.PoliceProvincial)
	addField("policeStation", req.PoliceStation)
	addField("userRecorder", req.UserRecorder)
	addField("userPosition", req.UserPosition)

	// ✅ แนบไฟล์จาก memory ถ้ามี
	if len(req.PhotoFile) > 0 && req.PhotoFileName != "" {
		part, err := writer.CreateFormFile("photo", req.PhotoFileName)
		if err != nil {
			return "", err
		}
		if _, err := io.Copy(part, bytes.NewReader(req.PhotoFile)); err != nil {
			return "", err
		}
		log.Info().Str("photo", req.PhotoFileName).Msg("📎 Attached photo file")
	}

	_ = writer.Close()

	// สร้าง HTTP request
	client := &http.Client{Timeout: 30 * time.Second}

	// Form-Data แบบส่ง token เอง
	url := os.Getenv("WATCHMAN_API") + "/api_create_person.php"
	token := ""
	httpReq, err := httputil.CreateFormRequest(ctx, http.MethodPost, url, &buf, writer.FormDataContentType(), token)
	if err != nil {
		return "", err
	}
	// httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, &buf)
	// if err != nil {
	// 	return "", err
	// }
	// httpReq.Header.Set("Content-Type", writer.FormDataContentType())

	log.Info().Str("url", url).Msg("📤 Sending CreateWatchlist request to WatchmanData API")

	resp, err := client.Do(httpReq)
	if err != nil {
		log.Error().Err(err).Msg("❌ Request failed")
		return "", err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	respStr := string(bytes.TrimSpace(body))

	log.Info().
		Int("status", resp.StatusCode).
		Str("response", respStr).
		Msg("✅ Received response from WatchmanData API")

	if resp.StatusCode >= 400 {
		return respStr, fmt.Errorf("watchman API error: %s", respStr)
	}

	return respStr, nil
}
