package eventsvc

import "errors"

var (
	ErrEventNotFound        = errors.New("event not found")
	ErrEventAlreadyApproved = errors.New("event already approved")
	ErrEventAlreadyRejected = errors.New("event already rejected")
	ErrInvalidEventType      = errors.New("invalid event type")
)
