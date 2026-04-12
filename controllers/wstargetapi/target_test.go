// controllers/wstargetapi/target_test.go
package wstargetapi

import (
	"context"
	"encoding/json"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/hotkhwan/gateway-api/internal/services/targetsvc"
	"github.com/hotkhwan/gateway-api/models/authzmod"
)

// ============================================================
// mockSvc — implements targetSvcI
// ============================================================

type mockSvc struct {
	createResult *authzmod.DeliveryTarget
	createErr    error

	listResult []authzmod.DeliveryTarget
	listTotal  int64
	listErr    error

	getOneResult *authzmod.DeliveryTarget
	getOneErr    error

	updateResult *authzmod.DeliveryTarget
	updateErr    error

	deleteErr error
}

func (m *mockSvc) Create(_ context.Context, _ targetsvc.CreateTargetInput) (*authzmod.DeliveryTarget, error) {
	return m.createResult, m.createErr
}
func (m *mockSvc) List(_ context.Context, _ targetsvc.ListTargetInput) ([]authzmod.DeliveryTarget, int64, error) {
	return m.listResult, m.listTotal, m.listErr
}
func (m *mockSvc) GetOne(_ context.Context, _, _, _, _ string) (*authzmod.DeliveryTarget, error) {
	return m.getOneResult, m.getOneErr
}
func (m *mockSvc) Update(_ context.Context, _ targetsvc.UpdateTargetInput) (*authzmod.DeliveryTarget, error) {
	return m.updateResult, m.updateErr
}
func (m *mockSvc) Delete(_ context.Context, _, _, _, _ string) error {
	return m.deleteErr
}

// ============================================================
// Test app helpers
// ============================================================

// authMiddleware injects auth locals so controllers see a valid session.
func authMiddleware(c fiber.Ctx) error {
	c.Locals("tenantId", "tenant-1")
	c.Locals("activeWorkspace", "ws-1")
	c.Locals("userId", "user-1")
	return c.Next()
}

func newApp(svc targetSvcI, useAuth bool) *fiber.App {
	app := fiber.New(fiber.Config{})
	ctrl := &WsTargetController{service: svc}

	if useAuth {
		app.Use(authMiddleware)
	}

	app.Post("/delivery-targets", ctrl.Create)
	app.Get("/delivery-targets", ctrl.List)
	app.Get("/delivery-targets/:id", ctrl.GetOne)
	app.Patch("/delivery-targets/:id", ctrl.Update)
	app.Delete("/delivery-targets/:id", ctrl.Delete)

	return app
}

// bodyJSON encodes v to a JSON io.Reader for request bodies.
func bodyJSON(v any) io.Reader {
	b, _ := json.Marshal(v)
	return strings.NewReader(string(b))
}

// parseBody reads response body into a generic map.
func parseBody(t *testing.T, resp io.ReadCloser) map[string]any {
	t.Helper()
	defer resp.Close()
	raw, err := io.ReadAll(resp)
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("unmarshal response: %v (body=%s)", err, raw)
	}
	return m
}

func sampleTarget() *authzmod.DeliveryTarget {
	return &authzmod.DeliveryTarget{
		TargetId: "tgt-1",
		OrgId:    "ws-1",
		Name:     "My Webhook",
		Type:     authzmod.TargetTypeWebhook,
		Enabled:  true,
	}
}

// ============================================================
// POST /delivery-targets — Create
// ============================================================

func TestCreate_NormalTarget_Returns201(t *testing.T) {
	t.Parallel()

	svc := &mockSvc{createResult: sampleTarget()}
	app := newApp(svc, true)

	req := httptest.NewRequest("POST", "/delivery-targets", bodyJSON(map[string]any{
		"name":    "My Webhook",
		"type":    "webhook",
		"enabled": true,
	}))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	if resp.StatusCode != 201 {
		t.Errorf("expected 201, got %d", resp.StatusCode)
	}

	body := parseBody(t, resp.Body)
	if body["code"] != "CREATED" {
		t.Errorf("expected code=CREATED, got %v", body["code"])
	}
	if body["status"] != true {
		t.Errorf("expected status=true, got %v", body["status"])
	}
	if body["details"] == nil {
		t.Error("expected non-nil details")
	}
}

func TestCreate_KlynxWithURL_Returns400(t *testing.T) {
	t.Parallel()

	svc := &mockSvc{createErr: targetsvc.ErrKlynxModeWithURL}
	app := newApp(svc, true)

	req := httptest.NewRequest("POST", "/delivery-targets", bodyJSON(map[string]any{
		"name": "Klynx Target",
		"type": "webhook",
		"mode": "klynx",
		"config": map[string]any{
			"url": "https://example.com/hook",
		},
	}))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	if resp.StatusCode != 400 {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}

	body := parseBody(t, resp.Body)
	if body["status"] != false {
		t.Errorf("expected status=false, got %v", body["status"])
	}
}

