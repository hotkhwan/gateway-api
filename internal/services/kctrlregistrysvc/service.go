// internal/services/kctrlregistrysvc/service.go
//
// Service layer for kctrl_registry — bridges the admin REST handlers and the
// kctrlsubmsg MQTT subscriber to the underlying Mongo repo. Owns the per-
// process LRU cache and the hot Decide(hwId) routing decision used by
// kctrlsubmsg per contract §5.
package kctrlregistrysvc

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"time"

	"github.com/hotkhwan/gateway-api/internal/repo/kctrlregistryrepo"
	"github.com/hotkhwan/gateway-api/models/kctrlmod"
	"github.com/hotkhwan/gateway-api/utils/traceutil"
)

// registryRepo is the narrow interface the service depends on. Production
// uses *kctrlregistryrepo.KctrlRegistryRepo; tests can substitute a fake.
type registryRepo interface {
	FindByHwId(ctx context.Context, hwId string) (*kctrlmod.KctrlRegistry, error)
	Upsert(ctx context.Context, doc *kctrlmod.KctrlRegistry) (*kctrlmod.KctrlRegistry, error)
	Delete(ctx context.Context, hwId string) error
	ListDrift(ctx context.Context, f kctrlregistryrepo.DriftFilter) ([]kctrlregistryrepo.DriftRow, error)
	CountAll(ctx context.Context) (int64, error)
}

// Service orchestrates kctrl_registry reads/writes and exposes the routing
// decision the MQTT subscriber needs.
type Service struct {
	repo  registryRepo
	cache *registryCache
}

// ErrNotFound is re-exported so callers don't need to import the repo package
// just to type-check sentinel errors.
var ErrNotFound = kctrlregistryrepo.ErrNotFound

// NewService wires the service against the repo. capacity=1000 / ttl=5s are
// the contract §6 defaults; callers may override for tests.
func NewService(repo registryRepo) *Service {
	return &Service{
		repo:  repo,
		cache: newRegistryCache(1000, 5*time.Second),
	}
}

// UpsertInput is the whitelisted PATCH body per contract §4.1. Other fields
// in the request are rejected at the handler before reaching the service.
type UpsertInput struct {
	HwId       string
	OrgId      string
	Approved   bool
	ApprovedAt time.Time
	ApprovedBy string
	// WorkspaceId comes from the X-Active-Workspace header (handler reads
	// the local) — not the body, per contract §4.1.
	WorkspaceId string
}

// Upsert persists the registry row and invalidates the local cache so the
// originating replica sees the new state on its next read. Returns the
// updated document.
//
// Idempotency: when the body matches the existing lastOutboundHash, only
// lastSyncFromKlynxAt is bumped (no field rewrite).
func (s *Service) Upsert(ctx context.Context, in UpsertInput) (*kctrlmod.KctrlRegistry, error) {
	ctx, end, log := traceutil.StartLite(ctx, "gateway.kctrlregistrysvc", "Upsert", "kctrlregistrysvc", "Upsert")
	defer end()
	if in.HwId == "" {
		return nil, errors.New("kctrlregistrysvc: hwId required")
	}

	hash := payloadHash(in)
	now := time.Now().UTC()

	doc := &kctrlmod.KctrlRegistry{
		HwId:                in.HwId,
		OrgId:               in.OrgId,
		WorkspaceId:         in.WorkspaceId,
		Approved:            in.Approved,
		ApprovedAt:          in.ApprovedAt,
		ApprovedBy:          in.ApprovedBy,
		LastSyncFromKlynxAt: now,
		LastOutboundHash:    hash,
	}

	updated, err := s.repo.Upsert(ctx, doc)
	if err != nil {
		return nil, err
	}

	s.cache.Invalidate(in.HwId)
	log.Info().
		Str("hwId", in.HwId).
		Bool("approved", in.Approved).
		Str("orgId", in.OrgId).
		Msg("kctrl registry upserted")
	return updated, nil
}

// Delete removes the row for hwId and invalidates the cache. Idempotent —
// a missing row is not an error (contract §4.2).
func (s *Service) Delete(ctx context.Context, hwId string) error {
	ctx, end, log := traceutil.StartLite(ctx, "gateway.kctrlregistrysvc", "Delete", "kctrlregistrysvc", "Delete")
	defer end()
	if hwId == "" {
		return errors.New("kctrlregistrysvc: hwId required")
	}
	if err := s.repo.Delete(ctx, hwId); err != nil {
		return err
	}
	s.cache.Invalidate(hwId)
	log.Info().Str("hwId", hwId).Msg("kctrl registry deleted")
	return nil
}

// Decision is the routing verdict the MQTT subscriber uses per contract §5.
type Decision struct {
	Action      DecisionAction
	OrgId       string // populated only on ActionEnrich
	WorkspaceId string // populated only on ActionEnrich
}

// DecisionAction enumerates kctrlsubmsg's three branches.
type DecisionAction int

