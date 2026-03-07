// internal/services/subscriptionsvc/errors.go
package subscriptionsvc

import "errors"

var (
	ErrOrganizationLimitReached = errors.New("organization limit reached for this subscription")
	ErrInvalidLicenseKey        = errors.New("invalid license key")
	ErrSubscriptionUnavailable  = errors.New("subscription temporarily unavailable")
	ErrDeliveryQuotaExceeded    = errors.New("delivery target quota exceeded for current plan")
)
