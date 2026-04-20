// internal/services/targetsvc/targetSvc_test.go
package targetsvc

import (
	"context"
	"errors"
	"testing"

	"github.com/hotkhwan/gateway-api/internal/gateways/authzgw"
	"github.com/hotkhwan/gateway-api/models/authzmod"
	"go.mongodb.org/mongo-driver/bson"
)

// ============================================================
// Stubs
// ============================================================

// stubRepo is a zero-value stub that satisfies TargetRepoI.
// Override individual fields to control behaviour per test.
type stubRepo struct {
	existsByName bool
	existsErr    error
	insertErr    error
	findResult   *authzmod.DeliveryTarget
	findErr      error
}

func (r *stubRepo) ExistsByNameInOrg(_ context.Context, _, _, _, _ string) (bool, error) {
	return r.existsByName, r.existsErr
}
func (r *stubRepo) Insert(_ context.Context, t *authzmod.DeliveryTarget) error {
	if r.insertErr != nil {
		return r.insertErr
	}
	t.TargetId = "stub-id"
	return nil
}
func (r *stubRepo) FindByIDAndOrg(_ context.Context, _, _, _ string) (*authzmod.DeliveryTarget, error) {
	return r.findResult, r.findErr
}
func (r *stubRepo) List(_ context.Context, _, _, _ string, _, _ int, _, _ string) ([]authzmod.DeliveryTarget, int64, error) {
	return nil, 0, nil
}
func (r *stubRepo) Update(_ context.Context, _, _, _ string, _ bson.M) error { return nil }
func (r *stubRepo) Delete(_ context.Context, _, _, _ string) error           { return nil }
func (r *stubRepo) CountByTypeAndOrg(_ context.Context, _, _, _ string) (int64, error) {
	return 0, nil
}
func (r *stubRepo) CountMessageChannelsByOrg(_ context.Context, _, _ string) (int64, error) {
	return 0, nil
}

// stubAuthz satisfies authzgw.Client.
type stubAuthz struct {
	allowed bool
	err     error
}

func (a *stubAuthz) CheckPermissionWithSchemaVersion(_ context.Context, _, _, _, _, _, _, _ string) (bool, error) {
	return a.allowed, a.err
}
func (a *stubAuthz) WriteTuples(_ context.Context, _ string, _ []map[string]interface{}) error {
	return nil
}
func (a *stubAuthz) DeleteOrgRelationships(_ context.Context, _, _ string) error  { return nil }
func (a *stubAuthz) DeleteEntityRelationships(_ context.Context, _, _, _ string) error {
	return nil
}
func (a *stubAuthz) DeleteOrgTuples(_ context.Context, _, _ string) error { return nil }
func (a *stubAuthz) LookupOrganizations(_ context.Context, _, _ string) ([]string, error) {
	return nil, nil
}
func (a *stubAuthz) LookupWorkspaces(_ context.Context, _, _ string) ([]string, error) {
	return nil, nil
}
func (a *stubAuthz) ListEntityRelationships(_ context.Context, _, _, _ string) ([]authzgw.Relationship, error) {
	return nil, nil
}
func (a *stubAuthz) ListRelationshipsBySubject(_ context.Context, _, _, _ string) ([]authzgw.Relationship, error) {
	return nil, nil
}
func (a *stubAuthz) DeleteSpecificTupleWithRelation(_ context.Context, _, _, _, _, _, _ string) error {
	return nil
}
func (a *stubAuthz) DebugReadTuples(_ context.Context, _ string, _ authzgw.ReadTuplesRequest) (*authzgw.ReadTuplesResponse, error) {
	return nil, nil
}
func (a *stubAuthz) DebugDeleteTuples(_ context.Context, _ string, _ authzgw.DeleteTuplesRequest) error {
	return nil
}

// ============================================================
// Helpers
// ============================================================

func newSvc(repo TargetRepoI, authz authzgw.Client) *TargetService {
	return &TargetService{repo: repo, authzClient: authz, subSvc: nil}
}

func baseInput() CreateTargetInput {
	return CreateTargetInput{
		TenantId:    "t1",
		WorkspaceId: "o1",
		UserId:      "u1",
		Name:        "My Target",
		Type:        authzmod.TargetTypeWebhook,
		Mode:        "klynx",
		Enabled:     true,
		Config:      authzmod.TargetConfig{},
	}
}

// ============================================================
// Tests
// ============================================================

