// controllers/ingestapi/ingest_test.go
package ingestapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/hotkhwan/gateway-api/internal/adapters/alertdispatcher"
	"github.com/hotkhwan/gateway-api/internal/services/ingestsvc"
	"github.com/hotkhwan/gateway-api/models/ingestmod"
)

// ============================================================
// Stubs
// ============================================================

// stubIngestSvc always returns a successful, non-pending IngestResult.
type stubIngestSvc struct {
	result   *ingestsvc.IngestResult
	err      error
	gotCamID string // captured from the last call for assertions
}

func (s *stubIngestSvc) Ingest(_ context.Context, _, _, camID, _, _ string, _ []byte) (*ingestsvc.IngestResult, error) {
	s.gotCamID = camID
	if s.err != nil {
		return nil, s.err
	}
	if s.result != nil {
		return s.result, nil
	}
	return &ingestsvc.IngestResult{EventId: "evt-1", ReceivedAt: time.Now()}, nil
}

// spyDispatcher counts how many times Dispatch is called.
type spyDispatcher struct {
	calls int32
}

func (s *spyDispatcher) Dispatch(_ alertdispatcher.FastAlertEnvelope) bool {
	atomic.AddInt32(&s.calls, 1)
	return true
}

// stubAlertDet always returns HasAlert=false so Path A detection never fires.
type stubAlertDet struct{}

func (s *stubAlertDet) HasAlert(_ map[string]any) bool          { return false }
func (s *stubAlertDet) Extract(_ map[string]any) map[string]any { return nil }

// stubBindingGetter returns a fixed (bindings, hit) pair.
type stubBindingGetter struct {
	bindings []ingestmod.TemplateDeliveryBinding
	hit      bool
}

func (s *stubBindingGetter) GetRealtimeBindings(_ context.Context, _ string) ([]ingestmod.TemplateDeliveryBinding, bool) {
	return s.bindings, s.hit
}

// ============================================================
// Helpers
// ============================================================

// newIngestApp builds a Fiber app wired for the Ingest endpoint.
// alertDet=nil means Path A detection is disabled; spyDisp is shared for counting.
func newIngestApp(
	svc ingestSvcI,
	disp alertDispatcherI,
	det alertDetectorI,
	bsvc realtimeBindingGetter,
) *fiber.App {
	app := fiber.New(fiber.Config{})
	ctrl := NewIngestController(svc)
	if disp != nil {
		ctrl.SetAlertDispatcher(disp, det)
	}
	if bsvc != nil {
		ctrl.SetBindingService(bsvc)
	}
	app.Post("/events/:orgId/:sourceFamily", ctrl.Ingest)
	app.Post("/events/:orgId/:sourceFamily/:camID", ctrl.Ingest)
	return app
}

func jsonBody(v any) *bytes.Buffer {
	b, _ := json.Marshal(v)
	return bytes.NewBuffer(b)
}

// makeIngestReq creates a POST /events/:orgId/:sourceFamily request.
func makeIngestReq(orgId, family string, payload any) *http.Request {
	req := httptest.NewRequest("POST", "/events/"+orgId+"/"+family, jsonBody(payload))
	req.Header.Set("Content-Type", "application/json")
	return req
}

// ============================================================
// Path A realtime binding dispatch tests
// ============================================================

