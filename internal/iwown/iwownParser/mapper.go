// internal/iwown/iwownParser/mapper.go
package iwownParser

import (
	"time"

	"klynx/internal/iwown/iwownConv"
	pb "klynx/internal/iwown/protobuf"
	"klynx/models/iwownmod"
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