func TestCreate_KlynxWithoutURL_Returns201(t *testing.T) {
	t.Parallel()

	klynxTarget := &authzmod.DeliveryTarget{
		TargetId: "tgt-klynx",
		OrgId:    "ws-1",
		Name:     "Klynx Route",
		Type:     authzmod.TargetTypeWebhook,
		Mode:     "klynx",
	}
	svc := &mockSvc{createResult: klynxTarget}
	app := newApp(svc, true)

	req := httptest.NewRequest("POST", "/delivery-targets", bodyJSON(map[string]any{
		"name": "Klynx Route",
		"type": "webhook",
		"mode": "klynx",
		// no config.url
	}))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	if resp.StatusCode != 201 {
		t.Errorf("expected 201, got %d", resp.StatusCode)
	}

	body := parseBody(t, resp.Body)
	if body["code"] != "CREATED" {
		t.Errorf("expected code=CREATED, got %v", body["code"])
	}
}

func TestCreate_DuplicateName_Returns409(t *testing.T) {
	t.Parallel()

	svc := &mockSvc{createErr: targetsvc.ErrConflict}
	app := newApp(svc, true)

	req := httptest.NewRequest("POST", "/delivery-targets", bodyJSON(map[string]any{
		"name": "Duplicate",
		"type": "webhook",
	}))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	if resp.StatusCode != 409 {
		t.Errorf("expected 409, got %d", resp.StatusCode)
	}

	body := parseBody(t, resp.Body)
	if body["status"] != false {
		t.Errorf("expected status=false, got %v", body["status"])
	}
}

func TestCreate_NoJWT_Returns401(t *testing.T) {
	t.Parallel()

	svc := &mockSvc{}
	app := newApp(svc, false) // no auth middleware → locals empty

	req := httptest.NewRequest("POST", "/delivery-targets", bodyJSON(map[string]any{
		"name": "Target",
		"type": "webhook",
	}))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	if resp.StatusCode != 401 {
		t.Errorf("expected 401, got %d", resp.StatusCode)
	}

	body := parseBody(t, resp.Body)
	if body["status"] != false {
		t.Errorf("expected status=false, got %v", body["status"])
	}
}

// ============================================================
// Create — body validation (missing name / type)
// ============================================================

