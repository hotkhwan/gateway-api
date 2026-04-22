package entitlementsvc

import (
	"context"
	"errors"
	"testing"
)

type stubSubResolver struct {
	overlay *TenantOverlay
	err     error
	calls   []string
}

func (s *stubSubResolver) GetTenantEntitlementOverlay(_ context.Context, tenantId string) (*TenantOverlay, error) {
	s.calls = append(s.calls, tenantId)
	return s.overlay, s.err
}

type stubWsResolver struct {
	tenantByWs map[string]string
	err        error
	calls      []string
}

func (w *stubWsResolver) GetTenantIDForWorkspace(_ context.Context, workspaceId string) (string, error) {
	w.calls = append(w.calls, workspaceId)
	if w.err != nil {
		return "", w.err
	}
	return w.tenantByWs[workspaceId], nil
}

func newSvcForSynthesize(profile string, sub SubscriptionResolver, ws WorkspaceTenantResolver) *EntitlementService {
	return &EntitlementService{
		profile:   profile,
		catalog:   NewRuntimeEntitlementCatalog(),
		subLookup: sub,
		wsLookup:  ws,
	}
}

func TestSynthesize_SaasPublic_PaidTenant_AppliesOverlay(t *testing.T) {
	// Overlay with pro-plan-level EPS; plan switch kicks base over to pro.
	sub := &stubSubResolver{
		overlay: &TenantOverlay{PlanID: "pro", MaxEventsPerSecond: 250, WebhookTargetsLimit: 7},
	}
	ws := &stubWsResolver{tenantByWs: map[string]string{"ws-1": "tenant-1"}}
	svc := newSvcForSynthesize("saasPublic", sub, ws)

	ent, err := svc.synthesize(context.Background(), "ws-1", "")
	if err != nil {
		t.Fatalf("synthesize: unexpected error %v", err)
	}
	if ent.PlanCode != "pro" {
		t.Fatalf("PlanCode: got %q want %q", ent.PlanCode, "pro")
	}
	// Overlay overrode pro's default 100 EPS.
	if ent.MaxEventsPerSecond != 250 {
		t.Fatalf("MaxEventsPerSecond: got %d want 250", ent.MaxEventsPerSecond)
	}
	if ent.WebhookTargetsLimit != 7 {
		t.Fatalf("WebhookTargetsLimit: got %d want 7", ent.WebhookTargetsLimit)
	}
	// Proves the tenant resolution path ran on empty tenantId.
	if len(ws.calls) != 1 || ws.calls[0] != "ws-1" {
		t.Fatalf("workspace resolver calls: got %v", ws.calls)
	}
}

func TestSynthesize_Appliance_LicenseOverrideNarrowsDefault(t *testing.T) {
	// Appliance starts unlimited; a platform-license activation stored
	// narrower limits on the subscription. Overlay must narrow the result
	// without flipping the catalog label. Payload stays under the Kafka-safe
	// clamp (~900KB default when Kafka config isn't initialised in tests) so
	// this assertion reflects overlay behaviour; the clamp is exercised in
	// TestSynthesize_PayloadClampedToKafkaSafe below.
	const narrowPayload = 500 * 1024
	sub := &stubSubResolver{
		overlay: &TenantOverlay{MaxEventsPerSecond: 500, MaxPayloadBytes: narrowPayload},
	}
	ws := &stubWsResolver{tenantByWs: map[string]string{"ws-app": "tenant-app"}}
	svc := newSvcForSynthesize("appliance", sub, ws)

	ent, err := svc.synthesize(context.Background(), "ws-app", "tenant-app")
	if err != nil {
		t.Fatalf("synthesize: unexpected error %v", err)
	}
	if ent.PlanCode != "appliance" {
		// Profile catalog entry must not flip to a plan catalog entry in
		// non-saasPublic profiles — narrowing preserves the appliance label.
		t.Fatalf("PlanCode: got %q want %q", ent.PlanCode, "appliance")
	}
	if ent.MaxEventsPerSecond != 500 {
		t.Fatalf("MaxEventsPerSecond: got %d want 500", ent.MaxEventsPerSecond)
	}
	if ent.MaxPayloadBytes != narrowPayload {
		t.Fatalf("MaxPayloadBytes: got %d want %d", ent.MaxPayloadBytes, narrowPayload)
	}
	// Unlimited fields untouched by the overlay stay unlimited.
	if ent.MaxAssets != -1 {
		t.Fatalf("MaxAssets: overlay should not change unlimited default, got %d", ent.MaxAssets)
	}
}

