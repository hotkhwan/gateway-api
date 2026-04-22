// internal/services/templatematcher/matcher.go
package templatematcher

import (
	"fmt"
	"strings"

	"github.com/hotkhwan/gateway-api/models/ingestmod"
)

// Evaluate reports whether the given matchAll/matchAny condition sets pass
// against the supplied match bag.
//
// Semantics (per plan §5.4 and matching contract):
//   - Both matchAll and matchAny empty → true (back-compat pass-through).
//   - matchAll non-empty → every condition must pass (AND).
//   - matchAny non-empty → at least one condition must pass (OR).
//   - When both are set, both the AND set and the OR set must pass.
//
// The matcher is stage-agnostic: it operates on a generic map[string]any and
// performs no field-namespace mapping. Callers are responsible for shaping the
// bag appropriately (e.g. ingest callers may expose raw payload at top level
// and under a "raw" key; delivery callers expose canonical top-level fields).
func Evaluate(matchAll, matchAny []ingestmod.MatchCondition, bag map[string]any) bool {
	if len(matchAll) == 0 && len(matchAny) == 0 {
		return true
	}

	for _, cond := range matchAll {
		if !evaluateCondition(cond, bag) {
			return false
		}
	}

	if len(matchAny) > 0 {
		passed := false
		for _, cond := range matchAny {
			if evaluateCondition(cond, bag) {
				passed = true
				break
			}
		}
		if !passed {
			return false
		}
	}

	return true
}

// evaluateCondition resolves cond.Field via dotted-path lookup in bag and
// applies the configured operator to the resolved value.
// Supported operators: eq, in, contains, prefix. Unknown operator → false.
func evaluateCondition(cond ingestmod.MatchCondition, bag map[string]any) bool {
	val, found := getNestedValue(bag, cond.Field)
	if !found {
		return false
	}
	strVal := fmt.Sprintf("%v", val)

	switch strings.ToLower(strings.TrimSpace(cond.Operator)) {
	case "eq":
		for _, v := range cond.Values {
			if strVal == v {
				return true
			}
		}
		return false
	case "in":
		for _, v := range cond.Values {
			if strVal == v {
				return true
			}
		}
		return false
	case "contains":
		for _, v := range cond.Values {
			if strings.Contains(strVal, v) {
				return true
			}
		}
		return false
	case "prefix":
		for _, v := range cond.Values {
			if strings.HasPrefix(strVal, v) {
				return true
			}
		}
		return false
	default:
		return false
	}
}

// getNestedValue resolves a dotted path (e.g. "source.deviceId") in obj.
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
