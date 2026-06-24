// controllers/adminapi/deviceIdentity_test.go
package adminapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/hotkhwan/gateway-api/models/ingestmod"
)

type fakeProvisionSvc struct {
	called              int
	gotFamily, gotCamID string
	gotTenant, gotWS    string
	gotHints            ingestmod.AutoUpsertHints
	ret                 *ingestmod.DeviceManagement
	err                 error
}

func (f *fakeProvisionSvc) ProvisionByCamID(_ context.Context, tenantId, workspaceId, sourceFamily, camID string, hints ingestmod.AutoUpsertHints) (*ingestmod.DeviceManagement, error) {
	f.called++
	f.gotTenant, f.gotWS, f.gotFamily, f.gotCamID, f.gotHints = tenantId, workspaceId, sourceFamily, camID, hints
	if f.err != nil {
		return nil, f.err
	}
	if f.ret != nil {
		return f.ret, nil
	}
	return &ingestmod.DeviceManagement{DeviceMgmtId: "dm-1", EntityType: "camera", EntityId: camID, DeviceId: camID}, nil
}

func newProvisionApp(svc *fakeProvisionSvc) *fiber.App {
	app := fiber.New()
	ctrl := NewDeviceIdentityController(svc)
	app.Post("/admin/device-management/identities", ctrl.Provision)
	return app
}

func TestProvision_Success(t *testing.T) {
	svc := &fakeProvisionSvc{}
	app := newProvisionApp(svc)

	b, _ := json.Marshal(map[string]any{"sourceFamily": "dahua", "camId": "cam-7", "name": "Gate Cam", "lat": 13.7})
	req := httptest.NewRequest("POST", "/admin/device-management/identities", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Active-Workspace", "ws-1")

	resp, _ := app.Test(req)
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
	if svc.called != 1 {
		t.Fatalf("service called %d times, want 1", svc.called)
	}
	if svc.gotFamily != "dahua" || svc.gotCamID != "cam-7" {
		t.Fatalf("args: family=%q cam=%q", svc.gotFamily, svc.gotCamID)
	}
	if svc.gotWS != "ws-1" {
		t.Fatalf("workspaceId from X-Active-Workspace: got %q", svc.gotWS)
	}
	if svc.gotHints.DeviceId != "cam-7" || svc.gotHints.Name != "Gate Cam" || svc.gotHints.Lat != 13.7 {
		t.Fatalf("hints not threaded: %+v", svc.gotHints)
	}
}

func TestProvision_MissingCamId_400(t *testing.T) {
	svc := &fakeProvisionSvc{}
	app := newProvisionApp(svc)
	b, _ := json.Marshal(map[string]any{"sourceFamily": "dahua"})
	req := httptest.NewRequest("POST", "/admin/device-management/identities", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := app.Test(req)
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("want 400, got %d", resp.StatusCode)
	}
	if svc.called != 0 {
		t.Fatalf("service must not be called on bad request")
	}
}
