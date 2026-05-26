// internal/services/classification/classify_test.go
package classification

import (
	"testing"

	"github.com/hotkhwan/gateway-api/models/ingestmod"
)

// --- Matches: path-resolution rule (contract §5A.3) ---

func TestMatches_PayloadPrefix_StripsAndResolves(t *testing.T) {
	payload := map[string]any{"listType": 3}
	conds := []ingestmod.PayloadCondition{
		{Field: "payload.listType", Operator: "eq", Values: []string{"3"}},
	}
	if !Matches(payload, conds) {
		t.Fatalf("expected payload.listType to resolve to payload[listType]")
	}
}

func TestMatches_BarePath_BackwardCompat(t *testing.T) {
	payload := map[string]any{"listType": 3}
	conds := []ingestmod.PayloadCondition{
		{Field: "listType", Operator: "eq", Values: []string{"3"}},
	}
	if !Matches(payload, conds) {
		t.Fatalf("expected legacy bare path 'listType' to still resolve")
	}
}

func TestMatches_NestedWithPrefix(t *testing.T) {
	payload := map[string]any{
		"eventAttribute": map[string]any{"listType": 3},
	}
	conds := []ingestmod.PayloadCondition{
		{Field: "payload.eventAttribute.listType", Operator: "eq", Values: []string{"3"}},
	}
	if !Matches(payload, conds) {
		t.Fatalf("expected payload.eventAttribute.listType to resolve deeply")
	}
}

func TestMatches_NestedBare(t *testing.T) {
	payload := map[string]any{
		"eventAttribute": map[string]any{"listType": 3},
	}
	conds := []ingestmod.PayloadCondition{
		{Field: "eventAttribute.listType", Operator: "eq", Values: []string{"3"}},
	}
	if !Matches(payload, conds) {
		t.Fatalf("expected legacy nested bare 'eventAttribute.listType' to resolve")
	}
}

func TestMatches_PayloadAlone_NoMatch(t *testing.T) {
	payload := map[string]any{"listType": 3}
	conds := []ingestmod.PayloadCondition{
		{Field: "payload", Operator: "eq", Values: []string{"anything"}},
	}
	// "payload" alone strips to "" → resolveField returns (nil, false)
	// → no match — matches contract intent (PATCH validation will reject this).
	if Matches(payload, conds) {
		t.Fatalf("expected bare 'payload' to not match")
	}
}

func TestMatches_ReservedRoot_SourceField_NoMatch(t *testing.T) {
	payload := map[string]any{"listType": 3}
	conds := []ingestmod.PayloadCondition{
		// "source.deviceId" is a reserved root; at eval time it tries to find
		// key "source" inside payload, doesn't exist → no match. Phase 1 PATCH
		// validation will reject this at write-time with INVALID_ARGUMENT.
		{Field: "source.deviceId", Operator: "eq", Values: []string{"D1"}},
	}
	if Matches(payload, conds) {
		t.Fatalf("expected reserved 'source.deviceId' to not match (no such key in payload)")
	}
}

func TestMatches_EmptyField_NoMatch(t *testing.T) {
	payload := map[string]any{"listType": 3}
	conds := []ingestmod.PayloadCondition{
		{Field: "", Operator: "eq", Values: []string{"3"}},
	}
	if Matches(payload, conds) {
		t.Fatalf("expected empty field to not match")
	}
}

// --- Matches: operator semantics ---

func TestMatches_OperatorIn(t *testing.T) {
	payload := map[string]any{"listType": 2}
	conds := []ingestmod.PayloadCondition{
		{Field: "payload.listType", Operator: "in", Values: []string{"1", "2", "3"}},
	}
	if !Matches(payload, conds) {
		t.Fatalf("expected 'in' to match 2 in {1,2,3}")
	}
}

func TestMatches_OperatorIn_NoMatch(t *testing.T) {
	payload := map[string]any{"listType": 9}
	conds := []ingestmod.PayloadCondition{
		{Field: "payload.listType", Operator: "in", Values: []string{"1", "2", "3"}},
	}
	if Matches(payload, conds) {
		t.Fatalf("expected 'in' to reject 9")
	}
}

func TestMatches_UnsupportedOperator_LenientPass(t *testing.T) {
	// Preserves pre-refactor delivery semantic: unsupported operators are
	// treated as if the condition wasn't there. PATCH validation will block
	// these at write-time.
	payload := map[string]any{"listType": 3}
	conds := []ingestmod.PayloadCondition{
		{Field: "payload.listType", Operator: "neq", Values: []string{"3"}},
	}
	if !Matches(payload, conds) {
		t.Fatalf("unsupported operator should be lenient pass to preserve original semantic")
	}
}

func TestMatches_AndLogic_AllMustPass(t *testing.T) {
	payload := map[string]any{"listType": 3, "name": "alice"}
	conds := []ingestmod.PayloadCondition{
		{Field: "payload.listType", Operator: "eq", Values: []string{"3"}},
		{Field: "payload.name", Operator: "eq", Values: []string{"alice"}},
	}
	if !Matches(payload, conds) {
		t.Fatalf("AND logic: both must pass")
	}
	conds[1].Values = []string{"bob"}
	if Matches(payload, conds) {
		t.Fatalf("AND logic: one fail → overall fail")
	}
}

func TestMatches_EmptyConditions_AlwaysPass(t *testing.T) {
	if !Matches(nil, nil) {
		t.Fatalf("empty conditions should always pass")
	}
}

// --- Apply: end-to-end rule evaluation ---

