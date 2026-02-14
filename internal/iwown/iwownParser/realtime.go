// internal/iwown/iwownParser/realtime.go
package iwownParser

import (
	"google.golang.org/protobuf/proto"

	"klynx/internal/iwown"
	pb "klynx/internal/iwown/protobuf"
)

// Parse realtime payload -> RtNotification (ตรงกับ proto ที่ generate)
func ParseRealtimeData(payload []byte) (*pb.RtNotification, error) {
	if payload == nil {
		return nil, iwown.ErrNilInput
	}
	if len(payload) == 0 {
		return nil, iwown.ErrEmptyPayload
	}

	var msg pb.RtNotification
	if err := proto.Unmarshal(payload, &msg); err != nil {
		return nil, iwown.Wrap(iwown.ErrProtobufUnmarshal, err.Error())
	}
	return &msg, nil
}
