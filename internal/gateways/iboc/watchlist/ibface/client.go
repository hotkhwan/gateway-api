// internal/gateways/iboc/watchlist/ibface/client.go
package ibface

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"klynx/internal/gateways/gwcom"
)

// NOTE: ถ้า Client/Errors ประกาศใน types.go แล้ว ให้ลบบล็อกนี้ทิ้ง
type Client struct {
	BaseURL string
	Token   string
}

func NewFromEnv(prefix string) *Client {
	return &Client{
		BaseURL: os.Getenv(prefix + "_API"),
		Token:   os.Getenv(prefix + "_TOKEN"),
	}
}
func (c *Client) Configured() bool { return c.BaseURL != "" && c.Token != "" }

func sniffMime(b []byte) string {
	if len(b) >= 8 && string(b[:8]) == "\x89PNG\r\n\x1a\n" {
		return "image/png"
	}
	if len(b) >= 3 && b[0] == 0xFF && b[1] == 0xD8 && b[2] == 0xFF {
		return "image/jpeg"
	}
	return "image/jpeg"
}

// ------- AnalyzeCrop: ส่ง original image → analytics, คืนรูปครอป + mime -------
func (c *Client) AnalyzeCrop(ctx context.Context, img []byte) ([]byte, string, error) {
	if !c.Configured() {
		return nil, "", fmt.Errorf("iboc not configured")
	}

	req := map[string]any{
		"images": []map[string]string{
			{"image": base64.StdEncoding.EncodeToString(img)},
		},
	}
	body, _ := json.Marshal(req)

	b, status, e := gwcom.PostJSON(
		ctx,
		c.BaseURL+"/analytics/representation/face-reid",
		map[string]string{
			"Authorization": "Bearer " + c.Token,
			"Accept":        "application/json",
		},
		bytes.NewReader(body),
	)
	if e != nil {
		return nil, "", e
	}
	switch {
	case status == 401 || status == 403:
		return nil, "", ErrUnauthorized
	case status >= 500:
		return nil, "", ErrServer
	}

	var ar AnalyzeResp
	if err := json.Unmarshal(b, &ar); err != nil {
		s := string(b)
		if len(s) > 512 {
			s = s[:512]
		}
		return nil, "", fmt.Errorf("bad json: %w (body: %s)", err, s)
	}
	if len(ar.Images) == 0 || len(ar.Images[0].Boxes.Image) == 0 {
		return nil, "", ErrNoFaceDetected
	}

	var firstImageBase64 string
	for _, v := range ar.Images[0].Boxes.Image {
		firstImageBase64 = v.Image
		break
	}
	if firstImageBase64 == "" {
		return nil, "", ErrNoFaceDetected
	}

	data, err := base64.StdEncoding.DecodeString(firstImageBase64)
	if err != nil {
		return nil, "", fmt.Errorf("decode cropped image: %w", err)
	}
	return data, sniffMime(data), nil
}

// ------- EnsurePerson: POST /mgmt/person -------
func (c *Client) EnsurePerson(ctx context.Context,
	firstName, lastName, idcard,
	alertType, alertDesc, crimesType, policeStation string,
) (string, error) {
	if !c.Configured() {
		return "", fmt.Errorf("iboc not configured")
	}

	payload := map[string]any{
		"firstName":     firstName,
		"lastName":      lastName,
		"identityDocId": idcard,
		"organization":  policeStation,
		"enabled":       true,
	}

	// tags ตามสเปก: เป็น []string
	var tags []string
	if s := strings.TrimSpace(alertType); s != "" {
		tags = append(tags, s)
	}
	// if s := strings.TrimSpace(crimesType); s != "" {
	// 	tags = append(tags, s)
	// }
	if len(tags) > 0 {
		payload["tags"] = tags
	}

	// metadata: เก็บรายละเอียดเสริม (desc + ตำรวจ)
	meta := map[string]any{}
	if s := strings.TrimSpace(alertDesc); s != "" {
		meta[crimesType] = s
	}
	// if s := strings.TrimSpace(policeRegion); s != "" {
	// 	meta["policeRegion"] = s
	// }
	// if s := strings.TrimSpace(policeProvincial); s != "" {
	// 	meta["policeProvincial"] = s
	// }
	// if s := strings.TrimSpace(policeStation); s != "" {
	// 	meta["policeStation"] = s
	// }
	if len(meta) > 0 {
		payload["metadata"] = meta
	}

	b, _ := json.Marshal(payload)
	rb, status, err := gwcom.PostJSON(
		ctx, c.BaseURL+"/mgmt/person",
		map[string]string{
			"Authorization": "Bearer " + c.Token,
			"Accept":        "application/json",
		},
		bytes.NewReader(b),
	)
	if err != nil {
		return "", err
	}
	switch {
	case status == 401 || status == 403:
		return "", ErrUnauthorized
	case status == 409:
		return "", ErrDuplicate
	case status >= 500:
		return "", ErrServer
	case status >= 400:
		s := string(rb)
		if len(s) > 600 {
			s = s[:600] + "...(truncated)"
		}
		return "", fmt.Errorf("iboc create person: http %d, body=%s", status, s)
	}

	var resp struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(rb, &resp); err != nil || resp.ID == "" {
		return "", fmt.Errorf("parse person resp: %w", err)
	}
	return resp.ID, nil
}

