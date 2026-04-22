package workspaceapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/hotkhwan/gateway-api/internal/services/entitlementsvc"
)

type mockEntitlement struct {
	result *entitlementsvc.RuntimeEntitlement
	err    error
	calls  int
	lastWs string
	lastTn string
}

func (m *mockEntitlement) GetForWorkspace(_ context.Context, workspaceId, tenantId string) (*entitlementsvc.RuntimeEntitlement, error) {
	m.calls++
	m.lastWs = workspaceId
	m.lastTn = tenantId
	return m.result, m.err
}

func newEntitlementApp(svc entitlementReader, workspaceId, tenantId string) *fiber.App {
	app := fiber.New()
	ctrl := &WorkspaceEntitlementController{svc: svc}
	app.Use(func(c fiber.Ctx) error {
		if workspaceId != "" {
			c.Locals("activeWorkspace", workspaceId)
		}
		if tenantId != "" {
			c.Locals("tenantId", tenantId)
		}
		return c.Next()
	})
	app.Get("/workspaces/entitlement", ctrl.GetEntitlement)
	return app
}

func decodeJSON(t *testing.T, body io.ReadCloser) map[string]any {
	t.Helper()
	defer body.Close()
	raw, _ := io.ReadAll(body)
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("decode: %v (raw=%q)", err, raw)
	}
	return m
}

func TestEntitlement_CacheMissSynthesized_Returns200(t *testing.T) {
	svc := &mockEntitlement{
		result: &entitlementsvc.RuntimeEntitlement{
			WorkspaceID:        "ws-1",
			PlanCode:           "pro",
			MaxEventsPerSecond: 100,
			MaxPayloadBytes:    40 * 1024 * 1024,
		},
	}
	app := newEntitlementApp(svc, "ws-1", "tenant-1")

	req := httptest.NewRequest("GET", "/workspaces/entitlement", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("status: got %d want 200", resp.StatusCode)
	}
	if svc.lastWs != "ws-1" || svc.lastTn != "tenant-1" {
		t.Fatalf("controller did not forward locals: ws=%q tn=%q", svc.lastWs, svc.lastTn)
	}

	body := decodeJSON(t, resp.Body)
	details, _ := body["details"].(map[string]any)
	if details == nil {
		t.Fatalf("response missing details: %v", body)
	}
	if details["planCode"] != "pro" {
		t.Fatalf("details.planCode: got %v want pro", details["planCode"])
	}
}

func TestEntitlement_MissingWorkspaceHeader_Returns400(t *testing.T) {
	svc := &mockEntitlement{}
	app := newEntitlementApp(svc, "", "tenant-1") // no activeWorkspace in locals

	req := httptest.NewRequest("GET", "/workspaces/entitlement", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	if resp.StatusCode != 400 {
		t.Fatalf("status: got %d want 400", resp.StatusCode)
	}
	if svc.calls != 0 {
		t.Fatalf("service should not be called when header missing, got %d calls", svc.calls)
	}
}

func TestEntitlement_ServiceError_Returns500(t *testing.T) {
	// Real Redis read error (or subscription overlay failure) — cache miss
	// synthesis is supposed to stay out of this branch; only actual failures
	// produce 500.
	svc := &mockEntitlement{err: errors.New("redis unavailable")}
	app := newEntitlementApp(svc, "ws-2", "tenant-2")

	req := httptest.NewRequest("GET", "/workspaces/entitlement", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	if resp.StatusCode != 500 {
		t.Fatalf("status: got %d want 500", resp.StatusCode)
	}
}
