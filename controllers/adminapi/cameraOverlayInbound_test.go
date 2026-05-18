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
	"github.com/hotkhwan/gateway-api/internal/services/devicemgmtsvc"
	"github.com/hotkhwan/gateway-api/models/ingestmod"
)

// fakeOverlaySvc records the last call and returns scripted results.
type fakeOverlaySvc struct {
	updated *ingestmod.DeviceManagement
	status  devicemgmtsvc.IfMatchStatus
	err     error

	lastTenant      string
	lastWorkspace   string
	lastDeviceMgmt  string
	lastBody        map[string]any
	lastIfMatch     string
	called          int
}

func (s *fakeOverlaySvc) ApplyKlynxOverlay(
	_ context.Context,
	tenantId, workspaceId, deviceMgmtId string,
	body map[string]any,
	ifMatch string,
) (*ingestmod.DeviceManagement, devicemgmtsvc.IfMatchStatus, error) {
	s.called++
	s.lastTenant = tenantId
	s.lastWorkspace = workspaceId
	s.lastDeviceMgmt = deviceMgmtId
	s.lastBody = body
	s.lastIfMatch = ifMatch
	return s.updated, s.status, s.err
}

func newOverlayApp(svc *fakeOverlaySvc) *fiber.App {
	app := fiber.New()
	ctrl := &CameraOverlayInboundController{svc: svc}
	app.Patch("/admin/device-management/cameras/:gwDeviceMgmtId", ctrl.Apply)
	return app
}

func patchBody(t *testing.T, payload any) *bytes.Reader {
	t.Helper()
	b, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	return bytes.NewReader(b)
}

func decode(t *testing.T, body io.ReadCloser) map[string]any {
	t.Helper()
	defer body.Close()
	raw, _ := io.ReadAll(body)
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("decode: %v (raw=%q)", err, raw)
	}
	return m
}

func TestOverlay_Apply_Success(t *testing.T) {
	svc := &fakeOverlaySvc{
		updated: &ingestmod.DeviceManagement{
			DeviceMgmtId:     "dm-1",
			WorkspaceId:      "ws-1",
			Name:             "Cam85 renamed",
			LastOutboundHash: "abc123",
			UpdatedAt:        time.Now().UTC(),
		},
		status: devicemgmtsvc.IfMatchAbsent,
	}
	app := newOverlayApp(svc)

	req := httptest.NewRequest("PATCH", "/admin/device-management/cameras/dm-1",
		patchBody(t, map[string]any{"name": "Cam85 renamed"}))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Active-Workspace", "ws-1")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("status: got %d want 200", resp.StatusCode)
	}
	if got := resp.Header.Get("X-If-Match-Status"); got != "absent" {
		t.Errorf("X-If-Match-Status header: got %q want %q", got, "absent")
	}
	if svc.called != 1 {
		t.Errorf("service called %d times, want 1", svc.called)
	}
	if svc.lastWorkspace != "ws-1" {
		t.Errorf("workspaceId: got %q want %q (should fall back to X-Active-Workspace header when locals empty)", svc.lastWorkspace, "ws-1")
	}
	body := decode(t, resp.Body)
	if body["status"] != true {
		t.Errorf("body.status: got %v want true", body["status"])
	}
}

func TestOverlay_Apply_IfMatchMismatched_ReturnsHeader(t *testing.T) {
	// v1 contract (§8.7): If-Match never gates the write. Even when mismatched,
	// the response is 200 with X-If-Match-Status: mismatched.
	svc := &fakeOverlaySvc{
		updated: &ingestmod.DeviceManagement{DeviceMgmtId: "dm-1"},
		status:  devicemgmtsvc.IfMatchMismatched,
	}
	app := newOverlayApp(svc)

	req := httptest.NewRequest("PATCH", "/admin/device-management/cameras/dm-1",
		patchBody(t, map[string]any{"name": "x"}))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Active-Workspace", "ws-1")
	req.Header.Set("If-Match", "stale-hash")

	resp, _ := app.Test(req)
	if resp.StatusCode != 200 {
		t.Fatalf("status: got %d want 200 (no 409 in v1)", resp.StatusCode)
	}
	if got := resp.Header.Get("X-If-Match-Status"); got != "mismatched" {
		t.Errorf("X-If-Match-Status: got %q want %q", got, "mismatched")
	}
	if svc.lastIfMatch != "stale-hash" {
		t.Errorf("If-Match forwarded: got %q want %q", svc.lastIfMatch, "stale-hash")
	}
}