// AttachFaceFromOriginal: วิเคราะห์จาก "รูปต้นฉบับ" เพื่อเอา cropped + representation
func (c *Client) AttachFaceFromOriginal(ctx context.Context, personID string, original []byte, idemKey string) (string, error) {
	if !c.Configured() {
		return "", fmt.Errorf("iboc not configured")
	}
	if personID == "" {
		return "", fmt.Errorf("personID is empty")
	}

	origB64 := base64.StdEncoding.EncodeToString(original)
	model, dim, vec, cropB64, status, err := c.analyzeForRep(ctx, origB64)
	if err != nil || cropB64 == "" || len(vec) == 0 {
		return "", fmt.Errorf("analyzeForRep failed (status %d): %v", status, err)
	}

	req := map[string]any{
		"image": cropB64,
		"representation": map[string]any{
			"model":     model,
			"dimension": dim,
			"vector":    vec,
		},
	}
	body, _ := json.Marshal(req)

	rb, status2, err2 := gwcom.PostJSON(
		ctx,
		c.BaseURL+"/mgmt/person/"+personID+"/face",
		map[string]string{
			"Authorization":   "Bearer " + c.Token,
			"Accept":          "application/json",
			"Idempotency-Key": idemKey,
		},
		bytes.NewReader(body),
	)
	if err2 != nil {
		return "", err2
	}
	switch {
	case status2 == 401 || status2 == 403:
		return "", ErrUnauthorized
	case status2 >= 500:
		return "", ErrServer
	}
	if id := extractFaceID(rb); id != "" {
		return id, nil
	}
	snippet := string(rb)
	if len(snippet) > 300 {
		snippet = snippet[:300] + "...(truncated)"
	}
	return "", fmt.Errorf("iboc: empty face id (status %d, body: %s)", status2, snippet)
}

// ------- AttachFace: POST /mgmt/person/{personId}/face (คำนวณ representation ภายใน) -------
func (c *Client) AttachFace(ctx context.Context, personID string, cropped []byte, mime string, idemKey string) (string, error) {
	if !c.Configured() {
		return "", fmt.Errorf("iboc not configured")
	}
	if personID == "" {
		return "", fmt.Errorf("personID is empty")
	}
	imgB64 := base64.StdEncoding.EncodeToString(cropped)

	payload := map[string]any{"image": imgB64}
	body, _ := json.Marshal(payload)

	rb, status, err := gwcom.PostJSON(
		ctx,
		c.BaseURL+"/mgmt/person/"+personID+"/face",
		map[string]string{
			"Authorization":   "Bearer " + c.Token,
			"Accept":          "application/json",
			"Idempotency-Key": idemKey,
		},
		bytes.NewReader(body),
	)
	if err != nil {
		return "", err
	}
	switch {
	case status == 401 || status == 403:
		return "", ErrUnauthorized
	case status >= 500:
		return "", ErrServer
	}

	if id := extractFaceID(rb); id != "" {
		return id, nil
	}

	model, dim, vec, cropB64, _, aerr := c.analyzeForRep(ctx, imgB64)
	if aerr != nil || cropB64 == "" || len(vec) == 0 {
		snippet := string(rb)
		if len(snippet) > 300 {
			snippet = snippet[:300] + "...(truncated)"
		}
		return "", fmt.Errorf("iboc: empty face id (status %d, body: %s)", status, snippet)
	}

	req2 := map[string]any{
		"image": cropB64,
		"representation": map[string]any{
			"model":     model,
			"dimension": dim,
			"vector":    vec,
		},
	}
	b2, _ := json.Marshal(req2)

	rb2, status2, err2 := gwcom.PostJSON(
		ctx,
		c.BaseURL+"/mgmt/person/"+personID+"/face",
		map[string]string{
			"Authorization":   "Bearer " + c.Token,
			"Accept":          "application/json",
			"Idempotency-Key": idemKey + "-rep",
		},
		bytes.NewReader(b2),
	)
	if err2 != nil {
		return "", err2
	}
	switch {
	case status2 == 401 || status2 == 403:
		return "", ErrUnauthorized
	case status2 >= 500:
		return "", ErrServer
	}
	if id := extractFaceID(rb2); id != "" {
		return id, nil
	}
	snippet := string(rb2)
	if len(snippet) > 300 {
		snippet = snippet[:300] + "...(truncated)"
	}
	return "", fmt.Errorf("iboc: empty face id (status %d, body: %s)", status2, snippet)
}

