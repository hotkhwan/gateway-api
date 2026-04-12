// controllers/ingesttmplapi/ingestTemplate_test.go
package ingesttmplapi

import (
	"context"
	"encoding/json"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/hotkhwan/gateway-api/internal/services/ingesttmplsvc"
	"github.com/hotkhwan/gateway-api/models/gmod"
	"github.com/hotkhwan/gateway-api/models/ingestmod"
)

// ============================================================
// mockSvc — implements ingestTmplSvcI
// ============================================================

type mockSvc struct {
	createResult *ingestmod.IngestTemplate
	createErr    error

	listResult []ingestmod.IngestTemplate
	listPag    *gmod.Pagination
	listErr    error

	getOneResult *ingestmod.IngestTemplate
	getOneErr    error

	updateResult *ingestmod.IngestTemplate
	updateErr    error

	deleteErr error
}

func (m *mockSvc) Create(_ context.Context, _ ingesttmplsvc.CreateIngestTemplateInput) (*ingestmod.IngestTemplate, error) {
	return m.createResult, m.createErr
}
func (m *mockSvc) List(_ context.Context, _ string, _, _ int) ([]ingestmod.IngestTemplate, *gmod.Pagination, error) {
	return m.listResult, m.listPag, m.listErr
}
func (m *mockSvc) GetOne(_ context.Context, _, _ string) (*ingestmod.IngestTemplate, error) {
	return m.getOneResult, m.getOneErr
}
func (m *mockSvc) Update(_ context.Context, _ ingesttmplsvc.UpdateIngestTemplateInput) (*ingestmod.IngestTemplate, error) {
	return m.updateResult, m.updateErr
}
func (m *mockSvc) Delete(_ context.Context, _, _ string) error { return m.deleteErr }

// ============================================================
// Test app helpers
// ============================================================

func newApp(svc ingestTmplSvcI, withAuth bool) *fiber.App {
	app := fiber.New()
	ctrl := &IngestTemplateController{service: svc}

	if withAuth {
		app.Use(func(c fiber.Ctx) error {
			c.Locals("activeWorkspace", "ws-1")
			return c.Next()
		})
	}

	app.Post("/ingest-templates", ctrl.Create)
	app.Get("/ingest-templates", ctrl.List)
	app.Get("/ingest-templates/:id", ctrl.GetOne)
	app.Patch("/ingest-templates/:id", ctrl.Update)
	app.Delete("/ingest-templates/:id", ctrl.Delete)

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
		t.Fatalf("unmarshal response: %v (raw=%s)", err, raw)
	}
	return m
}

func sampleTemplate() *ingestmod.IngestTemplate {
	return &ingestmod.IngestTemplate{
		ID:           "tmpl-1",
		WorkspaceID:  "ws-1",
		Name:         "Dahua Motion",
		SourceFamily: "dahua",
		Enabled:      true,
	}
}

// ============================================================
// POST /ingest-templates — Create
// ============================================================

