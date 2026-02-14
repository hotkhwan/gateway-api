// internal/iwown/iwownParser/protowireDump.go
package iwownParser

import (
	"encoding/hex"
	"fmt"

	"google.golang.org/protobuf/encoding/protowire"
)

func DumpProtoWire(payload []byte, maxFields int) string {
	if len(payload) == 0 {
		return "empty payload"
	}

	s := fmt.Sprintf("len=%d hex=%s\n", len(payload), hex.EncodeToString(payload))
	b := payload
	count := 0
	offset := 0

	for len(b) > 0 {
		if maxFields > 0 && count >= maxFields {
			s += fmt.Sprintf("... truncated after %d fields\n", count)
			break
		}

		num, typ, n := protowire.ConsumeTag(b)
		if n < 0 {
			s += fmt.Sprintf("ConsumeTag error: %v at offset=%d\n", protowire.ParseError(n), offset)
			break
		}
		b = b[n:]
		offset += n

		switch typ {
		case protowire.VarintType:
			v, m := protowire.ConsumeVarint(b)
			if m < 0 {
				s += fmt.Sprintf("field=%d varint error: %v at offset=%d\n", num, protowire.ParseError(m), offset)
				return s
			}
			s += fmt.Sprintf("field=%d type=varint value=%d\n", num, v)
			b = b[m:] // ✅ ต้องเป็น slice
			offset += m

		case protowire.Fixed32Type:
			v, m := protowire.ConsumeFixed32(b)
			if m < 0 {
				s += fmt.Sprintf("field=%d fixed32 error: %v at offset=%d\n", num, protowire.ParseError(m), offset)
				return s
			}
			s += fmt.Sprintf("field=%d type=fixed32 value=%d\n", num, v)
			b = b[m:] // ✅
			offset += m

		case protowire.Fixed64Type:
			v, m := protowire.ConsumeFixed64(b)
			if m < 0 {
				s += fmt.Sprintf("field=%d fixed64 error: %v at offset=%d\n", num, protowire.ParseError(m), offset)
				return s
			}
			s += fmt.Sprintf("field=%d type=fixed64 value=%d\n", num, v)
			b = b[m:] // ✅
			offset += m

		case protowire.BytesType:
			v, m := protowire.ConsumeBytes(b)
			if m < 0 {
				s += fmt.Sprintf("field=%d bytes error: %v at offset=%d\n", num, protowire.ParseError(m), offset)
				return s
			}
			preview := v
			if len(preview) > 32 {
				preview = preview[:32]
			}
			s += fmt.Sprintf("field=%d type=bytes len=%d hex32=%s\n",
				num, len(v), hex.EncodeToString(preview))
			b = b[m:] // ✅
			offset += m

		default:
			s += fmt.Sprintf("field=%d type=%v (unsupported) at offset=%d\n", num, typ, offset)
			return s
		}

		count++
	}

	return s
}
