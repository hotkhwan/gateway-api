// models/optmod/seedmod.go
package optmod

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// ใช้รับ payload จาก seed API (รองรับ parent_id และ parentId)
type StationRaw struct {
	ID       string  `json:"id"`
	Code     IntCode `json:"code"`      // ไม่ได้ใช้ทำ relation แล้ว แต่เก็บได้เผื่อ debug
	Name     string  `json:"name"`      // จะ map ไปเป็น title
	ParentID *string `json:"parent_id"` // รองรับ input เดิม
	ParentId *string `json:"parentId"`  // รองรับ input ใหม่
	Level    int     `json:"level"`     // 1=Region, 2=Provincial, 3=Station
	Address  *string `json:"address"`
	Location *string `json:"location_id"`
	Status   *int    `json:"status"`
}

// Helper: ดึง parent id จากช่องใดช่องหนึ่ง (รองรับทั้งสองชื่อ)
func (s StationRaw) GetParentID() *string {
	if s.ParentId != nil && *s.ParentId != "" {
		return s.ParentId
	}
	return s.ParentID
}

type IntCode int

func (i *IntCode) UnmarshalJSON(b []byte) error {
	// null -> 0
	if bytes.Equal(b, []byte("null")) {
		*i = 0
		return nil
	}

	// ถ้าเป็น string: "123" หรือ " 1,234 "
	if len(b) > 0 && b[0] == '"' {
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return fmt.Errorf("code: invalid string: %w", err)
		}
		s = strings.TrimSpace(s)
		s = strings.ReplaceAll(s, ",", "") // เผื่อมี comma
		if s == "" {
			*i = 0
			return nil
		}
		n, err := strconv.Atoi(s)
		if err != nil {
			return fmt.Errorf("code: atoi failed: %w", err)
		}
		*i = IntCode(n)
		return nil
	}

	// อย่างอื่นพยายาม parse เป็น number (JSON number -> float64)
	var f float64
	if err := json.Unmarshal(b, &f); err != nil {
		return fmt.Errorf("code: invalid number: %w", err)
	}
	*i = IntCode(int(f))
	return nil
}

// Helper แปลงกลับเป็น int ปกติ (เผื่ออยากใช้เป็นฟังก์ชัน)
func (i IntCode) Int() int { return int(i) }
