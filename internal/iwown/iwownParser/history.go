// internal/iwown/iwownParser/history.go
package iwownParser

import (
	"google.golang.org/protobuf/proto"

	"github.com/hotkhwan/gateway-api/internal/iwown"
	pb "github.com/hotkhwan/gateway-api/internal/iwown/protobuf"
)

// const minHistoryPayloadLen = 23 // จาก log ของคุณ

// ปรับชื่อ pb message ให้ตรงกับ proto ของคุณจริง ๆ
func ProceedHistoryData(payload []byte) (*pb.HisData, error) {
	// ... validation เดิมได้

	var msg pb.HisData
	if err := proto.Unmarshal(payload, &msg); err != nil {
		// ✅ สำคัญ: dump field จริงที่มากับ payload
		// (จำกัด field ซัก 50 พอ กัน log ระเบิด)
		dump := DumpProtoWire(payload, 50)
		return nil, iwown.Wrap(iwown.ErrProtobufUnmarshal, err.Error()+" | dump:\n"+dump)
	}
	return &msg, nil
}

// func ProceedHistoryData(payload []byte) (*pb.HisData, error) {
// 	if payload == nil {
// 		return nil, iwown.ErrNilInput
// 	}
// 	if len(payload) == 0 {
// 		return nil, iwown.ErrEmptyPayload
// 	}
// 	if len(payload) < minHistoryPayloadLen {
// 		return nil, fmt.Errorf("%w: got=%d want>=%d", iwown.ErrPayloadTooShort, len(payload), minHistoryPayloadLen)
// 	}

// 	var msg pb.HisData
// 	if err := proto.Unmarshal(payload, &msg); err != nil {
// 		return nil, iwown.Wrap(iwown.ErrProtobufUnmarshal, err.Error())
// 	}
// 	return &msg, nil
// }
