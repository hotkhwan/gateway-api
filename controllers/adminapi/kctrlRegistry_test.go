package adminapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/hotkhwan/gateway-api/internal/services/kctrlregistrysvc"
	"github.com/hotkhwan/gateway-api/models/kctrlmod"
)

// fakeKctrlSvc is a scripted stand-in for kctrlregistrysvc.Service used by the
// handler tests. It captures the last call args so assertions can verify the
// handler passed through the right inputs.
type fakeKctrlSvc struct {
	upserted *kctrlmod.KctrlRegistry
	err      error
	drift    *kctrlregistrysvc.DriftReport

	lastInput  kctrlregistrysvc.UpsertInput
	deleteCall string
	driftCall  time.Duration
}

func (s *fakeKctrlSvc) Upsert(_ context.Context, in kctrlregistrysvc.UpsertInput) (*kctrlmod.KctrlRegistry, error) {
	s.lastInput = in
	return s.upserted, s.err
}

func (s *fakeKctrlSvc) Delete(_ context.Context, hwId string) error {
	s.deleteCall = hwId
	return s.err
}

func (s *fakeKctrlSvc) ListDrift(_ context.Context, staleAfter time.Duration) (*kctrlregistrysvc.DriftReport, error) {
	s.driftCall = staleAfter
	return s.drift, s.err
}

func newKctrlApp(svc *fakeKctrlSvc) *fiber.App {
	app := fiber.New()
	ctrl := &KctrlRegistryController{svc: svc}
	app.Patch("/admin/kctrl-registry/:hwId", ctrl.Upsert)
	app.Delete("/admin/kctrl-registry/:hwId", ctrl.Delete)
	app.Get("/admin/system/kctrlRegistryDrift", ctrl.Drift)
	app.Post("/admin/system/kctrlRegistryRetry/:hwId", ctrl.Retry)
	return app
}

func decodeBody(t *testing.T, body io.ReadCloser) map[string]any {
	t.Helper()
	defer body.Close()
	raw, _ := io.ReadAll(body)
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("decode: %v (raw=%q)", err, raw)
	}
	return m
}

