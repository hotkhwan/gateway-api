// controllers/bindingapi/binding_test.go
package bindingapi

import (
	"context"
	"encoding/json"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/hotkhwan/gateway-api/internal/services/bindingsvc"
	"github.com/hotkhwan/gateway-api/models/gmod"
	"github.com/hotkhwan/gateway-api/models/ingestmod"
)

// ============================================================
// mockSvc — implements bindingSvcI
// ============================================================

type mockSvc struct {
	createResult *ingestmod.TemplateDeliveryBinding
	createErr    error

	listResult []ingestmod.TemplateDeliveryBinding
	listPag    *gmod.Pagination
	listErr    error

	getOneResult *ingestmod.TemplateDeliveryBinding
	getOneErr    error

	updateResult *ingestmod.TemplateDeliveryBinding
	updateErr    error

	deleteErr error
}

func (m *mockSvc) Create(_ context.Context, _ bindingsvc.CreateBindingInput) (*ingestmod.TemplateDeliveryBinding, error) {
	return m.createResult, m.createErr
}
func (m *mockSvc) List(_ context.Context, _ string, _, _ int) ([]ingestmod.TemplateDeliveryBinding, *gmod.Pagination, error) {
	return m.listResult, m.listPag, m.listErr
}
func (m *mockSvc) GetOne(_ context.Context, _, _ string) (*ingestmod.TemplateDeliveryBinding, error) {
	return m.getOneResult, m.getOneErr
}
func (m *mockSvc) Update(_ context.Context, _ bindingsvc.UpdateBindingInput) (*ingestmod.TemplateDeliveryBinding, error) {
	return m.updateResult, m.updateErr
}
func (m *mockSvc) Delete(_ context.Context, _, _ string) error { return m.deleteErr }

// ============================================================
// Test app helpers
// ============================================================

func newApp(svc bindingSvcI, withAuth bool) *fiber.App {
	app := fiber.New()
	ctrl := &BindingController{service: svc}

	if withAuth {
		app.Use(func(c fiber.Ctx) error {
			c.Locals("activeWorkspace", "ws-1")
			return c.Next()
		})
	}

	app.Post("/delivery-bindings", ctrl.Create)
	app.Get("/delivery-bindings", ctrl.List)
	app.Get("/delivery-bindings/:id", ctrl.GetOne)
	app.Patch("/delivery-bindings/:id", ctrl.Update)
	app.Delete("/delivery-bindings/:id", ctrl.Delete)

	return app
}

func bodyJSON(v any) io.Reader {
	b, _ := json.Marshal(v)
	return strings.NewReader(string(b))
}

func parseBody(t *testing.T, body io.ReadCloser) map[string]any {
	t.Helper()
	defer body.Close()
	raw, _ := io.ReadAll(body)
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("unmarshal response body: %v (raw=%s)", err, raw)
	}
	return m
}

func sampleBinding() *ingestmod.TemplateDeliveryBinding {
	return &ingestmod.TemplateDeliveryBinding{
		ID:            "bind-1",
		WorkspaceID:   "ws-1",
		TemplateID:    "tpl-1",
		TargetID:      "tgt-1",
		DispatchStage: ingestmod.DispatchStageRealtime,
		Enabled:       true,
	}
}

func boolPtr(b bool) *bool { return &b }

// ============================================================
// POST /delivery-bindings — Create
// ============================================================

