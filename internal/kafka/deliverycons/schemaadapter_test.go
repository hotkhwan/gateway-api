// internal/kafka/deliverycons/schemaadapter_test.go
package deliverycons

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/hotkhwan/gateway-api/internal/eventschema"
)

// TestFromEventSchema_FromRealKafkaFixture round-trips the captured wire
// payload through FromEventSchema and verifies every relevant field lands
// correctly on the ingestmod shape. Uses the same real-wire fixture as the
// narrow-v2 adapter test (wide slice reuses this evidence).
func TestFromEventSchema_FromRealKafkaFixture(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "gw_events_normalized_v1_sample.json"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	var src eventschema.NormalizedEvent
	if err := json.Unmarshal(raw, &src); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	out := FromEventSchema(&src)
	if out == nil {
		t.Fatal("FromEventSchema returned nil")
	}

	// Root fields
	if out.EventId != src.EventID {
		t.Errorf("EventId mismatch")
	}
	if out.TenantId != src.OrgID {
		t.Errorf("TenantId should map from OrgID")
	}
	if out.EventType != src.SourceType {
		t.Errorf("EventType = %q, want %q", out.EventType, src.SourceType)
	}
	if out.EventCategory != src.SourceCategory {
		t.Errorf("EventCategory mismatch")
	}
	if out.EventAction != src.SourceAction {
		t.Errorf("EventAction mismatch")
	}
	if !out.OccurredAt.Equal(src.OccurredAt) {
		t.Errorf("OccurredAt mismatch")
	}
	if out.Meta.NormalizedAt != src.ReceivedAt {
		t.Errorf("Meta.NormalizedAt should come from ReceivedAt")
	}
	if out.Meta.SchemaVersion != src.SchemaVersion {
		t.Errorf("Meta.SchemaVersion mismatch")
	}
	if out.Meta.TemplateId != src.TemplateID {
		t.Errorf("Meta.TemplateId mismatch")
	}

	// Source block — these are the fields the narrow bridge could NOT deliver.
	// Wide slice MUST preserve them.
	if out.Source.DeviceId != src.Source.DeviceID {
		t.Errorf("Source.DeviceId mismatch")
	}
	if out.Source.DeviceMgmtId != src.Source.DeviceMgmtID {
		t.Errorf("Source.DeviceMgmtId should land via FromEventSchema; got %q want %q",
			out.Source.DeviceMgmtId, src.Source.DeviceMgmtID)
	}
	if out.Source.SN != src.Source.SN {
		t.Errorf("Source.SN mismatch: got %q want %q", out.Source.SN, src.Source.SN)
	}
	if out.Source.EdgeName != src.Source.EdgeName {
		t.Errorf("Source.EdgeName mismatch: got %q want %q", out.Source.EdgeName, src.Source.EdgeName)
	}
	if out.Source.OrgId != src.Source.OrgID {
		t.Errorf("Source.OrgId mismatch: got %q want %q", out.Source.OrgId, src.Source.OrgID)
	}
	if out.Source.DeviceType != src.SourceFamily {
		t.Errorf("Source.DeviceType should carry SourceFamily: got %q want %q",
			out.Source.DeviceType, src.SourceFamily)
	}
	if out.Source.WorkspaceId != src.Source.WorkspaceID {
		t.Errorf("Source.WorkspaceId mismatch")
	}

	// Location / Geo / GeoCell / Payload / BinaryRefs — hydrate verification
	if out.Location.Zone != src.Location.Zone {
		t.Errorf("Location.Zone mismatch")
	}
	if out.Geo.AdminCode != src.Geo.AdminCode {
		t.Errorf("Geo.AdminCode mismatch")
	}
	if out.GeoCell.Cell != src.GeoCell.Cell {
		t.Errorf("GeoCell.Cell mismatch")
	}
	if len(out.BinaryRefs) != len(src.BinaryRefs) {
		t.Errorf("BinaryRefs count mismatch: got %d want %d", len(out.BinaryRefs), len(src.BinaryRefs))
	}
	if _, ok := out.ByAdminArea["TH-83"]; !ok {
		t.Errorf("ByAdminArea did not carry TH-83 key")
	}
	if out.Payload["helmet_label"] != "Not Wearing Helmet" {
		t.Errorf("Payload not preserved")
	}
}

func TestFromEventSchema_NilInput(t *testing.T) {
	if FromEventSchema(nil) != nil {
		t.Error("expected nil output for nil input")
	}
}

func TestFromEventSchema_NestedNilsTolerated(t *testing.T) {
	src := &eventschema.NormalizedEvent{
		EventID:     "evt-1",
		WorkspaceID: "ws-1",
		// Source / Location / Geo / GeoCell all nil
	}
	out := FromEventSchema(src)
	if out == nil {
		t.Fatal("unexpected nil output")
	}
	if out.EventId != "evt-1" {
		t.Error("root EventId lost")
	}
	if out.Source.WorkspaceId != "ws-1" {
		t.Error("root WorkspaceID should populate Source.WorkspaceId when Source block is nil")
	}
}

// TestFromEventSchema_NestedSourceWorkspaceIdWins guards the invariant that
// when both root WorkspaceID and Source.WorkspaceID are set, the nested one
// wins (canonical is the nested field).
func TestFromEventSchema_NestedSourceWorkspaceIdWins(t *testing.T) {
	src := &eventschema.NormalizedEvent{
		WorkspaceID: "ROOT",
		Source: &eventschema.NormalizedSource{
			WorkspaceID: "NESTED",
		},
	}
	out := FromEventSchema(src)
	if out.Source.WorkspaceId != "NESTED" {
		t.Errorf("nested WorkspaceID should win; got %q", out.Source.WorkspaceId)
	}
}

// Layer C — klynx-api docs/contracts/event-severity-forwarding.md §6 wire
// adapter rule: when the wire payload carries severity (producer stamped at
// buildBridgeEvent time), FromEventSchema must map it back onto the
// ingestmod shape so the delivery consumer reuses the value verbatim
// instead of re-running classification rules.
func TestFromEventSchema_CarriesSeverityAndEventClass(t *testing.T) {
	src := &eventschema.NormalizedEvent{
		EventID:    "evt-sev-1",
		Severity:   "high",
		EventClass: "intrusion",
	}
	out := FromEventSchema(src)
	if out == nil {
		t.Fatal("unexpected nil output")
	}
	if out.EventSeverity != "high" {
		t.Errorf("EventSeverity = %q, want high", out.EventSeverity)
	}
	if out.EventClass != "intrusion" {
		t.Errorf("EventClass = %q, want intrusion", out.EventClass)
	}
}

// Pre-feature wire payload (no severity) maps to empty ingestmod fields;
// existing filter.go fallback then re-runs classification rules. Locks the
// "empty wire severity triggers re-classify fallback" semantic preserved
// from contract §6 (Codex round-1 lock).
func TestFromEventSchema_EmptySeverityPassesThrough(t *testing.T) {
	src := &eventschema.NormalizedEvent{
		EventID: "evt-sev-empty",
		// Severity + EventClass intentionally left empty (zero value).
	}
	out := FromEventSchema(src)
	if out.EventSeverity != "" {
		t.Errorf("EventSeverity should be empty, got %q", out.EventSeverity)
	}
	if out.EventClass != "" {
		t.Errorf("EventClass should be empty, got %q", out.EventClass)
	}
}