func TestKctrlRegistry_Upsert_Success(t *testing.T) {
	svc := &fakeKctrlSvc{
		upserted: &kctrlmod.KctrlRegistry{
			HwId: "h-1", OrgId: "org-1", Approved: true,
			ApprovedAt: time.Date(2026, 5, 18, 10, 0, 0, 0, time.UTC),
		},
	}
	app := newKctrlApp(svc)

	body, _ := json.Marshal(map[string]any{
		"orgId":      "org-1",
		"approved":   true,
		"approvedAt": "2026-05-18T10:00:00Z",
		"approvedBy": "u-1",
	})
	req := httptest.NewRequest("PATCH", "/admin/kctrl-registry/h-1", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Active-Workspace", "ws-1")

	resp, _ := app.Test(req)
	if resp.StatusCode != 200 {
		t.Fatalf("status %d, want 200", resp.StatusCode)
	}
	if got := resp.Header.Get("X-Sync-State-Echo"); got != "synced" {
		t.Errorf("X-Sync-State-Echo = %q, want %q", got, "synced")
	}
	if svc.lastInput.HwId != "h-1" || svc.lastInput.OrgId != "org-1" || !svc.lastInput.Approved {
		t.Errorf("lastInput mismatch: %+v", svc.lastInput)
	}
	if svc.lastInput.WorkspaceId != "ws-1" {
		t.Errorf("workspaceId not threaded from header: %q", svc.lastInput.WorkspaceId)
	}
}

func TestKctrlRegistry_Upsert_RejectsExtraField(t *testing.T) {
	svc := &fakeKctrlSvc{}
	app := newKctrlApp(svc)

	body, _ := json.Marshal(map[string]any{
		"orgId":   "org-1",
		"hwId":    "shouldnt-be-here", // path param duplicate — not in whitelist
		"frobnit": "extra",
	})
	req := httptest.NewRequest("PATCH", "/admin/kctrl-registry/h-1", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Active-Workspace", "ws-1")

	resp, _ := app.Test(req)
	if resp.StatusCode != 400 {
		t.Fatalf("status %d, want 400", resp.StatusCode)
	}
	got := decodeBody(t, resp.Body)
	if got["code"] != "FIELD_NOT_ACCEPTED" {
		t.Errorf("code = %v, want FIELD_NOT_ACCEPTED", got["code"])
	}
	details, _ := got["details"].(map[string]any)
	fields, _ := details["fields"].([]any)
	if len(fields) != 2 {
		t.Errorf("expected 2 rejected fields, got %v", fields)
	}
}

func TestKctrlRegistry_Upsert_InvalidApprovedAt_400(t *testing.T) {
	svc := &fakeKctrlSvc{}
	app := newKctrlApp(svc)

	body, _ := json.Marshal(map[string]any{
		"orgId":      "org-1",
		"approvedAt": "not-a-timestamp",
	})
	req := httptest.NewRequest("PATCH", "/admin/kctrl-registry/h-1", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Active-Workspace", "ws-1")

	resp, _ := app.Test(req)
	if resp.StatusCode != 400 {
		t.Fatalf("status %d, want 400", resp.StatusCode)
	}
}

func TestKctrlRegistry_Upsert_EmptyBody_400(t *testing.T) {
	svc := &fakeKctrlSvc{}
	app := newKctrlApp(svc)

	req := httptest.NewRequest("PATCH", "/admin/kctrl-registry/h-1", nil)
	resp, _ := app.Test(req)
	if resp.StatusCode != 400 {
		t.Fatalf("status %d, want 400", resp.StatusCode)
	}
}

func TestKctrlRegistry_Upsert_InternalError_500(t *testing.T) {
	svc := &fakeKctrlSvc{err: errors.New("mongo down")}
	app := newKctrlApp(svc)

	body, _ := json.Marshal(map[string]any{"orgId": "org-1", "approved": true})
	req := httptest.NewRequest("PATCH", "/admin/kctrl-registry/h-1", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Active-Workspace", "ws-1")

	resp, _ := app.Test(req)
	if resp.StatusCode != 500 {
		t.Fatalf("status %d, want 500", resp.StatusCode)
	}
}

func TestKctrlRegistry_Delete_NoContent(t *testing.T) {
	svc := &fakeKctrlSvc{}
	app := newKctrlApp(svc)

	req := httptest.NewRequest("DELETE", "/admin/kctrl-registry/h-1", nil)
	resp, _ := app.Test(req)
	if resp.StatusCode != 204 {
		t.Fatalf("status %d, want 204", resp.StatusCode)
	}
	if svc.deleteCall != "h-1" {
		t.Errorf("delete arg = %q, want h-1", svc.deleteCall)
	}
}

func TestKctrlRegistry_Delete_IdempotentOnMissing(t *testing.T) {
	// Repo returns nil for missing row (contract §4.2). Handler must still
	// return 204 — not 404.
	svc := &fakeKctrlSvc{} // err = nil
	app := newKctrlApp(svc)

	req := httptest.NewRequest("DELETE", "/admin/kctrl-registry/missing", nil)
	resp, _ := app.Test(req)
	if resp.StatusCode != 204 {
		t.Fatalf("status %d, want 204 (idempotent)", resp.StatusCode)
	}
}

func TestKctrlRegistry_Drift_Default1h(t *testing.T) {
	svc := &fakeKctrlSvc{drift: &kctrlregistrysvc.DriftReport{
		Items: []kctrlregistrysvc.DriftItem{
			{HwId: "h-1", Reason: "stale"},
		},
		Summary: kctrlregistrysvc.DriftSummary{Total: 12, Stale: 1},
	}}
	app := newKctrlApp(svc)

	req := httptest.NewRequest("GET", "/admin/system/kctrlRegistryDrift", nil)
	resp, _ := app.Test(req)
	if resp.StatusCode != 200 {
		t.Fatalf("status %d, want 200", resp.StatusCode)
	}
	if svc.driftCall != time.Hour {
		t.Errorf("default staleAfter = %v, want 1h", svc.driftCall)
	}
}

func TestKctrlRegistry_Drift_CustomStaleSecs(t *testing.T) {
	svc := &fakeKctrlSvc{drift: &kctrlregistrysvc.DriftReport{}}
	app := newKctrlApp(svc)

	req := httptest.NewRequest("GET", "/admin/system/kctrlRegistryDrift?staleSecs=120", nil)
	resp, _ := app.Test(req)
	if resp.StatusCode != 200 {
		t.Fatalf("status %d, want 200", resp.StatusCode)
	}
	if svc.driftCall != 2*time.Minute {
		t.Errorf("staleAfter = %v, want 2m (from staleSecs=120)", svc.driftCall)
	}
}

func TestKctrlRegistry_Retry_Acknowledges(t *testing.T) {
	svc := &fakeKctrlSvc{}
	app := newKctrlApp(svc)

	req := httptest.NewRequest("POST", "/admin/system/kctrlRegistryRetry/h-1", nil)
	resp, _ := app.Test(req)
	if resp.StatusCode != 200 {
		t.Fatalf("status %d, want 200", resp.StatusCode)
	}
}
