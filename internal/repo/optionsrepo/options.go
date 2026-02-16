// internal/repo/optionsrepo/options.go
package optionsrepo

import (
	"context"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/hotkhwan/gateway-api/models/systemmod"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const (
	collName = "options"
	docID    = "system.setting"
)

// Repo คือ interface ที่ export สำหรับใช้งานภายนอก (cache, controller)
type Repo interface {
	// อ่าน effective config (DB -> ENV -> default)
	LoadEffective(ctx context.Context) (*systemmod.EffectiveConfig, error)
	// อัปเดตเป็นฟิลด์แบบ flat ลง _id=system.setting
	PatchFlat(ctx context.Context, set map[string]any) error
}

type repo struct {
	coll *mongo.Collection
}

func New(db *mongo.Database) Repo {
	return &repo{coll: db.Collection(collName)}
}

// PatchFlat: อัปเดตด้วย $set
func (r *repo) PatchFlat(ctx context.Context, set map[string]any) error {
	if len(set) == 0 {
		return nil
	}
	now := time.Now().UTC()
	set["updatedAt"] = now
	opts := options.Update().SetUpsert(true)
	_, err := r.coll.UpdateByID(ctx, docID, bson.M{"$set": set}, opts)
	return err
}

// LoadEffective: DB -> ENV -> defaults
func (r *repo) LoadEffective(ctx context.Context) (*systemmod.EffectiveConfig, error) {
	var dbdoc bson.M
	_ = r.coll.FindOne(ctx, bson.M{"_id": docID}).Decode(&dbdoc)

	// defaults
	eff := &systemmod.EffectiveConfig{
		BasePath:               "/api/v1",
		AuditCaptureResponse:   "errors",
		AuditMaxRespBytes:      16384,
		AuditCaptureJSONOnly:   true,
		AuditRetentionDays:     90,
		MaxRecordRequest:       10000,
		KafkaPublishTimeout:    "5s",
		KwatchBatchSize:        5000,
		KctrlWatchdogInterval:  5,
		KctrlWarnMultiplier:    3,
		KctrlOfflineMultiplier: 6,
	}

	// ENV override
	if v := strings.TrimSpace(os.Getenv("BASE_PATH")); v != "" {
		eff.BasePath = v
	}
	if v := strings.ToLower(strings.TrimSpace(os.Getenv("AUDIT_CAPTURE_RESPONSE"))); v != "" {
		eff.AuditCaptureResponse = v
	}
	if v := os.Getenv("AUDIT_MAX_RESP_BYTES"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			eff.AuditMaxRespBytes = n
		}
	}
	if v := os.Getenv("AUDIT_CAPTURE_FOR_JSON_ONLY"); v != "" {
		eff.AuditCaptureJSONOnly = strings.EqualFold(v, "true")
	}
	if v := os.Getenv("AUDIT_RETENTION_DAYS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 1 {
			eff.AuditRetentionDays = n
		}
	}
	// (สะกด MAX_RECOARD_REQUEST ตาม ENV เดิม)
	if v := os.Getenv("MAX_RECOARD_REQUEST"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			eff.MaxRecordRequest = n
		}
	}
	if v := os.Getenv("KAFKA_PUBLISH_TIMEOUT"); v != "" {
		eff.KafkaPublishTimeout = v
	}
	if v := os.Getenv("KWATCH_BATCHSIZE"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			eff.KwatchBatchSize = n
		}
	}
	if v := os.Getenv("KCTRL_WATCHDOG_INTERVAL"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			eff.KctrlWatchdogInterval = n
		}
	}
	if v := os.Getenv("KCTRL_WARN_MULTIPLIER"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			eff.KctrlWarnMultiplier = n
		}
	}
	if v := os.Getenv("KCTRL_OFFLINE_MULTIPLIER"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			eff.KctrlOfflineMultiplier = n
		}
	}
	// DB override
	getStr := func(key string) (string, bool) {
		if s, ok := dbdoc[key].(string); ok && strings.TrimSpace(s) != "" {
			return s, true
		}
		return "", false
	}
	getInt := func(key string) (int, bool) {
		switch x := dbdoc[key].(type) {
		case int32:
			return int(x), true
		case int64:
			return int(x), true
		case float64:
			return int(x), true
		case int:
			return x, true
		}
		return 0, false
	}
	getBool := func(key string) (bool, bool) {
		if b, ok := dbdoc[key].(bool); ok {
			return b, true
		}
		return false, false
	}

	if s, ok := getStr("basePath"); ok {
		eff.BasePath = s
	}
	if s, ok := getStr("auditCaptureResponse"); ok {
		eff.AuditCaptureResponse = strings.ToLower(strings.TrimSpace(s))
	}
	if n, ok := getInt("auditMaxRespBytes"); ok && n > 0 {
		eff.AuditMaxRespBytes = n
	}
	if b, ok := getBool("auditCaptureJSONOnly"); ok {
		eff.AuditCaptureJSONOnly = b
	}
	if n, ok := getInt("auditRetentionDays"); ok && n >= 1 {
		eff.AuditRetentionDays = n
	}
	if n, ok := getInt("maxRecordRequest"); ok && n > 0 {
		eff.MaxRecordRequest = n
	}
	if s, ok := getStr("kafkaPublishTimeout"); ok {
		eff.KafkaPublishTimeout = s
	}
	if n, ok := getInt("kwatchBatchSize"); ok && n >= 0 {
		eff.KwatchBatchSize = n
	}
	if n, ok := getInt("kctrlWatchdogInterval"); ok && n >= 0 {
		eff.KctrlWatchdogInterval = n
	}
	if n, ok := getInt("kctrlWarnMultiplier"); ok && n >= 0 {
		eff.KctrlWarnMultiplier = n
	}
	if n, ok := getInt("kctrlOfflineMultiplier"); ok && n >= 0 {
		eff.KctrlOfflineMultiplier = n
	}

	return eff, nil
}
