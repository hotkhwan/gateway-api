// internal/adapters/alertdispatcher/dispatcher.go
package alertdispatcher

import (
	"github.com/hotkhwan/gateway-api/internal/logger"
)

// FastAlertEnvelope is the provisional alert sent via MQTT (Path A).
// It always has Provisional=true, Canonical=false.
// UI must reconcile with the canonical event (Path B) using EventID.
type FastAlertEnvelope struct {
	EventID      string         `json:"eventId"`      // same as Path B canonical eventId
	WorkspaceID  string         `json:"workspaceId"`
	SourceFamily string         `json:"sourceFamily"`
	OccurredAt   string         `json:"occurredAt"`   // RFC3339 UTC
	Provisional  bool           `json:"provisional"`  // always true
	Canonical    bool           `json:"canonical"`    // always false
	AlertFields  map[string]any `json:"alertFields"`  // extracted alert fields only
}

// Handler is the function that sends an alert (e.g. MQTT publish).
type Handler func(alert FastAlertEnvelope)

// Dispatcher is a bounded async worker pool for fire-and-forget fast alerts.
// When the queue is full, incoming alerts are dropped (drop-newest policy).
// This prevents goroutine storm during ingestion bursts.
//
// Telemetry: use Dropped() to read the cumulative drop counter.
type Dispatcher struct {
	queue   chan FastAlertEnvelope
	handler Handler
	dropped uint64
}

// New creates a Dispatcher with the given buffer size and worker count.
// Recommended: bufferSize=1000, workers=4.
func New(bufferSize, workers int, handler Handler) *Dispatcher {
	d := &Dispatcher{
		queue:   make(chan FastAlertEnvelope, bufferSize),
		handler: handler,
	}
	for i := 0; i < workers; i++ {
		go d.run()
	}
	return d
}

// Dispatch enqueues an alert for async delivery.
// Returns false and increments the drop counter if the queue is full.
func (d *Dispatcher) Dispatch(alert FastAlertEnvelope) bool {
	select {
	case d.queue <- alert:
		return true
	default:
		// drop-newest: new incoming alert dropped, queue not modified
		d.dropped++
		log := logger.Boot("alertdispatcher", "Dispatch")
		log.Warn().Str("eventId", alert.EventID).Str("workspaceId", alert.WorkspaceID).
			Msg("[alertdispatcher] queue full — alert dropped")
		return false
	}
}

// Dropped returns the cumulative count of dropped alerts.
func (d *Dispatcher) Dropped() uint64 { return d.dropped }

func (d *Dispatcher) run() {
	for alert := range d.queue {
		d.handler(alert)
	}
}