func TestCreate_Valid_Returns201(t *testing.T) {
	t.Parallel()

	svc := &mockSvc{createResult: sampleTemplate()}
	app := newApp(svc, true)

	req := httptest.NewRequest("POST", "/ingest-templates", bodyJSON(map[string]any{
		"name":         "Dahua Motion",
		"sourceFamily": "dahua",
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

// ============================================================
// POST — sourceFamily required
// ============================================================

func TestCreate_MissingSourceFamily_Returns400(t *testing.T) {
	t.Parallel()

	svc := &mockSvc{}
	app := newApp(svc, true)

	req := httptest.NewRequest("POST", "/ingest-templates", bodyJSON(map[string]any{
		"name": "No Family",
		// sourceFamily omitted
	}))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	if resp.StatusCode != 400 {
		t.Errorf("expected 400 when sourceFamily missing, got %d", resp.StatusCode)
	}

	body := parseBody(t, resp.Body)
	if body["status"] != false {
		t.Errorf("expected status=false, got %v", body["status"])
	}
}

func TestCreate_WhitespaceOnlySourceFamily_Returns400(t *testing.T) {
	t.Parallel()

	svc := &mockSvc{}
	app := newApp(svc, true)

	req := httptest.NewRequest("POST", "/ingest-templates", bodyJSON(map[string]any{
		"name":         "Template",
		"sourceFamily": "   ",
	}))
	req.Header.Set("Content-Type", "application/json")

	resp, _ := app.Test(req)
	if resp.StatusCode != 400 {
		t.Errorf("expected 400 for whitespace-only sourceFamily, got %d", resp.StatusCode)
	}
}

// ============================================================
// POST — name required
// ============================================================

func TestCreate_MissingName_Returns400(t *testing.T) {
	t.Parallel()

	svc := &mockSvc{}
	app := newApp(svc, true)

	req := httptest.NewRequest("POST", "/ingest-templates", bodyJSON(map[string]any{
		"sourceFamily": "dahua",
		// name omitted
	}))
	req.Header.Set("Content-Type", "application/json")

	resp, _ := app.Test(req)
	if resp.StatusCode != 400 {
		t.Errorf("expected 400 when name missing, got %d", resp.StatusCode)
	}
}

// ============================================================
// POST — all missing-field cases (table-driven)
// ============================================================

func TestCreate_RequiredFields_Validation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		payload map[string]any
	}{
		{"missing name", map[string]any{"sourceFamily": "dahua"}},
		{"missing sourceFamily", map[string]any{"name": "T"}},
		{"both missing", map[string]any{}},
		{"whitespace name", map[string]any{"name": "  ", "sourceFamily": "dahua"}},
		{"whitespace sourceFamily", map[string]any{"name": "T", "sourceFamily": "  "}},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			svc := &mockSvc{}
			app := newApp(svc, true)

			req := httptest.NewRequest("POST", "/ingest-templates", bodyJSON(tc.payload))
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
// POST — no auth → 401
// ============================================================

func TestCreate_NoAuth_Returns401(t *testing.T) {
	t.Parallel()

	svc := &mockSvc{}
	app := newApp(svc, false)

	req := httptest.NewRequest("POST", "/ingest-templates", bodyJSON(map[string]any{
		"name":         "T",
		"sourceFamily": "dahua",
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
// POST — duplicate name → 409
// ============================================================

func TestCreate_DuplicateName_Returns409(t *testing.T) {
	t.Parallel()

	svc := &mockSvc{createErr: ingesttmplsvc.ErrConflict}
	app := newApp(svc, true)

	req := httptest.NewRequest("POST", "/ingest-templates", bodyJSON(map[string]any{
		"name":         "Taken",
		"sourceFamily": "dahua",
	}))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	if resp.StatusCode != 409 {
		t.Errorf("expected 409, got %d", resp.StatusCode)
	}
}

// ============================================================
// POST — with matchRules → 201
// ============================================================

func TestCreate_WithMatchRules_Returns201(t *testing.T) {
	t.Parallel()

	svc := &mockSvc{createResult: sampleTemplate()}
	app := newApp(svc, true)

	req := httptest.NewRequest("POST", "/ingest-templates", bodyJSON(map[string]any{
		"name":         "Dahua Intrusion",
		"sourceFamily": "dahua",
		"matchRules": []map[string]any{
			{"vendor": "dahua", "eventType": "intrusion"},
		},
	}))
	req.Header.Set("Content-Type", "application/json")

	resp, _ := app.Test(req)
	if resp.StatusCode != 201 {
		t.Errorf("expected 201 with matchRules, got %d", resp.StatusCode)
	}
}

// ============================================================
// GET /ingest-templates — List
// ============================================================

func TestList_Returns200WithDetailsItems(t *testing.T) {
	t.Parallel()

	items := []ingestmod.IngestTemplate{*sampleTemplate()}
	svc := &mockSvc{
		listResult: items,
		listPag:    &gmod.Pagination{Page: 1, PerPages: 20, TotalRecords: 1, TotalPages: 1},
	}
	app := newApp(svc, true)

	req := httptest.NewRequest("GET", "/ingest-templates", nil)
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

	// details must be an object, never a bare array
	details, ok := body["details"].(map[string]any)
	if !ok {
		t.Fatalf("details must be JSON object, got %T", body["details"])
	}
	itemsVal, hasItems := details["items"]
	if !hasItems {
		t.Fatal("details.items key missing")
	}
	if _, isArr := itemsVal.([]any); !isArr {
		t.Errorf("details.items must be array, got %T", itemsVal)
	}
}

func TestList_EmptyResult_ItemsIsArray(t *testing.T) {
	t.Parallel()

	svc := &mockSvc{listResult: []ingestmod.IngestTemplate{}}
	app := newApp(svc, true)

	req := httptest.NewRequest("GET", "/ingest-templates", nil)
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
	itemsVal := details["items"]
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

	req := httptest.NewRequest("GET", "/ingest-templates", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	if resp.StatusCode != 401 {
		t.Errorf("expected 401, got %d", resp.StatusCode)
	}
}

// ============================================================
// GET /ingest-templates/:id — GetOne
// ============================================================

func TestGetOne_Returns200(t *testing.T) {
	t.Parallel()

	svc := &mockSvc{getOneResult: sampleTemplate()}
	app := newApp(svc, true)

	req := httptest.NewRequest("GET", "/ingest-templates/tmpl-1", nil)
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

func TestGetOne_NonExistent_Returns404(t *testing.T) {
	t.Parallel()

	svc := &mockSvc{getOneErr: ingesttmplsvc.ErrNotFound}
	app := newApp(svc, true)

	req := httptest.NewRequest("GET", "/ingest-templates/ghost", nil)
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
// PATCH /ingest-templates/:id — Update
// ============================================================

func TestUpdate_MatchRules_Returns200(t *testing.T) {
	t.Parallel()

	updated := sampleTemplate()
	updated.MatchRules = []ingestmod.MatchRule{
		{Vendor: "dahua", EventType: "motion"},
	}
	svc := &mockSvc{updateResult: updated}
	app := newApp(svc, true)

	req := httptest.NewRequest("PATCH", "/ingest-templates/tmpl-1", bodyJSON(map[string]any{
		"matchRules": []map[string]any{
			{"vendor": "dahua", "eventType": "motion"},
		},
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

func TestUpdate_SourceFamily_Returns200(t *testing.T) {
	t.Parallel()

	updated := sampleTemplate()
	updated.SourceFamily = "hikvision"
	svc := &mockSvc{updateResult: updated}
	app := newApp(svc, true)

	req := httptest.NewRequest("PATCH", "/ingest-templates/tmpl-1", bodyJSON(map[string]any{
		"sourceFamily": "hikvision",
	}))
	req.Header.Set("Content-Type", "application/json")

	resp, _ := app.Test(req)
	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}

func TestUpdate_Enabled_Returns200(t *testing.T) {
	t.Parallel()

	updated := sampleTemplate()
	updated.Enabled = false
	svc := &mockSvc{updateResult: updated}
	app := newApp(svc, true)

	req := httptest.NewRequest("PATCH", "/ingest-templates/tmpl-1", bodyJSON(map[string]any{
		"enabled": false,
	}))
	req.Header.Set("Content-Type", "application/json")

	resp, _ := app.Test(req)
	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}

func TestUpdate_NotFound_Returns404(t *testing.T) {
	t.Parallel()

	svc := &mockSvc{updateErr: ingesttmplsvc.ErrNotFound}
	app := newApp(svc, true)

	req := httptest.NewRequest("PATCH", "/ingest-templates/ghost", bodyJSON(map[string]any{
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
// DELETE /ingest-templates/:id — Delete
// ============================================================

func TestDelete_Returns200(t *testing.T) {
	t.Parallel()

	svc := &mockSvc{deleteErr: nil}
	app := newApp(svc, true)

	req := httptest.NewRequest("DELETE", "/ingest-templates/tmpl-1", nil)
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
}

func TestDelete_NotFound_Returns404(t *testing.T) {
	t.Parallel()

	svc := &mockSvc{deleteErr: ingesttmplsvc.ErrNotFound}
	app := newApp(svc, true)

	req := httptest.NewRequest("DELETE", "/ingest-templates/ghost", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	if resp.StatusCode != 404 {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
}

// ============================================================
// Response envelope contract
// ============================================================

func TestCreate_ResponseEnvelope(t *testing.T) {
	t.Parallel()

	svc := &mockSvc{createResult: sampleTemplate()}
	app := newApp(svc, true)

	req := httptest.NewRequest("POST", "/ingest-templates", bodyJSON(map[string]any{
		"name":         "T",
		"sourceFamily": "dahua",
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
