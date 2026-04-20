// controllers/msgtmplapi/msgTemplate_test.go
package msgtmplapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/hotkhwan/gateway-api/internal/services/msgtmplsvc"
	"github.com/hotkhwan/gateway-api/models/gmod"
	"github.com/hotkhwan/gateway-api/models/ingestmod"
)

// ============================================================
// Mock service
// ============================================================

type mockMsgTmplSvc struct {
	createFn func(ctx context.Context, input msgtmplsvc.CreateMsgTemplateInput) (*ingestmod.WorkspaceMessageTemplate, error)
	listFn   func(ctx context.Context, workspaceId string, page, perPage int) ([]ingestmod.WorkspaceMessageTemplate, *gmod.Pagination, error)
	getOneFn func(ctx context.Context, workspaceId, id string) (*ingestmod.WorkspaceMessageTemplate, error)
	updateFn func(ctx context.Context, input msgtmplsvc.UpdateMsgTemplateInput) (*ingestmod.WorkspaceMessageTemplate, error)
	deleteFn func(ctx context.Context, workspaceId, id string) error
}

func (m *mockMsgTmplSvc) Create(ctx context.Context, input msgtmplsvc.CreateMsgTemplateInput) (*ingestmod.WorkspaceMessageTemplate, error) {
	if m.createFn != nil {
		return m.createFn(ctx, input)
	}
	return &ingestmod.WorkspaceMessageTemplate{ID: "tmpl-1", WorkspaceID: input.WorkspaceID, Name: input.Name, Channel: input.Channel}, nil
}

func (m *mockMsgTmplSvc) List(ctx context.Context, workspaceId string, page, perPage int) ([]ingestmod.WorkspaceMessageTemplate, *gmod.Pagination, error) {
	if m.listFn != nil {
		return m.listFn(ctx, workspaceId, page, perPage)
	}
	return []ingestmod.WorkspaceMessageTemplate{}, nil, nil
}

func (m *mockMsgTmplSvc) GetOne(ctx context.Context, workspaceId, id string) (*ingestmod.WorkspaceMessageTemplate, error) {
	if m.getOneFn != nil {
		return m.getOneFn(ctx, workspaceId, id)
	}
	return &ingestmod.WorkspaceMessageTemplate{ID: id, WorkspaceID: workspaceId}, nil
}

func (m *mockMsgTmplSvc) Update(ctx context.Context, input msgtmplsvc.UpdateMsgTemplateInput) (*ingestmod.WorkspaceMessageTemplate, error) {
	if m.updateFn != nil {
		return m.updateFn(ctx, input)
	}
	return &ingestmod.WorkspaceMessageTemplate{ID: input.ID, WorkspaceID: input.WorkspaceID}, nil
}

func (m *mockMsgTmplSvc) Delete(ctx context.Context, workspaceId, id string) error {
	if m.deleteFn != nil {
		return m.deleteFn(ctx, workspaceId, id)
	}
	return nil
}

// ============================================================
// Test helpers
// ============================================================

func newMsgTmplApp(svc msgTmplSvcI, withAuth bool) *fiber.App {
	app := fiber.New(fiber.Config{})
	ctrl := NewMsgTemplateController(svc)

	auth := func(c fiber.Ctx) error {
		if withAuth {
			c.Locals("activeWorkspace", "ws-1")
		}
		return c.Next()
	}

	app.Post("/workspaces/:workspaceId/message-templates", auth, ctrl.Create)
	app.Get("/workspaces/:workspaceId/message-templates", auth, ctrl.List)
	app.Get("/workspaces/:workspaceId/message-templates/:id", auth, ctrl.GetOne)
	app.Patch("/workspaces/:workspaceId/message-templates/:id", auth, ctrl.Update)
	app.Delete("/workspaces/:workspaceId/message-templates/:id", auth, ctrl.Delete)

	return app
}

func bodyMsgTmpl(v any) *bytes.Buffer {
	b, _ := json.Marshal(v)
	return bytes.NewBuffer(b)
}

func parseMsgTmplBody(t *testing.T, raw []byte) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("parse body: %v", err)
	}
	return out
}

// ============================================================
// Create tests
// ============================================================

func TestMsgTmplCreate_NoName_400(t *testing.T) {
	t.Parallel()
	app := newMsgTmplApp(&mockMsgTmplSvc{}, true)
	req := httptest.NewRequest("POST", "/workspaces/ws-1/message-templates",
		bodyMsgTmpl(map[string]any{"channel": "line"}))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := app.Test(req)
	if resp.StatusCode != 400 {
		t.Errorf("want 400, got %d", resp.StatusCode)
	}
}

