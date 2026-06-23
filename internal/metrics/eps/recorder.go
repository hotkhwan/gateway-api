// internal/metrics/eps/recorder.go
package eps

import (
	"sync"
	"time"
)

// ringSize is the number of per-second buckets retained. It must be strictly
// greater than the widest window (60s) so the bucket 60 seconds ago never
// shares a modulo index with the current second. 64 gives margin.
const ringSize = 64

// idleEvictSeconds is how long a workspace may go without an observed event
// before its window is pruned (bounds memory + Prometheus series cardinality).
const idleEvictSeconds = int64(5 * 60)

// Recorder accumulates per-workspace event counts on the producer hot path and
// exposes rolling-window rates (1s / 10s / 1m) plus a smoothed peak. It is
// observe-only: it never affects event flow.
//
// Hot path (Observe) is O(1): a sharded map lookup plus a tiny per-window
// critical section (advance ring bucket + increment). The peak and idle-prune
// bookkeeping run out-of-band on a 1 Hz roller goroutine, so the publish path
// never pays for them.
type Recorder struct {
	mu      sync.RWMutex
	windows map[string]*window
	now     func() int64 // overridable in tests; unix seconds

	stopCh  chan struct{}
	stopped bool
}

// NewRecorder returns a started-on-demand recorder. Call Start to run the
// background roller (peak tracking + idle prune); Observe works without it.
func NewRecorder() *Recorder {
	return &Recorder{
		windows: make(map[string]*window),
		now:     func() int64 { return time.Now().Unix() },
		stopCh:  make(chan struct{}),
	}
}

// Observe records exactly one event for workspaceID. Safe on a nil receiver
// (disabled metric). Empty workspaceID is skipped so an un-keyed event never
// collapses into a junk catch-all series.
func (r *Recorder) Observe(workspaceID string) {
	if r == nil || workspaceID == "" {
		return
	}
	now := r.now()

	r.mu.RLock()
	w := r.windows[workspaceID]
	r.mu.RUnlock()

	if w == nil {
		r.mu.Lock()
		if w = r.windows[workspaceID]; w == nil {
			w = &window{}
			r.windows[workspaceID] = w
		}
		r.mu.Unlock()
	}

	w.observe(now)
}

// Start launches the 1 Hz roller that updates smoothed peaks and prunes idle
// workspaces. Idempotent-safe to call once after construction.
func (r *Recorder) Start() {
	if r == nil {
		return
	}
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-r.stopCh:
				return
			case <-ticker.C:
				r.roll(r.now())
			}
		}
	}()
}

// Stop halts the roller goroutine. Safe to call once.
func (r *Recorder) Stop() {
	if r == nil {
		return
	}
	r.mu.Lock()
	if !r.stopped {
		r.stopped = true
		close(r.stopCh)
	}
	r.mu.Unlock()
}

// roll updates each window's smoothed peak from its current 10s-average rate,
// then evicts idle windows. Run out-of-band by the ticker (and directly in
// tests with a synthetic clock).
func (r *Recorder) roll(now int64) {
	r.mu.RLock()
	ws := make([]*window, 0, len(r.windows))
	for _, w := range r.windows {
		ws = append(ws, w)
	}
	r.mu.RUnlock()

	for _, w := range ws {
		w.mu.Lock()
		snap := w.computeLocked(now)
		if snap.Rate10s > w.peak {
			w.peak = snap.Rate10s
		}
		w.mu.Unlock()
	}

	r.pruneIdle(now)
}

// pruneIdle removes windows with no events for longer than idleEvictSeconds.
func (r *Recorder) pruneIdle(now int64) {
	r.mu.Lock()
	for id, w := range r.windows {
		w.mu.Lock()
		idle := w.lastSeen != 0 && now-w.lastSeen > idleEvictSeconds
		w.mu.Unlock()
		if idle {
			delete(r.windows, id)
		}
	}
	r.mu.Unlock()
}

// Snapshot is a point-in-time view of a window's rolling rates.
type Snapshot struct {
	Current float64 // 1s rate — events in the most recent completed second
	Rate10s float64 // 10s average rate
	Rate1m  float64 // 60s average rate
	Peak    float64 // smoothed peak: max 10s-average rate observed
}

// WorkspaceSnapshot pairs a workspace id with its snapshot.
type WorkspaceSnapshot struct {
	WorkspaceID string
	Snapshot
}

// SnapshotAll returns a snapshot per active workspace. Read out-of-band by the
// Prometheus collector on scrape.
func (r *Recorder) SnapshotAll() []WorkspaceSnapshot {
	if r == nil {
		return nil
	}
	now := r.now()

	type entry struct {
		id string
		w  *window
	}
	r.mu.RLock()
	entries := make([]entry, 0, len(r.windows))
	for id, w := range r.windows {
		entries = append(entries, entry{id: id, w: w})
	}
	r.mu.RUnlock()

	out := make([]WorkspaceSnapshot, 0, len(entries))
	for _, e := range entries {
		out = append(out, WorkspaceSnapshot{WorkspaceID: e.id, Snapshot: e.w.snapshot(now)})
	}
	return out
}

// window holds one workspace's per-second ring buffer and peak.
type window struct {
	mu       sync.Mutex
	buckets  [ringSize]int64 // event count per second slot
	epochs   [ringSize]int64 // unix second each slot currently represents
	peak     float64         // smoothed peak (max 10s-avg)
	lastSeen int64           // unix second of the most recent observed event
}

func (w *window) observe(now int64) {
	idx := int(now % ringSize)
	w.mu.Lock()
	if w.epochs[idx] != now {
		// Slot belongs to an older second — reset before reuse.
		w.buckets[idx] = 0
		w.epochs[idx] = now
	}
	w.buckets[idx]++
	w.lastSeen = now
	w.mu.Unlock()
}

func (w *window) snapshot(now int64) Snapshot {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.computeLocked(now)
}

// computeLocked sums completed seconds only (age >= 1): the in-progress current
// second (age 0) is excluded so a partial second never reads as a dip.
// Caller must hold w.mu.
func (w *window) computeLocked(now int64) Snapshot {
	var sum1, sum10, sum60 int64
	for i := 0; i < ringSize; i++ {
		e := w.epochs[i]
		if e == 0 {
			continue
		}
		age := now - e // seconds ago this slot's second occurred
		if age < 1 || age > 60 {
			continue // current/in-progress second, future, or stale
		}
		c := w.buckets[i]
		sum60 += c
		if age <= 10 {
			sum10 += c
		}
		if age == 1 {
			sum1 += c
		}
	}
	return Snapshot{
		Current: float64(sum1),
		Rate10s: float64(sum10) / 10.0,
		Rate1m:  float64(sum60) / 60.0,
		Peak:    w.peak,
	}
}
