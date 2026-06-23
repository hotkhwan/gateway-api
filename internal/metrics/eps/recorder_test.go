// internal/metrics/eps/recorder_test.go
package eps

import "testing"

// newTestRecorder returns a recorder whose clock is driven by *clk (unix sec).
func newTestRecorder(clk *int64) *Recorder {
	r := NewRecorder()
	r.now = func() int64 { return *clk }
	return r
}

func snapFor(snaps []WorkspaceSnapshot, ws string) (Snapshot, bool) {
	for _, s := range snaps {
		if s.WorkspaceID == ws {
			return s.Snapshot, true
		}
	}
	return Snapshot{}, false
}

func TestRollingWindowRates(t *testing.T) {
	var clk int64 = 1000
	r := newTestRecorder(&clk)

	for i := 0; i < 3; i++ {
		r.Observe("ws1")
	}
	clk = 1001
	for i := 0; i < 7; i++ {
		r.Observe("ws1")
	}
	// now = 1002: completed seconds are 1000 (3) and 1001 (7); 1002 in progress.
	clk = 1002

	s, ok := snapFor(r.SnapshotAll(), "ws1")
	if !ok {
		t.Fatal("ws1 snapshot missing")
	}
	if s.Current != 7 { // most recent completed second = 1001
		t.Fatalf("Current: got %v want 7", s.Current)
	}
	if s.Rate10s != 1.0 { // (3+7)/10
		t.Fatalf("Rate10s: got %v want 1.0", s.Rate10s)
	}
	if want := 10.0 / 60.0; s.Rate1m != want { // (3+7)/60
		t.Fatalf("Rate1m: got %v want %v", s.Rate1m, want)
	}
}

func TestCurrentExcludesInProgressSecond(t *testing.T) {
	var clk int64 = 5000
	r := newTestRecorder(&clk)

	// Events only in the current (in-progress) second must not show as Current.
	for i := 0; i < 9; i++ {
		r.Observe("ws1")
	}
	s, _ := snapFor(r.SnapshotAll(), "ws1")
	if s.Current != 0 {
		t.Fatalf("Current (in-progress second): got %v want 0", s.Current)
	}
}

func TestPeakTrackedByRoller(t *testing.T) {
	var clk int64 = 2000
	r := newTestRecorder(&clk)

	for i := 0; i < 20; i++ {
		r.Observe("ws1")
	}
	// Roll at 2001 — second 2000 (20 events) is a completed second:
	// 10s-avg = 20/10 = 2.0 → peak = 2.0.
	clk = 2001
	r.roll(clk)

	s, _ := snapFor(r.SnapshotAll(), "ws1")
	if s.Peak != 2.0 {
		t.Fatalf("Peak after burst: got %v want 2.0", s.Peak)
	}

	// A later quiet period must not lower the peak.
	clk = 2030
	r.roll(clk)
	s, _ = snapFor(r.SnapshotAll(), "ws1")
	if s.Peak != 2.0 {
		t.Fatalf("Peak must not decay: got %v want 2.0", s.Peak)
	}
}

func TestPerWorkspaceIsolation(t *testing.T) {
	var clk int64 = 3000
	r := newTestRecorder(&clk)

	for i := 0; i < 4; i++ {
		r.Observe("wsA")
	}
	for i := 0; i < 11; i++ {
		r.Observe("wsB")
	}
	clk = 3001

	a, _ := snapFor(r.SnapshotAll(), "wsA")
	b, _ := snapFor(r.SnapshotAll(), "wsB")
	if a.Current != 4 {
		t.Fatalf("wsA Current: got %v want 4", a.Current)
	}
	if b.Current != 11 {
		t.Fatalf("wsB Current: got %v want 11", b.Current)
	}
}

func TestEmptyWorkspaceSkipped(t *testing.T) {
	var clk int64 = 4000
	r := newTestRecorder(&clk)
	r.Observe("")
	clk = 4001
	if snaps := r.SnapshotAll(); len(snaps) != 0 {
		t.Fatalf("empty workspace must be skipped, got %d series", len(snaps))
	}
}

func TestIdleWorkspacePruned(t *testing.T) {
	var clk int64 = 7000
	r := newTestRecorder(&clk)
	r.Observe("stale")

	// Past the idle window → roll prunes it.
	clk = 7000 + idleEvictSeconds + 2
	r.roll(clk)
	if _, ok := snapFor(r.SnapshotAll(), "stale"); ok {
		t.Fatal("idle workspace should have been pruned")
	}
}

func TestNilRecorderSafe(t *testing.T) {
	var r *Recorder
	r.Observe("x") // must not panic
	if got := r.SnapshotAll(); got != nil {
		t.Fatalf("nil recorder SnapshotAll: got %v want nil", got)
	}
	r.Start() // must not panic
	r.Stop()  // must not panic
}
