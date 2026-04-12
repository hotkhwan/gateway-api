// internal/services/msgtmplsvc/service_test.go
package msgtmplsvc

import (
	"context"
	"errors"
	"testing"

	"github.com/hotkhwan/gateway-api/internal/repo/msgtmplrepo"
	"github.com/hotkhwan/gateway-api/models/gmod"
	"github.com/hotkhwan/gateway-api/models/ingestmod"
	"go.mongodb.org/mongo-driver/bson"
)

// ============================================================
// stubRepo — implements msgTemplateRepoI
// ============================================================

type stubRepo struct {
	existsResult bool
	existsErr    error

	insertErr error

	findResult *ingestmod.WorkspaceMessageTemplate
	findErr    error

	listResult []ingestmod.WorkspaceMessageTemplate
	listErr    error

	updateErr error
	deleteErr error
}

func (r *stubRepo) ExistsByName(_ context.Context, _, _, _ string) (bool, error) {
	return r.existsResult, r.existsErr
}
func (r *stubRepo) Insert(_ context.Context, t *ingestmod.WorkspaceMessageTemplate) error {
	if r.insertErr != nil {
		return r.insertErr
	}
	t.ID = "stub-msgtmpl-id"
	return nil
}
func (r *stubRepo) FindByID(_ context.Context, _, _ string) (*ingestmod.WorkspaceMessageTemplate, error) {
	return r.findResult, r.findErr
}
func (r *stubRepo) List(_ context.Context, _ string, _, _ int) ([]ingestmod.WorkspaceMessageTemplate, *gmod.Pagination, error) {
	return r.listResult, nil, r.listErr
}
func (r *stubRepo) Update(_ context.Context, _, _ string, _ bson.M) error { return r.updateErr }
func (r *stubRepo) Delete(_ context.Context, _, _ string) error           { return r.deleteErr }

// ============================================================
// Helpers
// ============================================================

func newSvc(repo msgTemplateRepoI) *MsgTemplateService {
	return &MsgTemplateService{repo: repo}
}

func sampleTemplate(wsId string) *ingestmod.WorkspaceMessageTemplate {
	return &ingestmod.WorkspaceMessageTemplate{
		ID:          "tmpl-1",
		WorkspaceID: wsId,
		Name:        "Alert Template",
		Channel:     "line",
		Body:        "Motion detected at {{.Zone}}",
		Locale:      "th",
	}
}

func strPtr(s string) *string { return &s }

// ============================================================
// Create — valid channels → success (table-driven)
// ============================================================

func TestCreate_ValidChannels(t *testing.T) {
	t.Parallel()

	channels := []string{"line", "webhook", "telegram", "discord"}

	for _, ch := range channels {
		ch := ch
		t.Run("channel="+ch, func(t *testing.T) {
			t.Parallel()

			repo := &stubRepo{existsResult: false}
			svc := newSvc(repo)

			got, err := svc.Create(context.Background(), CreateMsgTemplateInput{
				WorkspaceID: "ws1",
				Name:        "Template " + ch,
				Channel:     ch,
				Body:        "hello",
				Locale:      "en",
			})
			if err != nil {
				t.Fatalf("channel=%q: unexpected error: %v", ch, err)
			}
			if got == nil {
				t.Fatalf("channel=%q: expected non-nil result", ch)
			}
			if got.ID == "" {
				t.Errorf("channel=%q: expected non-empty ID", ch)
			}
			if got.Channel != ch {
				t.Errorf("channel=%q: got Channel=%q", ch, got.Channel)
			}
		})
	}
}

// ============================================================
// Create — empty channel is allowed (optional field)
// ============================================================

func TestCreate_EmptyChannel_Allowed(t *testing.T) {
	t.Parallel()

	repo := &stubRepo{existsResult: false}
	svc := newSvc(repo)

	got, err := svc.Create(context.Background(), CreateMsgTemplateInput{
		WorkspaceID: "ws1",
		Name:        "No Channel Template",
		Channel:     "", // optional
	})
	if err != nil {
		t.Fatalf("expected success with empty channel, got: %v", err)
	}
	if got.Channel != "" {
		t.Errorf("expected empty channel, got %q", got.Channel)
	}
}

// ============================================================
// Create — invalid channel → ErrBadRequest
// ============================================================

