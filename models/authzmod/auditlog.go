// models/authzmod/auditlog.go
package authzmod

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type AuditLog struct {
	ID        primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	ActorType string             `bson:"actorType" json:"actorType"`                     // "user", "service", etc.
	ActorId   string             `bson:"actorId" json:"actorId"`                         // sub / client_id / "unknown"
	ActorName string             `bson:"actorName,omitempty" json:"actorName,omitempty"` // preferred_username / client_id / "anonymous"
	ActorIp   string             `bson:"actorIp,omitempty" json:"actorIp,omitempty"`     // real client ip

	Operation string `bson:"operation" json:"operation"`
	Resource  string `bson:"resource" json:"resource"`
	Method    string `bson:"method" json:"method"`
	Path      string `bson:"path" json:"path"`
	Query     string `bson:"query,omitempty" json:"query,omitempty"`
	Status    int    `bson:"status" json:"status"`
	LatencyMs int64  `bson:"latencyMs" json:"latencyMs"`

	Payload  any    `bson:"payload,omitempty" json:"payload,omitempty"`
	Response any    `bson:"response,omitempty" json:"response,omitempty"`
	TraceId  string `bson:"traceId,omitempty" json:"traceId,omitempty"`

	CreatedAt time.Time `bson:"createdAt" json:"createdAt"`
}