// ------- helper: รองรับหลาย schema ของ response -------
func extractFaceID(rb []byte) string {
	var p struct{ ID, FaceID string }
	if json.Unmarshal(rb, &p) == nil {
		if p.ID != "" {
			return p.ID
		}
		if p.FaceID != "" {
			return p.FaceID
		}
	}
	var d struct {
		Data struct{ ID, FaceID string } `json:"data"`
	}
	if json.Unmarshal(rb, &d) == nil {
		if d.Data.ID != "" {
			return d.Data.ID
		}
		if d.Data.FaceID != "" {
			return d.Data.FaceID
		}
	}
	var r struct {
		Result struct{ ID, FaceID string } `json:"result"`
	}
	if json.Unmarshal(rb, &r) == nil {
		if r.Result.ID != "" {
			return r.Result.ID
		}
		if r.Result.FaceID != "" {
			return r.Result.FaceID
		}
	}
	var s string
	if json.Unmarshal(rb, &s) == nil && s != "" {
		return s
	}
	return ""
}

// ------- helper: วิเคราะห์ representation (ทนทานทั้ง array|map) -------
func (c *Client) analyzeForRep(ctx context.Context, imgBase64 string) (model string, dim int, vec []float64, cropped string, status int, err error) {
	req := map[string]any{"images": []map[string]string{{"image": imgBase64}}}
	body, _ := json.Marshal(req)

	rb, status, e := gwcom.PostJSON(
		ctx,
		c.BaseURL+"/analytics/representation/face-reid",
		map[string]string{
			"Authorization": "Bearer " + c.Token,
			"Accept":        "application/json",
		},
		bytes.NewReader(body),
	)
	if e != nil {
		return "", 0, nil, "", status, e
	}

	type entry struct {
		Image          string  `json:"image"`
		Score          float64 `json:"score"`
		Representation struct {
			Model     string    `json:"model"`
			Dimension int       `json:"dimension"`
			Vector    []float64 `json:"vector"`
		} `json:"representation"`
	}
	var parsed struct {
		Images []struct {
			Boxes struct {
				Image any `json:"image"` // map[string]entry หรือ []entry
			} `json:"boxes"`
		} `json:"images"`
	}
	if err := json.Unmarshal(rb, &parsed); err != nil {
		return "", 0, nil, "", status, fmt.Errorf("bad json: %w", err)
	}
	if len(parsed.Images) == 0 {
		return "", 0, nil, "", status, fmt.Errorf("empty images")
	}

	switch v := parsed.Images[0].Boxes.Image.(type) {
	case map[string]any:
		for _, raw := range v {
			b, _ := json.Marshal(raw)
			var e entry
			if json.Unmarshal(b, &e) == nil && e.Image != "" && len(e.Representation.Vector) > 0 {
				return e.Representation.Model, e.Representation.Dimension, e.Representation.Vector, e.Image, status, nil
			}
		}
	case []any:
		if len(v) > 0 {
			b, _ := json.Marshal(v[0])
			var e entry
			if json.Unmarshal(b, &e) == nil && e.Image != "" && len(e.Representation.Vector) > 0 {
				return e.Representation.Model, e.Representation.Dimension, e.Representation.Vector, e.Image, status, nil
			}
		}
	default:
	}
	return "", 0, nil, "", status, fmt.Errorf("no face/representation")
}

func (c *Client) DeletePerson(ctx context.Context, personID, faceID string) error {
	if !c.Configured() {
		return fmt.Errorf("iboc not configured")
	}
	personID = strings.TrimSpace(personID)
	if personID == "" {
		return nil
	}

	// ทำเป็น inactive + ใส่เหตุผลลง metadata
	payload := map[string]any{
		"enabled": false,
		"metadata": map[string]any{
			"crimes":    "ยกเลิกหมายจับจาก crimes",
			"deletedAt": time.Now().UTC().Format(time.RFC3339),
			"deletedBy": "klynx", // ปรับตามระบบคุณได้
			"reason":    "soft-delete via UpdatePerson",
		},
		// จะใส่ tags ช่วยด้วยก็ได้ เช่น mark INACTIVE
		"tags": []string{"INACTIVE"},
	}

	body, _ := json.Marshal(payload)
	rb, status, err := gwcom.PostJSON(
		ctx,
		c.BaseURL+"/mgmt/person/"+personID, // ใช้เส้นทางเดียวกับ UpdatePerson
		map[string]string{
			"Authorization": "Bearer " + c.Token,
			"Accept":        "application/json",
		},
		bytes.NewReader(body),
	)
	if err != nil {
		return err
	}
	switch {
	case status == 401 || status == 403:
		return ErrUnauthorized
	case status >= 500:
		return ErrServer
	case status >= 400:
		s := string(rb)
		if len(s) > 600 {
			s = s[:600] + "...(truncated)"
		}
		return fmt.Errorf("iboc soft delete: http %d, body=%s", status, s)
	}

	// ✅ ไม่ลบ face / person จริง ๆ อีกต่อไป (soft delete เท่านั้น)
	return nil
}

