// internal/kafka/normalizedcons/consumer_test.go
package normalizedcons

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hotkhwan/gateway-api/config"
	"github.com/hotkhwan/gateway-api/internal/eventschema"
	"github.com/hotkhwan/gateway-api/models/authzmod"
	"github.com/hotkhwan/gateway-api/models/ingestmod"
	"github.com/rs/zerolog"
	"github.com/segmentio/kafka-go"
)

// TestMain sets a dummy KAFKA_BROKER before any test runs so that
// config.ensureKafkaWriter initialises a non-nil Writer on the first call.
// Without this, ensureKafkaWriter stores the "not set" error in a local var;
// on the second call kafkaOnce.Do is skipped, the error is lost, and
// SendToKafkaWithCtx proceeds with a nil Writer → nil pointer panic.
func TestMain(m *testing.M) {
	os.Setenv("KAFKA_BROKER", "localhost:9092") // dummy — never actually connected
	config.InitKafka()                          // pre-warm once with a non-nil Writer
	os.Exit(m.Run())
}

// ============================================================
// Test stubs
// ============================================================

// stubEventDetailsRepo returns nil on Upsert (success).
type stubEventDetailsRepo struct {
	upsertErr error
}

func (s *stubEventDetailsRepo) Upsert(_ context.Context, _ *ingestmod.NormalizedEvent) error {
	return s.upsertErr
}

// stubBindingQuerier returns a fixed list of bindings.
type stubBindingQuerier struct {
	bindings []ingestmod.TemplateDeliveryBinding
	err      error
}

func (s *stubBindingQuerier) GetNormalizeBindings(_ context.Context, _ string) ([]ingestmod.TemplateDeliveryBinding, error) {
	return s.bindings, s.err
}

// stubTargetLookup returns a target for known IDs.
type stubTargetLookup struct {
	targets map[string]*authzmod.DeliveryTarget
}

func (s *stubTargetLookup) FindByIDAndOrg(_ context.Context, targetId, _, _ string) (*authzmod.DeliveryTarget, error) {
	if t, ok := s.targets[targetId]; ok {
		return t, nil
	}
	return nil, nil
}

// spyEventBridgePub counts Publish calls.
type spyEventBridgePub struct {
	calls int32
}

func (s *spyEventBridgePub) Publish(_ context.Context, _ eventschema.NormalizedEvent) error {
	atomic.AddInt32(&s.calls, 1)
	return nil
}

// stubKlynxTargetChecker returns a fixed has/err pair.
type stubKlynxTargetChecker struct {
	has bool
	err error
}

func (s *stubKlynxTargetChecker) HasKlynxTarget(_ context.Context, _ string) (bool, error) {
	return s.has, s.err
}

// ============================================================
// Helpers
// ============================================================

// noopLogger returns a zerolog.Logger that discards all output.
func noopLogger() zerolog.Logger {
	return zerolog.Nop()
}

// canonicalEventJSON returns minimal valid CanonicalEvent JSON.
// lat=0, lng=0 so geo functions return early without touching BoundaryIndex.
func canonicalEventJSON(eventId, orgId string) []byte {
	ev := ingestmod.CanonicalEvent{
		EventId:     eventId,
		TenantId:    "tenant-1",
		SourceFamily: "test",
		EventType:   "test.event",
		OccurredAt:  time.Now().UTC(),
		Source: ingestmod.SourceInfo{
			DeviceId:   "dev-1",
			DeviceType: "camera",
			OrgId:      orgId,
		},
		Location: ingestmod.LocationInfo{Lat: 0, Lng: 0},
		Payload:  map[string]any{"status": "ok"},
	}
	b, _ := json.Marshal(ev)
	return b
}

