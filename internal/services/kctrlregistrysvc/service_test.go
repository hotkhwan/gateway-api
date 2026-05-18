package kctrlregistrysvc

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/hotkhwan/gateway-api/internal/repo/kctrlregistryrepo"
	"github.com/hotkhwan/gateway-api/models/kctrlmod"
)

// fakeRepo is an in-memory registry implementation used by the service tests.
// Calls to FindByHwId / Upsert / Delete are recorded so tests can assert the
// service does not over- or under-read the persistence layer.
type fakeRepo struct {
	rows map[string]*kctrlmod.KctrlRegistry

	findCalls   int
	upsertCalls int
	deleteCalls int

	driftRows []kctrlregistryrepo.DriftRow
	driftErr  error
	countAll  int64
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{rows: map[string]*kctrlmod.KctrlRegistry{}}
}

func (f *fakeRepo) FindByHwId(_ context.Context, hwId string) (*kctrlmod.KctrlRegistry, error) {
	f.findCalls++
	if r, ok := f.rows[hwId]; ok {
		copyRow := *r
		return &copyRow, nil
	}
	return nil, kctrlregistryrepo.ErrNotFound
}

func (f *fakeRepo) Upsert(_ context.Context, doc *kctrlmod.KctrlRegistry) (*kctrlmod.KctrlRegistry, error) {
	f.upsertCalls++
	copyRow := *doc
	f.rows[doc.HwId] = &copyRow
	ret := copyRow
	return &ret, nil
}

func (f *fakeRepo) Delete(_ context.Context, hwId string) error {
	f.deleteCalls++
	delete(f.rows, hwId)
	return nil
}

func (f *fakeRepo) ListDrift(_ context.Context, _ kctrlregistryrepo.DriftFilter) ([]kctrlregistryrepo.DriftRow, error) {
	return f.driftRows, f.driftErr
}

func (f *fakeRepo) CountAll(_ context.Context) (int64, error) {
	return f.countAll, nil
}

