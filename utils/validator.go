// utils/validator.go
package utils

import (
	"github.com/go-playground/validator/v10"
)

var Validate = validator.New()

func ValidateStruct(s interface{}) error {
	if err := Validate.Struct(s); err != nil {
		return err
	}
	return nil
}
