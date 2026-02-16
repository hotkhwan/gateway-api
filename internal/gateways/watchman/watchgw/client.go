// internal/gateways/watchman/watchgw/client.go
package watchgw

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"github.com/hotkhwan/gateway-api/internal/logger"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

type Client struct {
	BaseURL string
	Token   string // optional
}

func NewFromEnv(prefix string) *Client {
	return &Client{
		BaseURL: os.Getenv(prefix + "_API"),
		Token:   os.Getenv(prefix + "_TOKEN"),
	}
}

func (c *Client) Configured() bool { return c.BaseURL != "" }

// -------------------- Lookup by idcard --------------------

func (c *Client) LookupIDByIDCard(ctx context.Context, idcard string) (int64, int, error) {
	log := logger.FromCtx(ctx, "watchgw", "LookupIDByIDCard")
	if !c.Configured() {
		return 0, 0, fmt.Errorf("watchman not configured")
	}
	idcard = strings.TrimSpace(idcard)
	u := c.BaseURL + "/api_get_person.php?findby=idcard&id=" + url.QueryEscape(idcard)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return 0, 0, fmt.Errorf("build request: %w", err)
	}
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return 0, 0, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	log.Info(). // ✅ เรียกผ่านตัวแปร
			Str("endpoint", "/api_get_person.php").
			Str("findby", "idcard").
			Str("idcard", idcard).
			Int("status", resp.StatusCode).
			Str("resp", crop(body, 600)).
			Msg("🔎 watchman lookup result")
	if resp.StatusCode != http.StatusOK {
		return 0, resp.StatusCode, fmt.Errorf("watchman lookup status=%d body=%s", resp.StatusCode, string(body))
	}

	var out getByIDCardResp
	if err := json.Unmarshal(body, &out); err != nil {
		return 0, resp.StatusCode, fmt.Errorf("unmarshal response: %w", err)
	}

	// ไม่เจอ: status=false หรือ details ว่าง/id<=0
	if !out.Status || out.Details == nil || out.Details.ID == "" {
		return 0, resp.StatusCode, ErrNotFound
	}
	id, _ := out.Details.ID.Int64()
	if id <= 0 {
		return 0, resp.StatusCode, ErrNotFound
	}
	return id, resp.StatusCode, nil
}

// -------------------- multipart helper --------------------

func (c *Client) doMultipart(ctx context.Context, endpoint string, fields map[string]string, photoName string, photo []byte) (int, []byte, error) {
	log := logger.FromCtx(ctx, "watchgw", "doMultipart") // ✅
	if !c.Configured() {
		return 0, nil, fmt.Errorf("watchman not configured")
	}

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)

	for k, v := range fields {
		if strings.TrimSpace(v) == "" {
			continue
		}
		_ = w.WriteField(k, v)
	}

	if len(photo) > 0 {
		fn := filepath.Base(photoName)
		if ext := strings.ToLower(filepath.Ext(fn)); ext == "" {
			fn += ".jpg" // บังคับนามสกุลสื่อสารง่าย ๆ
		}
		part, err := w.CreateFormFile("photo", fn)
		if err != nil {
			return 0, nil, fmt.Errorf("create file field: %w", err)
		}
		if _, err = part.Write(photo); err != nil {
			return 0, nil, fmt.Errorf("write photo: %w", err)
		}
	}

	if err := w.Close(); err != nil {
		return 0, nil, fmt.Errorf("close writer: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+endpoint, &buf)
	if err != nil {
		return 0, nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}

	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()

	rb, _ := io.ReadAll(resp.Body)

	// แปลง fields เป็นสตริงสั้นๆ เพื่อ log (ซ่อนค่าใหญ่/ว่าง)
	safe := make([]string, 0, len(fields))
	for k, v := range fields {
		vv := strings.TrimSpace(v)
		if vv == "" {
			continue
		}
		if len(vv) > 64 {
			vv = vv[:64] + "...(cut)"
		}
		safe = append(safe, k+"="+vv)
	}
	sort.Strings(safe)

	log.Info().
		Str("endpoint", endpoint).
		Int("status", resp.StatusCode).
		Str("photoName", filepath.Base(photoName)).
		Int("photoBytes", len(photo)).
		Str("fields", strings.Join(safe, "&")). // 👈 เห็นชัด ๆ
		Str("resp", crop(rb, 800)).
		Msg("📤 watchman multipart response")

	return resp.StatusCode, rb, nil
}

// -------------------- Create / Update --------------------

func (c *Client) CreatePerson(ctx context.Context, fields map[string]string, photoName string, photo []byte) (int, []byte, error) {
	return c.doMultipart(ctx, "/api_create_person.php", fields, photoName, photo)
}

func (c *Client) UpdatePerson(ctx context.Context, id string, fields map[string]string, photoName string, photo []byte) (int, []byte, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return 0, nil, fmt.Errorf("missing id")
	}
	ff := make(map[string]string, len(fields)+1)
	for k, v := range fields {
		ff[k] = v
	}
	ff["id"] = id
	return c.doMultipart(ctx, "/api_update_person.php", ff, photoName, photo)
}