func TestSynthesize_PayloadClampedToKafkaSafe(t *testing.T) {
	// Overlay asks for 100MB; synthesis must clamp so the ingest hot path
	// never serves a limit Kafka can't actually transport.
	sub := &stubSubResolver{
		overlay: &TenantOverlay{MaxPayloadBytes: 100 * 1024 * 1024},
	}
	ws := &stubWsResolver{tenantByWs: map[string]string{"ws-c": "tenant-c"}}
	svc := newSvcForSynthesize("appliance", sub, ws)

	ent, err := svc.synthesize(context.Background(), "ws-c", "tenant-c")
	if err != nil {
		t.Fatalf("synthesize: unexpected error %v", err)
	}
	if ent.MaxPayloadBytes >= 100*1024*1024 {
		t.Fatalf("MaxPayloadBytes: got %d, expected Kafka-safe clamp below 100MB", ent.MaxPayloadBytes)
	}
}

func TestSynthesize_SaasPublic_EmptyTenantResolvedFromWorkspace(t *testing.T) {
	// Regression test for the ingest-hot-path bug: cache miss with empty
	// tenantId used to short-circuit the overlay and fall back to freemium.
	sub := &stubSubResolver{
		overlay: &TenantOverlay{PlanID: "pro", MaxEventsPerSecond: 120},
	}
	ws := &stubWsResolver{tenantByWs: map[string]string{"ws-paid": "tenant-paid"}}
	svc := newSvcForSynthesize("saasPublic", sub, ws)

	ent, err := svc.synthesize(context.Background(), "ws-paid", "")
	if err != nil {
		t.Fatalf("synthesize: unexpected error %v", err)
	}
	if ent.PlanCode != "pro" {
		t.Fatalf("PlanCode: got %q want pro — workspace→tenant resolution did not run", ent.PlanCode)
	}
	if len(sub.calls) != 1 || sub.calls[0] != "tenant-paid" {
		t.Fatalf("subscription resolver calls: got %v want [tenant-paid]", sub.calls)
	}
}

func TestSynthesize_WorkspaceResolverErrorFallsBackToCatalog(t *testing.T) {
	// Tenant directory briefly unreachable — ingest should not hard-fail; we
	// return catalog defaults for the profile so the request proceeds under
	// the stricter "unknown tenant" regime instead of erroring out.
	sub := &stubSubResolver{
		overlay: &TenantOverlay{PlanID: "pro", MaxEventsPerSecond: 999},
	}
	ws := &stubWsResolver{err: errors.New("mongo unavailable")}
	svc := newSvcForSynthesize("saasPublic", sub, ws)

	ent, err := svc.synthesize(context.Background(), "ws-x", "")
	if err != nil {
		t.Fatalf("synthesize: unexpected error %v", err)
	}
	if ent.PlanCode != "freemium" {
		t.Fatalf("PlanCode: got %q want freemium (catalog fallback)", ent.PlanCode)
	}
	if len(sub.calls) != 0 {
		t.Fatalf("subscription resolver should not be called with empty tenantId, got %v", sub.calls)
	}
}

func TestSynthesize_NoSubResolver_UsesCatalogOnly(t *testing.T) {
	svc := newSvcForSynthesize("appliance", nil, nil)
	ent, err := svc.synthesize(context.Background(), "ws-a", "tenant-a")
	if err != nil {
		t.Fatalf("synthesize: unexpected error %v", err)
	}
	if ent.PlanCode != "appliance" {
		t.Fatalf("PlanCode: got %q want appliance", ent.PlanCode)
	}
	if ent.MaxAssets != -1 {
		t.Fatalf("MaxAssets: got %d want -1", ent.MaxAssets)
	}
}