func TestService_Upsert_PersistsAndCachesAfterInvalidation(t *testing.T) {
	repo := newFakeRepo()
	svc := NewService(repo)

	ctx := context.Background()
	in := UpsertInput{
		HwId:        "h-1",
		OrgId:       "org-1",
		Approved:    true,
		ApprovedAt:  time.Date(2026, 5, 18, 10, 0, 0, 0, time.UTC),
		ApprovedBy:  "user-1",
		WorkspaceId: "ws-1",
	}
	out, err := svc.Upsert(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	if out == nil || out.HwId != "h-1" || out.OrgId != "org-1" || !out.Approved {
		t.Errorf("upsert result missing fields: %+v", out)
	}
	if out.LastOutboundHash == "" {
		t.Error("expected lastOutboundHash to be populated")
	}
	// Service must invalidate the cache so the originating replica sees fresh
	// state on the next Decide() call. Decide() will round-trip to the repo.
	dec := svc.Decide(ctx, "h-1")
	if dec.Action != ActionEnrich {
		t.Errorf("Decide after Upsert: got %s, want enrich", dec.Action)
	}
	if dec.OrgId != "org-1" || dec.WorkspaceId != "ws-1" {
		t.Errorf("Decide returned wrong enrichment: %+v", dec)
	}
}

func TestService_Decide_ApprovedFalse_Drops(t *testing.T) {
	repo := newFakeRepo()
	repo.rows["h-1"] = &kctrlmod.KctrlRegistry{HwId: "h-1", OrgId: "org-1", Approved: false}
	svc := NewService(repo)

	dec := svc.Decide(context.Background(), "h-1")
	if dec.Action != ActionDrop {
		t.Errorf("got %s, want drop", dec.Action)
	}
}

func TestService_Decide_NotFound_Forwards(t *testing.T) {
	repo := newFakeRepo()
	svc := NewService(repo)

	dec := svc.Decide(context.Background(), "unknown")
	if dec.Action != ActionForward {
		t.Errorf("got %s, want forward", dec.Action)
	}
	// Negative cache: second call must NOT hit the repo again.
	first := repo.findCalls
	_ = svc.Decide(context.Background(), "unknown")
	if repo.findCalls != first {
		t.Errorf("second Decide bypassed cache: findCalls %d → %d", first, repo.findCalls)
	}
}

func TestService_Decide_RepoErrorFallsThroughToForward(t *testing.T) {
	// Wrap fakeRepo with one that returns a non-sentinel error.
	repo := &errorRepo{}
	svc := NewService(repo)

	dec := svc.Decide(context.Background(), "h-x")
	if dec.Action != ActionForward {
		t.Errorf("got %s, want forward (degraded mode)", dec.Action)
	}
}

func TestService_Delete_InvalidatesCache(t *testing.T) {
	repo := newFakeRepo()
	repo.rows["h-1"] = &kctrlmod.KctrlRegistry{HwId: "h-1", OrgId: "org-1", Approved: true}
	svc := NewService(repo)

	// Warm cache via Decide
	if dec := svc.Decide(context.Background(), "h-1"); dec.Action != ActionEnrich {
		t.Fatalf("expected enrich, got %s", dec.Action)
	}
	if err := svc.Delete(context.Background(), "h-1"); err != nil {
		t.Fatal(err)
	}
	if repo.deleteCalls != 1 {
		t.Errorf("repo.deleteCalls = %d, want 1", repo.deleteCalls)
	}
	// After delete, Decide must NOT return cached enrich — should observe absence.
	dec := svc.Decide(context.Background(), "h-1")
	if dec.Action != ActionForward {
		t.Errorf("after Delete: got %s, want forward", dec.Action)
	}
}

func TestService_Upsert_IdempotentHash(t *testing.T) {
	repo := newFakeRepo()
	svc := NewService(repo)

	in := UpsertInput{
		HwId:       "h-1",
		OrgId:      "org-1",
		Approved:   true,
		ApprovedAt: time.Date(2026, 5, 18, 10, 0, 0, 0, time.UTC),
		ApprovedBy: "user-1",
	}
	first, err := svc.Upsert(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	second, err := svc.Upsert(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	// Same body → same canonical hash. Different lastSyncFromKlynxAt is
	// acceptable (the bookkeeping field bumps on every PATCH).
	if first.LastOutboundHash != second.LastOutboundHash {
		t.Errorf("hash drift across identical PATCHes: %q vs %q", first.LastOutboundHash, second.LastOutboundHash)
	}
}

// Phase A.1 strict-mode tests — contract §5.2 fourth branch.

func TestService_StrictMode_OffPreservesCompatForward(t *testing.T) {
	// strictMode=false (Phase A default) → ROW NOT FOUND always FORWARDS.
	repo := newFakeRepo()
	svc := NewServiceWithOptions(repo, Options{StrictMode: false})

	if dec := svc.Decide(context.Background(), "unknown"); dec.Action != ActionForward {
		t.Errorf("got %s, want forward (strict off)", dec.Action)
	}
}

func TestService_StrictMode_OnButWithinGrace_StillForwards(t *testing.T) {
	// strictMode=true but the registry has had recent writes (within the
	// 5-minute grace window) → keep compat behavior so backfill doesn't get
	// pre-empted mid-flight.
	repo := newFakeRepo()
	svc := NewServiceWithOptions(repo, Options{StrictMode: true})

	now := time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC)
	svc.clock = func() time.Time { return now }
	// Simulate a recent Upsert by manually bumping lastWriteAt — this is
	// what NewServiceWithOptions already does at construction time. We then
	// advance the clock by 4 minutes (still inside the 5-minute grace).
	svc.lastWriteAt.Store(now.UnixNano())
	now = now.Add(4 * time.Minute)

	if dec := svc.Decide(context.Background(), "unknown"); dec.Action != ActionForward {
		t.Errorf("got %s, want forward (within grace)", dec.Action)
	}
}

func TestService_StrictMode_OnAndStale_Drops(t *testing.T) {
	// strictMode=true AND the registry has been quiet for > 5 min → DROP
	// the unknown-hwId message at the gateway boundary.
	repo := newFakeRepo()
	svc := NewServiceWithOptions(repo, Options{StrictMode: true})

	now := time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC)
	svc.clock = func() time.Time { return now }
	svc.lastWriteAt.Store(now.UnixNano())
	now = now.Add(5*time.Minute + 1*time.Nanosecond) // just outside grace

	if dec := svc.Decide(context.Background(), "unknown"); dec.Action != ActionDrop {
		t.Errorf("got %s, want drop (registry stale, strict on)", dec.Action)
	}
}

func TestService_StrictMode_UpsertRefreshesGrace(t *testing.T) {
	// A new Upsert must reset the grace window — so a long-quiet registry
	// that just received a klynx-api PATCH switches back to FORWARD mode
	// until 5 minutes elapse again.
	repo := newFakeRepo()
	svc := NewServiceWithOptions(repo, Options{StrictMode: true})

	now := time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC)
	svc.clock = func() time.Time { return now }
	svc.lastWriteAt.Store(now.UnixNano())
	now = now.Add(10 * time.Minute) // outside grace — strict drop would apply

	// Pre-flight: confirm strict drop is now in effect.
	if dec := svc.Decide(context.Background(), "unknown"); dec.Action != ActionDrop {
		t.Fatalf("pre-flight: got %s, want drop", dec.Action)
	}

	// Now an Upsert arrives → bumps lastWriteAt to "now"; grace resets.
	in := UpsertInput{HwId: "h-new", OrgId: "org-1", Approved: true, ApprovedAt: now}
	if _, err := svc.Upsert(context.Background(), in); err != nil {
		t.Fatal(err)
	}
	// Within the new grace, unknowns FORWARD again.
	if dec := svc.Decide(context.Background(), "other-unknown"); dec.Action != ActionForward {
		t.Errorf("after Upsert: got %s, want forward (grace refreshed)", dec.Action)
	}
}

