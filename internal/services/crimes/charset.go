// internal/services/crimes/charset.go
package crimes

import (
	"bytes"
	"io"
	"unicode/utf8"

	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding/unicode"
	"golang.org/x/text/transform"
)

func decodeToUTF8(raw []byte) io.Reader {
	// 1) ถ้าไฟล์เป็น UTF-8 อยู่แล้ว → ใช้เลย (สำคัญที่สุด)
	if utf8.Valid(raw) {
		return bytes.NewReader(raw)
	}
	// 2) UTF-16 with BOM
	if len(raw) >= 2 {
		if raw[0] == 0xFF && raw[1] == 0xFE {
			return transform.NewReader(bytes.NewReader(raw),
				unicode.UTF16(unicode.LittleEndian, unicode.ExpectBOM).NewDecoder())
		}
		if raw[0] == 0xFE && raw[1] == 0xFF {
			return transform.NewReader(bytes.NewReader(raw),
				unicode.UTF16(unicode.BigEndian, unicode.ExpectBOM).NewDecoder())
		}
	}
	// 3) Thai legacy (Windows-874 / TIS-620)
	return transform.NewReader(bytes.NewReader(raw), charmap.Windows874.NewDecoder())
}
