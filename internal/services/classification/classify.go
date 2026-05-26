// internal/services/classification/classify.go
//
// Package classification evaluates per-template ClassificationRules to derive
// the canonical eventClass / eventSeverity on a NormalizedEvent. It is the
// single source of rule evaluation across both the producer (normalizedcons)
// and the delivery (deliverycons) hot paths so the two cannot drift.
//
// Path-resolution rule (klynx-api/docs/contracts/template-classification-rules.md §5A.3):
//
//	"payload.<key>"  → walk event.Payload from <key>   (recommended convention)
//	"<key>"          → walk event.Payload from <key>   (legacy bare path)
//	"source.*" / "event.*" / "meta.*" → no match here; rejected at PATCH-time
package classification

import (
	"fmt"
	"strings"

	"github.com/hotkhwan/gateway-api/models/ingestmod"
)

// Apply evaluates rules in order and sets EventClass / EventSeverity on event.
// First matching rule wins. Defaults are applied unconditionally so downstream
// consumers always see non-empty values:
//
//	EventClass    defaults to "unknown"
//	EventSeverity defaults to "none"
//
// Apply is idempotent — calling it twice on the same event with the same
// (unchanged) rules produces the same result (first-match-wins is deterministic).
// A nil event is a no-op.
func Apply(event *ingestmod.NormalizedEvent, rules []ingestmod.ClassificationRule) {
	if event == nil {
		return
	}
	if event.EventClass == "" {
		event.EventClass = "unknown"
	}
	if event.EventSeverity == "" {
		event.EventSeverity = "none"
	}

	// Insertion sort by Order — rules are usually small (≤50) and near-sorted.
	sorted := make([]ingestmod.ClassificationRule, len(rules))
	copy(sorted, rules)
	for i := 1; i < len(sorted); i++ {
		for j := i; j > 0 && sorted[j].Order < sorted[j-1].Order; j-- {
			sorted[j], sorted[j-1] = sorted[j-1], sorted[j]
		}
	}

	for _, rule := range sorted {
		if Matches(event.Payload, rule.When) {
			if rule.Set.EventClass != "" {
				event.EventClass = rule.Set.EventClass
			}
			if rule.Set.EventSeverity != "" {
				event.EventSeverity = rule.Set.EventSeverity
			}
			return
		}
	}
}

// Matches reports whether payload satisfies ALL conditions (AND logic).
// An empty condition slice always matches. Unsupported operators silently
// pass — the canonical write-time rejection happens at PATCH validation
// (Phase 1), and being lenient here preserves the pre-refactor semantic.
func Matches(payload map[string]any, conditions []ingestmod.PayloadCondition) bool {
	for _, cond := range conditions {
		val, found := resolveField(payload, cond.Field)
		if !found {
			return false
		}
		strVal := fmt.Sprintf("%v", val)
		switch cond.Operator {
		case "eq":
			if len(cond.Values) == 0 || strVal != cond.Values[0] {
				return false
			}
		case "in":
			matched := false
			for _, v := range cond.Values {
				if strVal == v {
					matched = true
					break
				}
			}
			if !matched {
				return false
			}
		}
	}
	return true
}

// resolveField applies the path-resolution rule then walks the resulting
// dotted path against payload. "payload.X" resolves to payload["X"], while
// a bare "X" also resolves to payload["X"] for backward compatibility with
// legacy rules.
func resolveField(payload map[string]any, field string) (any, bool) {
	if field == "" {
		return nil, false
	}
	if rest, ok := strings.CutPrefix(field, "payload."); ok {
		if rest == "" {
			return nil, false
		}
		return getNestedValue(payload, rest)
	}
	return getNestedValue(payload, field)
}

// getNestedValue resolves a dotted path (e.g. "eventAttribute.listType") inside obj.
func getNestedValue(obj map[string]any, path string) (any, bool) {
	parts := strings.SplitN(path, ".", 2)
	val, ok := obj[parts[0]]
	if !ok {
		return nil, false
	}
	if len(parts) == 1 {
		return val, true
	}
	child, ok := val.(map[string]any)
	if !ok {
		return nil, false
	}
	return getNestedValue(child, parts[1])
}
