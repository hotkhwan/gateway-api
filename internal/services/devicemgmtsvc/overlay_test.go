// internal/services/devicemgmtsvc/overlay_test.go
package devicemgmtsvc

import (
	"testing"
)

// Service-layer integration via the public Apply path requires Mongo + Kafka
// (publishDevicesChanged is non-blocking but reaches config.SendToKafkaWithCtx).
// We test the validation + hash logic in isolation here — handler-level tests
// drive the Apply flow against a fake service.

func TestClassifyIfMatch(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		supplied string
		current  string
		want     IfMatchStatus
	}{
		{"absent", "", "abc123", IfMatchAbsent},
		{"absent_both_empty", "", "", IfMatchAbsent},
		{"matched", "abc123", "abc123", IfMatchMatched},
		{"mismatched", "abc123", "def456", IfMatchMismatched},
		{"mismatched_current_empty", "abc123", "", IfMatchMismatched},
	}
	for _, tc := range cases {
		got := classifyIfMatch(tc.supplied, tc.current)
		if got != tc.want {
			t.Errorf("%s: classifyIfMatch(%q, %q) = %q, want %q", tc.name, tc.supplied, tc.current, got, tc.want)
		}
	}
}

func TestCanonicalHash_Deterministic(t *testing.T) {
	t.Parallel()

	// Same accepted fields in different insertion order must produce the same hash —
	// canonical ordering is the whole point.
	a := map[string]any{"name": "Cam1", "lat": 12.5, "lng": 100.5}
	b := map[string]any{"lng": 100.5, "name": "Cam1", "lat": 12.5}

	ha, err := canonicalHash(a)
	if err != nil {
		t.Fatal(err)
	}
	hb, err := canonicalHash(b)
	if err != nil {
		t.Fatal(err)
	}
	if ha != hb {
		t.Errorf("canonicalHash not deterministic across key order: %q vs %q", ha, hb)
	}
}

func TestCanonicalHash_DifferentValuesProduceDifferentHash(t *testing.T) {
	t.Parallel()

	a := map[string]any{"name": "Cam1"}
	b := map[string]any{"name": "Cam2"}

	ha, _ := canonicalHash(a)
	hb, _ := canonicalHash(b)
	if ha == hb {
		t.Errorf("canonicalHash collided for distinct values: %q", ha)
	}
}

func TestValidateAcceptedField_Strings(t *testing.T) {
	t.Parallel()

	for _, field := range []string{"name", "description", "site", "zone", "serialNo"} {
		if err := validateAcceptedField(field, "ok"); err != nil {
			t.Errorf("%s: string accepted but got error: %v", field, err)
		}
		if err := validateAcceptedField(field, ""); err != nil {
			t.Errorf("%s: empty string should be allowed (per §8.3 set-to-empty), got %v", field, err)
		}
		if err := validateAcceptedField(field, 42); err == nil {
			t.Errorf("%s: int should reject (must be string)", field)
		}
		if err := validateAcceptedField(field, nil); err == nil {
			t.Errorf("%s: null should reject (per §8.3 null → 400 VALIDATION_FAILED)", field)
		}
	}
}

func TestValidateAcceptedField_Numbers(t *testing.T) {
	t.Parallel()

	for _, field := range []string{"lat", "lng"} {
		if err := validateAcceptedField(field, 12.5); err != nil {
			t.Errorf("%s: float64 should accept, got %v", field, err)
		}
		if err := validateAcceptedField(field, 10); err != nil {
			t.Errorf("%s: int should accept, got %v", field, err)
		}
		if err := validateAcceptedField(field, "12.5"); err == nil {
			t.Errorf("%s: string should reject (per §8.3 wrong-type)", field)
		}
		if err := validateAcceptedField(field, nil); err == nil {
			t.Errorf("%s: null should reject", field)
		}
	}
}

func TestFieldWhitelistClassification(t *testing.T) {
	t.Parallel()

	// Sanity-check the 3-way classification used by Apply.
	cases := []struct {
		field   string
		expect  string // "accepted" | "notAccepted" | "readonly" | "metadata" | "unknown"
	}{
		{"name", "accepted"},
		{"description", "accepted"},
		{"lat", "accepted"},
		{"lng", "accepted"},
		{"site", "accepted"},
		{"zone", "accepted"},
		{"serialNo", "accepted"},
		// Stream credentials + brand/ip/district/location/type — klynx canonical, rejected.
		{"url", "notAccepted"},
		{"user", "notAccepted"},
		{"password", "notAccepted"},
		{"streamUrl", "notAccepted"},
		{"streamUser", "notAccepted"},
		{"streamPassword", "notAccepted"},
		{"brand", "notAccepted"},
		{"ip", "notAccepted"},
		{"district", "notAccepted"},
		{"location", "notAccepted"},
		{"type", "notAccepted"},
		// System / path-param fields — readonly over PATCH.
		{"deviceMgmtId", "readonly"},
		{"tenantId", "readonly"},
		{"workspaceId", "readonly"},
		{"sourceFamily", "readonly"},
		{"entityType", "readonly"},
		{"entityId", "readonly"},
		{"deviceId", "readonly"},
		{"createdAt", "readonly"},
		{"updatedAt", "readonly"},
		{"lastOutboundHash", "readonly"},
		// Metadata-only — accepted by parser, not persisted.
		{"klynxEditAt", "metadata"},
		// Random unknown.
		{"frobnicate", "unknown"},
	}
	for _, tc := range cases {
		var got string
		switch {
		case isAccepted(tc.field):
			got = "accepted"
		case isMetadataOnly(tc.field):
			got = "metadata"
		case isNotAccepted(tc.field):
			got = "notAccepted"
		case isReadonly(tc.field):
			got = "readonly"
		default:
			got = "unknown"
		}
		if got != tc.expect {
			t.Errorf("%s: classified as %q, want %q", tc.field, got, tc.expect)
		}
	}
}
