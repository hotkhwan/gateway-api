// internal/services/ingesttmplsvc/service_test.go
package ingesttmplsvc

import (
	"context"
	"errors"
	"testing"

	"github.com/hotkhwan/gateway-api/internal/repo/ingesttmplrepo"
	"github.com/hotkhwan/gateway-api/models/gmod"
	"github.com/hotkhwan/gateway-api/models/ingestmod"
	"go.mongodb.org/mongo-driver/bson"
)

// ============================================================
// stubRepo — implements ingestTemplateRepoI
// ============================================================

type stubRepo struct {
	existsResult bool
	existsErr    error

	insertErr error

	findResult *ingestmod.IngestTemplate
	findErr    error

	listResult []ingestmod.IngestTemplate
	listErr    error

	updateErr error
	deleteErr error

	// call capture
	insertedID  string
	updatedFields bson.M
}

func (r *stubRepo) ExistsByName(_ context.Context, _, _, _ string) (bool, error) {
	return r.existsResult, r.existsErr
}

func (r *stubRepo) Insert(_ context.Context, t *ingestmod.IngestTemplate) error {
	if r.insertErr != nil {
		return r.insertErr
	}
	t.ID = "stub-tmpl-id"
	r.insertedID = t.ID
	return nil
}

func (r *stubRepo) FindByID(_ context.Context, _, _ string) (*ingestmod.IngestTemplate, error) {
	return r.findResult, r.findErr
}

func (r *stubRepo) List(_ context.Context, _ string, _, _ int) ([]ingestmod.IngestTemplate, *gmod.Pagination, error) {
	return r.listResult, nil, r.listErr
}

func (r *stubRepo) Update(_ context.Context, _, _ string, fields bson.M) error {
	r.updatedFields = fields
	return r.updateErr
}

func (r *stubRepo) Delete(_ context.Context, _, _ string) error {
	return r.deleteErr
}

// ============================================================
// Helpers
// ============================================================

func newSvc(repo ingestTemplateRepoI) *IngestTemplateService {
	return &IngestTemplateService{repo: repo}
}

func sampleTemplate(wsId string) *ingestmod.IngestTemplate {
	return &ingestmod.IngestTemplate{
		ID:           "tmpl-1",
		WorkspaceID:  wsId,
		Name:         "Camera Motion",
		SourceFamily: "dahua",
		Enabled:      true,
	}
}

func strPtr(s string) *string  { return &s }
func boolPtr(b bool) *bool     { return &b }

// ============================================================
// Create — happy path returns document with id
// ============================================================

