// models/kctrlmod/device.go
package kctrlmod

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type MQTTConfig struct {
	Server       string `bson:"server,omitempty"       json:"server,omitempty"`
	Port         int    `bson:"port,omitempty"         json:"port,omitempty"`
	Username     string `bson:"username,omitempty"     json:"username,omitempty"`
	Password     string `bson:"password,omitempty"     json:"-"` // hidden from API
	Certificated string `bson:"certificated,omitempty" json:"certificated,omitempty"`
}

/* ===== Interval spec (per input / sensor) ===== */
type IntervalSpec struct {
	ID     string `bson:"id,omitempty"       json:"id,omitempty"`
	Name   string `bson:"name,omitempty"     json:"name,omitempty"`
	Values int64  `bson:"values,omitempty"   json:"values,omitempty"`
}

/* ===== Stats ===== */

/* ===== Device ===== */
type Device struct {
	ID          primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	HwID        string             `bson:"hwId"          json:"hwId"`
	Ip          string             `bson:"ip"          json:"ip"`
	Name        string             `bson:"name,omitempty"     json:"name,omitempty"`
	Brand       string             `bson:"brand,omitempty"    json:"brand,omitempty"`
	Owner       string             `bson:"owner,omitempty"    json:"owner,omitempty"`
	Phone       string             `bson:"phone,omitempty"    json:"phone,omitempty"`
	Description string             `bson:"description,omitempty" json:"description,omitempty"`
	Code        string             `bson:"code,omitempty"     json:"code,omitempty"`
	District    string             `bson:"district,omitempty" json:"district,omitempty"`
	Lat         float64            `bson:"lat,omitempty"      json:"lat,omitempty"`
	Lng         float64            `bson:"lng,omitempty"      json:"lng,omitempty"`
	Alarm       bool               `bson:"alarm"             json:"alarm"`
	Approved    bool               `bson:"approved"           json:"approved"`
	Status      string             `bson:"status,omitempty"   json:"status,omitempty"`
	LastSeen    time.Time          `bson:"lastSeen,omitempty" json:"lastSeen,omitempty"`
	GroupIds    []string           `bson:"groupIds,omitempty" json:"groupIds,omitempty"`

	// intervals & thresholds (ms)
	HealthInterval  int64 `bson:"healthInterval,omitempty"  json:"healthInterval,omitempty"`
	WarnThresholdMs int64 `bson:"warnThresholdMs,omitempty" json:"warnThresholdMs,omitempty"`
	OffThresholdMs  int64 `bson:"offThresholdMs,omitempty"  json:"offThresholdMs,omitempty"`

	// per-input intervals (object form)
	IntervalIN1 *IntervalSpec `bson:"interval_IN_1,omitempty" json:"interval_IN_1,omitempty"`
	IntervalIN2 *IntervalSpec `bson:"interval_IN_2,omitempty" json:"interval_IN_2,omitempty"`
	IntervalIN3 *IntervalSpec `bson:"interval_IN_3,omitempty" json:"interval_IN_3,omitempty"`
	IntervalIN4 *IntervalSpec `bson:"interval_IN_4,omitempty" json:"interval_IN_4,omitempty"`

	// สำหรับ Frontend: sensor[] ที่ map จาก interval_IN_* (id/name/values)
	Sensor []IntervalSpec `bson:"sensor,omitempty" json:"sensor,omitempty"`

	/* runtime info from health */
	FwVersion string         `bson:"fwVersion,omitempty" json:"fwVersion,omitempty"`
	FwBuild   string         `bson:"fwBuild,omitempty"   json:"fwBuild,omitempty"`
	Source    string         `bson:"source,omitempty"    json:"source,omitempty"`
	HeapFree  int64          `bson:"heapFree,omitempty"  json:"heapFree,omitempty"`
	HeapMin   int64          `bson:"heapMin,omitempty"   json:"heapMin,omitempty"`
	Metrics   *HealthMetrics `bson:"metrics,omitempty"   json:"metrics,omitempty"`
	TempC     float64        `bson:"tempC,omitempty"     json:"tempC,omitempty"`

	// MQTT & OTA
	MQTT        *MQTTConfig `bson:"mqtt,omitempty"        json:"mqtt,omitempty"`
	FirmwareURL string      `bson:"firmwareURL,omitempty" json:"firmwareURL,omitempty"`

	// telemetry ล่าสุด (raw JSON)
	LastTelemetry any   `bson:"lastTelemetry,omitempty" json:"lastTelemetry,omitempty"`
	Stats         Stats `bson:"stats,omitempty"         json:"stats,omitempty"`

	// watchdog info
	InactiveForMs    int64     `bson:"inactiveForMs,omitempty"   json:"inactiveForMs,omitempty"`
	DateTimeCreate   time.Time `bson:"dateTimeCreate,omitempty"  json:"dateTimeCreate,omitempty"`
	DateTimeUpdate   time.Time `bson:"dateTimeUpdate,omitempty"  json:"dateTimeUpdate,omitempty"`
	LastStatusChange time.Time `bson:"lastStatusChange,omitempty" json:"lastStatusChange,omitempty"`
}

type DeviceApproval struct {
	Approved *bool `bson:"approved,omitempty"`
}

type Access struct {
	Allow  Allow  `json:"allow"`
	Source Source `json:"source"`
}

type Allow struct {
	Read   bool `json:"read"`
	Create bool `json:"create"`
	Update bool `json:"update"`
	Delete bool `json:"delete"`
}

type Source struct {
	Mode            string   `json:"mode"`
	MatchedGroupIDs []string `json:"matchedGroupIds"`
	Subject         Subject  `json:"subject"`
}

type Subject struct {
	Type string `json:"type"`
	ID   string `json:"id"`
}
