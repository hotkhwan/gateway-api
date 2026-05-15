// internal/eventschema/normalized_severity_test.go
package eventschema

import (
	"encoding/json"
	"strings"
	"testing"
)

// Layer C — klynx-api docs/contracts/event-severity-forwarding.md §6.
// Wire JSON tags MUST match the klynx-api copy-by-convention mirror at
// internal/eventbridge/types.go. Any rename / case drift breaks Phase 2
// dashboard severity end-to-end.
func TestNormalizedEvent_SeverityJSONTag(t *testing.T) {
	ev := NormalizedEvent{Severity: "high", EventClass: "intrusion"}
	raw, err := json.Marshal(ev)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	out := string(raw)
	if !strings.Contains(out, `"severity":"high"`) {
		t.Errorf(`expected "severity":"high" in %s`, out)
	}
	if !strings.Contains(out, `"eventClass":"intrusion"`) {
		t.Errorf(`expected "eventClass":"intrusion" in %s`, out)
	}
}

// Empty classification fields must be omitted entirely (omitempty) so
// pre-feature consumers don't see a literal "" they have to ignore — keeps
// the wire compact + makes "field absent" the canonical legacy signal.
func TestNormalizedEvent_OmitemptyOnEmptySeverity(t *testing.T) {
	ev := NormalizedEvent{EventID: "evt-1"}
	raw, err := json.Marshal(ev)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	out := string(raw)
	if strings.Contains(out, `"severity"`) {
		t.Errorf(`expected no severity field in %s`, out)
	}
	if strings.Contains(out, `"eventClass"`) {
		t.Errorf(`expected no eventClass field in %s`, out)
	}
}

// Unmarshal round-trip with both classification fields — locks the legacy
// "no severity" payload AND the new "has severity" payload semantic.
func TestNormalizedEvent_UnmarshalRoundTrip(t *testing.T) {
	cases := []struct {
		name      string
		raw       string
		wantSev   string
		wantClass string
	}{
		{"with-severity",
			`{"eventId":"e1","sourceType":"alarm","sourceFamily":"AIBOX","occurredAt":"2026-05-15T10:03:21Z","receivedAt":"2026-05-15T10:03:21Z","payload":{},"severity":"high","eventClass":"intrusion"}`,
			"high", "intrusion"},
		{"legacy-no-severity",
			`{"eventId":"e2","sourceType":"motion","sourceFamily":"AIBOX","occurredAt":"2026-05-15T10:03:21Z","receivedAt":"2026-05-15T10:03:21Z","payload":{}}`,
			"", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var ev NormalizedEvent
			if err := json.Unmarshal([]byte(tc.raw), &ev); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if ev.Severity != tc.wantSev {
				t.Errorf("Severity = %q, want %q", ev.Severity, tc.wantSev)
			}
			if ev.EventClass != tc.wantClass {
				t.Errorf("EventClass = %q, want %q", ev.EventClass, tc.wantClass)
			}
		})
	}
}