// minimalDeps returns ConsumerDeps wired for unit tests:
// - no TemplateRepo (nil → applyTemplate skips with empty templateId)
// - no DLQRepo (nil-guarded in consumer)
// - no BindingQuerier/TargetLookup (disables binding dispatch goroutine)
// - zero GeoConfig (safe at lat=0,lng=0)
func minimalDeps(edr eventDetailsRepoI) ConsumerDeps {
	return ConsumerDeps{
		EventDetailsRepo: edr,
		TemplateRepo:     nil,
		DLQRepo:          nil,
		GeoCfg:           GeoConfig{},
		Logger:           noopLogger(),
	}
}

// kafkaMsg wraps payload bytes into a kafka.Message with optional headers.
func kafkaMsg(value []byte, headers map[string]string) kafka.Message {
	m := kafka.Message{Value: value}
	for k, v := range headers {
		m.Headers = append(m.Headers, kafka.Header{Key: k, Value: []byte(v)})
	}
	return m
}

// ============================================================
// bindingMatchesFields — pure function (table-driven)
// ============================================================

func TestBindingMatchesFields(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		payload     map[string]any
		matchFields map[string]any
		want        bool
	}{
		{
			name:        "empty matchFields is wildcard",
			payload:     map[string]any{"status": "ok"},
			matchFields: map[string]any{},
			want:        true,
		},
		{
			name:        "nil matchFields is wildcard",
			payload:     map[string]any{"status": "ok"},
			matchFields: nil,
			want:        true,
		},
		{
			name:        "single field match",
			payload:     map[string]any{"eventType": "intrusion"},
			matchFields: map[string]any{"eventType": "intrusion"},
			want:        true,
		},
		{
			name:        "single field no match",
			payload:     map[string]any{"eventType": "motion"},
			matchFields: map[string]any{"eventType": "intrusion"},
			want:        false,
		},
		{
			name:        "multiple fields all match",
			payload:     map[string]any{"type": "A", "zone": "1"},
			matchFields: map[string]any{"type": "A", "zone": "1"},
			want:        true,
		},
		{
			name:        "multiple fields partial match",
			payload:     map[string]any{"type": "A", "zone": "2"},
			matchFields: map[string]any{"type": "A", "zone": "1"},
			want:        false,
		},
		{
			name:        "field missing from payload",
			payload:     map[string]any{"status": "ok"},
			matchFields: map[string]any{"missing": "x"},
			want:        false,
		},
		{
			name:        "numeric value matches as string",
			payload:     map[string]any{"level": 5},
			matchFields: map[string]any{"level": 5},
			want:        true,
		},
		{
			name:        "numeric mismatch",
			payload:     map[string]any{"level": 5},
			matchFields: map[string]any{"level": 9},
			want:        false,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := bindingMatchesFields(tc.payload, tc.matchFields)
			if got != tc.want {
				t.Errorf("bindingMatchesFields() = %v, want %v", got, tc.want)
			}
		})
	}
}

// ============================================================
// dispatchNormalizeBindings — routing & matchFields gate
// ============================================================

// countingServer creates an httptest.Server that counts received POST requests.
func countingServer(t *testing.T) (*httptest.Server, *int32) {
	t.Helper()
	var count int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&count, 1)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)
	return srv, &count
}

// webhookTarget returns an enabled webhook DeliveryTarget pointing at url.
func webhookTarget(id, url string) *authzmod.DeliveryTarget {
	return &authzmod.DeliveryTarget{
		TargetId:    id,
		WorkspaceId: "org-1",
		Type:        authzmod.TargetTypeWebhook,
		Enabled:     true,
		Config:      authzmod.TargetConfig{URL: url, TimeoutMs: 5000},
	}
}

// normalizedEvent returns a minimal NormalizedEvent for binding dispatch tests.
func normalizedEvent(orgId string) *ingestmod.NormalizedEvent {
	return &ingestmod.NormalizedEvent{
		EventId:  "evt-1",
		TenantId: "tenant-1",
		Payload:  map[string]any{"status": "ok", "zone": "A"},
	}
}

