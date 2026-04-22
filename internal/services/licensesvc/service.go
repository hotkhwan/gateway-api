// internal/services/licensesvc/service.go
package licensesvc

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/hotkhwan/gateway-api/internal/repo/subscriprepo"
	"github.com/hotkhwan/gateway-api/models/subscripmod"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

var (
	ErrInvalidKey      = errors.New("invalid license key")
	ErrAlreadyAssigned = errors.New("license already assigned")
)

// Service provides HTTP-callable license admin operations backed by the
// existing license_keys collection. The CLI (cmd/license.go) is now a thin
// wrapper over this service.
type Service struct {
	repo *subscriprepo.LicenseRepo
}

// New builds a license service over the existing LicenseRepo.
func New(repo *subscriprepo.LicenseRepo) *Service {
	if repo == nil {
		panic("licensesvc: repo is required")
	}
	return &Service{repo: repo}
}

// Secret returns the HMAC secret read from LIC_SEC_KEY at call time so
// rotation takes effect without restarting the service. Returns empty when
// unset — callers decide how to report that.
func Secret() string {
	return os.Getenv("LIC_SEC_KEY")
}

// IssueOptions carries the optional payload for creating a license record.
type IssueOptions struct {
	PlanID string
	Notes  *string
	Limits *subscripmod.SubscriptionLimits
}

// Issue generates a new license key, persists it as an available record, and
// returns the full document. planId defaults to "enterprise" when not set.
func (s *Service) Issue(ctx context.Context, opts IssueOptions) (*subscripmod.LicenseKey, error) {
	secret := Secret()
	if secret == "" {
		return nil, ErrSecretRequired
	}

	key, err := Generate(secret)
	if err != nil {
		return nil, err
	}

	planID := opts.PlanID
	if planID == "" {
		planID = "enterprise"
	}

	license := &subscripmod.LicenseKey{
		ID:        primitive.NewObjectID(),
		Key:       key,
		PlanId:    planID,
		Limits:    opts.Limits,
		Status:    subscripmod.LicenseStatusAvailable,
		CreatedAt: time.Now().UTC(),
		Notes:     opts.Notes,
	}
	if err := s.repo.Create(ctx, license); err != nil {
		return nil, fmt.Errorf("license persist: %w", err)
	}
	return license, nil
}

// List returns every license record. Filter stays wide on purpose — paging
// and status filters can be added when the admin UI needs them.
func (s *Service) List(ctx context.Context) ([]*subscripmod.LicenseKey, error) {
	return s.repo.List(ctx, bson.M{})
}

// Get returns one license by id.
func (s *Service) Get(ctx context.Context, id primitive.ObjectID) (*subscripmod.LicenseKey, error) {
	return s.repo.FindById(ctx, id)
}

// Revoke marks a license as revoked. Idempotent: calling on an already revoked
// license still returns without error.
func (s *Service) Revoke(ctx context.Context, id primitive.ObjectID) error {
	return s.repo.Revoke(ctx, id)
}

// Validate checks that key is well-formed AND exists in the repo with an
// activatable status.
func (s *Service) Validate(ctx context.Context, key string) (*subscripmod.LicenseKey, error) {
	if !ValidateFormat(key) {
		return nil, ErrInvalidKey
	}
	lic, err := s.repo.ValidateLicenseKey(ctx, key)
	if err != nil {
		return nil, err
	}
	return lic, nil
}

// Activate binds a license to a tenant and records the activation timestamp.
// Returns the activated license and the hashed form for callers that need to
// upsert downstream subscription state.
func (s *Service) Activate(ctx context.Context, key, tenantId string) (*subscripmod.LicenseKey, string, error) {
	if tenantId == "" {
		return nil, "", errors.New("tenantId is required")
	}
	lic, err := s.Validate(ctx, key)
	if err != nil {
		return nil, "", err
	}

	now := time.Now().UTC()
	if err := s.repo.MarkLicenseActivated(ctx, lic.ID, tenantId, now); err != nil {
		return nil, "", err
	}

	hasher := sha256.New()
	hasher.Write([]byte(key))
	hash := hex.EncodeToString(hasher.Sum(nil))

	lic.Status = subscripmod.LicenseStatusActivated
	lic.TenantId = &tenantId
	lic.ActivatedAt = &now

	return lic, hash, nil
}
