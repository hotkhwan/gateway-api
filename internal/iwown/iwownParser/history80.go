// internal/iwown/iwownParser/history80.go
package iwownParser

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"time"

	"google.golang.org/protobuf/encoding/protowire"
)

type History80Record struct {
	Time   time.Time `json:"time"`
	Minute time.Time `json:"minute"` // time floored to minute (UTC)
	ValueA uint32    `json:"valueA"`
	ValueB uint32    `json:"valueB"`
	RawHex string    `json:"rawHex"` // debug
}

type History80 struct {
	Seq     uint64            `json:"seq"`
	Records []History80Record `json:"records"`
}

// ParseHistory80 parses protocol 0x80 payload.
//
// Reality check from your repo:
// - vendor generated proto shows seq is fixed32 field=1 (his_data.pb.go: fixed32,1,req,name=seq)
// - "blob" isn't guaranteed to be field=3 in every packet; some packets omit it or use different field number.
// - Some packets may already be a stream of repeated record bytes (field=1 bytes) without outer wrapper.
//
// So this parser is tolerant:
// 1) Try to parse outer wrapper; grab seq (field=1 fixed32 OR varint) and choose the "best" bytes field as blob.
// 2) If no outer blob found, fallback: treat payload itself as inner stream of records.
func ParseHistory80(payload []byte) (*History80, error) {
	if len(payload) == 0 {
		return nil, fmt.Errorf("history80: empty payload")
	}

	out := &History80{}

	// --- 1) Try outer parse ---
	seq, blob, okOuter := parseHistory80Outer(payload)
	if okOuter {
		out.Seq = seq
		if len(blob) > 0 {
			records := parseHistory80InnerRecords(blob)
			out.Records = records
			if len(out.Records) > 0 {
				return out, nil
			}
			// if blob parsed but yielded no records, still fallthrough to inner-on-payload heuristic
		}
	}

	// --- 2) Fallback: treat payload itself as inner stream ---
	records := parseHistory80InnerRecords(payload)
	if len(records) == 0 {
		// keep error informative
		if okOuter && len(blob) == 0 {
			return nil, fmt.Errorf("history80: missing blob (outer wrapper had no bytes field)")
		}
		return nil, fmt.Errorf("history80: cannot decode any records (payloadLen=%d)", len(payload))
	}

	out.Records = records
	return out, nil
}

// parseHistory80Outer tries to parse an "outer wrapper" and extract:
// - seq from field 1 (fixed32 or varint)
// - blob as the largest bytes field found (prefer field=3 if present)
// Returns okOuter=false only when payload is clearly not protobuf-tag stream.
func parseHistory80Outer(payload []byte) (seq uint64, blob []byte, okOuter bool) {
	b := payload
	okOuter = true

	var (
		// keep candidates of bytes fields
		bytesField3 []byte
		largest     []byte
	)

	for len(b) > 0 {
		num, typ, n := protowire.ConsumeTag(b)
		if n < 0 {
			// not a protobuf stream (or truncated)
			return 0, nil, false
		}
		b = b[n:]

		switch {
		// seq: vendor proto says fixed32, but sometimes you may see varint
		case num == 1 && typ == protowire.Fixed32Type:
			v, m := protowire.ConsumeFixed32(b)
			if m < 0 {
				return 0, nil, false
			}
			seq = uint64(v)
			b = b[m:]

		case num == 1 && typ == protowire.VarintType:
			v, m := protowire.ConsumeVarint(b)
			if m < 0 {
				return 0, nil, false
			}
			seq = v
			b = b[m:]

		// blob (common is field=3 bytes, but don't assume)
		case typ == protowire.BytesType:
			v, m := protowire.ConsumeBytes(b)
			if m < 0 {
				return 0, nil, false
			}
			if num == 3 {
				bytesField3 = v
			}
			// track largest bytes field as fallback
			if len(v) > len(largest) {
				largest = v
			}
			b = b[m:]

		default:
			m := protowire.ConsumeFieldValue(num, typ, b)
			if m < 0 {
				return 0, nil, false
			}
			b = b[m:]
		}
	}

	if len(bytesField3) > 0 {
		return seq, bytesField3, true
	}
	if len(largest) > 0 {
		return seq, largest, true
	}
	return seq, nil, true
}

// parseHistory80InnerRecords expects a stream of repeated:
//   field=1 bytes  (each bytes is a record message)
// Any non-matching field will be skipped.
func parseHistory80InnerRecords(blob []byte) []History80Record {
	var out []History80Record

	ib := blob
	for len(ib) > 0 {
		num, typ, n := protowire.ConsumeTag(ib)
		if n < 0 {
			// stop on garbage
			break
		}
		ib = ib[n:]

		if num != 1 || typ != protowire.BytesType {
			m := protowire.ConsumeFieldValue(num, typ, ib)
			if m < 0 {
				break
			}
			ib = ib[m:]
			continue
		}

		recBytes, m := protowire.ConsumeBytes(ib)
		if m < 0 {
			break
		}
		ib = ib[m:]

		rec, err := parseHistory80Record(recBytes)
		if err != nil {
			// tolerant: skip bad record
			continue
		}
		out = append(out, *rec)
	}

	// heuristic: some payload might be a *single* record message without field=1 wrapper
	if len(out) == 0 {
		if rec, err := parseHistory80Record(blob); err == nil && rec != nil {
			out = append(out, *rec)
		}
	}

	return out
}

// record format (from your DB rawhex):
// - field1 bytes(len=5): 0x0d + 4 bytes little-endian seconds (fixed32 wire inside bytes)
// - field2 fixed32: valueA
// - field3 fixed32: valueB
func parseHistory80Record(b []byte) (*History80Record, error) {
	orig := b

	var (
		sec     uint32
		a       uint32
		bb      uint32
		gotTime bool
	)

	for len(b) > 0 {
		num, typ, n := protowire.ConsumeTag(b)
		if n < 0 {
			return nil, fmt.Errorf("rec ConsumeTag: %v", protowire.ParseError(n))
		}
		b = b[n:]

		switch {
		case num == 1 && typ == protowire.BytesType:
			v, m := protowire.ConsumeBytes(b)
			if m < 0 {
				return nil, fmt.Errorf("rec time bytes: %v", protowire.ParseError(m))
			}
			b = b[m:]

			// v should be 5 bytes: 0d + 4 bytes little-endian seconds
			if len(v) == 5 && v[0] == 0x0d {
				sec = binary.LittleEndian.Uint32(v[1:5])
				gotTime = true
			}

		case num == 2 && typ == protowire.Fixed32Type:
			v, m := protowire.ConsumeFixed32(b)
			if m < 0 {
				return nil, fmt.Errorf("rec valueA: %v", protowire.ParseError(m))
			}
			a = v
			b = b[m:]

		case num == 3 && typ == protowire.Fixed32Type:
			v, m := protowire.ConsumeFixed32(b)
			if m < 0 {
				return nil, fmt.Errorf("rec valueB: %v", protowire.ParseError(m))
			}
			bb = v
			b = b[m:]

		default:
			m := protowire.ConsumeFieldValue(num, typ, b)
			if m < 0 {
				return nil, fmt.Errorf("rec skip field=%d: %v", num, protowire.ParseError(m))
			}
			b = b[m:]
		}
	}

	if !gotTime {
		return nil, fmt.Errorf("record missing time")
	}

	t := time.Unix(int64(sec), 0).UTC()
	min := t.Truncate(time.Minute)

	return &History80Record{
		Time:   t,
		Minute: min,
		ValueA: a,
		ValueB: bb,
		RawHex: hex.EncodeToString(orig),
	}, nil
}
