// models/systemmod/options.go
package systemmod

import "time"

// Document in: db=klynx, coll=options, _id=system.setting
type SystemSetting struct {
	ID string `bson:"_id" json:"id"` // always "system.setting"

	// ===== Audit =====
	AuditCaptureResponse string `bson:"AUDIT_CAPTURE_RESPONSE,omitempty" json:"AUDIT_CAPTURE_RESPONSE,omitempty"` // none|errors|all
	AuditMaxRespBytes    int    `bson:"AUDIT_MAX_RESP_BYTES,omitempty" json:"AUDIT_MAX_RESP_BYTES,omitempty"`
	AuditCaptureJSONOnly *bool  `bson:"AUDIT_CAPTURE_FOR_JSON_ONLY,omitempty" json:"AUDIT_CAPTURE_FOR_JSON_ONLY,omitempty"`
	AuditRetentionDays   int    `bson:"AUDIT_RETENTION_DAYS,omitempty" json:"AUDIT_RETENTION_DAYS,omitempty"`

	// ===== App/Batch/Timeout =====
	MaxRecordRequest       int    `bson:"MAX_RECOARD_REQUEST,omitempty" json:"MAX_RECOARD_REQUEST,omitempty"`     // ตาม env เดิม
	KafkaPublishTimeout    string `bson:"KAFKA_PUBLISH_TIMEOUT,omitempty" json:"KAFKA_PUBLISH_TIMEOUT,omitempty"` // "5s"
	KwatchBatchSize        int    `bson:"KWATCH_BATCHSIZE,omitempty" json:"KWATCH_BATCHSIZE,omitempty"`
	KctrlWatchdogInterval  int    `bson:"KCTRL_WATCHDOG_INTERVAL" json:"KCTRL_WATCHDOG_INTERVAL"`
	KctrlWarnMultiplier    int    `bson:"KCTRL_WARN_MULTIPLIER" json:"KCTRL_WARN_MULTIPLIER"`
	KctrlOfflineMultiplier int    `bson:"KCTRL_OFFLINE_MULTIPLIER" json:"KCTRL_OFFLINE_MULTIPLIER"`

	// meta
	UpdatedAt time.Time `bson:"updatedAt,omitempty" json:"updatedAt,omitempty"`
}

// Effective config (หลัง merge DB→ENV→DEFAULT) ที่ฝั่ง API/บริการอื่นจะใช้
type EffectiveConfig struct {
	BasePath             string `json:"BASE_PATH,omitempty"`
	IwownPath            string `json:"IWOWN_PATH,omitempty"`
	AuditCaptureResponse string `json:"AUDIT_CAPTURE_RESPONSE"`
	AuditMaxRespBytes    int    `json:"AUDIT_MAX_RESP_BYTES"`
	AuditCaptureJSONOnly bool   `json:"AUDIT_CAPTURE_FOR_JSON_ONLY"`
	AuditRetentionDays   int    `json:"AUDIT_RETENTION_DAYS"`

	MaxRecordRequest       int    `json:"MAX_RECOARD_REQUEST"`
	KafkaPublishTimeout    string `json:"KAFKA_PUBLISH_TIMEOUT"`
	KwatchBatchSize        int    `json:"KWATCH_BATCHSIZE"`
	KctrlWatchdogInterval  int    `json:"KCTRL_WATCHDOG_INTERVAL"`
	KctrlWarnMultiplier    int    `json:"KCTRL_WARN_MULTIPLIER"`
	KctrlOfflineMultiplier int    `json:"KCTRL_OFFLINE_MULTIPLIER"`
}
