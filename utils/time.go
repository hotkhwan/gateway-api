// utils/time.go
package utils

import (
	"strings"
	"time"
)

func FormatTimeOrEmpty(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(time.RFC3339)
}

func MustParseTime(v interface{}) time.Time {
	switch t := v.(type) {
	case time.Time:
		return t
	case string:
		parsed, _ := time.Parse(time.RFC3339, t)
		return parsed
	default:
		return time.Time{}
	}
}

func CurrentDateTime() string {
	return time.Now().Format(time.RFC3339)
}

// ✅ default dateTime: today 00:00:00 (Asia/Bangkok) -> now, both in UTC RFC3339
func DefaultDateTimeRangeBangkokToNowUTC() (fromUtc string, toUtc string) {
	loc, err := time.LoadLocation("Asia/Bangkok")
	if err != nil {
		loc = time.FixedZone("Asia/Bangkok", 7*3600)
	}

	nowLocal := time.Now().In(loc)
	startLocal := time.Date(nowLocal.Year(), nowLocal.Month(), nowLocal.Day(), 0, 0, 0, 0, loc)

	return startLocal.UTC().Format(time.RFC3339), time.Now().UTC().Format(time.RFC3339)
}

// dateTime query format: "fromUtc,toUtc" (RFC3339)
func ParseDateTimeRangeQuery(dateTime string) (from string, to string, ok bool) {
	dt := strings.TrimSpace(dateTime)
	if dt == "" {
		return "", "", false
	}

	parts := strings.Split(dt, ",")
	if len(parts) != 2 {
		return "", "", false
	}

	from = strings.TrimSpace(parts[0])
	to = strings.TrimSpace(parts[1])

	if _, err := time.Parse(time.RFC3339, from); err != nil {
		return "", "", false
	}
	if _, err := time.Parse(time.RFC3339, to); err != nil {
		return "", "", false
	}

	return from, to, true
}
