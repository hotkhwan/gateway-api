// utils/perpage.go
package utils

func PerPage(perPages int) int {
	min1 := 1
	max := Int("PERPAGE_MAX", 250, IntOpt{Min: &min1}) // default 250
	def := Int("PERPAGE", 10, IntOpt{Min: &min1, Max: &max})

	if perPages <= 0 {
		return def
	}
	if perPages > max {
		return max
	}
	return perPages
}