func TestOverlay_Apply_FieldNotAccepted_Returns400(t *testing.T) {
	svc := &fakeOverlaySvc{
		err: &devicemgmtsvc.OverlayValidationError{
			Code:   "FIELD_NOT_ACCEPTED",
			Fields: []string{"password", "url"},
		},
	}
	app := newOverlayApp(svc)

	req := httptest.NewRequest("PATCH", "/admin/device-management/cameras/dm-1",
		patchBody(t, map[string]any{"url": "rtsp://x", "password": "secret"}))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Active-Workspace", "ws-1")

	resp, _ := app.Test(req)
	if resp.StatusCode != 400 {
		t.Fatalf("status: got %d want 400", resp.StatusCode)
	}
	body := decode(t, resp.Body)
	if body["code"] != "FIELD_NOT_ACCEPTED" {
		t.Errorf("code: got %v want %q", body["code"], "FIELD_NOT_ACCEPTED")
	}
	details, ok := body["details"].(map[string]any)
	if !ok {
		t.Fatalf("details: got %v (type %T), want map", body["details"], body["details"])
	}
	fields, ok := details["fields"].([]any)
	if !ok {
		t.Fatalf("details.fields: got %v (type %T), want array", details["fields"], details["fields"])
	}
	if len(fields) != 2 || fields[0] != "password" || fields[1] != "url" {
		t.Errorf("details.fields: got %v want [password url]", fields)
	}
}

func TestOverlay_Apply_FieldReadonly_Returns400(t *testing.T) {
	svc := &fakeOverlaySvc{
		err: &devicemgmtsvc.OverlayValidationError{
			Code:   "FIELD_READONLY",
			Fields: []string{"deviceMgmtId"},
		},
	}
	app := newOverlayApp(svc)

	req := httptest.NewRequest("PATCH", "/admin/device-management/cameras/dm-1",
		patchBody(t, map[string]any{"deviceMgmtId": "other-id"}))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Active-Workspace", "ws-1")

	resp, _ := app.Test(req)
	if resp.StatusCode != 400 {
		t.Fatalf("status: got %d want 400", resp.StatusCode)
	}
	body := decode(t, resp.Body)
	if body["code"] != "FIELD_READONLY" {
		t.Errorf("code: got %v want FIELD_READONLY", body["code"])
	}
}

func TestOverlay_Apply_FieldUnknown_Returns400(t *testing.T) {
	svc := &fakeOverlaySvc{
		err: &devicemgmtsvc.OverlayValidationError{
			Code:   "FIELD_UNKNOWN",
			Fields: []string{"frobnicate"},
		},
	}
	app := newOverlayApp(svc)

	req := httptest.NewRequest("PATCH", "/admin/device-management/cameras/dm-1",
		patchBody(t, map[string]any{"frobnicate": "yes"}))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Active-Workspace", "ws-1")

	resp, _ := app.Test(req)
	if resp.StatusCode != 400 {
		t.Fatalf("status: got %d want 400", resp.StatusCode)
	}
}

func TestOverlay_Apply_DeviceNotFound_Returns404(t *testing.T) {
	svc := &fakeOverlaySvc{err: devicemgmtsvc.ErrNotFound}
	app := newOverlayApp(svc)

	req := httptest.NewRequest("PATCH", "/admin/device-management/cameras/dm-missing",
		patchBody(t, map[string]any{"name": "x"}))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Active-Workspace", "ws-1")

	resp, _ := app.Test(req)
	if resp.StatusCode != 404 {
		t.Fatalf("status: got %d want 404", resp.StatusCode)
	}
	body := decode(t, resp.Body)
	if body["code"] != "DEVICE_NOT_FOUND" {
		t.Errorf("code: got %v want DEVICE_NOT_FOUND", body["code"])
	}
}

func TestOverlay_Apply_EmptyBody_Returns400(t *testing.T) {
	svc := &fakeOverlaySvc{}
	app := newOverlayApp(svc)

	req := httptest.NewRequest("PATCH", "/admin/device-management/cameras/dm-1", nil)
	resp, _ := app.Test(req)
	if resp.StatusCode != 400 {
		t.Fatalf("status: got %d want 400", resp.StatusCode)
	}
	if svc.called != 0 {
		t.Errorf("service should not be called on empty body, got %d calls", svc.called)
	}
}

func TestOverlay_Apply_MalformedJSON_Returns400(t *testing.T) {
	svc := &fakeOverlaySvc{}
	app := newOverlayApp(svc)

	req := httptest.NewRequest("PATCH", "/admin/device-management/cameras/dm-1",
		bytes.NewBufferString(`{not json`))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := app.Test(req)
	if resp.StatusCode != 400 {
		t.Fatalf("status: got %d want 400", resp.StatusCode)
	}
	if svc.called != 0 {
		t.Errorf("service should not be called on malformed body, got %d calls", svc.called)
	}
}

func TestOverlay_Apply_InternalError_Returns500(t *testing.T) {
	svc := &fakeOverlaySvc{err: errors.New("boom")}
	app := newOverlayApp(svc)

	req := httptest.NewRequest("PATCH", "/admin/device-management/cameras/dm-1",
		patchBody(t, map[string]any{"name": "x"}))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Active-Workspace", "ws-1")

	resp, _ := app.Test(req)
	if resp.StatusCode != 500 {
		t.Fatalf("status: got %d want 500", resp.StatusCode)
	}
}