func TestDispatchNormalizeBindings_MatchedBinding_WebhookCalled(t *testing.T) {
	t.Parallel()

	srv, count := countingServer(t)

	deps := ConsumerDeps{
		Logger: noopLogger(),
		BindingQuerier: &stubBindingQuerier{
			bindings: []ingestmod.TemplateDeliveryBinding{
				{ID: "b1", TargetID: "tgt-1", MatchFields: nil, Enabled: true},
			},
		},
		TargetLookup: &stubTargetLookup{
			targets: map[string]*authzmod.DeliveryTarget{
				"tgt-1": webhookTarget("tgt-1", srv.URL),
			},
		},
	}

	event := normalizedEvent("org-1")
	dispatchNormalizeBindings(context.Background(), "org-1", "tenant-1", event, deps)

	if atomic.LoadInt32(count) != 1 {
		t.Errorf("want 1 webhook call, got %d", atomic.LoadInt32(count))
	}
}

func TestDispatchNormalizeBindings_NoMatchFields_WebhookNotCalled(t *testing.T) {
	t.Parallel()

	srv, count := countingServer(t)

	deps := ConsumerDeps{
		Logger: noopLogger(),
		BindingQuerier: &stubBindingQuerier{
			bindings: []ingestmod.TemplateDeliveryBinding{
				{
					ID:       "b1",
					TargetID: "tgt-1",
					// matchFields requires "zone":"MISS" but event has "zone":"A"
					MatchFields: map[string]any{"zone": "MISS"},
					Enabled:     true,
				},
			},
		},
		TargetLookup: &stubTargetLookup{
			targets: map[string]*authzmod.DeliveryTarget{
				"tgt-1": webhookTarget("tgt-1", srv.URL),
			},
		},
	}

	event := normalizedEvent("org-1")
	dispatchNormalizeBindings(context.Background(), "org-1", "tenant-1", event, deps)

	if atomic.LoadInt32(count) != 0 {
		t.Errorf("want 0 webhook calls, got %d", atomic.LoadInt32(count))
	}
}

func TestDispatchNormalizeBindings_DisabledTarget_NotCalled(t *testing.T) {
	t.Parallel()

	srv, count := countingServer(t)

	disabledTarget := webhookTarget("tgt-1", srv.URL)
	disabledTarget.Enabled = false

	deps := ConsumerDeps{
		Logger: noopLogger(),
		BindingQuerier: &stubBindingQuerier{
			bindings: []ingestmod.TemplateDeliveryBinding{
				{ID: "b1", TargetID: "tgt-1", MatchFields: nil, Enabled: true},
			},
		},
		TargetLookup: &stubTargetLookup{
			targets: map[string]*authzmod.DeliveryTarget{"tgt-1": disabledTarget},
		},
	}

	event := normalizedEvent("org-1")
	dispatchNormalizeBindings(context.Background(), "org-1", "tenant-1", event, deps)

	if atomic.LoadInt32(count) != 0 {
		t.Errorf("want 0 webhook calls for disabled target, got %d", atomic.LoadInt32(count))
	}
}

func TestDispatchNormalizeBindings_MultipleBindings_EachDispatched(t *testing.T) {
	t.Parallel()

	srv, count := countingServer(t)

	deps := ConsumerDeps{
		Logger: noopLogger(),
		BindingQuerier: &stubBindingQuerier{
			bindings: []ingestmod.TemplateDeliveryBinding{
				{ID: "b1", TargetID: "tgt-1", MatchFields: nil, Enabled: true},
				{ID: "b2", TargetID: "tgt-2", MatchFields: nil, Enabled: true},
				{ID: "b3", TargetID: "tgt-3", MatchFields: nil, Enabled: true},
			},
		},
		TargetLookup: &stubTargetLookup{
			targets: map[string]*authzmod.DeliveryTarget{
				"tgt-1": webhookTarget("tgt-1", srv.URL),
				"tgt-2": webhookTarget("tgt-2", srv.URL),
				"tgt-3": webhookTarget("tgt-3", srv.URL),
			},
		},
	}

	event := normalizedEvent("org-1")
	dispatchNormalizeBindings(context.Background(), "org-1", "tenant-1", event, deps)

	if got := atomic.LoadInt32(count); got != 3 {
		t.Errorf("want 3 webhook calls for 3 bindings, got %d", got)
	}
}