func TestMsgTmplCreate_WhitespaceName_400(t *testing.T) {
	t.Parallel()
	app := newMsgTmplApp(&mockMsgTmplSvc{}, true)
	req := httptest.NewRequest("POST", "/workspaces/ws-1/message-templates",
		bodyMsgTmpl(map[string]any{"name": "   ", "channel": "line"}))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := app.Test(req)
	if resp.StatusCode != 400 {
		t.Errorf("want 400, got %d", resp.StatusCode)
	}
}

func TestMsgTmplCreate_InvalidChannel_400(t *testing.T) {
	t.Parallel()
	svc := &mockMsgTmplSvc{
		createFn: func(_ context.Context, _ msgtmplsvc.CreateMsgTemplateInput) (*ingestmod.WorkspaceMessageTemplate, error) {
			return nil, msgtmplsvc.ErrBadRequest
		},
	}
	app := newMsgTmplApp(svc, true)
	req := httptest.NewRequest("POST", "/workspaces/ws-1/message-templates",
		bodyMsgTmpl(map[string]any{"name": "Alert", "channel": "sms"}))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := app.Test(req)
	if resp.StatusCode != 400 {
		t.Errorf("want 400, got %d", resp.StatusCode)
	}
}

func TestMsgTmplCreate_Valid_201(t *testing.T) {
	t.Parallel()
	app := newMsgTmplApp(&mockMsgTmplSvc{}, true)
	req := httptest.NewRequest("POST", "/workspaces/ws-1/message-templates",
		bodyMsgTmpl(map[string]any{"name": "Alert", "channel": "line", "body": "hello"}))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := app.Test(req)
	if resp.StatusCode != 201 {
		t.Errorf("want 201, got %d", resp.StatusCode)
	}
}

func TestMsgTmplCreate_NoChannel_201(t *testing.T) {
	t.Parallel()
	// channel is optional — omitting it should still succeed
	app := newMsgTmplApp(&mockMsgTmplSvc{}, true)
	req := httptest.NewRequest("POST", "/workspaces/ws-1/message-templates",
		bodyMsgTmpl(map[string]any{"name": "No Channel"}))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := app.Test(req)
	if resp.StatusCode != 201 {
		t.Errorf("want 201, got %d", resp.StatusCode)
	}
}

func TestMsgTmplCreate_Duplicate_409(t *testing.T) {
	t.Parallel()
	svc := &mockMsgTmplSvc{
		createFn: func(_ context.Context, _ msgtmplsvc.CreateMsgTemplateInput) (*ingestmod.WorkspaceMessageTemplate, error) {
			return nil, msgtmplsvc.ErrConflict
		},
	}
	app := newMsgTmplApp(svc, true)
	req := httptest.NewRequest("POST", "/workspaces/ws-1/message-templates",
		bodyMsgTmpl(map[string]any{"name": "Dup"}))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := app.Test(req)
	if resp.StatusCode != 409 {
		t.Errorf("want 409, got %d", resp.StatusCode)
	}
}

func TestMsgTmplCreate_NoAuth_401(t *testing.T) {
	t.Parallel()
	app := newMsgTmplApp(&mockMsgTmplSvc{}, false)
	req := httptest.NewRequest("POST", "/workspaces/ws-1/message-templates",
		bodyMsgTmpl(map[string]any{"name": "Alert"}))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := app.Test(req)
	if resp.StatusCode != 401 {
		t.Errorf("want 401, got %d", resp.StatusCode)
	}
}

// ============================================================
// Create response shape
// ============================================================

func TestMsgTmplCreate_ResponseEnvelope(t *testing.T) {
	t.Parallel()
	svc := &mockMsgTmplSvc{
		createFn: func(_ context.Context, input msgtmplsvc.CreateMsgTemplateInput) (*ingestmod.WorkspaceMessageTemplate, error) {
			return &ingestmod.WorkspaceMessageTemplate{ID: "tmpl-abc", Name: input.Name, Channel: input.Channel}, nil
		},
	}
	app := newMsgTmplApp(svc, true)
	req := httptest.NewRequest("POST", "/workspaces/ws-1/message-templates",
		bodyMsgTmpl(map[string]any{"name": "MyTmpl", "channel": "webhook"}))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := app.Test(req)

	if resp.StatusCode != 201 {
		t.Fatalf("want 201, got %d", resp.StatusCode)
	}

	var buf bytes.Buffer
	buf.ReadFrom(resp.Body)
	body := parseMsgTmplBody(t, buf.Bytes())

	if body["status"] != true {
		t.Errorf("want status=true, got %v", body["status"])
	}
	if body["code"] != "CREATED" {
		t.Errorf("want code=CREATED, got %v", body["code"])
	}
	details, ok := body["details"].(map[string]any)
	if !ok {
		t.Fatalf("details must be an object, got %T", body["details"])
	}
	if details["id"] != "tmpl-abc" {
		t.Errorf("want id=tmpl-abc, got %v", details["id"])
	}
}

