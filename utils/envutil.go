// utils/envutil.go
package utils

import (
	"os"
	"strconv"
	"strings"
	"time"
)

func Getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func GetenvInt(k string, def int) int {
	if v := os.Getenv(k); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return def
}

// GetenvBool อ่านค่า env แบบ boolean (รองรับ true/false, 1/0, yes/no, on/off)
func GetenvBool(name string, def bool) bool {
	v := strings.TrimSpace(os.Getenv(name))
	if v == "" {
		return def
	}
	switch strings.ToLower(v) {
	case "1", "t", "true", "y", "yes", "on":
		return true
	case "0", "f", "false", "n", "no", "off":
		return false
	default:
		return def
	}
}

// GetEnvDurationSec อ่านค่า ENV แล้วคืนค่าเป็น time.Duration (หน่วยวินาที)
func GetEnvDurationSec(key string, defSec int) time.Duration {
	if s := os.Getenv(key); s != "" {
		if v, err := strconv.Atoi(s); err == nil && v > 0 {
			return time.Duration(v) * time.Second
		}
	}
	return time.Duration(defSec) * time.Second
}

type IntOpt struct {
	Min *int
	Max *int
}

func Int(name string, def int, opt ...IntOpt) int {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return clamp(def, opt...)
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return clamp(def, opt...)
	}
	return clamp(v, opt...)
}

func clamp(v int, opt ...IntOpt) int {
	if len(opt) == 0 {
		return v
	}
	o := opt[0]
	if o.Min != nil && v < *o.Min {
		return *o.Min
	}
	if o.Max != nil && v > *o.Max {
		return *o.Max
	}
	return v
}
