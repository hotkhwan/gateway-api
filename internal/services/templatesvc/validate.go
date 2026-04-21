// internal/services/templatesvc/validate.go
package templatesvc

import (
	"fmt"
	"strings"

	"github.com/hotkhwan/gateway-api/models/ingestmod"
)

// deliveryAllowedOperators enumerates operator names accepted on
// deliveryMatchAll / deliveryMatchAny conditions (locked by plan decision D7).
var deliveryAllowedOperators = map[string]struct{}{
	"eq":       {},
	"in":       {},
	"contains": {},
	"prefix":   {},
}

// validateDeliveryRule checks a single delivery-stage condition list.
// Returns *InvalidDeliveryRuleError for any unsupported field namespace
// or unknown operator (plan decisions D2, D7, D8).
//
// The bagName parameter names the list being validated (e.g. "deliveryMatchAll")
// so the error message pinpoints the offending field in request shape.
func validateDeliveryRule(bagName string, conds []ingestmod.MatchCondition) error {
	for i, cond := range conds {
		field := strings.TrimSpace(cond.Field)
		if field == "" {
			return &InvalidDeliveryRuleError{
				Msg: fmt.Sprintf("%s[%d]: 'field' is required", bagName, i),
			}
		}
		// D2: raw.* is ingest-only; reject at the delivery stage.
		if strings.HasPrefix(field, "raw.") || field == "raw" {
			return &InvalidDeliveryRuleError{
				Msg: fmt.Sprintf(
					"%s[%d]: field '%s' not available at delivery stage; use a canonical field (see contract §5.5)",
					bagName, i, field,
				),
			}
		}
		// D7: operator set is locked.
		op := strings.ToLower(strings.TrimSpace(cond.Operator))
		if _, ok := deliveryAllowedOperators[op]; !ok {
			return &InvalidDeliveryRuleError{
				Msg: fmt.Sprintf(
					"%s[%d]: unknown operator '%s'; allowed: eq, in, contains, prefix",
					bagName, i, cond.Operator,
				),
			}
		}
		if len(cond.Values) == 0 {
			return &InvalidDeliveryRuleError{
				Msg: fmt.Sprintf("%s[%d]: 'values' must have at least one entry", bagName, i),
			}
		}
	}
	return nil
}

// validateDeliveryRules validates both delivery-stage condition lists.
func validateDeliveryRules(matchAll, matchAny []ingestmod.MatchCondition) error {
	if err := validateDeliveryRule("deliveryMatchAll", matchAll); err != nil {
		return err
	}
	return validateDeliveryRule("deliveryMatchAny", matchAny)
}