// ============================================================
// List tests
// ============================================================

func TestMsgTmplList_200_ItemsArray(t *testing.T) {
	t.Parallel()
	svc := &mockMsgTmplSvc{
		listFn: func(_ context.Context, _ string, _, _ int) ([]ingestmod.WorkspaceMessageTemplate, *gmod.Pagination, error) {
			return []ingestmod.WorkspaceMessageTemplate{
				{ID: "t1", Name: "T1"},
				{ID: "t2", Name: "T2"},
			}, nil, nil
		},
	}
	app := newMsgTmplApp(svc, true)
	req := httptest.NewRequest("GET", "/workspaces/ws-1/message-templates", nil)
	resp, _ := app.Test(req)

	if resp.StatusCode != 200 {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}

	var buf bytes.Buffer
	buf.ReadFrom(resp.Body)
	body := parseMsgTmplBody(t, buf.Bytes())

	details, ok := body["details"].(map[string]any)
	if !ok {
		t.Fatalf("details must be object, got %T", body["details"])
	}
	items, ok := details["items"].([]any)
	if !ok {
		t.Fatalf("details.items must be array, got %T", details["items"])
	}
	if len(items) != 2 {
		t.Errorf("want 2 items, got %d", len(items))
	}
}

func TestMsgTmplList_EmptyItems(t *testing.T) {
	t.Parallel()
	app := newMsgTmplApp(&mockMsgTmplSvc{}, true)
	req := httptest.NewRequest("GET", "/workspaces/ws-1/message-templates", nil)
	resp, _ := app.Test(req)

	if resp.StatusCode != 200 {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}

	var buf bytes.Buffer
	buf.ReadFrom(resp.Body)
	body := parseMsgTmplBody(t, buf.Bytes())

	details, ok := body["details"].(map[string]any)
	if !ok {
		t.Fatalf("details must be object, got %T", body["details"])
	}
	// items should be an array (even if empty)
	_, ok = details["items"]
	if !ok {
		t.Errorf("details.items key missing")
	}
}

func TestMsgTmplList_NoAuth_401(t *testing.T) {
	t.Parallel()
	app := newMsgTmplApp(&mockMsgTmplSvc{}, false)
	req := httptest.NewRequest("GET", "/workspaces/ws-1/message-templates", nil)
	resp, _ := app.Test(req)
	if resp.StatusCode != 401 {
		t.Errorf("want 401, got %d", resp.StatusCode)
	}
}

func TestMsgTmplList_ServiceError_500(t *testing.T) {
	t.Parallel()
	svc := &mockMsgTmplSvc{
		listFn: func(_ context.Context, _ string, _, _ int) ([]ingestmod.WorkspaceMessageTemplate, *gmod.Pagination, error) {
			return nil, nil, errors.New("db error")
		},
	}
	app := newMsgTmplApp(svc, true)
	req := httptest.NewRequest("GET", "/workspaces/ws-1/message-templates", nil)
	resp, _ := app.Test(req)
	if resp.StatusCode != 500 {
		t.Errorf("want 500, got %d", resp.StatusCode)
	}
}

// ============================================================
// GetOne tests
// ============================================================

func TestMsgTmplGetOne_200(t *testing.T) {
	t.Parallel()
	app := newMsgTmplApp(&mockMsgTmplSvc{}, true)
	req := httptest.NewRequest("GET", "/workspaces/ws-1/message-templates/tmpl-1", nil)
	resp, _ := app.Test(req)
	if resp.StatusCode != 200 {
		t.Errorf("want 200, got %d", resp.StatusCode)
	}
}

func TestMsgTmplGetOne_NotFound_404(t *testing.T) {
	t.Parallel()
	svc := &mockMsgTmplSvc{
		getOneFn: func(_ context.Context, _, _ string) (*ingestmod.WorkspaceMessageTemplate, error) {
			return nil, msgtmplsvc.ErrNotFound
		},
	}
	app := newMsgTmplApp(svc, true)
	req := httptest.NewRequest("GET", "/workspaces/ws-1/message-templates/missing", nil)
	resp, _ := app.Test(req)
	if resp.StatusCode != 404 {
		t.Errorf("want 404, got %d", resp.StatusCode)
	}
}