func TestDispatchNormalizeBindings_BindingQuerierError_NoCalls(t *testing.T) {
	t.Parallel()

	srv, count := countingServer(t)
	_ = srv // server present but should not receive any call

	deps := ConsumerDeps{
		Logger: noopLogger(),
		BindingQuerier: &stubBindingQuerier{
			err: errTest,
		},
		TargetLookup: &stubTargetLookup{
			targets: map[string]*authzmod.DeliveryTarget{},
		},
	}

	event := normalizedEvent("org-1")
	dispatchNormalizeBindings(context.Background(), "org-1", "tenant-1", event, deps)

	if atomic.LoadInt32(count) != 0 {
		t.Errorf("want 0 calls on querier error, got %d", atomic.LoadInt32(count))
	}
}

func TestDispatchNormalizeBindings_MixedMatchFields(t *testing.T) {
	t.Parallel()

	srv, count := countingServer(t)

	// binding1: zone=A → matches event (zone=A)
	// binding2: zone=B → does NOT match
	deps := ConsumerDeps{
		Logger: noopLogger(),
		BindingQuerier: &stubBindingQuerier{
			bindings: []ingestmod.TemplateDeliveryBinding{
				{ID: "b1", TargetID: "tgt-1", MatchFields: map[string]any{"zone": "A"}, Enabled: true},
				{ID: "b2", TargetID: "tgt-2", MatchFields: map[string]any{"zone": "B"}, Enabled: true},
			},
		},
		TargetLookup: &stubTargetLookup{
			targets: map[string]*authzmod.DeliveryTarget{
				"tgt-1": webhookTarget("tgt-1", srv.URL),
				"tgt-2": webhookTarget("tgt-2", srv.URL),
			},
		},
	}

	event := normalizedEvent("org-1")
	dispatchNormalizeBindings(context.Background(), "org-1", "tenant-1", event, deps)

	if got := atomic.LoadInt32(count); got != 1 {
		t.Errorf("want 1 webhook call (only matching binding), got %d", got)
	}
}

// sentinel for tests
var errTest = &testErr{"test error"}

type testErr struct{ msg string }

func (e *testErr) Error() string { return e.msg }

// ============================================================
// EventBridge gate (handleRawEvent) — sequential (t.Setenv)
// ============================================================

