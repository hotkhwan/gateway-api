// internal/grpc/targetgrpc/server_test.go
package targetgrpc

import (
	"context"
	"errors"
	"testing"

	"github.com/hotkhwan/gateway-api/internal/repo/ingestrepo"
	"github.com/hotkhwan/gateway-api/internal/services/targetsvc"
	"github.com/hotkhwan/gateway-api/models/authzmod"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ─── Stubs ───────────────────────────────────────────────────────────────────

type stubWSLookup struct {
	tenantID string
	err      error
}

func (s stubWSLookup) GetByID(_ context.Context, _ string) (*WorkspaceLookupResult, error) {
	if s.err != nil {
		return nil, s.err
	}
	if s.tenantID == "" {
		return nil, nil
	}
	return &WorkspaceLookupResult{TenantID: s.tenantID}, nil
}

type stubTargetSvc struct {
	createIn  targetsvc.CreateTargetInput
	createOut *authzmod.DeliveryTarget
	createErr error

	listOut   []authzmod.DeliveryTarget
	listTotal int64
	listErr   error

	getOut *authzmod.DeliveryTarget
	getErr error

	updateIn  targetsvc.UpdateTargetInput
	updateOut *authzmod.DeliveryTarget
	updateErr error

	deleteErr error
}

func (s *stubTargetSvc) Create(_ context.Context, in targetsvc.CreateTargetInput) (*authzmod.DeliveryTarget, error) {
	s.createIn = in
	return s.createOut, s.createErr
}
func (s *stubTargetSvc) List(_ context.Context, _ targetsvc.ListTargetInput) ([]authzmod.DeliveryTarget, int64, error) {
	return s.listOut, s.listTotal, s.listErr
}
func (s *stubTargetSvc) GetOne(_ context.Context, _, _, _, _ string, _ bool) (*authzmod.DeliveryTarget, error) {
	return s.getOut, s.getErr
}
func (s *stubTargetSvc) Update(_ context.Context, in targetsvc.UpdateTargetInput) (*authzmod.DeliveryTarget, error) {
	s.updateIn = in
	return s.updateOut, s.updateErr
}
func (s *stubTargetSvc) Delete(_ context.Context, _, _, _, _ string, _ bool) error {
	return s.deleteErr
}

// methodHandler is the grpc method handler signature produced by the
// *handler() methods on TargetServiceServer.
type methodHandler = func(srv any, ctx context.Context, dec func(any) error, interceptor grpc.UnaryServerInterceptor) (any, error)

// call drives a handler by pre-baking the decoder to copy `req` into the
// handler's request pointer. Keeps each test focused on observable behavior.
func call[T any](t *testing.T, h methodHandler, req *T) (any, error) {
	t.Helper()
	dec := func(dst any) error {
		ptr, ok := dst.(*T)
		if !ok {
			t.Fatalf("unexpected decode target %T", dst)
		}
		*ptr = *req
		return nil
	}
	return h(nil, context.Background(), dec, nil)
}

// ─── Create ──────────────────────────────────────────────────────────────────

func TestCreate_ForwardsPlatformAdminAndCallerID(t *testing.T) {
	svc := &stubTargetSvc{
		createOut: &authzmod.DeliveryTarget{
			TargetId:    "t-1",
			WorkspaceId: "ws-1",
			TenantId:    "tenant-1",
			Name:        "hook",
			Type:        authzmod.TargetTypeWebhook,
			Enabled:     true,
			Config:      authzmod.TargetConfig{URL: "https://example.com"},
		},
	}
	s := newTargetServerWithI(svc, stubWSLookup{tenantID: "tenant-1"})

	enabled := true
	out, err := call(t, s.createHandler(), &CreateTargetRequest{
		WorkspaceID:  "ws-1",
		CallerUserID: "user-42",
		Name:         "hook",
		Type:         authzmod.TargetTypeWebhook,
		Enabled:      &enabled,
		Config:       authzmod.TargetConfig{URL: "https://example.com"},
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	resp := out.(*CreateTargetResponse)
	if resp.Target == nil || resp.Target.TargetID != "t-1" {
		t.Fatalf("unexpected response: %+v", resp)
	}
	if !svc.createIn.IsPlatformAdmin {
		t.Error("IsPlatformAdmin must be true (klynx-api authorized upstream)")
	}
	if svc.createIn.UserId != "user-42" {
		t.Errorf("UserId forwarded as CreatedBy: want user-42, got %q", svc.createIn.UserId)
	}
	if svc.createIn.TenantId != "tenant-1" {
		t.Errorf("TenantId resolved from workspace: want tenant-1, got %q", svc.createIn.TenantId)
	}
}

func TestCreate_MissingCallerUserIDRejected(t *testing.T) {
	s := newTargetServerWithI(&stubTargetSvc{}, stubWSLookup{tenantID: "tenant-1"})
	_, err := call(t, s.createHandler(), &CreateTargetRequest{WorkspaceID: "ws-1"})
	if code := status.Code(err); code != codes.InvalidArgument {
		t.Errorf("want InvalidArgument, got %s (%v)", code, err)
	}
}

func TestCreate_WorkspaceNotFoundMapsToNotFound(t *testing.T) {
	s := newTargetServerWithI(&stubTargetSvc{}, stubWSLookup{err: errors.New("boom")})
	_, err := call(t, s.createHandler(), &CreateTargetRequest{
		WorkspaceID: "ws-unknown", CallerUserID: "u",
	})
	if code := status.Code(err); code != codes.NotFound {
		t.Errorf("want NotFound, got %s", code)
	}
}

func TestCreate_ConflictMapsToAlreadyExists(t *testing.T) {
	svc := &stubTargetSvc{createErr: targetsvc.ErrConflict}
	s := newTargetServerWithI(svc, stubWSLookup{tenantID: "tenant-1"})
	_, err := call(t, s.createHandler(), &CreateTargetRequest{
		WorkspaceID: "ws-1", CallerUserID: "u", Name: "dup", Type: authzmod.TargetTypeWebhook,
	})
	if code := status.Code(err); code != codes.AlreadyExists {
		t.Errorf("want AlreadyExists, got %s", code)
	}
}

// ─── Get redaction ───────────────────────────────────────────────────────────

func TestGet_RedactsSecretFields(t *testing.T) {
	svc := &stubTargetSvc{
		getOut: &authzmod.DeliveryTarget{
			TargetId:    "t-2",
			WorkspaceId: "ws-1",
			TenantId:    "tenant-1",
			Name:        "line-1",
			Type:        authzmod.TargetTypeLine,
			Config: authzmod.TargetConfig{
				ChannelAccessToken: "SECRET_LINE_TOKEN_NEVER_SHIP_THIS",
				To:                 authzmod.StringSlice{"U1234567"},
			},
		},
	}
	s := newTargetServerWithI(svc, stubWSLookup{tenantID: "tenant-1"})
	out, err := call(t, s.getHandler(), &GetTargetRequest{WorkspaceID: "ws-1", TargetID: "t-2"})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	resp := out.(*GetTargetResponse)
	if !resp.Target.Config.ChannelAccessTokenSet {
		t.Error("ChannelAccessTokenSet must be true when token is present")
	}
	// The wire type has no raw token field — redaction is enforced by type.
	if len(resp.Target.Config.To) != 1 || resp.Target.Config.To[0] != "U1234567" {
		t.Errorf("recipients must pass through: got %v", resp.Target.Config.To)
	}
}

// ─── Delete blocked by usage ─────────────────────────────────────────────────

func TestDelete_TargetInUseReturnsFailedPreconditionWithTemplates(t *testing.T) {
	svc := &stubTargetSvc{deleteErr: &targetsvc.TargetInUseError{
		Templates: []ingestrepo.TemplateUsageRef{
			{TemplateId: "tmpl-1", Name: "Door Alert"},
			{TemplateId: "tmpl-2", Name: ""}, // name fallback to id
		},
	}}
	s := newTargetServerWithI(svc, stubWSLookup{tenantID: "tenant-1"})
	out, err := call(t, s.deleteHandler(), &DeleteTargetRequest{
		WorkspaceID: "ws-1", TargetID: "t-3", CallerUserID: "u",
	})
	if code := status.Code(err); code != codes.FailedPrecondition {
		t.Errorf("want FailedPrecondition, got %s", code)
	}
	resp, ok := out.(*DeleteTargetResponse)
	if !ok || resp == nil {
		t.Fatalf("response must carry TemplatesInUse; got %T %+v", out, out)
	}
	if len(resp.TemplatesInUse) != 2 {
		t.Fatalf("want 2 blocking templates, got %d", len(resp.TemplatesInUse))
	}
	if resp.TemplatesInUse[0] != "Door Alert" {
		t.Errorf("first entry uses Name: got %q", resp.TemplatesInUse[0])
	}
	if resp.TemplatesInUse[1] != "tmpl-2" {
		t.Errorf("missing name falls back to TemplateId: got %q", resp.TemplatesInUse[1])
	}
}

func TestDelete_NotFoundMapsToNotFound(t *testing.T) {
	svc := &stubTargetSvc{deleteErr: targetsvc.ErrNotFound}
	s := newTargetServerWithI(svc, stubWSLookup{tenantID: "tenant-1"})
	_, err := call(t, s.deleteHandler(), &DeleteTargetRequest{
		WorkspaceID: "ws-1", TargetID: "t-missing", CallerUserID: "u",
	})
	if code := status.Code(err); code != codes.NotFound {
		t.Errorf("want NotFound, got %s", code)
	}
}

// ─── List ────────────────────────────────────────────────────────────────────

func TestList_ReturnsRedactedItemsAndPagination(t *testing.T) {
	svc := &stubTargetSvc{
		listOut: []authzmod.DeliveryTarget{
			{TargetId: "t-a", WorkspaceId: "ws-1", TenantId: "tenant-1", Type: authzmod.TargetTypeTelegram,
				Config: authzmod.TargetConfig{BotToken: "SECRET", ChatId: "@phibek"}},
			{TargetId: "t-b", WorkspaceId: "ws-1", TenantId: "tenant-1", Type: authzmod.TargetTypeWebhook,
				Config: authzmod.TargetConfig{URL: "https://example.com", SigningSecret: "SIG"}},
		},
		listTotal: 2,
	}
	s := newTargetServerWithI(svc, stubWSLookup{tenantID: "tenant-1"})
	out, err := call(t, s.listHandler(), &ListTargetsRequest{WorkspaceID: "ws-1"})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	resp := out.(*ListTargetsResponse)
	if resp.TotalRecords != 2 || len(resp.Items) != 2 {
		t.Fatalf("want 2 items total 2; got items=%d total=%d", len(resp.Items), resp.TotalRecords)
	}
	if !resp.Items[0].Config.BotTokenSet {
		t.Error("BotTokenSet must be true for telegram target with token")
	}
	if !resp.Items[1].Config.SigningSecretSet {
		t.Error("SigningSecretSet must be true for webhook with signingSecret")
	}
}

func TestList_DefaultPaginationClampsPerPage(t *testing.T) {
	svc := &stubTargetSvc{listOut: nil, listTotal: 0}
	s := newTargetServerWithI(svc, stubWSLookup{tenantID: "tenant-1"})
	out, _ := call(t, s.listHandler(), &ListTargetsRequest{
		WorkspaceID: "ws-1", Page: 0, PerPage: 9999,
	})
	resp := out.(*ListTargetsResponse)
	if resp.Page != 1 || resp.PerPage != 20 {
		t.Errorf("want page=1 perPage=20 (default/clamped), got %d / %d", resp.Page, resp.PerPage)
	}
}

// ─── Update ──────────────────────────────────────────────────────────────────

func TestUpdate_ForwardsPartialFields(t *testing.T) {
	svc := &stubTargetSvc{
		updateOut: &authzmod.DeliveryTarget{TargetId: "t-x", WorkspaceId: "ws-1", TenantId: "tenant-1"},
	}
	s := newTargetServerWithI(svc, stubWSLookup{tenantID: "tenant-1"})
	name := "renamed"
	enabled := false
	_, err := call(t, s.updateHandler(), &UpdateTargetRequest{
		WorkspaceID: "ws-1", TargetID: "t-x", CallerUserID: "u", Name: &name, Enabled: &enabled,
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if svc.updateIn.Name == nil || *svc.updateIn.Name != "renamed" {
		t.Errorf("Name forwarded: got %v", svc.updateIn.Name)
	}
	if svc.updateIn.Enabled == nil || *svc.updateIn.Enabled != false {
		t.Errorf("Enabled forwarded: got %v", svc.updateIn.Enabled)
	}
	if !svc.updateIn.IsPlatformAdmin {
		t.Error("IsPlatformAdmin must be true")
	}
}