// ============================================================
// Update tests
// ============================================================

func TestMsgTmplUpdate_Channel_200(t *testing.T) {
	t.Parallel()
	ch := "telegram"
	svc := &mockMsgTmplSvc{
		updateFn: func(_ context.Context, input msgtmplsvc.UpdateMsgTemplateInput) (*ingestmod.WorkspaceMessageTemplate, error) {
			return &ingestmod.WorkspaceMessageTemplate{ID: input.ID, Channel: *input.Channel}, nil
		},
	}
	app := newMsgTmplApp(svc, true)
	req := httptest.NewRequest("PATCH", "/workspaces/ws-1/message-templates/tmpl-1",
		bodyMsgTmpl(map[string]any{"channel": ch}))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := app.Test(req)
	if resp.StatusCode != 200 {
		t.Errorf("want 200, got %d", resp.StatusCode)
	}
}

func TestMsgTmplUpdate_Body_200(t *testing.T) {
	t.Parallel()
	app := newMsgTmplApp(&mockMsgTmplSvc{}, true)
	req := httptest.NewRequest("PATCH", "/workspaces/ws-1/message-templates/tmpl-1",
		bodyMsgTmpl(map[string]any{"body": "updated body"}))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := app.Test(req)
	if resp.StatusCode != 200 {
		t.Errorf("want 200, got %d", resp.StatusCode)
	}
}

func TestMsgTmplUpdate_NotFound_404(t *testing.T) {
	t.Parallel()
	svc := &mockMsgTmplSvc{
		updateFn: func(_ context.Context, _ msgtmplsvc.UpdateMsgTemplateInput) (*ingestmod.WorkspaceMessageTemplate, error) {
			return nil, msgtmplsvc.ErrNotFound
		},
	}
	app := newMsgTmplApp(svc, true)
	req := httptest.NewRequest("PATCH", "/workspaces/ws-1/message-templates/missing",
		bodyMsgTmpl(map[string]any{"body": "x"}))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := app.Test(req)
	if resp.StatusCode != 404 {
		t.Errorf("want 404, got %d", resp.StatusCode)
	}
}

func TestMsgTmplUpdate_NoAuth_400(t *testing.T) {
	t.Parallel()
	// Update combines workspaceId+id check → 400 when no auth (workspaceId empty)
	app := newMsgTmplApp(&mockMsgTmplSvc{}, false)
	req := httptest.NewRequest("PATCH", "/workspaces/ws-1/message-templates/tmpl-1",
		bodyMsgTmpl(map[string]any{"body": "x"}))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := app.Test(req)
	if resp.StatusCode != 400 {
		t.Errorf("want 400, got %d", resp.StatusCode)
	}
}

// ============================================================
// Delete tests
// ============================================================

func TestMsgTmplDelete_200(t *testing.T) {
	t.Parallel()
	app := newMsgTmplApp(&mockMsgTmplSvc{}, true)
	req := httptest.NewRequest("DELETE", "/workspaces/ws-1/message-templates/tmpl-1", nil)
	resp, _ := app.Test(req)
	if resp.StatusCode != 200 {
		t.Errorf("want 200, got %d", resp.StatusCode)
	}
}

func TestMsgTmplDelete_NotFound_404(t *testing.T) {
	t.Parallel()
	svc := &mockMsgTmplSvc{
		deleteFn: func(_ context.Context, _, _ string) error {
			return msgtmplsvc.ErrNotFound
		},
	}
	app := newMsgTmplApp(svc, true)
	req := httptest.NewRequest("DELETE", "/workspaces/ws-1/message-templates/missing", nil)
	resp, _ := app.Test(req)
	if resp.StatusCode != 404 {
		t.Errorf("want 404, got %d", resp.StatusCode)
	}
}

func TestMsgTmplDelete_NoAuth_400(t *testing.T) {
	t.Parallel()
	// Delete combines workspaceId+id check → 400 when no auth (workspaceId empty)
	app := newMsgTmplApp(&mockMsgTmplSvc{}, false)
	req := httptest.NewRequest("DELETE", "/workspaces/ws-1/message-templates/tmpl-1", nil)
	resp, _ := app.Test(req)
	if resp.StatusCode != 400 {
		t.Errorf("want 400, got %d", resp.StatusCode)
	}
}
