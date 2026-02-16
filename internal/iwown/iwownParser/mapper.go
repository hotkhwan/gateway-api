// internal/iwown/iwownParser/mapper.go
package iwownParser

import (
	"time"

	"github.com/hotkhwan/gateway-api/internal/iwown/iwownConv"
	pb "github.com/hotkhwan/gateway-api/internal/iwown/protobuf"
	"github.com/hotkhwan/gateway-api/models/iwownmod"
)

type MapOptions struct {
	Location *time.Location
}

func MapHistoryPB(h *pb.HisData, opt MapOptions) (*iwownmod.HistoryBatch, error) {
	if h == nil {
		return nil, nil
	}

	out := &iwownmod.HistoryBatch{
		Raw: h,
	}

	_ = iwownConv.FromUnixSeconds
	return out, nil
}

func MapRealtimePB(r *pb.RtNotification, opt MapOptions) (*iwownmod.Realtime, error) {
	if r == nil {
		return nil, nil
	}

	out := &iwownmod.Realtime{
		Raw:        r,
		Attributes: map[string]any{},
	}

	return out, nil
}
