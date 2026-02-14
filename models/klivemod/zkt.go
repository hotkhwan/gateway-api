// models/klivehmod/zkt.go
package klivemod

import (
	"encoding/json"
	"time"
)

// โครง payload ที่ ZLMediaKit ส่งมา (ตัดมาเฉพาะฟิลด์ที่เราสนใจ + รองรับ Raw)
type ZLMRaw struct {
	App         string `json:"app" bson:"app,omitempty"`
	HookIndex   int64  `json:"hook_index" bson:"hook_index,omitempty"`
	ID          string `json:"id" bson:"id,omitempty"` // session id ใน raw
	IP          string `json:"ip" bson:"ip,omitempty"`
	MediaServer string `json:"mediaServerId" bson:"mediaServerId,omitempty"`
	Params      string `json:"params" bson:"params,omitempty"`
	Port        int    `json:"port" bson:"port,omitempty"`
	Protocol    string `json:"protocol" bson:"protocol,omitempty"`
	Schema      string `json:"schema" bson:"schema,omitempty"`
	Stream      string `json:"stream" bson:"stream,omitempty"`
	VHost       string `json:"vhost" bson:"vhost,omitempty"`
}

// โครง Kafka event เดิม (ยังใช้ได้ เพราะมี id,event อยู่)
type LiveEvent struct {
	ID               string `json:"id"`
	Event            string `json:"event"`
	Time             string `json:"time"`
	Name             string `json:"name"`
	Status           bool   `json:"status"`
	State            string `json:"state"`
	VideoKey         string `json:"videoKey"`
	VideoContentType string `json:"videoContentType"`
	VideoSize        int64  `json:"videoSize"`
	Rev              int64  `json:"rev,omitempty"`
	// เพิ่มฟิลด์จากตัวอย่าง hook ที่ Kafka ส่งมา
	App         string `json:"app"`
	ClientIP    string `json:"client_ip"`
	MediaServer string `json:"mediaServerId"`
	Params      string `json:"params"`
	Port        int    `json:"port"`
	Schema      string `json:"schema"`
	Stream      string `json:"stream"`
	TS          string `json:"ts"`
	VHost       string `json:"vhost"`
	Raw         ZLMRaw `json:"raw"`
}

// เอกสารสำหรับเก็บลง MongoDB
type KliveEventDoc struct {
	ID          string      `bson:"_id,omitempty" json:"_id,omitempty"` // ไม่ได้ใช้ก็ปล่อยว่าง
	KliveID     string      `bson:"kliveId" json:"kliveId"`             // จาก msg.ID
	Event       string      `bson:"event" json:"event"`                 // normalized
	App         string      `bson:"app,omitempty" json:"app,omitempty"`
	ClientIP    string      `bson:"clientIP,omitempty" json:"clientIP,omitempty"`
	ClientInfo  *ClientInfo `bson:"clientInfo,omitempty" json:"clientInfo,omitempty"`
	Schema      string      `bson:"schema,omitempty" json:"schema,omitempty"`
	Stream      string      `bson:"stream,omitempty" json:"stream,omitempty"`
	VHost       string      `bson:"vhost,omitempty" json:"vhost,omitempty"`
	Params      string      `bson:"params,omitempty" json:"params,omitempty"`
	Session     string      `bson:"session,omitempty" json:"session,omitempty"` // ดึงจาก params/raw.id
	MediaServer string      `bson:"mediaServerId,omitempty" json:"mediaServerId,omitempty"`
	Port        int         `bson:"port,omitempty" json:"port,omitempty"`
	HookIndex   int64       `bson:"hookIndex,omitempty" json:"hookIndex,omitempty"`
	Protocol    string      `bson:"protocol,omitempty" json:"protocol,omitempty"`
	// Timestamp  int64  `json:"timestamp"` // UnixMilli
	Timestamp time.Time `bson:"ts,omitempty" json:"ts,omitempty"`

	// ReceivedAt  time.Time              `bson:"receivedAt" json:"receivedAt"`
	CreatedAt time.Time              `bson:"createdAt" json:"createdAt"`
	Rev       int64                  `bson:"rev,omitempty" json:"rev,omitempty"`
	Raw       map[string]interface{} `bson:"raw,omitempty" json:"raw,omitempty"` // เก็บ raw ไว้ดูย้อนหลัง
}
type GeoInfo struct {
	Country string  `json:"country,omitempty" bson:"country,omitempty"`
	City    string  `json:"city,omitempty" bson:"city,omitempty"`
	Region  string  `json:"region,omitempty" bson:"region,omitempty"`
	Lat     float64 `json:"lat,omitempty" bson:"lat,omitempty"`
	Lon     float64 `json:"lon,omitempty" bson:"lon,omitempty"`
}

type ClientInfo struct {
	IP      string `json:"ip,omitempty" bson:"ip,omitempty"`
	UA      string `json:"ua,omitempty" bson:"ua,omitempty"`
	Browser string `json:"browser,omitempty" bson:"browser,omitempty"`
	OS      string `json:"os,omitempty" bson:"os,omitempty"`
	Device  string `json:"device,omitempty" bson:"device,omitempty"`
	Lang    string `json:"lang,omitempty" bson:"lang,omitempty"`

	// ✅ ใส่ให้ compile ผ่าน + รองรับ geo
	Country string   `json:"country,omitempty" bson:"country,omitempty"`
	Geo     *GeoInfo `json:"geo,omitempty" bson:"geo,omitempty"`

	// optional debug fields (ถ้าคุณใช้ตาม patch ก่อนหน้า)
	IPChain    []string          `json:"ipChain,omitempty" bson:"ipChain,omitempty"`
	RawHeaders map[string]string `json:"rawHeaders,omitempty" bson:"rawHeaders,omitempty"`
	Origin     string            `json:"origin,omitempty" bson:"origin,omitempty"`
	Referer    string            `json:"referer,omitempty" bson:"referer,omitempty"`

	TraceID string `json:"traceId,omitempty" bson:"traceId,omitempty"`
}

// รองรับทั้ง clientIp และ clientIp
func (e *LiveEvent) UnmarshalJSON(b []byte) error {
	type Alias LiveEvent
	aux := struct {
		*Alias
		ClientIp  string `json:"clientIp"`
		Client_ip string `json:"client_ip"`
	}{Alias: (*Alias)(e)}

	if err := json.Unmarshal(b, &aux); err != nil {
		return err
	}
	if e.ClientIP == "" {
		if aux.ClientIp != "" {
			e.ClientIP = aux.ClientIp
		} else if aux.Client_ip != "" {
			e.ClientIP = aux.Client_ip
		}
	}
	return nil
}

type StreamConfig struct {
	ID             string    `bson:"_id,omitempty" json:"id"`
	Name           string    `bson:"name" json:"name"`
	SessionTimeout int       `bson:"sessionTimeout" json:"sessionTimeout"` // seconds
	Enabled        bool      `bson:"enabled" json:"enabled"`
	CreatedAt      time.Time `bson:"createdAt"`
	UpdatedAt      time.Time `bson:"updatedAt"`
}