// -------------------- UpsertThenEnsureID --------------------
// Flow: lookup → found=update → return id
//
//	not found → create → (ถ้า status=true และได้ id ใน body ใช้เลย; ไม่งั้น retry lookup backoff)
func (c *Client) UpsertThenEnsureID(ctx context.Context, fields map[string]string, photoName string, photo []byte) (int64, int, error) {
	idcard := strings.TrimSpace(fields["idcard"])
	if idcard == "" {
		return 0, 0, fmt.Errorf("idcard is empty")
	}

	// Fix สำคัญ: ถ้า type=2 ให้ลบ crimesType* ออกเพื่อลดโอกาส Invalid parameters
	ff := copyFields(fields)
	if strings.TrimSpace(ff["type"]) == "2" {
		// ถ้ามี field code ซ้ำซ้อนค่อยลบทิ้ง แต่ "crimesType" ต้องคงไว้
		delete(ff, "crimesTypeCode")
		delete(ff, "crimestypecode")
	}

	// 1) lookup ก่อน
	if wmID, st, lerr := c.LookupIDByIDCard(ctx, idcard); lerr == nil && wmID > 0 {
		stU, bodyU, uerr := c.UpdatePerson(ctx, strconv.FormatInt(wmID, 10), ff, photoName, photo)
		if uerr != nil || stU >= 400 {
			if stU == 401 || stU == 403 {
				return 0, stU, ErrUnauthorized
			}
			if stU >= 500 {
				return 0, stU, ErrServer
			}
			return 0, stU, fmt.Errorf("update failed: %v body=%s", uerr, string(bodyU))
		}
		return wmID, stU, nil
	} else if lerr != nil && !errors.Is(lerr, ErrNotFound) {
		return 0, st, lerr
	}

	// 2) ไม่พบ → create
	stC, bodyC, cerr := c.CreatePerson(ctx, ff, photoName, photo)
	if cerr != nil || stC >= 400 {
		if stC == 401 || stC == 403 {
			return 0, stC, ErrUnauthorized
		}
		if stC >= 500 {
			return 0, stC, ErrServer
		}
		return 0, stC, cerr
	}

	// พยายามอ่าน status/id จาก body
	var cr watchmanResp
	if json.Unmarshal(bodyC, &cr) == nil && cr.Status {
		if cr.Details != nil && cr.Details.ID != "" {
			if id, _ := cr.Details.ID.Int64(); id > 0 {
				return id, stC, nil
			}
		}
	}

	// lookup ซ้ำ (eventual consistency)
	for _, d := range []time.Duration{250 * time.Millisecond, 500 * time.Millisecond, 1 * time.Second, 2 * time.Second, 4 * time.Second} {
		select {
		case <-ctx.Done():
			return 0, stC, ctx.Err()
		case <-time.After(d):
		}
		if wmID, st2, gerr := c.LookupIDByIDCard(ctx, idcard); gerr == nil && wmID > 0 {
			return wmID, st2, nil
		}
	}
	return 0, stC, fmt.Errorf("created but lookup failed: watchman not ready for idcard=%s", idcard)
}

