// internal/iwown/iwownParser/hisMaps.go
package iwownParser

import (
	"encoding/binary"
	"fmt"
	"reflect"
	"time"

	pb "klynx/internal/iwown/protobuf"
)

// ---- DateTime helpers ----

// dateTimeToTime tries to extract unix seconds from vendor DateTime message.
// Supports common shapes:
//   - dt.GetDateTime().GetSeconds()   (like OM0 path)
//   - dt.GetSeconds()
func dateTimeToTime(dt *pb.DateTime) (time.Time, bool) {
	if dt == nil {
		return time.Time{}, false
	}

	// helper: convert reflect.Value -> int64 seconds safely
	toSec := func(v reflect.Value) (int64, bool) {
		if !v.IsValid() {
			return 0, false
		}
		// if it's an interface, unwrap
		if v.Kind() == reflect.Interface && !v.IsNil() {
			v = v.Elem()
		}
		switch v.Kind() {
		case reflect.Int, reflect.Int32, reflect.Int64:
			return v.Int(), true
		case reflect.Uint, reflect.Uint32, reflect.Uint64:
			return int64(v.Uint()), true
		default:
			return 0, false
		}
	}

	// 1) Try: dt.GetDateTime().GetSeconds()
	mGetDateTime := reflect.ValueOf(dt).MethodByName("GetDateTime")
	if mGetDateTime.IsValid() {
		out := mGetDateTime.Call(nil)
		if len(out) == 1 && out[0].IsValid() && out[0].Kind() == reflect.Ptr && !out[0].IsNil() {
			inner := out[0]
			mGetSeconds := inner.MethodByName("GetSeconds")
			if mGetSeconds.IsValid() {
				secOut := mGetSeconds.Call(nil)
				if len(secOut) == 1 {
					if sec, ok := toSec(secOut[0]); ok && sec > 0 {
						return time.Unix(sec, 0).UTC(), true
					}
				}
			}
		}
	}

	// 2) Try: dt.GetSeconds()
	mGetSeconds := reflect.ValueOf(dt).MethodByName("GetSeconds")
	if mGetSeconds.IsValid() {
		secOut := mGetSeconds.Call(nil)
		if len(secOut) == 1 {
			if sec, ok := toSec(secOut[0]); ok && sec > 0 {
				return time.Unix(sec, 0).UTC(), true
			}
		}
	}

	return time.Time{}, false
}

// ---- History80 helpers ----
// Your History80 record time encoding is 0x0d + fixed32 seconds LE (NOT pb.DateTime).
func fixed32SecondsToTimeLE(b []byte) (time.Time, bool) {
	if len(b) != 5 || b[0] != 0x0d {
		return time.Time{}, false
	}
	sec := binary.LittleEndian.Uint32(b[1:5])
	return time.Unix(int64(sec), 0).UTC(), true
}

// ---- Sub-message mappers ----

func MapHisDataSpo2(m *pb.HisDataSpo2) map[string]any {
	if m == nil {
		return nil
	}
	out := map[string]any{"type": "spo2"}

	if ts, ok := dateTimeToTime(m.GetTimeStamp()); ok {
		out["time"] = ts
	}

	arr := m.GetSpo2Data()
	out["spo2_data"] = arr

	// optional computed
	if len(arr) > 0 {
		minV, maxV := arr[0], arr[0]
		var sum uint64
		for _, v := range arr {
			if v < minV {
				minV = v
			}
			if v > maxV {
				maxV = v
			}
			sum += uint64(v)
		}
		out["min_oxy"] = minV
		out["max_oxy"] = maxV
		out["avg_oxy"] = float64(sum) / float64(len(arr))
	}
	return out
}

func MapHisDataTemperature(m *pb.HisDataTemperature) map[string]any {
	if m == nil {
		return nil
	}
	out := map[string]any{"type": "temperature"}

	if ts, ok := dateTimeToTime(m.GetTimeStamp()); ok {
		out["time"] = ts
	}

	tmp := m.GetTemperature()
	if tmp != nil {
		out["evi_body"] = tmp.GetEviBody()
		out["esti_arm"] = tmp.GetEstiArm()
	}
	return out
}

func MapHisDataGNSS(m *pb.HisDataGNSS) map[string]any {
	if m == nil {
		return nil
	}
	out := map[string]any{"type": "gnss"}

	if ts, ok := dateTimeToTime(m.GetTimeStamp()); ok {
		out["time"] = ts
	}
	out["frequency"] = m.GetFrequency()

	// store points (safe reflection because RtGNSS differs)
	var points []map[string]any
	for _, g := range m.GetGnss() {
		if g == nil {
			continue
		}
		p := map[string]any{}
		rv := reflect.ValueOf(g)

		call := func(name string) (any, bool) {
			mm := rv.MethodByName(name)
			if !mm.IsValid() {
				return nil, false
			}
			ret := mm.Call(nil)
			if len(ret) != 1 {
				return nil, false
			}
			v := ret[0]
			switch v.Kind() {
			case reflect.Int, reflect.Int32, reflect.Int64:
				return v.Int(), true
			case reflect.Uint, reflect.Uint32, reflect.Uint64:
				return v.Uint(), true
			case reflect.Float32, reflect.Float64:
				return v.Float(), true
			case reflect.String:
				return v.String(), true
			case reflect.Bool:
				return v.Bool(), true
			default:
				return nil, false
			}
		}

		if v, ok := call("GetLongitude"); ok {
			p["lon"] = v
		}
		if v, ok := call("GetLatitude"); ok {
			p["lat"] = v
		}
		if v, ok := call("GetAltitude"); ok {
			p["alt"] = v
		}
		if v, ok := call("GetSpeed"); ok {
			p["speed"] = v
		}
		if v, ok := call("GetCourse"); ok {
			p["course"] = v
		}

		points = append(points, p)
	}
	out["gnss"] = points
	return out
}

func MapHisDataMedic(m *pb.HisDataMedic) map[string]any {
	if m == nil {
		return nil
	}
	out := map[string]any{"type": "medic"}

	if ts, ok := dateTimeToTime(m.GetTimeStamp()); ok {
		out["time"] = ts
	}

	out["pain_level"] = m.GetPainLevel()
	out["fatigue_level"] = m.GetFatigueLevel()
	out["stiff_level"] = m.GetStiffLevel()
	out["stiff_time"] = m.GetStiffTime()

	// optional computed
	f := m.GetFatigueLevel()
	if f <= 100 {
		out["pressure"] = 100 - f
	}
	return out
}

func MapAnyHisSubMessage(msg any) (map[string]any, error) {
	switch m := msg.(type) {
	case *pb.HisDataSpo2:
		return MapHisDataSpo2(m), nil
	case *pb.HisDataTemperature:
		return MapHisDataTemperature(m), nil
	case *pb.HisDataGNSS:
		return MapHisDataGNSS(m), nil
	case *pb.HisDataMedic:
		return MapHisDataMedic(m), nil
	default:
		return nil, fmt.Errorf("unsupported his sub message type: %T", msg)
	}
}
