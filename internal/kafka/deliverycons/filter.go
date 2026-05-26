// internal/kafka/deliverycons/filter.go
package deliverycons

import "strings"

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
