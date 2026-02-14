// internal/iwown/iwownParser/oldMan.go
package iwownParser

import (
	"fmt"

	"klynx/internal/iwown/iwownConv"
	pb "klynx/internal/iwown/protobuf"

	"google.golang.org/protobuf/proto"
)

const int32Max = int64(^uint32(0) >> 1)

type OM0Parsed struct {
	Kind       string     `json:"kind"`
	Opt        int        `json:"opt"`
	PayloadLen int        `json:"payloadLen"`
	Time       string     `json:"time,omitempty"`
	Battery    int32      `json:"battery,omitempty"`
	RSSI       int32      `json:"rssi,omitempty"`
	Health     *OM0Health `json:"health,omitempty"`
	Tracks     []OM0Track `json:"tracks,omitempty"`
}

type OM0Health struct {
	Steps    int32   `json:"steps,omitempty"`
	Distance float32 `json:"distance,omitempty"` // meters (doc: divide by 10)
	Calorie  float32 `json:"calorie,omitempty"`  // cal (divide by 10)
}

type OM0Track struct {
	Time    string  `json:"time,omitempty"`
	Lon     float64 `json:"lon,omitempty"`
	Lat     float64 `json:"lat,omitempty"`
	GpsType string  `json:"gpsType,omitempty"` // 1/2/3
}

func ParseOM0Report(payload []byte, opt int) (*OM0Parsed, error) {
	var msg pb.OM0Report
	if err := proto.Unmarshal(payload, &msg); err != nil {
		return nil, fmt.Errorf("unmarshal OM0Report: %w", err)
	}

	sec := int64(msg.GetDateTime().GetDateTime().GetSeconds())
	out := &OM0Parsed{
		Kind:       "pb",
		Opt:        opt,
		PayloadLen: len(payload),
		Time:       iwownConv.ParsePbDateTime(sec),
		Battery:    int32(msg.GetBattery().GetLevel()),
	}

	// rssi uint32 แต่ติดลบได้ตาม doc
	rssiU := msg.GetRssi()
	if rssiU > uint32(int32Max) {
		out.RSSI = int32(^(rssiU)+1) * -1
	} else {
		out.RSSI = int32(rssiU)
	}

	if h := msg.GetHealth(); h != nil {
		out.Health = &OM0Health{
			Steps:    int32(h.GetSteps()),
			Distance: float32(h.GetDistance()) * 0.1,
			Calorie:  float32(h.GetCalorie()) * 0.1,
		}
	}

	// S1009: len(nil slice) == 0 อยู่แล้ว
	for _, tr := range msg.GetTrackData() {
		sec := int64(tr.GetTime().GetDateTime().GetSeconds())
		out.Tracks = append(out.Tracks, OM0Track{
			Time:    iwownConv.ParsePbDateTime(sec),
			Lon:     float64(tr.GetGnss().GetLongitude()),
			Lat:     float64(tr.GetGnss().GetLatitude()),
			GpsType: tr.GetGpsType().String(),
		})
	}

	return out, nil
}
