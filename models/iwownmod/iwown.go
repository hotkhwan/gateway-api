// models/iwownmod/iwown.go
package iwownmod

import "time"

// เก็บเป็น “กลาง” ให้ service layer เอาไป persist/transform ต่อได้
type HistoryBatch struct {
	DeviceID string
	UserID   string

	// Raw PB เผื่ออยาก debug/เก็บของเดิมไว้
	Raw any

	Records []HistoryRecord
}

type HistoryRecord struct {
	Timestamp time.Time
	Kind      string // เช่น "health", "gnss", "ecg", ...
	Raw       any    // record pb ตัวจริง
}

type Realtime struct {
	DeviceID   string
	Timestamp  time.Time
	Kind       string
	Raw        any
	Attributes map[string]any
}