func TestCreate_InvalidChannel(t *testing.T) {
	t.Parallel()

	invalidChannels := []string{"sms", "email", "push", "slack", "WEBHOOK", "LINE"}

	for _, ch := range invalidChannels {
		ch := ch
		t.Run("channel="+ch, func(t *testing.T) {
			t.Parallel()

			svc := newSvc(&stubRepo{})
			_, err := svc.Create(context.Background(), CreateMsgTemplateInput{
				WorkspaceID: "ws1",
				Name:        "Bad Template",
				Channel:     ch,
			})
			if !errors.Is(err, ErrBadRequest) {
				t.Errorf("channel=%q: expected ErrBadRequest, got %v", ch, err)
			}
		})
	}
}

// ============================================================
// Create — channel trim: whitespace around valid value
// ============================================================

func TestCreate_ChannelTrimmed(t *testing.T) {
	t.Parallel()

	repo := &stubRepo{existsResult: false}
	svc := newSvc(repo)

	got, err := svc.Create(context.Background(), CreateMsgTemplateInput{
		WorkspaceID: "ws1",
		Name:        "Spaced",
		Channel:     "  line  ",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Channel != "line" {
		t.Errorf("expected channel to be trimmed to \"line\", got %q", got.Channel)
	}
}

// ============================================================
// Create — validation errors (table-driven)
// ============================================================

func TestCreate_Validation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input CreateMsgTemplateInput
	}{
		{"empty workspaceId", CreateMsgTemplateInput{Name: "x", Channel: "line"}},
		{"empty name", CreateMsgTemplateInput{WorkspaceID: "ws1", Channel: "line"}},
		{"whitespace-only name", CreateMsgTemplateInput{WorkspaceID: "ws1", Name: "   ", Channel: "line"}},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			svc := newSvc(&stubRepo{})
			_, err := svc.Create(context.Background(), tc.input)
			if !errors.Is(err, ErrBadRequest) {
				t.Errorf("expected ErrBadRequest, got %v", err)
			}
		})
	}
}

// ============================================================
// Create — duplicate name → ErrConflict
// ============================================================

func TestCreate_DuplicateName_ReturnsConflict(t *testing.T) {
	t.Parallel()

	repo := &stubRepo{existsResult: true}
	svc := newSvc(repo)

	_, err := svc.Create(context.Background(), CreateMsgTemplateInput{
		WorkspaceID: "ws1",
		Name:        "Taken Name",
		Channel:     "webhook",
	})
	if !errors.Is(err, ErrConflict) {
		t.Errorf("expected ErrConflict, got %v", err)
	}
}

// ============================================================
// Create — returns correct fields
// ============================================================

func TestCreate_ReturnsCorrectFields(t *testing.T) {
	t.Parallel()

	repo := &stubRepo{}
	svc := newSvc(repo)

	got, err := svc.Create(context.Background(), CreateMsgTemplateInput{
		WorkspaceID: "ws-xyz",
		Name:        "My Alert",
		Channel:     "telegram",
		Body:        "Alert: {{.EventType}}",
		Locale:      "en",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.WorkspaceID != "ws-xyz" {
		t.Errorf("WorkspaceID mismatch: got %q", got.WorkspaceID)
	}
	if got.Name != "My Alert" {
		t.Errorf("Name mismatch: got %q", got.Name)
	}
	if got.Body != "Alert: {{.EventType}}" {
		t.Errorf("Body mismatch: got %q", got.Body)
	}
	if got.Locale != "en" {
		t.Errorf("Locale mismatch: got %q", got.Locale)
	}
}

// ============================================================
// GetOne — happy path
// ============================================================

func TestGetOne_HappyPath(t *testing.T) {
	t.Parallel()

	tmpl := sampleTemplate("ws1")
	repo := &stubRepo{findResult: tmpl}
	svc := newSvc(repo)

	got, err := svc.GetOne(context.Background(), "ws1", "tmpl-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.ID != tmpl.ID {
		t.Errorf("ID mismatch: got %q, want %q", got.ID, tmpl.ID)
	}
}

// ============================================================
// GetOne — non-existent → ErrNotFound
// ============================================================

func TestGetOne_NotFound(t *testing.T) {
	t.Parallel()

	repo := &stubRepo{findErr: msgtmplrepo.ErrMsgTemplateNotFound}
	svc := newSvc(repo)

	_, err := svc.GetOne(context.Background(), "ws1", "ghost-id")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

// ============================================================
// GetOne — validation errors
// ============================================================

func TestGetOne_Validation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		workspaceId string
		id          string
	}{
		{"empty workspaceId", "", "tmpl-1"},
		{"empty id", "ws1", ""},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			svc := newSvc(&stubRepo{})
			_, err := svc.GetOne(context.Background(), tc.workspaceId, tc.id)
			if !errors.Is(err, ErrBadRequest) {
				t.Errorf("expected ErrBadRequest, got %v", err)
			}
		})
	}
}

