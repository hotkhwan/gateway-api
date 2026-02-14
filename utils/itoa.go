// utils/Itoa.go
package utils

import "strconv"

func Itoa(v int) string {
	if v == 0 {
		return ""
	}
	return strconv.Itoa(v)
}
