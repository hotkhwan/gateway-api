// internal/services/crimes/datetime.go
package crimes

import (
	"strconv"
	"strings"
	"time"
)

var monthMap = map[string]time.Month{
	"JAN": time.January, "FEB": time.February, "MAR": time.March,
	"APR": time.April, "MAY": time.May, "JUN": time.June,
	"JUL": time.July, "AUG": time.August, "SEP": time.September,
	"OCT": time.October, "NOV": time.November, "DEC": time.December,
}

func parseThaiDate(s string) *time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}

	// 25680828 → YYYYMMDD (พ.ศ.)
	if len(s) == 8 && strings.HasPrefix(s, "25") {
		year, _ := strconv.Atoi(s[0:4])
		month, _ := strconv.Atoi(s[4:6])
		day, _ := strconv.Atoi(s[6:8])
		t := time.Date(year-543, time.Month(month), day, 0, 0, 0, 0, time.UTC)
		return &t
	}

	// 09-DEC-1985 [HH:MM[:SS]]
	parts := strings.Fields(s)
	if len(parts) >= 1 {
		dmy := strings.Split(parts[0], "-")
		if len(dmy) == 3 {
			day, _ := strconv.Atoi(dmy[0])
			monStr := strings.ToUpper(dmy[1])
			year, _ := strconv.Atoi(dmy[2])
			if m, ok := monthMap[monStr]; ok {
				t := time.Date(year, m, day, 0, 0, 0, 0, time.UTC)
				return &t
			}
		}
	}

	// รูปแบบทั่วไป
	layouts := []string{
		"02-01-2006", "02/01/2006",
		"02-01-2006 15:04", "02/01/2006 15:04",
		"02-01-2006 15:04:05", "02/01/2006 15:04:05",
	}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, s); err == nil {
			return &t
		}
	}
	return nil
}
