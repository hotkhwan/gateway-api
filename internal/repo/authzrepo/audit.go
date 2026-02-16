// internal/repo/authzrepo/audit.go
package authzrepo

import (
	"context"
	"os"
	"strconv"
	"strings"

	"github.com/hotkhwan/gateway-api/models/authzmod"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const (
	auditLogsCollection = "audit_logs"
)

// ========== Repo interfaces ==========

type AuditLogRepo interface {
	Create(ctx context.Context, log *authzmod.AuditLog) error
	List(ctx context.Context, filter bson.M, opts ...*options.FindOptions) ([]*authzmod.AuditLog, error)
}

type auditRepo struct {
	coll *mongo.Collection
}

func NewAuditLogRepo(db *mongo.Database) AuditLogRepo {
	return &auditRepo{coll: db.Collection(auditLogsCollection)}
}

func (r *auditRepo) Create(ctx context.Context, log *authzmod.AuditLog) error {
	_, err := r.coll.InsertOne(ctx, log)
	return err
}

func (r *auditRepo) List(ctx context.Context, filter bson.M, opts ...*options.FindOptions) ([]*authzmod.AuditLog, error) {
	cur, err := r.coll.Find(ctx, filter, opts...)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	var out []*authzmod.AuditLog
	if err := cur.All(ctx, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// ========== Index & Retention ==========

// EnsureAuditIndexes สร้างดัชนีที่จำเป็นและตั้ง TTL ตาม retentionDays (เช่น 90)
func EnsureAuditIndexes(ctx context.Context, db *mongo.Database, retentionDays int) error {
	coll := db.Collection(auditLogsCollection)

	// TTL on createdAt
	if retentionDays > 0 {
		secs := int32(retentionDays * 24 * 60 * 60)
		_, _ = coll.Indexes().CreateOne(ctx, mongo.IndexModel{
			Keys: bson.D{{Key: "createdAt", Value: 1}},
			Options: &options.IndexOptions{
				ExpireAfterSeconds: &secs,
				Name:               strPtr("ttl_createdAt"),
			},
		})
	}

	// Common query indexes
	_, _ = coll.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{Keys: bson.D{{Key: "actorId", Value: 1}, {Key: "createdAt", Value: -1}}, Options: &options.IndexOptions{Name: strPtr("ix_actorId_createdAt")}},
		{Keys: bson.D{{Key: "actorName", Value: 1}, {Key: "createdAt", Value: -1}}, Options: &options.IndexOptions{Name: strPtr("ix_actorName_createdAt")}},
		{Keys: bson.D{{Key: "actorIp", Value: 1}, {Key: "createdAt", Value: -1}}, Options: &options.IndexOptions{Name: strPtr("ix_actorIp_createdAt")}},
		{Keys: bson.D{{Key: "operation", Value: 1}, {Key: "createdAt", Value: -1}}, Options: &options.IndexOptions{Name: strPtr("ix_operation_createdAt")}},
		{Keys: bson.D{{Key: "resource", Value: 1}, {Key: "createdAt", Value: -1}}, Options: &options.IndexOptions{Name: strPtr("ix_resource_createdAt")}},
		{Keys: bson.D{{Key: "status", Value: 1}, {Key: "method", Value: 1}, {Key: "createdAt", Value: -1}}, Options: &options.IndexOptions{Name: strPtr("ix_status_method_createdAt")}},
	})

	return nil
}

// LoadAuditRetentionDays อ่าน ENV AUDIT_RETENTION_DAYS (เช่น "90"), default 90
func LoadAuditRetentionDays() int {
	v := strings.TrimSpace(os.Getenv("AUDIT_RETENTION_DAYS"))
	if v == "" {
		return 90
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		return 90
	}
	return n
}

func strPtr(s string) *string { return &s }
