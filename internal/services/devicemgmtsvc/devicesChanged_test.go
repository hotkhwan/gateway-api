// internal/services/devicemgmtsvc/devicesChanged_test.go
package devicemgmtsvc

import (
	"testing"

	"github.com/hotkhwan/gateway-api/models/ingestmod"
)

func TestMapChangeType(t *testing.T) {
	cases := map[string]string{
		"create": "created", "created": "created",
		"update": "updated", "updated": "updated",
		"delete": "deleted", "deleted": "deleted",
		"weird": "weird",
	}
	for in, want := range cases {
		if got := mapChangeType(in); got != want {
			t.Errorf("mapChangeType(%q) = %q, want %q", in, got, want)
		}
	}
}

// The klynx-api eventbridge.DeviceChangedEvent consumer decodes these json keys.
// Emitting the wrong names (entityId/action/workspaceId) leaves RemoteDeviceID
// empty and SyncFromGW skips — the duplicate-projection defect. Lock the schema.
func TestDevicesChangedPayload_WireSchema(t *testing.T) {
	d := &ingestmod.DeviceManagement{
		DeviceMgmtId: "dm-1",
		TenantId:     "klynx",
		WorkspaceId:  "ws-1",
		SourceFamily: "dahua",
		EntityType:   "camera",
		EntityId:     "cam-1",
		DeviceId:     "cam-1",
		Name:         "LRP",
		Lat:          13.75,
		Lng:          100.5,
	}
	p := devicesChangedPayload(d, "created")

	// Join field MUST be remoteDeviceId == deviceId (the camId join key).
	if p["remoteDeviceId"] != "cam-1" {
		t.Errorf("remoteDeviceId = %v, want cam-1", p["remoteDeviceId"])
	}
	// orgId + gwWorkspaceId both carry the workspace (klynx org == gw workspace).
	if p["orgId"] != "ws-1" || p["gwWorkspaceId"] != "ws-1" {
		t.Errorf("orgId/gwWorkspaceId = %v/%v, want ws-1/ws-1", p["orgId"], p["gwWorkspaceId"])
	}
	if p["changeType"] != "created" {
		t.Errorf("changeType = %v, want created", p["changeType"])
	}
	// All fields the consumer reads must be present (non-nil).
	for _, k := range []string{"remoteDeviceId", "deviceId", "changeType", "orgId", "gwWorkspaceId", "deviceMgmtId", "tenantId", "sourceFamily", "name"} {
		if _, ok := p[k]; !ok {
			t.Errorf("payload missing required key %q", k)
		}
	}
}

func TestDevicesChangedPayload_RemoteDeviceIdFallsBackToEntityId(t *testing.T) {
	// Reactive channel-keyed record with no DeviceId alias → remoteDeviceId = entityId.
	d := &ingestmod.DeviceManagement{
		WorkspaceId: "ws-1",
		EntityType:  "channel",
		EntityId:    "56",
	}
	p := devicesChangedPayload(d, "created")
	if p["remoteDeviceId"] != "56" {
		t.Errorf("remoteDeviceId = %v, want 56 (entityId fallback)", p["remoteDeviceId"])
	}
}
