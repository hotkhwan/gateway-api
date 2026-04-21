// internal/services/templatesvc/errors.go
package templatesvc

import "errors"

var (
	ErrTemplateNotFound  = errors.New("template not found")
	ErrTemplateDuplicate = errors.New("template with same name already exists")
)

// InvalidDeliveryRuleError signals a malformed deliveryMatchAll / deliveryMatchAny
// entry (unsupported field namespace or unknown operator). Controllers map this
// to 400 BAD_REQUEST.
type InvalidDeliveryRuleError struct {
	Msg string
}

func (e *InvalidDeliveryRuleError) Error() string { return e.Msg }

// IsInvalidDeliveryRule reports whether err is an InvalidDeliveryRuleError.
func IsInvalidDeliveryRule(err error) bool {
	var ire *InvalidDeliveryRuleError
	return errors.As(err, &ire)
}
