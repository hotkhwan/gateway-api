// internal/iwown/iwownConv/time.go
package iwownConv

import (
	"encoding/binary"
	"time"

	"github.com/hotkhwan/gateway-api/internal/iwown"
)

// ใช้ ICT เป็น default (ถ้าไม่ได้ส่ง loc มา)
func defaultLoc(loc *time.Location) *time.Location {
	if loc != nil {
		return loc
	}
	return time.FixedZone("ICT", 7*3600)
}

func FromUnixSeconds(sec uint64, loc *time.Location) time.Time {
	return time.Unix(int64(sec), 0).In(defaultLoc(loc))
}

// packed 6 bytes: YY MM DD hh mm ss  (year=2000+YY)
func ParsePackedYYMMDDhhmmss(b []byte, loc *time.Location) (time.Time, error) {
	if len(b) < 6 {
		return time.Time{}, iwown.ErrInvalidDateTime
	}
	year := int(b[0]) + 2000
	month := time.Month(b[1])
	day := int(b[2])
	hh := int(b[3])
	mm := int(b[4])
	ss := int(b[5])

	if month < 1 || month > 12 || day < 1 || day > 31 || hh > 23 || mm > 59 || ss > 59 {
		return time.Time{}, iwown.ErrInvalidDateTime
	}
	return time.Date(year, month, day, hh, mm, ss, 0, defaultLoc(loc)), nil
}

func ParseUnixSecondsLE(b []byte) (uint64, error) {
	if len(b) < 4 {
		return 0, iwown.ErrInvalidDateTime
	}
	if len(b) >= 8 {
		return binary.LittleEndian.Uint64(b[:8]), nil
	}
	return uint64(binary.LittleEndian.Uint32(b[:4])), nil
}

func ParseUnixSecondsBE(b []byte) (uint64, error) {
	if len(b) < 4 {
		return 0, iwown.ErrInvalidDateTime
	}
	if len(b) >= 8 {
		return binary.BigEndian.Uint64(b[:8]), nil
	}
	return uint64(binary.BigEndian.Uint32(b[:4])), nil
}

// ParsePbDateTime: iwown protobuf timestamp is seconds (UTC0 per doc).
// เราแปลงให้อยู่ ICT (+07) เป็น default เพื่อให้ log/DB อ่านง่าย
func ParsePbDateTime(sec int64) string {
	if sec <= 0 {
		return ""
	}
	t := FromUnixSeconds(uint64(sec), nil) // defaultLoc => ICT
	return t.Format(time.RFC3339)
}