// ============================================================
// Update — change channel to valid value → success
// ============================================================

func TestUpdate_ChangeChannel_Valid(t *testing.T) {
	t.Parallel()

	channels := []string{"line", "webhook", "telegram", "discord"}

	for _, ch := range channels {
		ch := ch
		t.Run("to="+ch, func(t *testing.T) {
			t.Parallel()

			existing := sampleTemplate("ws1")
			afterUpdate := *existing
			afterUpdate.Channel = ch

			calls := 0
			repo := &callCountRepo{
				onFind: func() (*ingestmod.WorkspaceMessageTemplate, error) {
					calls++
					if calls == 1 {
						return existing, nil
					}
					return &afterUpdate, nil
				},
			}
			svc := newSvc(repo)

			result, err := svc.Update(context.Background(), UpdateMsgTemplateInput{
				WorkspaceID: "ws1",
				ID:          "tmpl-1",
				Channel:     strPtr(ch),
			})
			if err != nil {
				t.Fatalf("channel=%q: unexpected error: %v", ch, err)
			}
			if result.Channel != ch {
				t.Errorf("channel=%q: result.Channel=%q", ch, result.Channel)
			}
		})
	}
}

// ============================================================
// Update — invalid channel → ErrBadRequest
// ============================================================

func TestUpdate_InvalidChannel_ReturnsBadRequest(t *testing.T) {
	t.Parallel()

	existing := sampleTemplate("ws1")
	repo := &stubRepo{findResult: existing}
	svc := newSvc(repo)

	_, err := svc.Update(context.Background(), UpdateMsgTemplateInput{
		WorkspaceID: "ws1",
		ID:          "tmpl-1",
		Channel:     strPtr("sms"),
	})
	if !errors.Is(err, ErrBadRequest) {
		t.Errorf("expected ErrBadRequest for invalid channel, got %v", err)
	}
}

// ============================================================
// Update — non-existent → ErrNotFound
// ============================================================

func TestUpdate_NotFound(t *testing.T) {
	t.Parallel()

	repo := &stubRepo{findErr: msgtmplrepo.ErrMsgTemplateNotFound}
	svc := newSvc(repo)

	_, err := svc.Update(context.Background(), UpdateMsgTemplateInput{
		WorkspaceID: "ws1",
		ID:          "ghost",
		Body:        strPtr("new body"),
	})
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

// ============================================================
// Update — duplicate name → ErrConflict
// ============================================================

func TestUpdate_DuplicateName_ReturnsConflict(t *testing.T) {
	t.Parallel()

	existing := sampleTemplate("ws1")
	repo := &stubWithNameCheck{
		findResult: existing,
		nameExists: true,
	}
	svc := newSvc(repo)

	_, err := svc.Update(context.Background(), UpdateMsgTemplateInput{
		WorkspaceID: "ws1",
		ID:          "tmpl-1",
		Name:        strPtr("Taken Name"),
	})
	if !errors.Is(err, ErrConflict) {
		t.Errorf("expected ErrConflict, got %v", err)
	}
}

// ============================================================
// Update — empty fields → ErrBadRequest
// ============================================================

func TestUpdate_EmptyFields_ReturnsBadRequest(t *testing.T) {
	t.Parallel()

	existing := sampleTemplate("ws1")
	repo := &stubRepo{findResult: existing}
	svc := newSvc(repo)

	_, err := svc.Update(context.Background(), UpdateMsgTemplateInput{
		WorkspaceID: "ws1",
		ID:          "tmpl-1",
		// no fields set
	})
	if !errors.Is(err, ErrBadRequest) {
		t.Errorf("expected ErrBadRequest for empty update, got %v", err)
	}
}

// ============================================================
// Update — validation errors
// ============================================================

func TestUpdate_Validation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input UpdateMsgTemplateInput
	}{
		{"empty workspaceId", UpdateMsgTemplateInput{ID: "x", Body: strPtr("b")}},
		{"empty id", UpdateMsgTemplateInput{WorkspaceID: "ws1", Body: strPtr("b")}},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			svc := newSvc(&stubRepo{})
			_, err := svc.Update(context.Background(), tc.input)
			if !errors.Is(err, ErrBadRequest) {
				t.Errorf("expected ErrBadRequest, got %v", err)
			}
		})
	}
}

// ============================================================
// Delete — happy path
// ============================================================

