// internal/iwown/errors.go
package iwown

import (
	"errors"
	"fmt"
)

var (
	ErrNilInput          = errors.New("iwown: nil input")
	ErrEmptyPayload      = errors.New("iwown: empty payload")
	ErrPayloadTooShort   = errors.New("iwown: payload too short")
	ErrProtobufUnmarshal = errors.New("iwown: protobuf unmarshal failed")
	ErrInvalidDateTime   = errors.New("iwown: invalid datetime")
	ErrInvalidScale      = errors.New("iwown: invalid scale")
	ErrInvalidBitOp      = errors.New("iwown: invalid bit operation")
)

func Wrap(err error, msg string) error {
	if err == nil {
		return errors.New("iwown: " + msg)
	}
	return fmt.Errorf("%s: %w", msg, err)
}

func Wrapf(err error, format string, args ...any) error {
	if err == nil {
		return fmt.Errorf(format, args...)
	}
	return fmt.Errorf(format+": %w", append(args, err)...)
}
