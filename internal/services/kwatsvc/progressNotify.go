// internal/services/kwatsvc/progress_notify.go
package kwatsvc

import "github.com/hotkhwan/gateway-api/internal/mqtt/kwatchmsg"

func NotifyIbocProgressStarted(jobID string) {
	_ = kwatchmsg.PubTopicToMsg("ui/msg", "watchlist.iboc.started", map[string]any{
		"jobId": jobID,
		"state": "started",
	})
}

func NotifyIbocProgressQueued(jobID string) {
	_ = kwatchmsg.PubTopicToMsg("ui/msg", "watchlist.iboc.queued", map[string]any{
		"jobId": jobID,
		"state": "queued",
	})
}

func NotifyIbocProgressEmpty(jobID string) {
	_ = kwatchmsg.PubTopicToMsg("ui/msg", "watchlist.iboc.empty", map[string]any{
		"jobId": jobID,
		"state": "empty",
	})
}

func NotifyIbocProgressFailed(jobID, reason string) {
	_ = kwatchmsg.PubTopicToMsg("ui/msg", "watchlist.iboc.failed", map[string]any{
		"jobId":  jobID,
		"state":  "failed",
		"reason": reason,
	})
}
