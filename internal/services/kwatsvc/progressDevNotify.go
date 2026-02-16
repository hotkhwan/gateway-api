// internal/services/kwatsvc/progressDevNotify.go
package kwatsvc

import (
	"time"

	"github.com/hotkhwan/gateway-api/internal/mqtt/kwatchmsg"
)

func NotifyIbocDevProgressStarted(jobID string) {
	_ = kwatchmsg.PubTopicToMsg("ui/msg", "watchlist.ibocdev.progress", map[string]any{
		"jobId": jobID,
		"state": "started",
		"time":  time.Now().UTC(),
	})
}

func NotifyIbocDevProgressQueued(jobID string) {
	_ = kwatchmsg.PubTopicToMsg("ui/msg", "watchlist.ibocdev.progress", map[string]any{
		"jobId": jobID,
		"state": "queued",
		"time":  time.Now().UTC(),
	})
}

func NotifyIbocDevProgressFailed(jobID, reason string) {
	_ = kwatchmsg.PubTopicToMsg("ui/msg", "watchlist.ibocdev.progress", map[string]any{
		"jobId":  jobID,
		"state":  "failed",
		"reason": reason,
		"time":   time.Now().UTC(),
	})
}