func TestCreate_HappyPath_ReturnsDocumentWithID(t *testing.T) {
	t.Parallel()

	repo := &stubRepo{existsResult: false}
	svc := newSvc(repo)

	got, err := svc.Create(context.Background(), CreateIngestTemplateInput{
		WorkspaceID:  "ws1",
		Name:         "My Template",
		SourceFamily: "dahua",
		Enabled:      true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil result")
	}
	if got.ID == "" {
		t.Error("expected non-empty ID on created template")
	}
	if got.Name != "My Template" {
		t.Errorf("name mismatch: got %q", got.Name)
	}
	if got.SourceFamily != "dahua" {
		t.Errorf("sourceFamily mismatch: got %q", got.SourceFamily)
	}
	if got.WorkspaceID != "ws1" {
		t.Errorf("workspaceId mismatch: got %q", got.WorkspaceID)
	}
	if !got.Enabled {
		t.Error("expected Enabled=true")
	}
}

// ============================================================
// Create — Name trims whitespace
// ============================================================

func TestCreate_TrimsWhitespace(t *testing.T) {
	t.Parallel()

	repo := &stubRepo{}
	svc := newSvc(repo)

	got, err := svc.Create(context.Background(), CreateIngestTemplateInput{
		WorkspaceID:  "ws1",
		Name:         "  trimmed  ",
		SourceFamily: "hikvision",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Name != "trimmed" {
		t.Errorf("expected Name to be trimmed, got %q", got.Name)
	}
}

// ============================================================
// Create — validation errors (table-driven)
// ============================================================

func TestCreate_Validation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input CreateIngestTemplateInput
	}{
		{"empty workspaceId", CreateIngestTemplateInput{Name: "x", SourceFamily: "y"}},
		{"empty name", CreateIngestTemplateInput{WorkspaceID: "ws1", SourceFamily: "y"}},
		{"whitespace-only name", CreateIngestTemplateInput{WorkspaceID: "ws1", Name: "   ", SourceFamily: "y"}},
		{"empty sourceFamily", CreateIngestTemplateInput{WorkspaceID: "ws1", Name: "x"}},
		{"whitespace sourceFamily", CreateIngestTemplateInput{WorkspaceID: "ws1", Name: "x", SourceFamily: "  "}},
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

	_, err := svc.Create(context.Background(), CreateIngestTemplateInput{
		WorkspaceID:  "ws1",
		Name:         "Existing Template",
		SourceFamily: "dahua",
	})
	if !errors.Is(err, ErrConflict) {
		t.Errorf("expected ErrConflict, got %v", err)
	}
}

// ============================================================
// Create — sourceFamily uniqueness: no per-sourceFamily uniqueness enforced
// (service only checks name uniqueness per workspace)
// ============================================================

func TestCreate_SameSourceFamily_DifferentName_Allowed(t *testing.T) {
	t.Parallel()

	// Two templates with same sourceFamily but different names — both allowed.
	repo := &stubRepo{existsResult: false} // name not taken
	svc := newSvc(repo)

	_, err := svc.Create(context.Background(), CreateIngestTemplateInput{
		WorkspaceID:  "ws1",
		Name:         "Template B",
		SourceFamily: "dahua", // same family as an imagined Template A
	})
	if err != nil {
		t.Errorf("expected no error for different name, same sourceFamily; got %v", err)
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

	repo := &stubRepo{findErr: ingesttmplrepo.ErrTemplateNotFound}
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
// Update — disabled template → success (Enabled=false allowed)
// ============================================================

func TestUpdate_DisabledTemplate_Success(t *testing.T) {
	t.Parallel()

	existing := sampleTemplate("ws1")
	existing.Enabled = false

	updated := *existing
	updated.Enabled = false // stays disabled

	// FindByID is called twice: existence check + return after update
	callCount := 0
	repo := &callCountRepo{
		onFind: func() (*ingestmod.IngestTemplate, error) {
			callCount++
			return &updated, nil
		},
	}
	svc := newSvc(repo)

	result, err := svc.Update(context.Background(), UpdateIngestTemplateInput{
		WorkspaceID: "ws1",
		ID:          "tmpl-1",
		Enabled:     boolPtr(false),
	})
	if err != nil {
		t.Fatalf("unexpected error updating disabled template: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Enabled {
		t.Error("expected Enabled=false on returned document")
	}
}

// ============================================================
// Update — enable a disabled template → success
// ============================================================

func TestUpdate_EnableDisabledTemplate(t *testing.T) {
	t.Parallel()

	existing := sampleTemplate("ws1")
	existing.Enabled = false

	afterUpdate := *existing
	afterUpdate.Enabled = true

	calls := 0
	repo := &callCountRepo{
		onFind: func() (*ingestmod.IngestTemplate, error) {
			calls++
			if calls == 1 {
				return existing, nil // existence check
			}
			return &afterUpdate, nil // return after update
		},
	}
	svc := newSvc(repo)

	result, err := svc.Update(context.Background(), UpdateIngestTemplateInput{
		WorkspaceID: "ws1",
		ID:          "tmpl-1",
		Enabled:     boolPtr(true),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Enabled {
		t.Error("expected Enabled=true after update")
	}
}

// ============================================================
// Update — non-existent → ErrNotFound
// ============================================================

func TestUpdate_NotFound(t *testing.T) {
	t.Parallel()

	repo := &stubRepo{findErr: ingesttmplrepo.ErrTemplateNotFound}
	svc := newSvc(repo)

	_, err := svc.Update(context.Background(), UpdateIngestTemplateInput{
		WorkspaceID: "ws1",
		ID:          "ghost",
		Enabled:     boolPtr(true),
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
		findResult:   existing,
		nameExists:   true,
	}
	svc := newSvc(repo)

	_, err := svc.Update(context.Background(), UpdateIngestTemplateInput{
		WorkspaceID: "ws1",
		ID:          "tmpl-1",
		Name:        strPtr("Taken Name"),
	})
	if !errors.Is(err, ErrConflict) {
		t.Errorf("expected ErrConflict, got %v", err)
	}
}

// ============================================================
// Update — empty fields (nothing to update) → ErrBadRequest
// ============================================================

func TestUpdate_EmptyFields_ReturnsBadRequest(t *testing.T) {
	t.Parallel()

	existing := sampleTemplate("ws1")
	repo := &stubRepo{findResult: existing}
	svc := newSvc(repo)

	_, err := svc.Update(context.Background(), UpdateIngestTemplateInput{
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
		input UpdateIngestTemplateInput
	}{
		{"empty workspaceId", UpdateIngestTemplateInput{ID: "x", Enabled: boolPtr(true)}},
		{"empty id", UpdateIngestTemplateInput{WorkspaceID: "ws1", Enabled: boolPtr(true)}},
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

	repo := &stubRepo{findErr: ingesttmplrepo.ErrTemplateNotFound}
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
// List — happy path propagates repo results
// ============================================================

func TestList_HappyPath(t *testing.T) {
	t.Parallel()

	items := []ingestmod.IngestTemplate{
		*sampleTemplate("ws1"),
	}
	repo := &stubRepo{listResult: items}
	svc := newSvc(repo)

	got, pag, err := svc.List(context.Background(), "ws1", 1, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Errorf("expected 1 item, got %d", len(got))
	}
	_ = pag // pagination comes from repo stub (nil is fine here)
}

// ============================================================
// Additional stubs for nuanced test scenarios
// ============================================================

// callCountRepo lets individual tests control per-call behaviour.
type callCountRepo struct {
	onFind func() (*ingestmod.IngestTemplate, error)
}

func (r *callCountRepo) ExistsByName(_ context.Context, _, _, _ string) (bool, error) {
	return false, nil
}
func (r *callCountRepo) Insert(_ context.Context, t *ingestmod.IngestTemplate) error {
	t.ID = "stub-id"
	return nil
}
func (r *callCountRepo) FindByID(_ context.Context, _, _ string) (*ingestmod.IngestTemplate, error) {
	return r.onFind()
}
func (r *callCountRepo) List(_ context.Context, _ string, _, _ int) ([]ingestmod.IngestTemplate, *gmod.Pagination, error) {
	return nil, nil, nil
}
func (r *callCountRepo) Update(_ context.Context, _, _ string, _ bson.M) error { return nil }
func (r *callCountRepo) Delete(_ context.Context, _, _ string) error           { return nil }

// stubWithNameCheck controls both FindByID and ExistsByName independently.
type stubWithNameCheck struct {
	findResult *ingestmod.IngestTemplate
	findErr    error
	nameExists bool
	nameErr    error
}

func (r *stubWithNameCheck) ExistsByName(_ context.Context, _, _, _ string) (bool, error) {
	return r.nameExists, r.nameErr
}
func (r *stubWithNameCheck) Insert(_ context.Context, t *ingestmod.IngestTemplate) error {
	t.ID = "stub-id"
	return nil
}
func (r *stubWithNameCheck) FindByID(_ context.Context, _, _ string) (*ingestmod.IngestTemplate, error) {
	return r.findResult, r.findErr
}
func (r *stubWithNameCheck) List(_ context.Context, _ string, _, _ int) ([]ingestmod.IngestTemplate, *gmod.Pagination, error) {
	return nil, nil, nil
}
func (r *stubWithNameCheck) Update(_ context.Context, _, _ string, _ bson.M) error { return nil }
func (r *stubWithNameCheck) Delete(_ context.Context, _, _ string) error           { return nil }