func TestCreate_KlynxMode_Validation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		mutate      func(*CreateTargetInput)
		envProfile  string
		wantErr     error
		authzAllows bool
	}{
		{
			name: "klynx with url → ErrKlynxModeWithURL",
			mutate: func(in *CreateTargetInput) {
				in.Config.URL = "https://example.com/hook"
			},
			wantErr: ErrKlynxModeWithURL,
		},
		{
			name: "klynx with signingSecret → ErrKlynxModeWithHMAC",
			mutate: func(in *CreateTargetInput) {
				in.Config.SigningSecret = "s3cr3t"
			},
			wantErr: ErrKlynxModeWithHMAC,
		},
		{
			name: "klynx with signingEnabled → ErrKlynxModeWithHMAC",
			mutate: func(in *CreateTargetInput) {
				in.Config.SigningEnabled = true
			},
			wantErr: ErrKlynxModeWithHMAC,
		},
		{
			name:        "klynx without url/hmac, appliance → success",
			mutate:      func(_ *CreateTargetInput) {},
			authzAllows: true,
			wantErr:     nil,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			repo := &stubRepo{existsByName: false}
			authz := &stubAuthz{allowed: tc.authzAllows}
			svc := newSvc(repo, authz)

			in := baseInput()
			tc.mutate(&in)

			_, err := svc.Create(context.Background(), in)

			if tc.wantErr == nil {
				if err != nil {
					t.Errorf("expected no error, got: %v", err)
				}
				return
			}
			if !errors.Is(err, tc.wantErr) {
				t.Errorf("expected %v, got %v", tc.wantErr, err)
			}
		})
	}
}

// TestCreate_KlynxMode_SaasPublicGuard is a sequential test because t.Setenv
// is incompatible with t.Parallel().
func TestCreate_KlynxMode_SaasPublicGuard(t *testing.T) {
	t.Setenv("DEPLOYMENT_PROFILE", "saasPublic")

	svc := newSvc(&stubRepo{}, &stubAuthz{})
	in := baseInput() // mode=klynx, no url/hmac

	_, err := svc.Create(context.Background(), in)
	if !errors.Is(err, ErrKlynxModeInSaasPublic) {
		t.Errorf("expected ErrKlynxModeInSaasPublic, got %v", err)
	}
}

func TestCreate_DuplicateName_ReturnsConflict(t *testing.T) {
	t.Parallel()

	repo := &stubRepo{existsByName: true} // duplicate exists
	authz := &stubAuthz{allowed: true}
	svc := newSvc(repo, authz)

	in := baseInput()
	in.Mode = "" // regular target so we don't hit klynx guards

	_, err := svc.Create(context.Background(), in)
	if !errors.Is(err, ErrConflict) {
		t.Errorf("expected ErrConflict, got %v", err)
	}
}

func TestCreate_KlynxMode_DuplicateName_ReturnsConflict(t *testing.T) {
	t.Parallel()

	repo := &stubRepo{existsByName: true}
	authz := &stubAuthz{allowed: true}
	svc := newSvc(repo, authz)

	in := baseInput() // mode=klynx, no url/hmac

	_, err := svc.Create(context.Background(), in)
	if !errors.Is(err, ErrConflict) {
		t.Errorf("expected ErrConflict, got %v", err)
	}
}

func TestCreate_MissingRequiredFields_ReturnsBadRequest(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		input CreateTargetInput
	}{
		{"empty name", CreateTargetInput{TenantId: "t1", WorkspaceId: "o1", UserId: "u1", Type: "webhook"}},
		{"empty workspaceId", CreateTargetInput{TenantId: "t1", Name: "x", UserId: "u1", Type: "webhook"}},
		{"empty tenantId", CreateTargetInput{WorkspaceId: "o1", Name: "x", UserId: "u1", Type: "webhook"}},
		{"invalid type", CreateTargetInput{TenantId: "t1", WorkspaceId: "o1", UserId: "u1", Name: "x", Type: "fax"}},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			svc := newSvc(&stubRepo{}, &stubAuthz{})
			_, err := svc.Create(context.Background(), tc.input)
			if !errors.Is(err, ErrBadRequest) {
				t.Errorf("expected ErrBadRequest, got %v", err)
			}
		})
	}
}

func TestCreate_AuthzDenied_ReturnsForbidden(t *testing.T) {
	t.Parallel()

	repo := &stubRepo{existsByName: false}
	authz := &stubAuthz{allowed: false}
	svc := newSvc(repo, authz)

	in := baseInput()
	in.Mode = "" // skip klynx guards

	_, err := svc.Create(context.Background(), in)
	if !errors.Is(err, ErrForbidden) {
		t.Errorf("expected ErrForbidden, got %v", err)
	}
}
