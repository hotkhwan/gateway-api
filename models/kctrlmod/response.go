// models/kctrlmodel/response.go
package kctrlmod

import (
	"klynx/models/gmod"
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

/* ===== Existing responses (คงไว้ได้ตามเดิม) ===== */
type DeviceResponse struct {
	Status  bool   `json:"status" example:"true"`
	Details Device `json:"details"`
}

type AlarmActor struct {
	ActorId   string `json:"actorId,omitempty"`
	ActorName string `json:"actorName,omitempty"`
	ActorIp   string `json:"actorIp,omitempty"`
}

type AlarmSopStep struct {
	StepId      primitive.ObjectID `json:"stepId"`
	Type        string             `json:"type"`
	Description string             `json:"description"`
	Detail      map[string]any     `json:"detail,omitempty"`
	StepStatus  string             `json:"stepStatus"`
	CreatedAt   string             `json:"createdAt"` // RFC3339
	ActorId     string             `json:"actorId,omitempty"`
	ActorName   string             `json:"actorName,omitempty"`
	ActorIp     string             `json:"actorIp,omitempty"`
}

type AlarmDetail struct {
	Status    string   `json:"status,omitempty"`
	CycleId   string   `json:"cycleId,omitempty"`
	Sensor    *int     `json:"sensor,omitempty"`
	StartedAt *int64   `json:"startedAt,omitempty"`
	AlarmAt   *int64   `json:"alarmAt,omitempty"`
	Elapsed   *float64 `json:"elapsed,omitempty"`

	Acknowledged   *bool       `json:"acknowledged,omitempty"`
	AcknowledgedAt string      `json:"acknowledgedAt,omitempty"` // RFC3339
	AcknowledgedBy *AlarmActor `json:"acknowledgedBy,omitempty"`

	Sop []AlarmSopStep `json:"sop,omitempty"`
}

type AlarmResponseItem struct {
	ID       primitive.ObjectID `json:"id"`
	Message  string             `json:"message"`
	Datetime string             `json:"datetime"`
	Name     string             `json:"name"`
	HwID     string             `json:"hwId"`
	District string             `json:"district"`
	Lat      float64            `json:"lat"`
	Lng      float64            `json:"lng"`
	Brand    string             `json:"brand"`

	// ✅ moved out of detail
	Status    string   `json:"status,omitempty"`
	CycleId   string   `json:"cycleId,omitempty"`
	Sensor    *int     `json:"sensor,omitempty"`
	StartedAt *int64   `json:"startedAt,omitempty"`
	AlarmAt   *int64   `json:"alarmAt,omitempty"`
	Elapsed   *float64 `json:"elapsed,omitempty"`

	Acknowledged   *bool       `json:"acknowledged,omitempty"`
	AcknowledgedAt string      `json:"acknowledgedAt,omitempty"`
	AcknowledgedBy *AlarmActor `json:"acknowledgedBy,omitempty"`

	Sop []AlarmSopStep `json:"sop,omitempty"`
}

type AlarmIconEvent struct {
	DeviceID string  // deviceId ใน MQTT
	HwID     string  // hwId
	Name     string  // แสดงใน InfoWindow / Sidebar
	District string  // เขต/พื้นที่
	Lat      float64 // lat (optional)
	Lng      float64 // lng (optional)
	Status   string  // "online" | "offline" | "alarm" (ใน map.vue จะเอาไปใช้)
	Action   string  // "raise" | "ack" หรือค่าอื่นที่ต้องการ
	Alarm    bool    // true = alarm, false = clear
	Message  string  // ข้อความเช่น "sensor detect", "alarm acknowledged"
}

type PaginationAlarmsResponse struct {
	Status     bool                `json:"status" example:"true"`
	Details    []AlarmResponseItem `json:"details"`
	Pagination gmod.Pagination     `json:"pagination"`
}

type PaginationKctrlResponse struct {
	Status     bool            `json:"status" example:"true"`
	Details    Device          `json:"details"`
	Pagination gmod.Pagination `json:"pagination"`
}

type SuccessMessageResponse struct {
	Code    string `json:"code" example:"SUCCESS"`
	Message string `json:"message" example:"Device updated successfully"`
	Status  bool   `json:"status" example:"true"`
}

/* ====== NEW: Flat Kafka messages (ไม่มี raw) ====== */

// Kafka → Alarms (flattened)
type AlarmMessage struct {
	Topic     string    `json:"topic"`
	HwID      string    `json:"hwId"`
	Timestamp time.Time `json:"timestamp"` // RFC3339 -> time.Time (std json supports)
	Kind      string    `json:"kind"`      // "alarms"

	// flattened payload
	Message   string   `json:"message,omitempty"` // e.g. "sensor2 alarm"
	Sensor    *int     `json:"sensor,omitempty"`  // optional (sensor1–4)
	CycleID   string   `json:"cycleId,omitempty"`
	StartedAt *int64   `json:"startedAt,omitempty"` // epoch sec
	AlarmAt   *int64   `json:"alarmAt,omitempty"`   // epoch sec
	Elapsed   *float64 `json:"elapsed,omitempty"`   // seconds
}

// Kafka → Sensor (flattened)
type SensorMessage struct {
	Topic     string    `json:"topic"`
	HwID      string    `json:"hwId"`
	Timestamp time.Time `json:"timestamp"`
	Kind      string    `json:"kind"` // "sensor"

	// flattened payload
	Sensor    int     `json:"sensor"`
	Event     string  `json:"event"`    // e.g. "detect"
	Duration  float64 `json:"duration"` // seconds
	CycleID   string  `json:"cycleId"`
	StartedAt int64   `json:"startedAt"` // epoch sec
}

/* ====== Mongo event document ====== */

type EventDoc struct {
	ID       interface{} `bson:"_id,omitempty"`
	Kind     string      `bson:"kind"` // "sensor" | "alarms" | "health"
	Topic    string      `bson:"topic"`
	HwID     string      `bson:"hwId"`
	Datetime time.Time   `bson:"datetime"`

	// Device snapshot
	DeviceID    interface{} `bson:"device,omitempty"`
	Name        string      `bson:"name,omitempty"`
	Owner       string      `bson:"owner,omitempty"`
	Phone       string      `bson:"phone,omitempty"`
	District    string      `bson:"district,omitempty"`
	Description string      `bson:"description,omitempty"`
	Lat         float64     `bson:"lat,omitempty"`
	Lng         float64     `bson:"lng,omitempty"`
	Brand       string      `bson:"brand,omitempty"`

	// Event data (flattened)
	Message   string   `bson:"message,omitempty"`
	Sensor    *int     `bson:"sensor,omitempty"`
	Event     string   `bson:"event,omitempty"`
	Duration  *float64 `bson:"duration,omitempty"`
	CycleID   string   `bson:"cycleId,omitempty"`
	StartedAt *int64   `bson:"startedAt,omitempty"`
	AlarmAt   *int64   `bson:"alarmAt,omitempty"`
	Elapsed   *float64 `bson:"elapsed,omitempty"`

	// Extra bag (optional)
	Extra map[string]any `bson:"extra,omitempty"`
}

type EventResponseItem struct {
	ID       primitive.ObjectID `json:"id"`
	Kind     string             `json:"kind"`
	Topic    string             `json:"topic,omitempty"`
	Message  string             `json:"message,omitempty"`
	Event    string             `json:"event,omitempty"`
	Datetime string             `json:"datetime"`

	Name     string  `json:"name,omitempty"`
	HwID     string  `json:"hwId,omitempty"`
	District string  `json:"district,omitempty"`
	Lat      float64 `json:"lat,omitempty"`
	Lng      float64 `json:"lng,omitempty"`
	Brand    string  `json:"brand,omitempty"`

	Sensor    *int     `json:"sensor,omitempty"`
	Duration  *float64 `json:"duration,omitempty"`
	CycleID   string   `json:"cycleId,omitempty"`
	StartedAt *int64   `json:"startedAt,omitempty"`
	AlarmAt   *int64   `json:"alarmAt,omitempty"`
	Elapsed   *float64 `json:"elapsed,omitempty"`
}