func TestCreate_ValidBinding_Returns201(t *testing.T) {
	t.Parallel()

	svc := &mockSvc{createResult: sampleBinding()}
	app := newApp(svc, true)

	req := httptest.NewRequest("POST", "/delivery-bindings", bodyJSON(map[string]any{
		"templateId":    "tpl-1",
		"targetId":      "tgt-1",
		"dispatchStage": "realtime",
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
		t.Error("expected non-nil details in response")
	}
}

func TestCreate_NormalizeStage_Returns201(t *testing.T) {
	t.Parallel()

	binding := sampleBinding()
	binding.DispatchStage = ingestmod.DispatchStageNormalize
	svc := &mockSvc{createResult: binding}
	app := newApp(svc, true)

	req := httptest.NewRequest("POST", "/delivery-bindings", bodyJSON(map[string]any{
		"templateId":    "tpl-1",
		"targetId":      "tgt-1",
		"dispatchStage": "normalize",
	}))
	req.Header.Set("Content-Type", "application/json")

	resp, _ := app.Test(req)
	if resp.StatusCode != 201 {
		t.Errorf("expected 201 for normalize stage, got %d", resp.StatusCode)
	}
}

// ============================================================
// POST — dispatchStage validation
// ============================================================

func TestCreate_InvalidDispatchStage_Returns400(t *testing.T) {
	t.Parallel()

	invalidStages := []string{"streaming", "batch", "", "REALTIME", "Normalize", "queue"}

	for _, stage := range invalidStages {
		stage := stage
		t.Run("stage="+stage, func(t *testing.T) {
			t.Parallel()

			svc := &mockSvc{}
			app := newApp(svc, true)

			payload := map[string]any{
				"templateId": "tpl-1",
				"targetId":   "tgt-1",
			}
			if stage != "" {
				payload["dispatchStage"] = stage
			}

			req := httptest.NewRequest("POST", "/delivery-bindings", bodyJSON(payload))
			req.Header.Set("Content-Type", "application/json")

			resp, err := app.Test(req)
			if err != nil {
				t.Fatalf("app.Test: %v", err)
			}
			if resp.StatusCode != 400 {
				t.Errorf("stage=%q: expected 400, got %d", stage, resp.StatusCode)
			}

			body := parseBody(t, resp.Body)
			if body["status"] != false {
				t.Errorf("expected status=false, got %v", body["status"])
			}
		})
	}
}

// ============================================================
// POST — missing required fields
// ============================================================

func TestCreate_MissingRequiredFields_Returns400(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		payload map[string]any
	}{
		{
			name: "missing templateId",
			payload: map[string]any{
				"targetId":      "tgt-1",
				"dispatchStage": "realtime",
			},
		},
		{
			name: "missing targetId",
			payload: map[string]any{
				"templateId":    "tpl-1",
				"dispatchStage": "realtime",
			},
		},
		{
			name: "missing dispatchStage",
			payload: map[string]any{
				"templateId": "tpl-1",
				"targetId":   "tgt-1",
			},
		},
		{
			name:    "all missing",
			payload: map[string]any{},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			svc := &mockSvc{}
			app := newApp(svc, true)

			req := httptest.NewRequest("POST", "/delivery-bindings", bodyJSON(tc.payload))
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
// POST — no auth (missing activeWorkspace) → 401
// ============================================================

func TestCreate_NoAuth_Returns401(t *testing.T) {
	t.Parallel()

	svc := &mockSvc{}
	app := newApp(svc, false) // no auth middleware

	req := httptest.NewRequest("POST", "/delivery-bindings", bodyJSON(map[string]any{
		"templateId":    "tpl-1",
		"targetId":      "tgt-1",
		"dispatchStage": "realtime",
	}))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	if resp.StatusCode != 401 {
		t.Errorf("expected 401, got %d", resp.StatusCode)
	}
}

// ============================================================
// POST — enabled defaults to true; explicit false also works
// ============================================================

func TestCreate_EnabledDefaults_ToTrue(t *testing.T) {
	t.Parallel()

	var capturedInput bindingsvc.CreateBindingInput

	// Use a custom mock that captures the input
	svc := &capturingSvc{
		onCreateCapture: func(in bindingsvc.CreateBindingInput) {
			capturedInput = in
		},
		createResult: sampleBinding(),
	}
	app := fiber.New()
	app.Use(func(c fiber.Ctx) error {
		c.Locals("activeWorkspace", "ws-1")
		return c.Next()
	})
	app.Post("/delivery-bindings", (&BindingController{service: svc}).Create)

	req := httptest.NewRequest("POST", "/delivery-bindings", bodyJSON(map[string]any{
		"templateId":    "tpl-1",
		"targetId":      "tgt-1",
		"dispatchStage": "realtime",
		// enabled omitted → should default to true
	}))
	req.Header.Set("Content-Type", "application/json")

	app.Test(req) //nolint:errcheck

	if !capturedInput.Enabled {
		t.Error("expected Enabled=true when not specified in body")
	}
}

// ============================================================
// GET /delivery-bindings — List
// ============================================================

func TestList_Returns200WithDetailsItems(t *testing.T) {
	t.Parallel()

	items := []ingestmod.TemplateDeliveryBinding{*sampleBinding()}
	svc := &mockSvc{
		listResult: items,
		listPag:    &gmod.Pagination{Page: 1, PerPage: 20, TotalRecords: 1, TotalPages: 1},
	}
	app := newApp(svc, true)

	req := httptest.NewRequest("GET", "/delivery-bindings", nil)

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

	// details must be an object (never bare array)
	details, ok := body["details"].(map[string]any)
	if !ok {
		t.Fatalf("details must be a JSON object, got %T: %v", body["details"], body["details"])
	}

	itemsVal, hasItems := details["items"]
	if !hasItems {
		t.Fatal("details.items key missing")
	}
	if _, isArr := itemsVal.([]any); !isArr {
		t.Errorf("details.items must be an array, got %T", itemsVal)
	}
}

func TestList_EmptyResult_ItemsIsArray(t *testing.T) {
	t.Parallel()

	svc := &mockSvc{listResult: []ingestmod.TemplateDeliveryBinding{}}
	app := newApp(svc, true)

	req := httptest.NewRequest("GET", "/delivery-bindings", nil)
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
		t.Fatalf("details must be object, got %T", body["details"])
	}
	itemsVal, hasItems := details["items"]
	if !hasItems {
		t.Fatal("details.items key must exist even for empty list")
	}
	arr, isArr := itemsVal.([]any)
	if !isArr {
		t.Errorf("details.items must be array, got %T", itemsVal)
	}
	if len(arr) != 0 {
		t.Errorf("expected empty array, got %d items", len(arr))
	}
}

func TestList_NoAuth_Returns401(t *testing.T) {
	t.Parallel()

	svc := &mockSvc{}
	app := newApp(svc, false)

	req := httptest.NewRequest("GET", "/delivery-bindings", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	if resp.StatusCode != 401 {
		t.Errorf("expected 401, got %d", resp.StatusCode)
	}
}

// ============================================================
// GET /delivery-bindings/:id — GetOne
// ============================================================

func TestGetOne_Returns200(t *testing.T) {
	t.Parallel()

	svc := &mockSvc{getOneResult: sampleBinding()}
	app := newApp(svc, true)

	req := httptest.NewRequest("GET", "/delivery-bindings/bind-1", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}

func TestGetOne_NotFound_Returns404(t *testing.T) {
	t.Parallel()

	svc := &mockSvc{getOneErr: bindingsvc.ErrNotFound}
	app := newApp(svc, true)

	req := httptest.NewRequest("GET", "/delivery-bindings/ghost", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	if resp.StatusCode != 404 {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
}

// ============================================================
// PATCH /delivery-bindings/:id — Update
// ============================================================

func TestUpdate_EnabledFalse_Returns200(t *testing.T) {
	t.Parallel()

	updated := sampleBinding()
	updated.Enabled = false
	svc := &mockSvc{updateResult: updated}
	app := newApp(svc, true)

	req := httptest.NewRequest("PATCH", "/delivery-bindings/bind-1", bodyJSON(map[string]any{
		"enabled": false,
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
	if body["status"] != true {
		t.Errorf("expected status=true, got %v", body["status"])
	}
}

func TestUpdate_DispatchStage_Returns200(t *testing.T) {
	t.Parallel()

	updated := sampleBinding()
	updated.DispatchStage = ingestmod.DispatchStageNormalize
	svc := &mockSvc{updateResult: updated}
	app := newApp(svc, true)

	req := httptest.NewRequest("PATCH", "/delivery-bindings/bind-1", bodyJSON(map[string]any{
		"dispatchStage": "normalize",
	}))
	req.Header.Set("Content-Type", "application/json")

	resp, _ := app.Test(req)
	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}

func TestUpdate_NotFound_Returns404(t *testing.T) {
	t.Parallel()

	svc := &mockSvc{updateErr: bindingsvc.ErrNotFound}
	app := newApp(svc, true)

	req := httptest.NewRequest("PATCH", "/delivery-bindings/ghost", bodyJSON(map[string]any{
		"enabled": true,
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
// DELETE /delivery-bindings/:id — Delete
// ============================================================

func TestDelete_Returns200(t *testing.T) {
	t.Parallel()

	svc := &mockSvc{deleteErr: nil}
	app := newApp(svc, true)

	req := httptest.NewRequest("DELETE", "/delivery-bindings/bind-1", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	// httputil.MessageOK → 200
	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	body := parseBody(t, resp.Body)
	if body["status"] != true {
		t.Errorf("expected status=true, got %v", body["status"])
	}
	if body["code"] != "SUCCESS" {
		t.Errorf("expected code=SUCCESS, got %v", body["code"])
	}
}

func TestDelete_NotFound_Returns404(t *testing.T) {
	t.Parallel()

	svc := &mockSvc{deleteErr: bindingsvc.ErrNotFound}
	app := newApp(svc, true)

	req := httptest.NewRequest("DELETE", "/delivery-bindings/ghost", nil)
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
// Response envelope contract — all success responses
// ============================================================

func TestCreate_ResponseEnvelope(t *testing.T) {
	t.Parallel()

	svc := &mockSvc{createResult: sampleBinding()}
	app := newApp(svc, true)

	req := httptest.NewRequest("POST", "/delivery-bindings", bodyJSON(map[string]any{
		"templateId":    "tpl-1",
		"targetId":      "tgt-1",
		"dispatchStage": "realtime",
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

// ============================================================
// Additional mock for input capture
// ============================================================

type capturingSvc struct {
	onCreateCapture func(bindingsvc.CreateBindingInput)
	createResult    *ingestmod.TemplateDeliveryBinding
	createErr       error
}

func (m *capturingSvc) Create(_ context.Context, in bindingsvc.CreateBindingInput) (*ingestmod.TemplateDeliveryBinding, error) {
	if m.onCreateCapture != nil {
		m.onCreateCapture(in)
	}
	return m.createResult, m.createErr
}
func (m *capturingSvc) List(_ context.Context, _ string, _, _ int) ([]ingestmod.TemplateDeliveryBinding, *gmod.Pagination, error) {
	return nil, nil, nil
}
func (m *capturingSvc) GetOne(_ context.Context, _, _ string) (*ingestmod.TemplateDeliveryBinding, error) {
	return nil, nil
}
func (m *capturingSvc) Update(_ context.Context, _ bindingsvc.UpdateBindingInput) (*ingestmod.TemplateDeliveryBinding, error) {
	return nil, nil
}
func (m *capturingSvc) Delete(_ context.Context, _, _ string) error { return nil }
