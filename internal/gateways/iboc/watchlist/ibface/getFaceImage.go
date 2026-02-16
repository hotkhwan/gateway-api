// internal/gateways/iboc/watchlist/ibface/getFaceImage.go
package ibface

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"path"
	"strings"

	"github.com/hotkhwan/gateway-api/internal/gateways/gwcom"
)

// NOTE:
// - อย่าประกาศ func (c *Client) Configured() bool ซ้ำที่นี่
// - ใช้ Client แบบเดิมตามที่ประกาศใน client.go

// // doPostJSON: wrapper gwcom.PostJSON + retry/backoff แบบเบาๆ สำหรับ 429/5xx และ network error
// func doPostJSON(ctx context.Context, url string, hdr map[string]string, body []byte) (rb []byte, status int, err error) {
// 	const (
// 		maxRetries    = 3
// 		firstBackoff  = 300 * time.Millisecond
// 		backoffFactor = 2.0
// 	)

// 	backoff := firstBackoff
// 	for attempt := 0; attempt <= maxRetries; attempt++ {
// 		rb, status, err = gwcom.PostJSON(ctx, url, hdr, bytes.NewReader(body))
// 		if err != nil {
// 			// network/ctx errors → retry ยกเว้น ctx canceled/deadline
// 			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
// 				return nil, 0, err
// 			}
// 			if attempt < maxRetries {
// 				time.Sleep(backoff)
// 				backoff *= backoffFactor
// 				continue
// 			}
// 			return nil, 0, err
// 		}

// 		// HTTP status handling
// 		if status == 429 || status >= 500 {
// 			if attempt < maxRetries {
// 				time.Sleep(backoff)
// 				backoff *= backoffFactor
// 				continue
// 			}
// 		}
// 		return rb, status, nil
// 	}
// 	return rb, status, err
// }

// GetFaceByIdcard ค้นหา person → face ด้วยเลขบัตรประชาชน
// return: personID, faceID, imagePath (relative), imageURL (absolute), error
func (c *Client) GetFaceByIdcard(ctx context.Context, idcard string) (string, string, string, string, error) {
	if !c.Configured() {
		return "", "", "", "", fmt.Errorf("iboc not configured")
	}
	idcard = strings.TrimSpace(idcard)
	if idcard == "" {
		return "", "", "", "", fmt.Errorf("idcard is empty")
	}

	// --- Step 1: person search by identityDocId (ตามสเป็กเดิม) ---
	type personSearchReq struct {
		Query map[string]any `json:"query"`
	}
	psReq := personSearchReq{
		Query: map[string]any{
			"identityDocId": idcard,
		},
	}
	b1, _ := json.Marshal(psReq)

	rb1, st1, err := gwcom.PostJSON(
		ctx,
		strings.TrimRight(c.BaseURL, "/")+"/mgmt/person/search",
		map[string]string{
			"Authorization": "Bearer " + c.Token,
			"Accept":        "application/json",
		},
		bytes.NewReader(b1),
	)
	if err != nil {
		return "", "", "", "", err
	}
	if st1 == 401 || st1 == 403 {
		return "", "", "", "", ErrUnauthorized
	}
	if st1 >= 500 {
		return "", "", "", "", ErrServer
	}
	if st1 >= 400 {
		return "", "", "", "", fmt.Errorf("person search http %d", st1)
	}

	var pResp struct {
		Result []struct {
			Doc struct {
				ID string `json:"id"`
			} `json:"doc"`
		} `json:"result"`
	}
	if err := json.Unmarshal(rb1, &pResp); err != nil {
		return "", "", "", "", fmt.Errorf("parse person search: %w", err)
	}
	if len(pResp.Result) == 0 || pResp.Result[0].Doc.ID == "" {
		return "", "", "", "", fmt.Errorf("person not found for idcard %s", idcard)
	}
	personID := strings.TrimSpace(pResp.Result[0].Doc.ID)

	// --- Step 2: face search by personId (ตามสเป็กเดิม; ไม่มี fields/sort) ---
	type faceSearchReq struct {
		Query map[string]any `json:"query"`
		Size  int            `json:"size,omitempty"`
	}
	fsReq := faceSearchReq{
		Query: map[string]any{
			"personId": map[string]any{
				"values": []string{personID},
			},
		},
		Size: 1,
	}
	b2, _ := json.Marshal(fsReq)

	rb2, st2, err := gwcom.PostJSON(
		ctx,
		strings.TrimRight(c.BaseURL, "/")+"/mgmt/person/face/search",
		map[string]string{
			"Authorization": "Bearer " + c.Token,
			"Accept":        "application/json",
		},
		bytes.NewReader(b2),
	)
	if err != nil {
		return "", "", "", "", err
	}
	if st2 == 401 || st2 == 403 {
		return "", "", "", "", ErrUnauthorized
	}
	if st2 >= 500 {
		return "", "", "", "", ErrServer
	}
	if st2 >= 400 {
		return "", "", "", "", fmt.Errorf("face search http %d", st2)
	}

	// รองรับหลาย key สำหรับ image path/url
	var fResp struct {
		Result []struct {
			Doc struct {
				ID       string  `json:"id"`
				ImageURL *string `json:"imageUrl,omitempty"`
				ImageUrl *string `json:"imageURL,omitempty"`
				Image    *string `json:"image_path,omitempty"`
			} `json:"doc"`
		} `json:"result"`
	}
	if err := json.Unmarshal(rb2, &fResp); err != nil {
		return "", "", "", "", fmt.Errorf("parse face search: %w", err)
	}
	if len(fResp.Result) == 0 || strings.TrimSpace(fResp.Result[0].Doc.ID) == "" {
		return personID, "", "", "", fmt.Errorf("face not found for person %s", personID)
	}
	faceID := strings.TrimSpace(fResp.Result[0].Doc.ID)

	var imagePath string
	if fResp.Result[0].Doc.ImageURL != nil && strings.TrimSpace(*fResp.Result[0].Doc.ImageURL) != "" {
		imagePath = strings.TrimSpace(*fResp.Result[0].Doc.ImageURL)
	} else if fResp.Result[0].Doc.ImageUrl != nil && strings.TrimSpace(*fResp.Result[0].Doc.ImageUrl) != "" {
		imagePath = strings.TrimSpace(*fResp.Result[0].Doc.ImageUrl)
	} else if fResp.Result[0].Doc.Image != nil && strings.TrimSpace(*fResp.Result[0].Doc.Image) != "" {
		imagePath = strings.TrimSpace(*fResp.Result[0].Doc.Image)
	}
	if imagePath == "" {
		return personID, faceID, "", "", fmt.Errorf("face image not found for %s", faceID)
	}

	imageURL := c.FaceImageAbsoluteURL(imagePath)
	return personID, faceID, imagePath, imageURL, nil
}

// FaceImageAbsoluteURL แปลง relative imagePath → absolute URL ตาม BaseURL
func (c *Client) FaceImageAbsoluteURL(imagePath string) string {
	if imagePath == "" {
		return ""
	}
	base := strings.TrimRight(c.BaseURL, "/")
	u, err := url.Parse(base)
	if err != nil || u.Host == "" {
		return base + "/" + strings.TrimLeft(imagePath, "/")
	}
	u.Path = path.Join(u.Path, strings.TrimLeft(imagePath, "/"))
	return u.String()
}
