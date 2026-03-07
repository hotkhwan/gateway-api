// internal/kafka/deliverycons/filter.go
package deliverycons

import (
	"fmt"
	"strings"

	"github.com/hotkhwan/gateway-api/models/ingestmod"
)

// matchesFilter returns true if the payload satisfies ALL conditions (AND logic).
// An empty condition list always passes.
func matchesFilter(payload map[string]any, conditions []ingestmod.PayloadCondition) bool {
	for _, cond := range conditions {
		val, found := getNestedValue(payload, cond.Field)
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

// applyClassificationRules evaluates rules in order and sets eventClass/eventSeverity
// on the event. First matching rule wins. Defaults: eventClass="unknown", eventSeverity="none".
func applyClassificationRules(event *ingestmod.NormalizedEvent, rules []ingestmod.ClassificationRule) {
	if event.EventClass == "" {
		event.EventClass = "unknown"
	}
	if event.EventSeverity == "" {
		event.EventSeverity = "none"
	}

	// Sort by Order (already expected to be sorted, but stable sort for safety)
	sorted := make([]ingestmod.ClassificationRule, len(rules))
	copy(sorted, rules)
	for i := 1; i < len(sorted); i++ {
		for j := i; j > 0 && sorted[j].Order < sorted[j-1].Order; j-- {
			sorted[j], sorted[j-1] = sorted[j-1], sorted[j]
		}
	}

	for _, rule := range sorted {
		if matchesFilter(event.Payload, rule.When) {
			if rule.Set.EventClass != "" {
				event.EventClass = rule.Set.EventClass
			}
			if rule.Set.EventSeverity != "" {
				event.EventSeverity = rule.Set.EventSeverity
			}
			return // first match wins
		}
	}
}

// matchesEventClasses returns true if the event's class is in the whitelist.
// An empty whitelist accepts all classes.
func matchesEventClasses(eventClass string, whitelist []string) bool {
	if len(whitelist) == 0 {
		return true
	}
	for _, c := range whitelist {
		if strings.EqualFold(eventClass, c) {
			return true
		}
	}
	return false
}

// matchesEventSeverities returns true if the event's severity is in the whitelist.
// An empty whitelist accepts all severities.
func matchesEventSeverities(eventSeverity string, whitelist []string) bool {
	if len(whitelist) == 0 {
		return true
	}
	for _, s := range whitelist {
		if strings.EqualFold(eventSeverity, s) {
			return true
		}
	}
	return false
}

// getNestedValue resolves a dotted path (e.g. "payload.listType") inside obj.
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