func TestApply_FirstMatchWins(t *testing.T) {
	event := &ingestmod.NormalizedEvent{
		Payload: map[string]any{"listType": 3},
	}
	rules := []ingestmod.ClassificationRule{
		{
			Name:  "blacklist-high",
			Order: 1,
			When: []ingestmod.PayloadCondition{
				{Field: "payload.listType", Operator: "eq", Values: []string{"3"}},
			},
			Set: ingestmod.ClassificationSet{EventClass: "security", EventSeverity: "high"},
		},
		{
			Name:  "fallback-low",
			Order: 2,
			When:  nil, // empty → always matches
			Set:   ingestmod.ClassificationSet{EventClass: "info", EventSeverity: "low"},
		},
	}
	Apply(event, rules)
	if event.EventClass != "security" || event.EventSeverity != "high" {
		t.Fatalf("first match should win: got class=%q severity=%q", event.EventClass, event.EventSeverity)
	}
}

func TestApply_DefaultsWhenNoMatch(t *testing.T) {
	event := &ingestmod.NormalizedEvent{
		Payload: map[string]any{"listType": 99},
	}
	rules := []ingestmod.ClassificationRule{
		{
			Name: "no-match",
			When: []ingestmod.PayloadCondition{
				{Field: "payload.listType", Operator: "eq", Values: []string{"3"}},
			},
			Set: ingestmod.ClassificationSet{EventClass: "security", EventSeverity: "high"},
		},
	}
	Apply(event, rules)
	if event.EventClass != "unknown" || event.EventSeverity != "none" {
		t.Fatalf("no match should default to unknown/none: got %q/%q", event.EventClass, event.EventSeverity)
	}
}

func TestApply_NoRules_DefaultsOnly(t *testing.T) {
	event := &ingestmod.NormalizedEvent{}
	Apply(event, nil)
	if event.EventClass != "unknown" || event.EventSeverity != "none" {
		t.Fatalf("no rules should still apply defaults")
	}
}

func TestApply_Idempotent(t *testing.T) {
	event := &ingestmod.NormalizedEvent{
		Payload: map[string]any{"listType": 3},
	}
	rules := []ingestmod.ClassificationRule{
		{
			Name: "r1",
			When: []ingestmod.PayloadCondition{
				{Field: "payload.listType", Operator: "eq", Values: []string{"3"}},
			},
			Set: ingestmod.ClassificationSet{EventClass: "security", EventSeverity: "high"},
		},
	}
	Apply(event, rules)
	gotClass, gotSev := event.EventClass, event.EventSeverity
	Apply(event, rules)
	if event.EventClass != gotClass || event.EventSeverity != gotSev {
		t.Fatalf("Apply not idempotent: first=(%q,%q) second=(%q,%q)",
			gotClass, gotSev, event.EventClass, event.EventSeverity)
	}
}

func TestApply_OrderRespected(t *testing.T) {
	// Rules supplied out of order — Apply should sort by Order ascending,
	// so order=1 fires before order=2 even if it appears later in the slice.
	event := &ingestmod.NormalizedEvent{
		Payload: map[string]any{"listType": 3},
	}
	rules := []ingestmod.ClassificationRule{
		{
			Name:  "later-rule",
			Order: 2,
			When: []ingestmod.PayloadCondition{
				{Field: "payload.listType", Operator: "eq", Values: []string{"3"}},
			},
			Set: ingestmod.ClassificationSet{EventClass: "B", EventSeverity: "medium"},
		},
		{
			Name:  "earlier-rule",
			Order: 1,
			When: []ingestmod.PayloadCondition{
				{Field: "payload.listType", Operator: "eq", Values: []string{"3"}},
			},
			Set: ingestmod.ClassificationSet{EventClass: "A", EventSeverity: "high"},
		},
	}
	Apply(event, rules)
	if event.EventClass != "A" || event.EventSeverity != "high" {
		t.Fatalf("expected order=1 rule to win: got %q/%q", event.EventClass, event.EventSeverity)
	}
}

func TestApply_NilEvent_NoPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Apply(nil, ...) should not panic, got: %v", r)
		}
	}()
	Apply(nil, []ingestmod.ClassificationRule{
		{Name: "x", When: nil, Set: ingestmod.ClassificationSet{EventClass: "c"}},
	})
}

func TestApply_AIBOXBlacklistScenario(t *testing.T) {
	// End-to-end mirror of contract §13.1 example.
	event := &ingestmod.NormalizedEvent{
		Payload: map[string]any{"listType": 3, "similarity": 0.95},
	}
	rules := []ingestmod.ClassificationRule{
		{
			Name: "blacklist-high", Order: 1,
			When: []ingestmod.PayloadCondition{
				{Field: "payload.listType", Operator: "eq", Values: []string{"3"}},
			},
			Set: ingestmod.ClassificationSet{EventClass: "security", EventSeverity: "high"},
		},
		{
			Name: "redlist-medium", Order: 2,
			When: []ingestmod.PayloadCondition{
				{Field: "payload.listType", Operator: "eq", Values: []string{"2"}},
			},
			Set: ingestmod.ClassificationSet{EventClass: "security", EventSeverity: "medium"},
		},
		{
			Name: "whitelist-low", Order: 3,
			When: []ingestmod.PayloadCondition{
				{Field: "payload.listType", Operator: "in", Values: []string{"0", "1"}},
			},
			Set: ingestmod.ClassificationSet{EventClass: "info", EventSeverity: "low"},
		},
	}
	Apply(event, rules)
	if event.EventClass != "security" || event.EventSeverity != "high" {
		t.Fatalf("AIBOX blacklist (listType=3) should classify as security/high: got %q/%q",
			event.EventClass, event.EventSeverity)
	}
}
