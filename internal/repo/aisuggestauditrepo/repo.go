// internal/repo/aisuggestauditrepo/repo.go
package aisuggestauditrepo

import (
	"context"
	"time"

	"github.com/hotkhwan/gateway-api/config"
	"github.com/hotkhwan/gateway-api/internal/repo/stomongo"
	"go.mongodb.org/mongo-driver/bson"
)

const collection = "ai_suggest_audit"

// AISuggestAuditEntry records metadata about an AI suggestion request.
// Contains NO payload, NO AI output, NO secrets.
type AISuggestAuditEntry struct {
	WorkspaceID   string    `bson:"workspaceId"`
	UserID        string    `bson:"userId"`
	Provider      string    `bson:"provider"`
	Model         string    `bson:"model"`
	PromptVersion string    `bson:"promptVersion"`
	Mode          string    `bson:"mode"` // aiAugmented | systemOnly | aiFailedFallback
	SourceFamily  string    `bson:"sourceFamily"`
	LatencyMs     int64     `bson:"latencyMs"`
	ParseSuccess  bool      `bson:"parseSuccess"`
	ConflictCount int       `bson:"conflictCount"`
	CreatedAt     time.Time `bson:"createdAt"`
	ExpiresAt     time.Time `bson:"expiresAt"` // TTL index — expire after 90 days
}

type AISuggestAuditRepo struct{}

func NewAISuggestAuditRepo() *AISuggestAuditRepo { return &AISuggestAuditRepo{} }

// EnsureIndexes creates a TTL index on expiresAt.
func (r *AISuggestAuditRepo) EnsureIndexes(ctx context.Context) error {
	return stomongo.EnsureTTLIndex(
		ctx,
		collection,
		bson.D{{Key: "expiresAt", Value: 1}},
		"idx_ai_suggest_audit_expires_at",
		90*24*time.Hour,
	)
}

// Save inserts a new audit entry into the collection.
func (r *AISuggestAuditRepo) Save(ctx context.Context, entry AISuggestAuditEntry) error {
	_, err := stomongo.InsertOne(ctx, collection, entry)
	return err
}

func init() {
	config.RegisterMongoBootstrap(func(ctx context.Context) error {
		return NewAISuggestAuditRepo().EnsureIndexes(ctx)
	})
}