// TestEventBridgeGate is deliberately NOT parallel — it calls t.Setenv which
// requires the test and all its subtests to be sequential.
func TestEventBridgeGate(t *testing.T) {
	makeMsg := func(orgId string) kafka.Message {
		return kafkaMsg(canonicalEventJSON("evt-eb", orgId), map[string]string{
			"orgId":    orgId,
			"tenantId": "tenant-1",
		})
	}

	t.Run("appliance_has_klynx_target_publishes", func(t *testing.T) {
		t.Setenv("DEPLOYMENT_PROFILE", "appliance")

		spy := &spyEventBridgePub{}
		deps := minimalDeps(&stubEventDetailsRepo{})
		deps.EventBridgePub = spy
		deps.KlynxTargetChecker = &stubKlynxTargetChecker{has: true}

		_ = handleRawEvent(context.Background(), makeMsg("org-eb"), deps)

		if atomic.LoadInt32(&spy.calls) != 1 {
			t.Errorf("want 1 EventBridge publish, got %d", atomic.LoadInt32(&spy.calls))
		}
	})

	t.Run("appliance_no_klynx_target_skips", func(t *testing.T) {
		t.Setenv("DEPLOYMENT_PROFILE", "appliance")

		spy := &spyEventBridgePub{}
		deps := minimalDeps(&stubEventDetailsRepo{})
		deps.EventBridgePub = spy
		deps.KlynxTargetChecker = &stubKlynxTargetChecker{has: false}

		_ = handleRawEvent(context.Background(), makeMsg("org-eb"), deps)

		if atomic.LoadInt32(&spy.calls) != 0 {
			t.Errorf("want 0 EventBridge publishes when no klynx target, got %d", atomic.LoadInt32(&spy.calls))
		}
	})

	t.Run("appliance_checker_nil_publishes_unconditionally", func(t *testing.T) {
		t.Setenv("DEPLOYMENT_PROFILE", "appliance")

		spy := &spyEventBridgePub{}
		deps := minimalDeps(&stubEventDetailsRepo{})
		deps.EventBridgePub = spy
		deps.KlynxTargetChecker = nil // no checker → always publish

		_ = handleRawEvent(context.Background(), makeMsg("org-eb"), deps)

		if atomic.LoadInt32(&spy.calls) != 1 {
			t.Errorf("want 1 publish when checker is nil, got %d", atomic.LoadInt32(&spy.calls))
		}
	})

	t.Run("appliance_checker_error_skips", func(t *testing.T) {
		t.Setenv("DEPLOYMENT_PROFILE", "appliance")

		spy := &spyEventBridgePub{}
		deps := minimalDeps(&stubEventDetailsRepo{})
		deps.EventBridgePub = spy
		deps.KlynxTargetChecker = &stubKlynxTargetChecker{has: false, err: errTest}

		_ = handleRawEvent(context.Background(), makeMsg("org-eb"), deps)

		if atomic.LoadInt32(&spy.calls) != 0 {
			t.Errorf("want 0 publishes when checker returns error, got %d", atomic.LoadInt32(&spy.calls))
		}
	})

	t.Run("non_appliance_profile_skips_eventbridge", func(t *testing.T) {
		t.Setenv("DEPLOYMENT_PROFILE", "other")

		spy := &spyEventBridgePub{}
		deps := minimalDeps(&stubEventDetailsRepo{})
		deps.EventBridgePub = spy
		deps.KlynxTargetChecker = &stubKlynxTargetChecker{has: true}

		_ = handleRawEvent(context.Background(), makeMsg("org-eb"), deps)

		if atomic.LoadInt32(&spy.calls) != 0 {
			t.Errorf("want 0 publishes on non-appliance profile, got %d", atomic.LoadInt32(&spy.calls))
		}
	})

	t.Run("no_eventbridge_pub_is_noop", func(t *testing.T) {
		t.Setenv("DEPLOYMENT_PROFILE", "appliance")

		deps := minimalDeps(&stubEventDetailsRepo{})
		deps.EventBridgePub = nil // no publisher wired

		err := handleRawEvent(context.Background(), makeMsg("org-eb"), deps)
		if err != nil {
			t.Errorf("want nil error, got %v", err)
		}
	})
}

// ============================================================
// Source enrichment persisted into event_details
// (event-source-enrichment-persistence.md — verifies the consumer
// backfills canonical.Source.DeviceMgmtId from the resolver and
// canonical.Source.SN from canonical.Payload["sn"] *before* the
// event_details.Upsert. Earlier these values reached the bridge
// republish struct but never the persisted document.)
// ============================================================

// recordingEventDetailsRepo captures the NormalizedEvent passed to Upsert
// so tests can inspect what would actually be persisted.
type recordingEventDetailsRepo struct {
	last *ingestmod.NormalizedEvent
}

func (r *recordingEventDetailsRepo) Upsert(_ context.Context, ev *ingestmod.NormalizedEvent) error {
	r.last = ev
	return nil
}

// stubDeviceMgmtResolver returns a fixed DeviceManagement record (or nil).
type stubDeviceMgmtResolver struct {
	rec *ingestmod.DeviceManagement
}

func (s *stubDeviceMgmtResolver) Resolve(_ context.Context, _, _, _, _, _ string) *ingestmod.DeviceManagement {
	return s.rec
}