// TestRealtimeDispatch_CacheHIT_MatchFields_DispatchCalled verifies that when
// GetRealtimeBindings returns a HIT and the binding's matchFields matches the
// payload, Dispatch is called once.
func TestRealtimeDispatch_CacheHIT_MatchFields_DispatchCalled(t *testing.T) {
	t.Parallel()

	spy := &spyDispatcher{}
	binding := ingestmod.TemplateDeliveryBinding{
		ID:            "b1",
		WorkspaceID:   "org-1",
		DispatchStage: ingestmod.DispatchStageRealtime,
		MatchFields:   map[string]any{"status": "alarm"},
		Enabled:       true,
	}
	bsvc := &stubBindingGetter{bindings: []ingestmod.TemplateDeliveryBinding{binding}, hit: true}

	app := newIngestApp(&stubIngestSvc{}, spy, &stubAlertDet{}, bsvc)
	req := makeIngestReq("org-1", "camera", map[string]any{"status": "alarm", "device": "cam-01"})
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}

	if resp.StatusCode != fiber.StatusAccepted {
		t.Errorf("want 202, got %d", resp.StatusCode)
	}
	if got := atomic.LoadInt32(&spy.calls); got != 1 {
		t.Errorf("want 1 Dispatch call, got %d", got)
	}
}

// TestRealtimeDispatch_CacheMISS_DispatchNotCalled verifies that when
// GetRealtimeBindings returns a MISS (ok=false), Dispatch is never called.
func TestRealtimeDispatch_CacheMISS_DispatchNotCalled(t *testing.T) {
	t.Parallel()

	spy := &spyDispatcher{}
	bsvc := &stubBindingGetter{hit: false} // MISS

	app := newIngestApp(&stubIngestSvc{}, spy, &stubAlertDet{}, bsvc)
	req := makeIngestReq("org-1", "camera", map[string]any{"status": "alarm"})
	resp, _ := app.Test(req)

	if resp.StatusCode != fiber.StatusAccepted {
		t.Errorf("want 202, got %d", resp.StatusCode)
	}
	if got := atomic.LoadInt32(&spy.calls); got != 0 {
		t.Errorf("want 0 Dispatch calls on cache MISS, got %d", got)
	}
}

// TestRealtimeDispatch_CacheHIT_NoMatchFields_DispatchNotCalled verifies that
// even when GetRealtimeBindings returns a HIT, if matchFields does NOT match the
// payload, Dispatch is not called.
func TestRealtimeDispatch_CacheHIT_NoMatchFields_DispatchNotCalled(t *testing.T) {
	t.Parallel()

	spy := &spyDispatcher{}
	binding := ingestmod.TemplateDeliveryBinding{
		ID:            "b1",
		WorkspaceID:   "org-1",
		DispatchStage: ingestmod.DispatchStageRealtime,
		// matchFields requires zone=NORTH but payload has zone=SOUTH
		MatchFields: map[string]any{"zone": "NORTH"},
		Enabled:     true,
	}
	bsvc := &stubBindingGetter{bindings: []ingestmod.TemplateDeliveryBinding{binding}, hit: true}

	app := newIngestApp(&stubIngestSvc{}, spy, &stubAlertDet{}, bsvc)
	req := makeIngestReq("org-1", "camera", map[string]any{"zone": "SOUTH"})
	resp, _ := app.Test(req)

	if resp.StatusCode != fiber.StatusAccepted {
		t.Errorf("want 202, got %d", resp.StatusCode)
	}
	if got := atomic.LoadInt32(&spy.calls); got != 0 {
		t.Errorf("want 0 Dispatch calls when matchFields no match, got %d", got)
	}
}

// TestRealtimeDispatch_MultipleBindings_OnlyMatchingDispatched verifies that
// when multiple bindings exist in cache, only the matching ones call Dispatch.
func TestRealtimeDispatch_MultipleBindings_OnlyMatchingDispatched(t *testing.T) {
	t.Parallel()

	spy := &spyDispatcher{}
	bindings := []ingestmod.TemplateDeliveryBinding{
		{ID: "b1", MatchFields: map[string]any{"type": "fire"}, Enabled: true},  // matches
		{ID: "b2", MatchFields: map[string]any{"type": "flood"}, Enabled: true}, // no match
		{ID: "b3", MatchFields: nil, Enabled: true},                             // wildcard — matches
	}
	bsvc := &stubBindingGetter{bindings: bindings, hit: true}

	app := newIngestApp(&stubIngestSvc{}, spy, &stubAlertDet{}, bsvc)
	req := makeIngestReq("org-1", "sensor", map[string]any{"type": "fire"})
	resp, _ := app.Test(req)

	if resp.StatusCode != fiber.StatusAccepted {
		t.Errorf("want 202, got %d", resp.StatusCode)
	}
	if got := atomic.LoadInt32(&spy.calls); got != 2 {
		t.Errorf("want 2 Dispatch calls (b1+b3), got %d", got)
	}
}

