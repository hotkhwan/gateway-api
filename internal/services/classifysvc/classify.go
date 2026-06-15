// internal/services/classifysvc/classify.go
//
// Shared event-classification helpers used by BOTH the delivery path
// (deliverycons — notification/webhook dispatch) and the normalize/producer
// path (normalizedcons — the canonical gw.events.normalized.v1 forwarded to
// klynx). Extracted from deliverycons/filter.go so a single implementation
// stamps eventClass / eventSeverity consistently on both sides.
//
// Contract: klynx-api/docs/contracts/event-severity-forwarding.md
// Plan:     docs/plan/severity-normalize-path-classification.md
package classifysvc

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/hotkhwan/gateway-api/models/ingestmod"
)

// MatchesFilter returns true if the payload satisfies ALL conditions (AND logic).
// An empty condition list always passes. Moved verbatim from
// deliverycons/filter.go so delivery-target filtering and classification share
// one matcher.
func MatchesFilter(payload map[string]any, conditions []ingestmod.PayloadCondition) bool {
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

// ApplyClassificationRules evaluates rules in Order (ascending); the first
// matching rule wins and sets EventClass / EventSeverity on the event.
//
// withDefaults gates the legacy "unknown"/"none" defaults:
//   - delivery path passes true  → byte-for-byte legacy behavior (every event
//     leaves with a non-empty class/severity).
//   - normalize/producer path passes false → unset fields stay "" so the
//     gw.events.normalized.v1 wire stays compact (omitempty) and klynx maps
//     ""→none. A watchlist default (see WatchlistSeverityDefault) fills severity
//     afterward only when still empty.
func ApplyClassificationRules(event *ingestmod.NormalizedEvent, rules []ingestmod.ClassificationRule, withDefaults bool) {
	if withDefaults {
		if event.EventClass == "" {
			event.EventClass = "unknown"
		}
		if event.EventSeverity == "" {
			event.EventSeverity = "none"
		}
	}

	// Stable insertion sort by Order (rule sets are tiny; avoids importing sort
	// and keeps equal-Order rules in declaration order — matches the original
	// deliverycons implementation).
	sorted := make([]ingestmod.ClassificationRule, len(rules))
	copy(sorted, rules)
	for i := 1; i < len(sorted); i++ {
		for j := i; j > 0 && sorted[j].Order < sorted[j-1].Order; j-- {
			sorted[j], sorted[j-1] = sorted[j-1], sorted[j]
		}
	}

	for _, rule := range sorted {
		if MatchesFilter(event.Payload, rule.When) {
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

// WatchlistSeverityDefault maps the AIBOX watchlist classification to a canonical
// severity, used as a platform default on the normalize path when no
// ClassificationRule has already set one:
//
//	3 (blacklist) → "high"
//	2 (redlist)   → "medium"
//	else          → "" (whitelist / stranger / absent carry no severity)
//
// Template ClassificationRules take precedence (run before this). Tolerates the
// flat `listType` (normalized/templated shape — what klynx receives) and the
// nested `eventAttribute.listType` (untemplated/raw) shape, mirroring klynx-api
// ingestsvc.extractListType so both stores agree.
func WatchlistSeverityDefault(payload map[string]any) string {
	code, ok := watchlistListType(payload)
	if !ok {
		return ""
	}
	switch code {
	case 3:
		return "high"
	case 2:
		return "medium"
	default:
		return ""
	}
}

func watchlistListType(payload map[string]any) (int, bool) {
	if payload == nil {
		return 0, false
	}
	read := func(v any) (int, bool) {
		switch n := v.(type) {
		case float64:
			return int(n), true
		case int:
			return n, true
		case int64:
			return int(n), true
		case string:
			if iv, err := strconv.Atoi(strings.TrimSpace(n)); err == nil {
				return iv, true
			}
		}
		return 0, false
	}
	if v, ok := payload["listType"]; ok {
		if code, ok := read(v); ok {
			return code, true
		}
	}
	if ea, ok := payload["eventAttribute"].(map[string]any); ok {
		if v, ok := ea["listType"]; ok {
			if code, ok := read(v); ok {
				return code, true
			}
		}
	}
	return 0, false
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