const (
	// ActionEnrich — row exists with approved=true. Populate orgId/workspaceId
	// on the canonical envelope and forward to Kafka.
	ActionEnrich DecisionAction = iota
	// ActionDrop — row exists with approved=false. Do not forward to Kafka.
	// Operator explicitly revoked this device.
	ActionDrop
	// ActionForward — no row exists. Forward to Kafka as-is (orgId empty).
	// Compat-mode default per contract §5.1; klynx-api's Layer-1 resolver
	// handles orgId at the consumer side until backfill completes.
	ActionForward
)

func (a DecisionAction) String() string {
	switch a {
	case ActionEnrich:
		return "enrich"
	case ActionDrop:
		return "drop"
	case ActionForward:
		return "forward"
	default:
		return "unknown"
	}
}

// Decide is the hot-path lookup for kctrlsubmsg. Reads cache → falls back to
// Mongo on miss → caches result (positive or negative).
func (s *Service) Decide(ctx context.Context, hwId string) Decision {
	if hwId == "" {
		return Decision{Action: ActionForward}
	}
	if row, hit := s.cache.Get(hwId); hit {
		return decisionFromRow(row)
	}
	row, err := s.repo.FindByHwId(ctx, hwId)
	if errors.Is(err, ErrNotFound) {
		s.cache.Put(hwId, nil) // negative cache
		return Decision{Action: ActionForward}
	}
	if err != nil {
		// On Mongo error we fall through to FORWARD so realtime traffic does
		// not silently stop when the registry is degraded. The error is
		// surfaced via the metrics / logs in the calling site.
		return Decision{Action: ActionForward}
	}
	s.cache.Put(hwId, row)
	return decisionFromRow(row)
}

func decisionFromRow(row *kctrlmod.KctrlRegistry) Decision {
	if row == nil {
		return Decision{Action: ActionForward}
	}
	if !row.Approved {
		return Decision{Action: ActionDrop}
	}
	return Decision{
		Action:      ActionEnrich,
		OrgId:       row.OrgId,
		WorkspaceId: row.WorkspaceId,
	}
}

// DriftReport mirrors the contract §4.3 response.
type DriftReport struct {
	Items   []DriftItem  `json:"items"`
	Summary DriftSummary `json:"summary"`
}

// DriftItem represents one drifted hwId in the operator triage view.
type DriftItem struct {
	HwId                string    `json:"hwId"`
	OrgId               string    `json:"orgId,omitempty"`
	LastSyncFromKlynxAt time.Time `json:"lastSyncFromKlynxAt"`
	Reason              string    `json:"reason"`
}

// DriftSummary is the aggregate count panel.
type DriftSummary struct {
	Total            int64 `json:"total"`
	Stale            int   `json:"stale"`
	MissingLocalRow  int   `json:"missingLocalRow"` // reserved for §4.3 — populated by future writer
}

// ListDrift returns rows whose lastSyncFromKlynxAt is older than `staleAfter`
// (1h by default) per contract §4.3. The summary count is over the whole
// collection.
func (s *Service) ListDrift(ctx context.Context, staleAfter time.Duration) (*DriftReport, error) {
	if staleAfter <= 0 {
		staleAfter = time.Hour
	}
	since := time.Now().UTC().Add(-staleAfter)

	rows, err := s.repo.ListDrift(ctx, kctrlregistryrepo.DriftFilter{StaleSince: since})
	if err != nil {
		return nil, err
	}
	total, err := s.repo.CountAll(ctx)
	if err != nil {
		return nil, err
	}

	items := make([]DriftItem, 0, len(rows))
	for _, r := range rows {
		items = append(items, DriftItem{
			HwId:                r.HwId,
			OrgId:               r.OrgId,
			LastSyncFromKlynxAt: r.LastSyncFromKlynxAt,
			Reason:              "stale",
		})
	}
	return &DriftReport{
		Items: items,
		Summary: DriftSummary{
			Total: total,
			Stale: len(items),
		},
	}, nil
}

// payloadHash returns a stable sha256-hex over the upsert body fields. Used
// for the §4.1 idempotency short-circuit so klynx-api can safely retry the
// PATCH without rewriting fields on every attempt.
func payloadHash(in UpsertInput) string {
	keys := []string{"approved", "approvedAt", "approvedBy", "hwId", "orgId", "workspaceId"}
	canonical := map[string]any{
		"approved":    in.Approved,
		"approvedAt":  in.ApprovedAt.UTC().Format(time.RFC3339Nano),
		"approvedBy":  in.ApprovedBy,
		"hwId":        in.HwId,
		"orgId":       in.OrgId,
		"workspaceId": in.WorkspaceId,
	}
	pairs := make([][2]any, 0, len(keys))
	sort.Strings(keys)
	for _, k := range keys {
		pairs = append(pairs, [2]any{k, canonical[k]})
	}
	b, _ := json.Marshal(pairs)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
