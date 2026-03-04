// models/subscripmod/license.go
package subscripmod

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// LicenseStatus defines license key status
type LicenseStatus string

const (
	LicenseStatusAvailable LicenseStatus = "available"
	LicenseStatusActivated LicenseStatus = "activated"
	LicenseStatusRevoked   LicenseStatus = "revoked"
)

// LicenseKey represents an issued enterprise license key
type LicenseKey struct {
	ID     primitive.ObjectID `bson:"_id"`
	Key    string             `bson:"key"`    // The license key (XXX-YYY-ZZZ)
	PlanId string             `bson:"planId"` // "enterprise"

	// Custom limits for this license (optional - uses plan defaults if nil)
	Limits *SubscriptionLimits `bson:"limits,omitempty"`

	// Activation tracking
	TenantId *string       `bson:"tenantId,omitempty"` // Set when activated
	Status   LicenseStatus `bson:"status"`             // "available", "activated", "revoked"

	// Timestamps
	CreatedAt   time.Time  `bson:"createdAt"`
	ActivatedAt *time.Time `bson:"activatedAt,omitempty"`

	// Optional notes for admin tracking
	Notes *string `bson:"notes,omitempty"`
}
