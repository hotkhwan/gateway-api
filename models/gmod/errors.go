// models/gmod/errors.go
package gmod

import "fmt"

// CodedError ใช้แนบ code ไปกับ error
type CodedError struct {
	Code string
	Msg  string
}

func (e CodedError) Error() string {
	if e.Msg == "" {
		return e.Code
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Msg)
}

// Errorf สร้าง CodedError พร้อม code + message
func Errorf(code, msg string) error {
	return CodedError{Code: code, Msg: msg}
}

// ErrCodeOf คืน code จาก error ถ้าเป็น CodedError; ถ้าไม่ใช่ ให้คืนสตริงว่าง
func ErrCodeOf(err error) string {
	if err == nil {
		return ""
	}
	if ce, ok := err.(CodedError); ok {
		return ce.Code
	}
	return ""
}
