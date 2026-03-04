// models/kctrlmod/alarmsLogs.go
package kctrlmod

import "time"

type AlarmAuditLogItem struct {
	Id        string `json:"id"`
	ActorType string `json:"actorType"`
	ActorId   string `json:"actorId"`
	ActorName string `json:"actorName,omitempty"`
	ActorIp   string `json:"actorIp,omitempty"`

	Operation string `json:"operation"`
	Resource  string `json:"resource"`
	Method    string `json:"method"`
	Path      string `json:"path"`
	Status    int    `json:"status"`
	LatencyMs int64  `json:"latencyMs"`

	Payload   any       `json:"payload,omitempty"`
	TraceId   string    `json:"traceId,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
}