func TestService_StrictMode_DegradedMongo_DoesNotDrop(t *testing.T) {
	// When Mongo lookups error, Decide() falls through to FORWARD
	// unconditionally — strict mode must not amplify a degraded backend
	// into a realtime outage.
	svc := NewServiceWithOptions(errorRepo{}, Options{StrictMode: true})

	now := time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC)
	svc.clock = func() time.Time { return now }
	svc.lastWriteAt.Store(now.UnixNano())
	now = now.Add(10 * time.Minute)

	if dec := svc.Decide(context.Background(), "any"); dec.Action != ActionForward {
		t.Errorf("degraded Mongo + strict: got %s, want forward", dec.Action)
	}
}

func TestService_ListDrift_ReportsSummary(t *testing.T) {
	repo := newFakeRepo()
	repo.countAll = 12
	staleAt := time.Now().UTC().Add(-2 * time.Hour)
	repo.driftRows = []kctrlregistryrepo.DriftRow{
		{HwId: "h-1", OrgId: "org-1", LastSyncFromKlynxAt: staleAt},
		{HwId: "h-2", OrgId: "org-2", LastSyncFromKlynxAt: staleAt},
	}
	svc := NewService(repo)

	rep, err := svc.ListDrift(context.Background(), time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Summary.Total != 12 || rep.Summary.Stale != 2 {
		t.Errorf("summary mismatch: total=%d stale=%d", rep.Summary.Total, rep.Summary.Stale)
	}
	if len(rep.Items) != 2 || rep.Items[0].Reason != "stale" {
		t.Errorf("items mismatch: %+v", rep.Items)
	}
}

// errorRepo returns a non-sentinel error to exercise the degraded-mode fall-
// through path in Decide.
type errorRepo struct{}

func (errorRepo) FindByHwId(context.Context, string) (*kctrlmod.KctrlRegistry, error) {
	return nil, errors.New("mongo down")
}
func (errorRepo) Upsert(context.Context, *kctrlmod.KctrlRegistry) (*kctrlmod.KctrlRegistry, error) {
	return nil, errors.New("mongo down")
}
func (errorRepo) Delete(context.Context, string) error {
	return errors.New("mongo down")
}
func (errorRepo) ListDrift(context.Context, kctrlregistryrepo.DriftFilter) ([]kctrlregistryrepo.DriftRow, error) {
	return nil, errors.New("mongo down")
}
func (errorRepo) CountAll(context.Context) (int64, error) {
	return 0, errors.New("mongo down")
}
