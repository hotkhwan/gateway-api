// models/kctrlmodel/stats.go
package kctrlmod

/* ===== Stats ===== */
type Stats struct {
	Online            int64 `bson:"online,omitempty"            json:"online,omitempty"`
	Warning           int64 `bson:"warning,omitempty"           json:"warning,omitempty"`
	Offline           int64 `bson:"offline,omitempty"           json:"offline,omitempty"`
	AlarmCount        int64 `bson:"alarmCount,omitempty"        json:"alarmCount,omitempty"`
	SensorDetectCount int64 `bson:"sensorDetectCount,omitempty" json:"sensorDetectCount,omitempty"`
}
