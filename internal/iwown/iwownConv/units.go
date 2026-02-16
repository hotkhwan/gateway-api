// internal/iwown/iwownConv/units.go
package iwownConv

import (
	"math"

	"github.com/hotkhwan/gateway-api/internal/iwown"
)

func ScaleInt(v int64, factor float64) (float64, error) {
	if factor == 0 || math.IsNaN(factor) || math.IsInf(factor, 0) {
		return 0, iwown.ErrInvalidScale
	}
	return float64(v) * factor, nil
}

func ScaleUint(v uint64, factor float64) (float64, error) {
	if factor == 0 || math.IsNaN(factor) || math.IsInf(factor, 0) {
		return 0, iwown.ErrInvalidScale
	}
	return float64(v) * factor, nil
}

// common vendor rule: "divide by 10"
func Scale01(v int64) float64     { return float64(v) / 10.0 }
func Scale01U(v uint64) float64   { return float64(v) / 10.0 }
func Scale01U32(v uint32) float64 { return float64(v) / 10.0 }

// sometimes used: divide by 100
func Scale001(v int64) float64     { return float64(v) / 100.0 }
func Scale001U(v uint64) float64   { return float64(v) / 100.0 }
func Scale001U32(v uint32) float64 { return float64(v) / 100.0 }

func BitRemember(x uint64, bit uint) (bool, error) {
	if bit >= 64 {
		return false, iwown.ErrInvalidBitOp
	}
	return (x & (1 << bit)) != 0, nil
}

func ExtractBits(x uint64, start uint, width uint) (uint64, error) {
	if width == 0 || start >= 64 || start+width > 64 {
		return 0, iwown.ErrInvalidBitOp
	}
	mask := uint64((1 << width) - 1)
	return (x >> start) & mask, nil
}