// TestRealtimeDispatch_NilBindingSvc_DispatchNotCalled verifies that when
// bindingSvc is nil (not wired), Dispatch is not called.
func TestRealtimeDispatch_NilBindingSvc_DispatchNotCalled(t *testing.T) {
	t.Parallel()

	spy := &spyDispatcher{}
	app := newIngestApp(&stubIngestSvc{}, spy, &stubAlertDet{}, nil) // no bindingSvc
	req := makeIngestReq("org-1", "camera", map[string]any{"status": "alarm"})
	resp, _ := app.Test(req)

	if resp.StatusCode != fiber.StatusAccepted {
		t.Errorf("want 202, got %d", resp.StatusCode)
	}
	if got := atomic.LoadInt32(&spy.calls); got != 0 {
		t.Errorf("want 0 Dispatch calls when bindingSvc is nil, got %d", got)
	}
}

// TestRealtimeDispatch_NilDispatcher_DoesNotPanic verifies that when alertDisp
// is nil (not wired), realtime dispatch is skipped and the request succeeds.
func TestRealtimeDispatch_NilDispatcher_DoesNotPanic(t *testing.T) {
	t.Parallel()

	binding := ingestmod.TemplateDeliveryBinding{
		ID:          "b1",
		MatchFields: nil,
		Enabled:     true,
	}
	bsvc := &stubBindingGetter{bindings: []ingestmod.TemplateDeliveryBinding{binding}, hit: true}

	// disp=nil, det=nil → both Path A and binding dispatch skip
	app := newIngestApp(&stubIngestSvc{}, nil, nil, bsvc)
	req := makeIngestReq("org-1", "camera", map[string]any{"status": "ok"})
	resp, _ := app.Test(req)

	if resp.StatusCode != fiber.StatusAccepted {
		t.Errorf("want 202, got %d", resp.StatusCode)
	}
}

// TestRealtimeDispatch_WildcardBinding_AlwaysDispatched verifies that a binding
// with empty matchFields dispatches for any payload (wildcard).
func TestRealtimeDispatch_WildcardBinding_AlwaysDispatched(t *testing.T) {
	t.Parallel()

	spy := &spyDispatcher{}
	binding := ingestmod.TemplateDeliveryBinding{
		ID:          "b1",
		MatchFields: map[string]any{}, // empty = wildcard
		Enabled:     true,
	}
	bsvc := &stubBindingGetter{bindings: []ingestmod.TemplateDeliveryBinding{binding}, hit: true}

	app := newIngestApp(&stubIngestSvc{}, spy, &stubAlertDet{}, bsvc)
	req := makeIngestReq("org-1", "camera", map[string]any{"anything": "goes"})
	resp, _ := app.Test(req)

	if resp.StatusCode != fiber.StatusAccepted {
		t.Errorf("want 202, got %d", resp.StatusCode)
	}
	if got := atomic.LoadInt32(&spy.calls); got != 1 {
		t.Errorf("want 1 Dispatch call for wildcard binding, got %d", got)
	}
}

// ============================================================
// Basic ingest controller tests
// ============================================================

func TestIngest_MissingOrgId_400(t *testing.T) {
	t.Parallel()
	app := newIngestApp(&stubIngestSvc{}, nil, nil, nil)
	req := httptest.NewRequest("POST", "/events//camera", jsonBody(map[string]any{"x": 1}))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := app.Test(req)
	// fiber routes don't match empty path param — 404
	if resp.StatusCode == fiber.StatusAccepted {
		t.Errorf("empty orgId should not return 202")
	}
}