func TestDelete_HappyPath(t *testing.T) {
	t.Parallel()

	existing := sampleTemplate("ws1")
	repo := &stubRepo{findResult: existing}
	svc := newSvc(repo)

	if err := svc.Delete(context.Background(), "ws1", "tmpl-1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// ============================================================
// Delete — non-existent → ErrNotFound
// ============================================================

func TestDelete_NotFound(t *testing.T) {
	t.Parallel()

	repo := &stubRepo{findErr: msgtmplrepo.ErrMsgTemplateNotFound}
	svc := newSvc(repo)

	err := svc.Delete(context.Background(), "ws1", "ghost")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

// ============================================================
// Delete — validation errors
// ============================================================

func TestDelete_Validation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		workspaceId string
		id          string
	}{
		{"empty workspaceId", "", "tmpl-1"},
		{"empty id", "ws1", ""},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			svc := newSvc(&stubRepo{})
			err := svc.Delete(context.Background(), tc.workspaceId, tc.id)
			if !errors.Is(err, ErrBadRequest) {
				t.Errorf("expected ErrBadRequest, got %v", err)
			}
		})
	}
}

// ============================================================
// List — empty workspaceId → ErrBadRequest
// ============================================================

func TestList_EmptyWorkspace_ReturnsBadRequest(t *testing.T) {
	t.Parallel()

	svc := newSvc(&stubRepo{})
	_, _, err := svc.List(context.Background(), "", 1, 20)
	if !errors.Is(err, ErrBadRequest) {
		t.Errorf("expected ErrBadRequest, got %v", err)
	}
}

// ============================================================
// List — happy path
// ============================================================

func TestList_HappyPath(t *testing.T) {
	t.Parallel()

	items := []ingestmod.WorkspaceMessageTemplate{*sampleTemplate("ws1")}
	repo := &stubRepo{listResult: items}
	svc := newSvc(repo)

	got, _, err := svc.List(context.Background(), "ws1", 1, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Errorf("expected 1 item, got %d", len(got))
	}
}

// ============================================================
// validChannels map — contract test (exhaustive)
// ============================================================

func TestValidChannels_Contract(t *testing.T) {
	t.Parallel()

	must := []string{"line", "webhook", "telegram", "discord"}
	for _, ch := range must {
		if !validChannels[ch] {
			t.Errorf("validChannels must include %q", ch)
		}
	}
}

// ============================================================
// Additional stubs for nuanced scenarios
// ============================================================

// callCountRepo controls per-call FindByID behaviour.
type callCountRepo struct {
	onFind func() (*ingestmod.WorkspaceMessageTemplate, error)
}

func (r *callCountRepo) ExistsByName(_ context.Context, _, _, _ string) (bool, error) {
	return false, nil
}
func (r *callCountRepo) Insert(_ context.Context, t *ingestmod.WorkspaceMessageTemplate) error {
	t.ID = "stub-id"
	return nil
}
func (r *callCountRepo) FindByID(_ context.Context, _, _ string) (*ingestmod.WorkspaceMessageTemplate, error) {
	return r.onFind()
}
func (r *callCountRepo) List(_ context.Context, _ string, _, _ int) ([]ingestmod.WorkspaceMessageTemplate, *gmod.Pagination, error) {
	return nil, nil, nil
}
func (r *callCountRepo) Update(_ context.Context, _, _ string, _ bson.M) error { return nil }
func (r *callCountRepo) Delete(_ context.Context, _, _ string) error           { return nil }

// stubWithNameCheck controls FindByID and ExistsByName independently.
type stubWithNameCheck struct {
	findResult *ingestmod.WorkspaceMessageTemplate
	findErr    error
	nameExists bool
	nameErr    error
}

func (r *stubWithNameCheck) ExistsByName(_ context.Context, _, _, _ string) (bool, error) {
	return r.nameExists, r.nameErr
}
func (r *stubWithNameCheck) Insert(_ context.Context, t *ingestmod.WorkspaceMessageTemplate) error {
	t.ID = "stub-id"
	return nil
}
func (r *stubWithNameCheck) FindByID(_ context.Context, _, _ string) (*ingestmod.WorkspaceMessageTemplate, error) {
	return r.findResult, r.findErr
}
func (r *stubWithNameCheck) List(_ context.Context, _ string, _, _ int) ([]ingestmod.WorkspaceMessageTemplate, *gmod.Pagination, error) {
	return nil, nil, nil
}
func (r *stubWithNameCheck) Update(_ context.Context, _, _ string, _ bson.M) error { return nil }
func (r *stubWithNameCheck) Delete(_ context.Context, _, _ string) error           { return nil }
