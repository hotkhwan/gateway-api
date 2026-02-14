// internal/services/crimes/warrantfmt.go
package crimes

import (
	"fmt"
	"strings"

	"klynx/models"
)

func nz(v any) string {
	switch x := v.(type) {
	case string:
		if strings.TrimSpace(x) == "" {
			return "-"
		}
		return x
	default:
		s := fmt.Sprint(v)
		if strings.TrimSpace(s) == "" {
			return "-"
		}
		return s
	}
}

// ต่อข้อความ alert จาก warrants ทั้งชุด (ใช้ใน sync)
func buildAlertDesc(warrants []models.Warrant) string {
	if len(warrants) == 0 {
		return ""
	}
	var b strings.Builder
	for i, w := range warrants {
		if i > 0 {
			b.WriteString("\n")
		}
		fmt.Fprintf(&b,
			"หมายจับ #%d: %s/%s | ข้อหา: %s | ภูมิภาค: %s | จังหวัด: %s | สถานี: %s",
			i+1,
			nz(w.WarrantNo), nz(w.WarrantYear),
			nz(w.Charge),
			nz(w.PoliceRegion), nz(w.PoliceProvincial), nz(w.PoliceStation),
		)
	}
	return b.String()
}