// -------------------- Delete --------------------
func (c *Client) DeleteByID(ctx context.Context, id string) (int, []byte, error) {
	log := logger.FromCtx(ctx, "watchgw", "DeleteByID")
	if !c.Configured() {
		return 0, nil, fmt.Errorf("watchman not configured")
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return 0, nil, fmt.Errorf("id empty")
	}

	// ✅ ใช้เส้นเดียว: HTTP DELETE ?idcard=<id>
	st, rb, err := c.doDeleteWithQuery(ctx, "/api_delete_person.php", id)

	// log ให้เห็นชัดว่าใช้เส้น DELETE (ไม่ใช้ multipart แล้ว)
	log.Debug().
		Str("endpoint", "/api_delete_person.php").
		Str("method", "DELETE").
		Str("idcard", id).
		Int("status", st).
		Str("resp", crop(rb, 800)).
		Msg("🗑 watchman delete by idcard")

	// นอร์มัลไลซ์ผลลัพธ์แบบ idempotent
	if err != nil {
		return st, rb, err
	}
	if !isDeleteSuccess(st, rb) {
		// คง rb ไว้ช่วย debug ได้
		return st, rb, fmt.Errorf("watchman delete not successful")
	}
	return st, rb, nil
}

// func (c *Client) DeleteByID(ctx context.Context, id string) (int, []byte, error) {
// 	log := logger.FromCtx(ctx, "watchgw", "DeleteByIDCard")
// 	if !c.Configured() {
// 		return 0, nil, fmt.Errorf("watchman not configured")
// 	}
// 	id = strings.TrimSpace(id)
// 	if id == "" {
// 		return 0, nil, fmt.Errorf("id empty")
// 	}

// 	// วิธีหลัก: POST multipart
// 	fields := map[string]string{"id": id}
// 	st, rb, err := c.doMultipart(ctx, "/api_delete_person.php", fields, "", nil)
// 	if err != nil || st >= 400 {
// 		return st, rb, err
// 	}

// 	// ถ้า body JSON แล้ว status=true ถือว่าสำเร็จ
// 	var out watchmanResp
// 	if json.Unmarshal(rb, &out) == nil && out.Status {
// 		return st, rb, nil
// 	}

// 	// fallback: HTTP DELETE ?id=...
// 	st2, rb2, err2 := c.doDeleteWithQuery(ctx, "/api_delete_person.php", id)
// 	if err2 != nil || st2 >= 400 {
// 		if len(rb2) > 0 {
// 			return st2, rb2, err2
// 		}
// 		return st, rb, fmt.Errorf("watchman delete failed")
// 	}
// 	log.Info(). // ✅
// 			Str("endpoint", "/api_delete_person.php").
// 			Str("method", "DELETE").
// 			Str("idcard", id).
// 			Str("resp", crop(rb, 800)).
// 			Msg("🗑 watchman delete by idcard")
// 	return st2, rb2, nil
// }

func (c *Client) doDeleteWithQuery(ctx context.Context, endpoint string, id string) (int, []byte, error) {
	u := c.BaseURL + endpoint + "?idcard=" + url.QueryEscape(strings.TrimSpace(id))
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, u, nil)
	if err != nil {
		return 0, nil, fmt.Errorf("build request: %w", err)
	}
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, body, nil
}

// -------------------- Delete --------------------

// isDeleteSuccess: นอร์มัลไลซ์ผลลบจาก Watchman ให้เป็น idempotent success
// - 2xx + "status":true → success
// - 2xx + ("person not found" | "not found") → ถือว่า success (ลบซ้ำ/ไม่มีอยู่แล้ว)
func isDeleteSuccess(status int, body []byte) bool {
	if status >= 400 {
		return false
	}
	low := bytes.ToLower(body)
	if bytes.Contains(low, []byte(`"status":true`)) {
		return true
	}
	if bytes.Contains(low, []byte("person not found")) || bytes.Contains(low, []byte("not found")) {
		return true
	}
	return false
}

// -------------------- small util --------------------

func copyFields(in map[string]string) map[string]string {
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func crop(rb []byte, n int) string {
	if n <= 0 {
		n = 800
	}
	s := string(rb)
	if len(s) > n {
		return s[:n] + "...(truncated)"
	}
	return s
}