// canonicalEventWithPayload builds a CanonicalEvent JSON allowing the caller
// to override the raw payload (so we can drop "sn" in).
func canonicalEventWithPayload(eventId, orgId string, payload map[string]any) []byte {
	ev := ingestmod.CanonicalEvent{
		EventId:      eventId,
		TenantId:     "tenant-1",
		SourceFamily: "AIBOX",
		EventType:    "test.event",
		OccurredAt:   time.Now().UTC(),
		Source: ingestmod.SourceInfo{
			DeviceId:   "channel-24",
			DeviceType: "channel",
			OrgId:      orgId,
		},
		Location: ingestmod.LocationInfo{Lat: 0, Lng: 0},
		Payload:  payload,
	}
	b, _ := json.Marshal(ev)
	return b
}

func TestSourceEnrichmentPersisted_DeviceMgmtIdFromResolver(t *testing.T) {
	t.Parallel()

	rec := &recordingEventDetailsRepo{}
	deps := minimalDeps(rec)
	deps.DeviceMgmtResolver = &stubDeviceMgmtResolver{
		rec: &ingestmod.DeviceManagement{
			DeviceMgmtId: "dm-uuid-abc",
			Lat:          0,
			Lng:          0,
		},
	}

	msg := kafkaMsg(canonicalEventWithPayload("evt-dm", "org-1", map[string]any{"status": "ok"}), map[string]string{
		"orgId":    "org-1",
		"tenantId": "tenant-1",
	})

	if err := handleRawEvent(context.Background(), msg, deps); err != nil {
		t.Fatalf("handleRawEvent: %v", err)
	}
	if rec.last == nil {
		t.Fatal("event_details Upsert was not called")
	}
	if got := rec.last.Source.DeviceMgmtId; got != "dm-uuid-abc" {
		t.Errorf("Source.DeviceMgmtId = %q, want %q", got, "dm-uuid-abc")
	}
}

func TestSourceEnrichmentPersisted_SNFromRawPayload(t *testing.T) {
	t.Parallel()

	rec := &recordingEventDetailsRepo{}
	deps := minimalDeps(rec)
	deps.DeviceMgmtResolver = nil // no DM enrichment — only SN should populate

	msg := kafkaMsg(canonicalEventWithPayload("evt-sn", "org-1", map[string]any{
		"sn":     "60109012431d6040",
		"status": "ok",
	}), map[string]string{
		"orgId":    "org-1",
		"tenantId": "tenant-1",
	})

	if err := handleRawEvent(context.Background(), msg, deps); err != nil {
		t.Fatalf("handleRawEvent: %v", err)
	}
	if rec.last == nil {
		t.Fatal("event_details Upsert was not called")
	}
	if got := rec.last.Source.SN; got != "60109012431d6040" {
		t.Errorf("Source.SN = %q, want %q", got, "60109012431d6040")
	}
	if got := rec.last.Source.DeviceMgmtId; got != "" {
		t.Errorf("Source.DeviceMgmtId = %q, want empty (resolver nil)", got)
	}
}

func TestSourceEnrichmentPersisted_NoEnrichmentLeavesFieldsEmpty(t *testing.T) {
	t.Parallel()

	rec := &recordingEventDetailsRepo{}
	deps := minimalDeps(rec)
	deps.DeviceMgmtResolver = &stubDeviceMgmtResolver{rec: nil} // resolver returns nil

	msg := kafkaMsg(canonicalEventWithPayload("evt-empty", "org-1", map[string]any{"status": "ok"}), map[string]string{
		"orgId":    "org-1",
		"tenantId": "tenant-1",
	})

	if err := handleRawEvent(context.Background(), msg, deps); err != nil {
		t.Fatalf("handleRawEvent: %v", err)
	}
	if rec.last == nil {
		t.Fatal("event_details Upsert was not called")
	}
	if got := rec.last.Source.DeviceMgmtId; got != "" {
		t.Errorf("Source.DeviceMgmtId = %q, want empty (resolver returned nil, no enrichment)", got)
	}
	if got := rec.last.Source.SN; got != "" {
		t.Errorf("Source.SN = %q, want empty (no sn in payload)", got)
	}
}
