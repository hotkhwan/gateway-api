// models/authzmod/org.go
package authzmod

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// OrgIngestConfig เก็บ config สำหรับ hot-path ingest endpoint /events/{orgId}
type OrgIngestConfig struct {
	IngestKey         string `bson:"ingestKey"`         // random secret (HMAC signing key)
	SignatureRequired bool   `bson:"signatureRequired"` // require X-Signature header
	RateLimitPerSec   int    `bson:"rateLimitPerSec"`   // max req/sec per org (default 10)
	RateLimitBurst    int    `bson:"rateLimitBurst"`    // burst allowance (default 20)
}

type Organization struct {
	ID    primitive.ObjectID `bson:"_id,omitempty"` // internal
	OrgId string             `bson:"orgId"`         // external stable id

	TenantId    string `bson:"tenantId"`
	Name        string `bson:"name"`
	Description string `bson:"description"`

	IngestConfig OrgIngestConfig `bson:"ingestConfig"` // config สำหรับ ingest endpoint

	CreatedBy string    `bson:"createdBy"`
	CreatedAt time.Time `bson:"createdAt"` // ✅ BSON Date
	UpdatedBy string    `bson:"updatedBy"`
	UpdatedAt time.Time `bson:"updatedAt"` // ✅ BSON Date

	// NEW: Billing owner for billing responsibility (separate from ownership)
	BillingOwnerId string `bson:"billingOwnerId,omitempty"`
	// NEW: Track if org is orphaned (no owners or no effective managers)
	IsOrphaned bool `bson:"isOrphaned,omitempty"`

	// NEW: Membership version for optimistic locking (race protection)
	// Increment on any membership change (invite, remove, promote, demote)
	MembershipVersion int `bson:"membershipVersion,omitempty"`

	SyncStatus string `bson:"syncStatus"`
}
