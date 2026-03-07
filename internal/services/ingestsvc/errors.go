// internal/services/ingestsvc/errors.go
package ingestsvc

import "errors"

var (
	// Event lifecycle errors
	ErrEventNotFound        = errors.New("event not found")
	ErrEventAlreadyApproved = errors.New("event already approved")
	ErrEventAlreadyRejected = errors.New("event already rejected")
	ErrInvalidEventType     = errors.New("invalid event type")

	// Approval gate errors (spec names)
	ErrAlreadyApproved  = ErrEventAlreadyApproved // alias
	ErrMappingRequired  = errors.New("mapping required before approval")
	ErrTemplateNotFound = errors.New("template not found")
	ErrInvalidStatus    = errors.New("invalid event status for this operation")
	ErrTemplateMismatch = errors.New("no matching template found for approved device")
)