func TestIngest_ServiceError_OrgNotFound_404(t *testing.T) {
	t.Parallel()
	svc := &stubIngestSvc{err: ingestsvc.ErrOrgNotFound}
	app := newIngestApp(svc, nil, nil, nil)
	req := makeIngestReq("org-x", "camera", map[string]any{"x": 1})
	resp, _ := app.Test(req)
	if resp.StatusCode != fiber.StatusNotFound {
		t.Errorf("want 404, got %d", resp.StatusCode)
	}
}

func TestIngest_ServiceError_RateLimited_429(t *testing.T) {
	t.Parallel()
	svc := &stubIngestSvc{err: ingestsvc.ErrRateLimited}
	app := newIngestApp(svc, nil, nil, nil)
	req := makeIngestReq("org-1", "camera", map[string]any{"x": 1})
	resp, _ := app.Test(req)
	if resp.StatusCode != fiber.StatusTooManyRequests {
		t.Errorf("want 429, got %d", resp.StatusCode)
	}
}

func TestIngest_ServiceError_PayloadTooLarge_413(t *testing.T) {
	t.Parallel()
	svc := &stubIngestSvc{err: ingestsvc.ErrPayloadTooLarge}
	app := newIngestApp(svc, nil, nil, nil)
	req := makeIngestReq("org-1", "camera", map[string]any{"x": 1})
	resp, _ := app.Test(req)
	if resp.StatusCode != fiber.StatusRequestEntityTooLarge {
		t.Errorf("want 413, got %d", resp.StatusCode)
	}
}

func TestIngest_Pending_202WithPendingCode(t *testing.T) {
	t.Parallel()
	svc := &stubIngestSvc{result: &ingestsvc.IngestResult{EventId: "ev-p", Pending: true}}
	app := newIngestApp(svc, nil, nil, nil)
	req := makeIngestReq("org-1", "camera", map[string]any{"x": 1})
	resp, _ := app.Test(req)
	if resp.StatusCode != fiber.StatusAccepted {
		t.Errorf("want 202, got %d", resp.StatusCode)
	}
	var body map[string]any
	json.NewDecoder(resp.Body).Decode(&body)
	if body["code"] != "PENDING_REVIEW" {
		t.Errorf("want code=PENDING_REVIEW, got %v", body["code"])
	}
}

func TestIngest_Success_202(t *testing.T) {
	t.Parallel()
	app := newIngestApp(&stubIngestSvc{}, nil, nil, nil)
	req := makeIngestReq("org-1", "camera", map[string]any{"data": "ok"})
	resp, _ := app.Test(req)
	if resp.StatusCode != fiber.StatusAccepted {
		t.Errorf("want 202, got %d", resp.StatusCode)
	}
}

func TestIngest_CamIDPath_PassesCamID(t *testing.T) {
	svc := &stubIngestSvc{}
	app := newIngestApp(svc, nil, nil, nil)
	req := httptest.NewRequest("POST", "/events/org-1/dahua/cam-42", jsonBody(map[string]any{"x": 1}))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := app.Test(req)
	if resp.StatusCode != fiber.StatusAccepted {
		t.Fatalf("want 202, got %d", resp.StatusCode)
	}
	if svc.gotCamID != "cam-42" {
		t.Fatalf("camID not threaded from path: got %q want cam-42", svc.gotCamID)
	}
}

func TestIngest_LegacyPath_EmptyCamID(t *testing.T) {
	svc := &stubIngestSvc{}
	app := newIngestApp(svc, nil, nil, nil)
	resp, _ := app.Test(makeIngestReq("org-1", "dahua", map[string]any{"x": 1}))
	if resp.StatusCode != fiber.StatusAccepted {
		t.Fatalf("want 202, got %d", resp.StatusCode)
	}
	if svc.gotCamID != "" {
		t.Fatalf("legacy path must have empty camID, got %q", svc.gotCamID)
	}
}
