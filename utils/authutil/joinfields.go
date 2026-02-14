// utils/joinfields.go
package authutil

import (
	"strings"
)

func JoinFields(fields []string) string {
	return strings.Join(fields, ", ")
}
