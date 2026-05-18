// models/kctrlmod/registry.go
package kctrlmod

import "time"

// KctrlRegistry is the gateway-api projection of klynx-api's kcontrol approval
// state. One row per approved hwId; klynx-api PATCHes on ApproveDevice and
// DELETEs on UnapproveDevice. The MQTT subscriber (kctrlsubmsg) reads this
// collection to decide enrich / drop / forward per contract §5.
//
// Canonical contract: klynx-api/docs/contracts/kcontrol-gw-managed-registry.md
// v0.1 §3.
//
// Collection: kctrl_registry (Mongo); unique index on hwId.
type KctrlRegistry struct {
	HwId                string    `json:"hwId"                  bson:"hwId"`
	OrgId               string    `json:"orgId"                 bson:"orgId"`
	WorkspaceId         string    `json:"workspaceId,omitempty" bson:"workspaceId,omitempty"`
	Approved            bool      `json:"approved"              bson:"approved"`
	ApprovedAt          time.Time `json:"approvedAt"            bson:"approvedAt"`
	ApprovedBy          string    `json:"approvedBy,omitempty"  bson:"approvedBy,omitempty"`
	LastSyncFromKlynxAt time.Time `json:"lastSyncFromKlynxAt"   bson:"lastSyncFromKlynxAt"`
	LastOutboundHash    string    `json:"lastOutboundHash,omitempty" bson:"lastOutboundHash,omitempty"`
}