// // ------- Delete: ลบ face (ถ้ามี) แล้วลบ person -------
// func (c *Client) DeletePerson(ctx context.Context, personID, faceID string) error {
// 	if !c.Configured() {
// 		return fmt.Errorf("iboc not configured")
// 	}
// 	if faceID != "" {
// 		_, status, _ := gwcom.Delete(ctx, c.BaseURL+"/mgmt/person/face/"+faceID,
// 			map[string]string{"Authorization": "Bearer " + c.Token},
// 		)
// 		if status >= 500 {
// 			return fmt.Errorf("delete face error: %d", status)
// 		}
// 	}
// 	if personID != "" {
// 		_, status, _ := gwcom.Delete(ctx, c.BaseURL+"/mgmt/person/"+personID,
// 			map[string]string{"Authorization": "Bearer " + c.Token},
// 		)
// 		if status >= 500 {
// 			return fmt.Errorf("delete person error: %d", status)
// 		}
// 	}
// 	return nil
// }

// // ------- DeleteFace: ลบหน้าเดี่ยว ๆ -------
// func (c *Client) DeleteFace(ctx context.Context, faceID string) error {
// 	if !c.Configured() {
// 		return fmt.Errorf("iboc not configured")
// 	}
// 	if strings.TrimSpace(faceID) == "" {
// 		return nil
// 	}
// 	_, status, err := gwcom.Delete(
// 		ctx,
// 		c.BaseURL+"/mgmt/person/face/"+faceID,
// 		map[string]string{
// 			"Authorization": "Bearer " + c.Token,
// 			"Accept":        "application/json",
// 		},
// 	)
// 	if err != nil {
// 		return err
// 	}
// 	if status == 401 || status == 403 {
// 		return ErrUnauthorized
// 	}
// 	if status >= 500 {
// 		return ErrServer
// 	}
// 	return nil
// }

// ------- UpdatePerson: อัปเดตข้อมูลบุคคล (POST /mgmt/person/{id}) -------
func (c *Client) UpdatePerson(ctx context.Context,
	personID, firstName, lastName, idcard,
	alertType, alertDesc, crimesType, alertPoliceStation string,
) error {
	if !c.Configured() {
		return fmt.Errorf("iboc not configured")
	}
	if strings.TrimSpace(personID) == "" {
		return fmt.Errorf("personID is empty")
	}

	payload := map[string]any{
		"firstName":     firstName,
		"lastName":      lastName,
		"identityDocId": idcard,
		"organization":  alertPoliceStation,
		"enabled":       true,
	}

	var tags []string
	if s := strings.TrimSpace(alertType); s != "" {
		tags = append(tags, s)
	}
	// if s := strings.TrimSpace(crimesType); s != "" {
	// 	tags = append(tags, s)
	// }
	if len(tags) > 0 {
		payload["tags"] = tags
	}

	meta := map[string]any{}
	if s := strings.TrimSpace(alertDesc); s != "" {
		meta[crimesType] = s
	}
	// if s := strings.TrimSpace(policeRegion); s != "" {
	// 	meta["policeRegion"] = s
	// }
	// if s := strings.TrimSpace(policeProvincial); s != "" {
	// 	meta["policeProvincial"] = s
	// }
	// if s := strings.TrimSpace(policeStation); s != "" {
	// 	meta["policeStation"] = s
	// }
	if len(meta) > 0 {
		payload["metadata"] = meta
	}

	b, _ := json.Marshal(payload)
	rb, status, err := gwcom.PostJSON(
		ctx, c.BaseURL+"/mgmt/person/"+personID, // ใช้ POST ตามที่คุณมี (ถ้า API กำหนด PATCH/PUT ให้เปลี่ยนตามนั้น)
		map[string]string{
			"Authorization": "Bearer " + c.Token,
			"Accept":        "application/json",
		},
		bytes.NewReader(b),
	)
	if err != nil {
		return err
	}
	switch {
	case status == 401 || status == 403:
		return ErrUnauthorized
	case status >= 500:
		return ErrServer
	case status >= 400:
		s := string(rb)
		if len(s) > 600 {
			s = s[:600] + "...(truncated)"
		}
		return fmt.Errorf("iboc update person: http %d, body=%s", status, s)
	}
	return nil
}