func TestCreate_MissingName_Returns400(t *testing.T) {
	t.Parallel()

	svc := &mockSvc{}
	app := newApp(svc, true)

	req := httptest.NewRequest("POST", "/delivery-targets", bodyJSON(map[string]any{
		"type": "webhook",
		// name missing
	}))
	req.Header.Set("Content-Type", "application/json")

	resp, _ := app.Test(req)
	if resp.StatusCode != 400 {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}

func TestCreate_MissingType_Returns400(t *testing.T) {
	t.Parallel()

	svc := &mockSvc{}
	app := newApp(svc, true)

	req := httptest.NewRequest("POST", "/delivery-targets", bodyJSON(map[string]any{
		"name": "Target",
		// type missing
	}))
	req.Header.Set("Content-Type", "application/json")

	resp, _ := app.Test(req)
	if resp.StatusCode != 400 {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}

// ============================================================
// GET /delivery-targets — List
// ============================================================

func TestList_Returns200WithItemsArray(t *testing.T) {
	t.Parallel()

	items := []authzmod.DeliveryTarget{*sampleTarget()}
	svc := &mockSvc{listResult: items, listTotal: 1}
	app := newApp(svc, true)

	req := httptest.NewRequest("GET", "/delivery-targets", nil)

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	body := parseBody(t, resp.Body)

	// details must be an object (never a bare array)
	details, ok := body["details"].(map[string]any)
	if !ok {
		t.Fatalf("details must be a JSON object, got %T: %v", body["details"], body["details"])
	}

	// details.items must be a JSON array
	itemsVal, hasItems := details["items"]
	if !hasItems {
		t.Fatal("details.items key missing")
	}
	if _, isArray := itemsVal.([]any); !isArray {
		t.Errorf("details.items must be an array, got %T", itemsVal)
	}

	// pagination must be top-level
	if body["pagination"] == nil {
		t.Error("pagination key missing from top-level response")
	}

	if body["code"] != "SUCCESS" {
		t.Errorf("expected code=SUCCESS, got %v", body["code"])
	}
}

func TestList_EmptyResult_ItemsIsEmptyArray(t *testing.T) {
	t.Parallel()

	svc := &mockSvc{listResult: []authzmod.DeliveryTarget{}, listTotal: 0}
	app := newApp(svc, true)

	req := httptest.NewRequest("GET", "/delivery-targets", nil)

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	body := parseBody(t, resp.Body)
	details, ok := body["details"].(map[string]any)
	if !ok {
		t.Fatalf("details must be a JSON object, got %T", body["details"])
	}

	itemsVal, hasItems := details["items"]
	if !hasItems {
		t.Fatal("details.items key missing even for empty result")
	}
	arr, isArray := itemsVal.([]any)
	if !isArray {
		t.Errorf("details.items must be an array, got %T", itemsVal)
	}
	if len(arr) != 0 {
		t.Errorf("expected empty array, got %d items", len(arr))
	}
}

func TestList_NoJWT_Returns401(t *testing.T) {
	t.Parallel()

	svc := &mockSvc{}
	app := newApp(svc, false)

	req := httptest.NewRequest("GET", "/delivery-targets", nil)

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	if resp.StatusCode != 401 {
		t.Errorf("expected 401, got %d", resp.StatusCode)
	}
}

// ============================================================
// GET /delivery-targets/:id — GetOne
// ============================================================

func TestGetOne_Returns200(t *testing.T) {
	t.Parallel()

	svc := &mockSvc{getOneResult: sampleTarget()}
	app := newApp(svc, true)

	req := httptest.NewRequest("GET", "/delivery-targets/tgt-1", nil)

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	body := parseBody(t, resp.Body)
	if body["code"] != "SUCCESS" {
		t.Errorf("expected code=SUCCESS, got %v", body["code"])
	}
}

func TestGetOne_NotFound_Returns404(t *testing.T) {
	t.Parallel()

	svc := &mockSvc{getOneErr: targetsvc.ErrNotFound}
	app := newApp(svc, true)

	req := httptest.NewRequest("GET", "/delivery-targets/ghost", nil)

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	if resp.StatusCode != 404 {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}

	body := parseBody(t, resp.Body)
	if body["status"] != false {
		t.Errorf("expected status=false, got %v", body["status"])
	}
}

// ============================================================
// DELETE /delivery-targets/:id — Delete
// ============================================================

func TestDelete_Returns200(t *testing.T) {
	t.Parallel()

	svc := &mockSvc{deleteErr: nil}
	app := newApp(svc, true)

	req := httptest.NewRequest("DELETE", "/delivery-targets/tgt-1", nil)

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	body := parseBody(t, resp.Body)
	if body["status"] != true {
		t.Errorf("expected status=true, got %v", body["status"])
	}
}

func TestDelete_NotFound_Returns404(t *testing.T) {
	t.Parallel()

	svc := &mockSvc{deleteErr: targetsvc.ErrNotFound}
	app := newApp(svc, true)

	req := httptest.NewRequest("DELETE", "/delivery-targets/ghost", nil)

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	if resp.StatusCode != 404 {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}

	body := parseBody(t, resp.Body)
	if body["status"] != false {
		t.Errorf("expected status=false, got %v", body["status"])
	}
}

// ============================================================
// PATCH /delivery-targets/:id — Update
// ============================================================

func TestUpdate_Returns200(t *testing.T) {
	t.Parallel()

	updated := sampleTarget()
	updated.Name = "Updated Name"
	svc := &mockSvc{updateResult: updated}
	app := newApp(svc, true)

	req := httptest.NewRequest("PATCH", "/delivery-targets/tgt-1", bodyJSON(map[string]any{
		"name": "Updated Name",
	}))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	body := parseBody(t, resp.Body)
	if body["code"] != "SUCCESS" {
		t.Errorf("expected code=SUCCESS, got %v", body["code"])
	}
}

func TestUpdate_NotFound_Returns404(t *testing.T) {
	t.Parallel()

	svc := &mockSvc{updateErr: targetsvc.ErrNotFound}
	app := newApp(svc, true)

	req := httptest.NewRequest("PATCH", "/delivery-targets/ghost", bodyJSON(map[string]any{
		"name": "x",
	}))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	if resp.StatusCode != 404 {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
}

// ============================================================
// klynx error variants table test
// ============================================================

func TestCreate_KlynxErrors_AllReturn400(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		err  error
	}{
		{"url present", targetsvc.ErrKlynxModeWithURL},
		{"hmac present", targetsvc.ErrKlynxModeWithHMAC},
		{"saasPublic profile", targetsvc.ErrKlynxModeInSaasPublic},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			svc := &mockSvc{createErr: tc.err}
			app := newApp(svc, true)

			req := httptest.NewRequest("POST", "/delivery-targets", bodyJSON(map[string]any{
				"name": "klynx",
				"type": "webhook",
				"mode": "klynx",
			}))
			req.Header.Set("Content-Type", "application/json")

			resp, err := app.Test(req)
			if err != nil {
				t.Fatalf("app.Test: %v", err)
			}
			if resp.StatusCode != 400 {
				t.Errorf("%s: expected 400, got %d", tc.name, resp.StatusCode)
			}
		})
	}
}

// ============================================================
// Response envelope contract
// ============================================================

func TestCreate_ResponseEnvelope_HasRequiredFields(t *testing.T) {
	t.Parallel()

	svc := &mockSvc{createResult: sampleTarget()}
	app := newApp(svc, true)

	req := httptest.NewRequest("POST", "/delivery-targets", bodyJSON(map[string]any{
		"name": "Test",
		"type": "webhook",
	}))
	req.Header.Set("Content-Type", "application/json")

	resp, _ := app.Test(req)
	body := parseBody(t, resp.Body)

	for _, field := range []string{"code", "message", "status", "details"} {
		if _, ok := body[field]; !ok {
			t.Errorf("response envelope missing field %q", field)
		}
	}
}
