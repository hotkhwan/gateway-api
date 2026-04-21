// internal/services/ingestsvc/templateMatch_test.go
package ingestsvc

import (
	"testing"

	"github.com/hotkhwan/gateway-api/internal/services/templatematcher"
	"github.com/hotkhwan/gateway-api/models/ingestmod"
)

// These tests regression-guard the ingest-time selector after moving the
// evaluator into the shared templatematcher package (plan decisions D5, D6).
//
// The key contract preserved from the pre-refactor behavior:
//   - `raw.<field>` conditions resolve against the raw payload.
//   - bare `<field>` conditions also resolve against the raw payload.
//   - `source.<field>` conditions resolve against the canonical source map.
//
// Before the refactor, the ingest-specific evaluator stripped the optional
// "raw." prefix inline. The shared matcher is pure; buildMatchBag mirrors the
// raw payload under a "raw" namespace so both forms resolve naturally.

func TestIngest_RawPrefixResolvesViaSharedMatcher(t *testing.T) {
	rawBody := map[string]any{
		"deviceId":  "51",
		"channelId": 7,
	}
	deviceRef := &ingestmod.DeviceIdentity{ID: "51", Type: "camera"}
	bag := buildMatchBag(rawBody, deviceRef, nil, "AIBOX")

	// raw.deviceId — prefixed form
	if !templatematcher.Evaluate([]ingestmod.MatchCondition{{Field: "raw.deviceId", Operator: "eq", Values: []string{"51"}}}, nil, bag) {
		t.Fatal("raw.deviceId should resolve through the raw namespace")
	}
	// deviceId — bare form (raw fields are mirrored at top level too)
	if !templatematcher.Evaluate([]ingestmod.MatchCondition{{Field: "deviceId", Operator: "eq", Values: []string{"51"}}}, nil, bag) {
		t.Fatal("bare deviceId should resolve at top level")
	}
}

func TestIngest_SourceNamespaceResolves(t *testing.T) {
	deviceRef := &ingestmod.DeviceIdentity{ID: "51", Type: "camera"}
	bag := buildMatchBag(map[string]any{}, deviceRef, nil, "AIBOX")

	cases := []ingestmod.MatchCondition{
		{Field: "source.deviceId", Operator: "eq", Values: []string{"51"}},
		{Field: "source.sourceFamily", Operator: "eq", Values: []string{"AIBOX"}},
	}
	for _, c := range cases {
		if !templatematcher.Evaluate([]ingestmod.MatchCondition{c}, nil, bag) {
			t.Errorf("%s should resolve via canonical source map", c.Field)
		}
	}
}

func TestIngest_EnrichmentFieldsResolveUnderSource(t *testing.T) {
	deviceRef := &ingestmod.DeviceIdentity{ID: "51", Type: "camera"}
	enrichment := &ingestmod.DeviceManagement{
		DeviceMgmtId: "dm-1",
		SerialNo:     "SN-9",
		Zone:         "PHK",
	}
	bag := buildMatchBag(map[string]any{"a": "b"}, deviceRef, enrichment, "AIBOX")

	checks := map[string]string{
		"source.deviceMgmtId": "dm-1",
		"source.sn":           "SN-9",
		"source.zone":         "PHK",
	}
	for field, want := range checks {
		cond := ingestmod.MatchCondition{Field: field, Operator: "eq", Values: []string{want}}
		if !templatematcher.Evaluate([]ingestmod.MatchCondition{cond}, nil, bag) {
			t.Errorf("%s should resolve to %q", field, want)
		}
	}
}

func TestIngest_UnmatchedTemplateReturnsFalse(t *testing.T) {
	rawBody := map[string]any{"deviceId": "51"}
	bag := buildMatchBag(rawBody, &ingestmod.DeviceIdentity{ID: "51"}, nil, "AIBOX")

	// Condition asks for a different deviceId
	conds := []ingestmod.MatchCondition{{Field: "raw.deviceId", Operator: "eq", Values: []string{"999"}}}
	if templatematcher.Evaluate(conds, nil, bag) {
		t.Fatal("mismatched deviceId should not pass")
	}
}
