package adminapi

import (
	"context"
	"encoding/json"
	"io"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/hotkhwan/gateway-api/internal/services/licensesvc"
	"github.com/hotkhwan/gateway-api/models/subscripmod"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type mockLicenseSvc struct {
	issueCalls int
	issued     *subscripmod.LicenseKey
	issueErr   error

	listResult []*subscripmod.LicenseKey
	listErr    error

	getResult *subscripmod.LicenseKey
	getErr    error

	revokeErr error
}

func (m *mockLicenseSvc) Issue(_ context.Context, _ licensesvc.IssueOptions) (*subscripmod.LicenseKey, error) {
	m.issueCalls++
	return m.issued, m.issueErr
}
func (m *mockLicenseSvc) List(_ context.Context) ([]*subscripmod.LicenseKey, error) {
	return m.listResult, m.listErr
}
func (m *mockLicenseSvc) Get(_ context.Context, _ primitive.ObjectID) (*subscripmod.LicenseKey, error) {
	return m.getResult, m.getErr
}
func (m *mockLicenseSvc) Revoke(_ context.Context, _ primitive.ObjectID) error {
	return m.revokeErr
}

func newIssueApp(svc licenseAdminSvc) *fiber.App {
	app := fiber.New()
	ctrl := &LicenseAdminController{svc: svc}
	app.Post("/admin/licenses", ctrl.Issue)
	return app
}

func parseJSON(t *testing.T, body io.ReadCloser) map[string]any {
	t.Helper()
	defer body.Close()
	raw, _ := io.ReadAll(body)
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("decode response: %v (raw=%q)", err, raw)
	}
	return m
}

func TestIssue_MalformedBodyReturns400_DoesNotMint(t *testing.T) {
	svc := &mockLicenseSvc{
		issued: &subscripmod.LicenseKey{
			ID:        primitive.NewObjectID(),
			Key:       "AAAA-BBBB-CCCC-DDDD",
			PlanId:    "enterprise",
			Status:    subscripmod.LicenseStatusAvailable,
			CreatedAt: time.Now(),
		},
	}
	app := newIssueApp(svc)

	req := httptest.NewRequest("POST", "/admin/licenses", strings.NewReader(`{not json`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	if resp.StatusCode != 400 {
		t.Fatalf("status: got %d want 400", resp.StatusCode)
	}
	if svc.issueCalls != 0 {
		t.Fatalf("Issue should not be called on malformed body, got %d calls", svc.issueCalls)
	}
	body := parseJSON(t, resp.Body)
	if body["status"] != false {
		t.Fatalf("response.status: got %v want false", body["status"])
	}
}

func TestIssue_EmptyBodyUsesDefaultsAndMints(t *testing.T) {
	svc := &mockLicenseSvc{
		issued: &subscripmod.LicenseKey{
			ID:        primitive.NewObjectID(),
			Key:       "AAAA-BBBB-CCCC-DDDD",
			PlanId:    "enterprise",
			Status:    subscripmod.LicenseStatusAvailable,
			CreatedAt: time.Now(),
		},
	}
	app := newIssueApp(svc)

	req := httptest.NewRequest("POST", "/admin/licenses", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("status: got %d want 200", resp.StatusCode)
	}
	if svc.issueCalls != 1 {
		t.Fatalf("Issue calls: got %d want 1", svc.issueCalls)
	}
}

func TestIssue_WellFormedBodyMints(t *testing.T) {
	svc := &mockLicenseSvc{
		issued: &subscripmod.LicenseKey{
			ID:        primitive.NewObjectID(),
			Key:       "AAAA-BBBB-CCCC-DDDD",
			PlanId:    "enterprise",
			Status:    subscripmod.LicenseStatusAvailable,
			CreatedAt: time.Now(),
		},
	}
	app := newIssueApp(svc)

	req := httptest.NewRequest("POST", "/admin/licenses", strings.NewReader(`{"notes":"customer X"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("status: got %d want 200", resp.StatusCode)
	}
	if svc.issueCalls != 1 {
		t.Fatalf("Issue calls: got %d want 1", svc.issueCalls)
	}
}

func TestIssue_MissingSecretReturns500(t *testing.T) {
	svc := &mockLicenseSvc{issueErr: licensesvc.ErrSecretRequired}
	app := newIssueApp(svc)

	req := httptest.NewRequest("POST", "/admin/licenses", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	if resp.StatusCode != 500 {
		t.Fatalf("status: got %d want 500", resp.StatusCode)
	}
}
