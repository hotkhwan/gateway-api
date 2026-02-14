// models/kctrlmodel/request.go
package kctrlmod

import (
	"encoding/json"
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type HealthMetrics struct {
	Detects int `bson:"detects,omitempty" json:"detects,omitempty"`
	Alarms  int `bson:"alarms,omitempty"  json:"alarms,omitempty"`
}

type HealthMessage struct {
	Message        string  `json:"message"`
	HwID           string  `json:"hwId"`
	Ip             string  `json:"ip,omitempty"`
	FwVersion      string  `json:"fwVersion,omitempty"`
	FwBuild        string  `json:"fwBuild,omitempty"`
	HealthInterval int64   `json:"healthInterval"`
	Source         string  `json:"source,omitempty"`
	HeapFree       int64   `json:"heapFree,omitempty"`
	HeapMin        int64   `json:"heapMin,omitempty"`
	TempC          float64 `json:"tempC,omitempty"`
	Name           string  `json:"name,omitempty"`

	// metrics {detects, alarms}
	Metrics *HealthMetrics `json:"metrics,omitempty"`

	// รับได้ทั้ง 10000 หรือ {"id":"...","name":"...","values":10000}
	IntervalIN1 any `json:"interval_IN_1,omitempty"`
	IntervalIN2 any `json:"interval_IN_2,omitempty"`
	IntervalIN3 any `json:"interval_IN_3,omitempty"`
	IntervalIN4 any `json:"interval_IN_4,omitempty"`
}

type SopStep struct {
	StepId      primitive.ObjectID `bson:"stepId" json:"stepId"`
	Type        string             `bson:"type" json:"type"`
	Description string             `bson:"description" json:"description"`
	Detail      map[string]any     `bson:"detail,omitempty" json:"detail,omitempty"`
	StepStatus  string             `bson:"stepStatus" json:"stepStatus"`
	CreatedAt   time.Time          `bson:"createdAt" json:"createdAt"`

	ActorId   string `bson:"actorId,omitempty" json:"actorId,omitempty"`
	ActorName string `bson:"actorName,omitempty" json:"actorName,omitempty"`
	ActorIp   string `bson:"actorIp,omitempty" json:"actorIp,omitempty"`
}

type AckAlarmRequest struct {
	Acknowledged bool   `json:"acknowledged"`
	HwID         string `json:"hwId"`

	// optional: ถ้าไม่ส่งมา จะ default เป็น "รับแจ้งเหตุ"
	Description string `json:"description,omitempty"`

	// optional object
	Detail map[string]any `json:"detail,omitempty"`
}

type SopStepRequest struct {
	Type        string         `json:"type"`                 // dispatch|arrive|resolve|note...
	Description string         `json:"description"`          // หัวข้อ
	Detail      map[string]any `json:"detail,omitempty"`     // object
	StepStatus  string         `json:"stepStatus,omitempty"` // default inProgress
}

type ResolvedAlarmRequest struct {
	Description string         `json:"description,omitempty"` // default: "ปิดงาน"
	Detail      map[string]any `json:"detail,omitempty"`      // result/note/attachments/etc.

	// optional: ถ้า true จะ append SOP update ปิด step inProgress ทั้งหมด
	AutoCloseOpenSteps *bool `json:"autoCloseOpenSteps,omitempty"`
}

type ResolvedAlarmResult struct {
	AlarmId    string    `json:"alarmId"`
	ResolvedAt time.Time `json:"resolvedAt"`
	Status     string    `json:"status"`
}

type EventMessage struct {
	Topic          string    `json:"topic"`
	Timestamp      time.Time `json:"timestamp"`
	Message        string    `json:"message"`
	HwID           string    `json:"hwId"`
	HealthInterval int       `json:"healthInterval"`
	AlarmInterval  int       `json:"alarmInterval"`
}

type EventMAPMessage struct {
	DeviceID        string      `json:"deviceId"`
	HWID            string      `json:"hwId"`
	HealthInterval  int64       `json:"healthInterval"`
	InactiveForMs   int64       `json:"inactiveForMs"`
	Kind            string      `json:"kind"`
	LastSeen        string      `json:"lastSeen"`
	Message         string      `json:"message"`
	Name            string      `json:"name"`
	OffThresholdMs  int64       `json:"offThresholdMs"`
	Sensor          interface{} `json:"sensor"` // หรือ []Sensor ถ้าอยาก strict
	Source          string      `json:"source"`
	Stats           interface{} `json:"stats"` // หรือ struct {Online,Warning,Offline int}
	Status          string      `json:"status"`
	Timestamp       string      `json:"timestamp"`
	Topic           string      `json:"topic"`
	WarnThresholdMs int64       `json:"warnThresholdMs"`
}

type EventEnvelope struct {
	Topic     string    `json:"topic"`
	HwID      string    `json:"hwId"`
	Timestamp time.Time `json:"timestamp"`
	// เก็บ payload เดิมทั้งหมดแบบ raw เพื่อไม่ทิ้งฟิลด์
	Raw json.RawMessage `json:"raw"`
	// เผื่อใช้ค้นหาง่าย ๆ ใน BI/Logs
	Kind string `json:"kind,omitempty"` // sensor|alarm|health|unknown
}

type ControlMessage struct {
	Topic string      `json:"topic"`
	HwID  interface{} `json:"hwId"` // "all" | "AA:BB..." | []string

	Message           string `json:"message,omitempty"`
	SetHealthInterval int    `json:"setHealthInterval,omitempty"`
	SetAlarmInterval  int    `json:"setAlarmInterval,omitempty"`

	// NEW: intervals per input
	IntervalIN1 int `json:"interval_IN_1,omitempty"`
	IntervalIN2 int `json:"interval_IN_2,omitempty"`
	IntervalIN3 int `json:"interval_IN_3,omitempty"`
	IntervalIN4 int `json:"interval_IN_4,omitempty"`

	// NEW: switch output (ยังคงของเดิม)
	SetOutput string `json:"setOutput,omitempty"` // OUT_1 | OUT_2
	Type      string `json:"type,omitempty"`      // ONCE | HIGH | LOW

	// NEW: MQTT endpoint + persist
	SetMqtt bool   `json:"setMqtt,omitempty"`
	Host    string `json:"host,omitempty"`
	Port    int    `json:"port,omitempty"`
	User    string `json:"user,omitempty"`
	Pass    string `json:"pass,omitempty"`
	Pem     string `json:"pem,omitempty"`
	Persist *bool  `json:"persist,omitempty"` // ใช้ pointer เพื่อรู้ว่า client ส่งมาจริงไหม

	SetOta bool   `json:"setOta,omitempty"`
	Url    string `json:"url,omitempty"`

	// NEW: clear NVS
	ClearConfig bool `json:"clearConfig,omitempty"`

	Timestamp time.Time `json:"timestamp,omitempty"`
}

type UpdatePayload struct {
	Name        string   `json:"name,omitempty"`
	District    string   `json:"district,omitempty"`
	Owner       string   `json:"owner,omitempty"`
	Phone       string   `json:"phone,omitempty"`
	Description string   `json:"description,omitempty"`
	Lat         float64  `json:"lat,omitempty"`
	Lng         float64  `json:"lng,omitempty"`
	Alarm       *bool    `json:"alarm,omitempty"`
	Approved    *bool    `json:"approved,omitempty"`
	GroupIds    []string `json:"groupIds,omitempty"`
}
