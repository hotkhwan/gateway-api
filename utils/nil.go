// utils/nil.go
package utils

func EmptyIfNil[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}
