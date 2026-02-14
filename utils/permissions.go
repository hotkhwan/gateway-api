// utils/permissions.go
package utils

// ToStringSlice แปลง interface{} ที่เป็น []interface{} ให้เป็น []string
func ToStringSlice(input interface{}) []string {
	arr, ok := input.([]interface{})
	if !ok {
		return nil
	}

	result := make([]string, 0, len(arr))
	for _, v := range arr {
		if str, ok := v.(string); ok {
			result = append(result, str)
		}
	}
	return result
}
