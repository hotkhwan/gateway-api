// internal/gateways/iboc/watchlist/ibface/types.go
package ibface

// DTO ของ IBOC ตามรูปแบบ API ภายนอก (ปรับ field ตามจริงของคุณ)
type AnalyzeResp struct {
	Images []struct {
		Boxes struct {
			Image []struct {
				Image string  `json:"image"` // base64 cropped
				Score float64 `json:"score"`
			} `json:"image"`
		} `json:"boxes"`
	} `json:"images"`
}

type CreatePersonResp struct {
	ID string `json:"id"`
}

type AttachFaceResp struct {
	ID string `json:"id"`
}
