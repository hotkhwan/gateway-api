// internal/kafka/deliverycons/filter.go
package deliverycons

import (
	"strings"

	"github.com/hotkhwan/gateway-api/internal/services/classifysvc"
	"github.com/hotkhwan/gateway-api/models/ingestmod"
)

// matchesFilter returns true if the payload satisfies ALL conditions (AND logic).
// An empty condition list always passes. Delegates to the shared classifysvc
// implementation (also used by the normalize/producer path).
func matchesFilter(payload map[string]any, conditions []ingestmod.PayloadCondition) bool {
	return classifysvc.MatchesFilter(payload, conditions)
}

// applyClassificationRules evaluates rules in order and sets eventClass/eventSeverity
// on the event. First matching rule wins. Defaults: eventClass="unknown",
// eventSeverity="none" (delivery path keeps the legacy defaults via withDefaults=true).
func applyClassificationRules(event *ingestmod.NormalizedEvent, rules []ingestmod.ClassificationRule) {
	classifysvc.ApplyClassificationRules(event, rules, true)
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
