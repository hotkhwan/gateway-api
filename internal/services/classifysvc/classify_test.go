// internal/services/classifysvc/classify_test.go
package classifysvc

import (
	"testing"

	"github.com/hotkhwan/gateway-api/models/ingestmod"
)

func TestMatchesFilter(t *testing.T) {
	payload := map[string]any{
		"listType": float64(3),
		"eventAttribute": map[string]any{
			"name": "Pharrapee",
		},
	}
	cases := []struct {
		name  string
		conds []ingestmod.PayloadCondition
		want  bool
	}{
		{"empty passes", nil, true},
		{"eq match", []ingestmod.PayloadCondition{{Field: "listType", Operator: "eq", Values: []string{"3"}}}, true},
		{"eq no match", []ingestmod.PayloadCondition{{Field: "listType", Operator: "eq", Values: []string{"2"}}}, false},
		{"in match", []ingestmod.PayloadCondition{{Field: "listType", Operator: "in", Values: []string{"2", "3"}}}, true},
		{"in no match", []ingestmod.PayloadCondition{{Field: "listType", Operator: "in", Values: []string{"1", "2"}}}, false},
		{"missing field", []ingestmod.PayloadCondition{{Field: "absent", Operator: "eq", Values: []string{"x"}}}, false},
		{"nested dotted path", []ingestmod.PayloadCondition{{Field: "eventAttribute.name", Operator: "eq", Values: []string{"Pharrapee"}}}, true},
		{"AND all must pass", []ingestmod.PayloadCondition{
			{Field: "listType", Operator: "eq", Values: []string{"3"}},
			{Field: "eventAttribute.name", Operator: "eq", Values: []string{"nope"}},
		}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := MatchesFilter(payload, tc.conds); got != tc.want {
				t.Errorf("MatchesFilter = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestApplyClassificationRules_FirstMatchAndOrder(t *testing.T) {
	rules := []ingestmod.ClassificationRule{
		{Name: "medium", Order: 2, When: []ingestmod.PayloadCondition{{Field: "listType", Operator: "in", Values: []string{"2", "3"}}}, Set: ingestmod.ClassificationSet{EventSeverity: "medium"}},
		{Name: "high", Order: 1, When: []ingestmod.PayloadCondition{{Field: "listType", Operator: "eq", Values: []string{"3"}}}, Set: ingestmod.ClassificationSet{EventSeverity: "high"}},
	}
	ev := &ingestmod.NormalizedEvent{Payload: map[string]any{"listType": float64(3)}}
	ApplyClassificationRules(ev, rules, false)
	// Order 1 (high) sorts first and matches → wins over the medium rule.
	if ev.EventSeverity != "high" {
		t.Errorf("first-match-by-order: got %q want high", ev.EventSeverity)
	}
}

func TestApplyClassificationRules_WithDefaults(t *testing.T) {
	// No rules + withDefaults=true → legacy delivery behavior (unknown/none).
	ev := &ingestmod.NormalizedEvent{Payload: map[string]any{}}
	ApplyClassificationRules(ev, nil, true)
	if ev.EventClass != "unknown" || ev.EventSeverity != "none" {
		t.Errorf("withDefaults=true: got class=%q sev=%q want unknown/none", ev.EventClass, ev.EventSeverity)
	}

	// No rules + withDefaults=false → normalize path leaves them empty.
	ev2 := &ingestmod.NormalizedEvent{Payload: map[string]any{}}
	ApplyClassificationRules(ev2, nil, false)
	if ev2.EventClass != "" || ev2.EventSeverity != "" {
		t.Errorf("withDefaults=false: got class=%q sev=%q want empty", ev2.EventClass, ev2.EventSeverity)
	}
}

func TestWatchlistSeverityDefault(t *testing.T) {
	cases := []struct {
		name    string
		payload map[string]any
		want    string
	}{
		{"flat blacklist 3 → high", map[string]any{"listType": float64(3)}, "high"},
		{"flat redlist 2 → medium", map[string]any{"listType": float64(2)}, "medium"},
		{"flat whitelist 1 → none", map[string]any{"listType": float64(1)}, ""},
		{"flat stranger 0 → none", map[string]any{"listType": float64(0)}, ""},
		{"absent → none", map[string]any{"name": "x"}, ""},
		{"nil payload → none", nil, ""},
		{"int code 3 → high", map[string]any{"listType": 3}, "high"},
		{"string code 3 → high", map[string]any{"listType": "3"}, "high"},
		{"nested eventAttribute.listType 3 → high", map[string]any{"eventAttribute": map[string]any{"listType": float64(3)}}, "high"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := WatchlistSeverityDefault(tc.payload); got != tc.want {
				t.Errorf("WatchlistSeverityDefault = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestRuleOverridesWatchlistDefault mirrors the normalize-path usage: rules run
// first, then the watchlist default fills only when severity is still empty.
func TestRuleOverridesWatchlistDefault(t *testing.T) {
	// A blacklist event whose template rule classifies it as "medium" must NOT
	// be overwritten by the watchlist default (high).
	ev := &ingestmod.NormalizedEvent{Payload: map[string]any{"listType": float64(3)}}
	rules := []ingestmod.ClassificationRule{
		{Name: "downgrade", Order: 1, When: []ingestmod.PayloadCondition{{Field: "listType", Operator: "eq", Values: []string{"3"}}}, Set: ingestmod.ClassificationSet{EventSeverity: "medium"}},
	}
	ApplyClassificationRules(ev, rules, false)
	if ev.EventSeverity == "" {
		ev.EventSeverity = WatchlistSeverityDefault(ev.Payload)
	}
	if ev.EventSeverity != "medium" {
		t.Errorf("template rule should win over watchlist default: got %q want medium", ev.EventSeverity)
	}
}
