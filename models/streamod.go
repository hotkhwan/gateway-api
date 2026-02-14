// models/streamod.go
package models

// ✅ รูปแบบ response มาตรฐาน
type APIResponse[T any] struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Status  bool   `json:"status"`
	Details T      `json:"details,omitempty"`
}

// ✅ โครงสร้าง body สำหรับสร้าง stream-proxy (pointer flags)
//   - nil  = ไม่ได้ส่งมา → ใช้ default
//   - 0/1  = ใช้ค่าที่ส่งมา
type CreateStreamRequest struct {
	VHost  string `json:"vhost"           example:"__defaultVhost__"`
	App    string `json:"app"             example:"live"`
	Stream string `json:"stream"          example:"test"`
	URL    string `json:"url"             example:"rtsp://admin:P@ssw0rd@192.168.110.102:554"`

	EnableHLS     *int `json:"enable_hls"      example:"1"`
	EnableFMP4    *int `json:"enable_fmp4"     example:"0"`
	EnableMP4     *int `json:"enable_mp4"      example:"0"`
	EnableRTMP    *int `json:"enable_rtmp"     example:"0"`
	EnableRTSP    *int `json:"enable_rtsp"     example:"1"`
	EnableTS      *int `json:"enable_ts"       example:"0"`
	EnableHLSFMP4 *int `json:"enable_hls_fmp4" example:"0"`
}

type ZlmOnPlayReq struct {
	Schema        string `json:"schema"`
	Vhost         string `json:"vhost"`
	App           string `json:"app"`
	Stream        string `json:"stream"`
	Params        string `json:"params"`
	IP            string `json:"ip"`
	Port          int    `json:"port"`
	MediaServerID string `json:"mediaServerId"`
}

// ✅ ใช้แทน payload/response ใดๆ จาก ZLMediaKit
type Any = map[string]any
