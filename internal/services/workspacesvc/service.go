// internal/services/workspacesvc/service.go
package workspacesvc

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/hotkhwan/gateway-api/config"
	"github.com/hotkhwan/gateway-api/internal/eventschema"
	"github.com/hotkhwan/gateway-api/internal/gateways/authzgw"
	kafkautil "github.com/hotkhwan/gateway-api/internal/kafka"
	"github.com/hotkhwan/gateway-api/internal/repo/workspacerepo"
	"github.com/hotkhwan/gateway-api/models/workspacemod"
	"github.com/hotkhwan/gateway-api/utils/traceutil"
)

// AuthzWriter is the minimal Permify interface needed for workspace tuple writes.
type AuthzWriter interface {
	WriteTuples(ctx context.Context, tenantId string, tuples []map[string]interface{}) error
}

// WorkspaceService handles workspace lifecycle driven by klynx org events.
type WorkspaceService struct {
	repo   *workspacerepo.WorkspaceRepo
	authz  AuthzWriter // optional — nil = skip Permify write
}

// New creates a WorkspaceService. authz may be nil (Permify write skipped).
func New(repo *workspacerepo.WorkspaceRepo, authz AuthzWriter) *WorkspaceService {
	return &WorkspaceService{repo: repo, authz: authz}
}

// ProvisionFromOrg creates (or is idempotent on existing) a workspace for a klynx org.
// Writes Permify owner tuple if authz is configured, then publishes gw.workspace.provisioned.v1.
// Returns the provisioned (or existing) workspace so callers can read WorkspaceID and EventURI.
func (s *WorkspaceService) ProvisionFromOrg(ctx context.Context, ev eventschema.OrgCreatedEvent) (*workspacemod.Workspace, error) {
	ctx, end, log := traceutil.StartLite(ctx,
		"github.com/hotkhwan/gateway-api/workspacesvc",
		"workspacesvc.ProvisionFromOrg",
		"workspacesvc", "ProvisionFromOrg",
	)
	defer end()

	log.Info().Str("klynxOrgId", ev.OrgID).Str("tenantId", ev.TenantID).Msg("[workspacesvc] provisioning workspace")

	// Idempotency: check if workspace already exists for this org
	existing, err := s.repo.FindByKlynxOrgID(ctx, ev.OrgID)
	if err == nil {
		// already provisioned — re-publish in case klynx missed the ack
		log.Info().Str("workspaceId", existing.WorkspaceID).Msg("[workspacesvc] workspace already exists — re-publishing provisioned event")
		if err := s.publishProvisioned(ctx, existing); err != nil {
			return nil, err
		}
		return existing, nil
	}

	workspaceID := uuid.NewString()
	now := time.Now().UTC()
	basePath := os.Getenv("BASE_PATH")
	if basePath == "" {
		basePath = "/api/v1"
	}

	ws := &workspacemod.Workspace{
		WorkspaceID: workspaceID,
		KlynxOrgID:  ev.OrgID,
		TenantID:    ev.TenantID,
		OwnerUserID: ev.CreatedBy,
		Name:        ev.Name,
		Status:      workspacemod.WorkspaceActive,
		EventURI:    fmt.Sprintf("/events/%s/", workspaceID),
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	if err := s.repo.Upsert(ctx, ws); err != nil {
		log.Error().Err(err).Str("workspaceId", workspaceID).Msg("[workspacesvc] upsert failed")
		return nil, fmt.Errorf("workspacesvc: upsert workspace: %w", err)
	}

	// P-W7: write Permify workspace + org tuples (non-fatal if authz not configured)
	if s.authz != nil {
		permifyTenant := config.PermifyTenantID
		tuples := []map[string]interface{}{
			// link workspace to platform so adminPlatform can access all workspaces
			{
				"entity":   map[string]interface{}{"type": "workspace", "id": workspaceID},
				"relation": "platform",
				"subject":  map[string]interface{}{"type": "platform", "id": permifyTenant},
			},
		}
		if ev.CreatedBy != "" {
			tuples = append(tuples,
				// workspace-level tuples
				map[string]interface{}{
					"entity":   map[string]interface{}{"type": "workspace", "id": workspaceID},
					"relation": "owner",
					"subject":  map[string]interface{}{"type": "user", "id": ev.CreatedBy},
				},
				// organization-level tuples — required for delivery target + binding management
				// (targetsvc/checkAdmin checks organization:workspaceId#manage which derives from owner|admin)
				map[string]interface{}{
					"entity":   map[string]interface{}{"type": "organization", "id": workspaceID},
					"relation": "owner",
					"subject":  map[string]interface{}{"type": "user", "id": ev.CreatedBy},
				},
				map[string]interface{}{
					"entity":   map[string]interface{}{"type": "organization", "id": workspaceID},
					"relation": "member",
					"subject":  map[string]interface{}{"type": "user", "id": ev.CreatedBy},
				},
			)
		}
		if err := s.authz.WriteTuples(ctx, permifyTenant, tuples); err != nil {
			// Non-fatal: workspace record exists; Permify can be retried separately
			log.Warn().Err(err).Str("workspaceId", workspaceID).Msg("[workspacesvc] Permify tuple write failed (non-fatal)")
		}
	}

	if err := s.publishProvisioned(ctx, ws); err != nil {
		return nil, err
	}
	return ws, nil
}

// CreateStandalone creates a new phibek workspace that is NOT linked to a klynx org.
// Used when phibek is deployed as a standalone product (not managed by klynx lifecycle).
// The caller (userId) becomes the workspace owner in Permify.
func (s *WorkspaceService) CreateStandalone(ctx context.Context, tenantID, name, createdBy string) (*workspacemod.Workspace, error) {
	ctx, end, log := traceutil.StartLite(ctx,
		"github.com/hotkhwan/gateway-api/workspacesvc",
		"workspacesvc.CreateStandalone",
		"workspacesvc", "CreateStandalone",
	)
	defer end()

	workspaceID := uuid.NewString()
	now := time.Now().UTC()
	ws := &workspacemod.Workspace{
		WorkspaceID: workspaceID,
		KlynxOrgID:  "", // standalone — no klynx org reference
		TenantID:    tenantID,
		OwnerUserID: createdBy,
		Name:        name,
		Status:      workspacemod.WorkspaceActive,
		EventURI:    fmt.Sprintf("/events/%s/", workspaceID),
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	if err := s.repo.Upsert(ctx, ws); err != nil {
		return nil, fmt.Errorf("workspacesvc: upsert standalone workspace: %w", err)
	}

	if s.authz != nil {
		permifyTenant := config.PermifyTenantID
		tuples := []map[string]interface{}{
			// link workspace to platform so adminPlatform can access all workspaces
			{
				"entity":   map[string]interface{}{"type": "workspace", "id": workspaceID},
				"relation": "platform",
				"subject":  map[string]interface{}{"type": "platform", "id": permifyTenant},
			},
		}
		if createdBy != "" {
			tuples = append(tuples, map[string]interface{}{
				"entity":   map[string]interface{}{"type": "workspace", "id": workspaceID},
				"relation": "owner",
				"subject":  map[string]interface{}{"type": "user", "id": createdBy},
			})
		}
		if err := s.authz.WriteTuples(ctx, permifyTenant, tuples); err != nil {
			log.Warn().Err(err).Str("workspaceId", workspaceID).Msg("[workspacesvc] Permify tuple write failed (non-fatal)")
		}
	}

	log.Info().Str("workspaceId", workspaceID).Str("name", name).Msg("[workspacesvc] standalone workspace created")
	return ws, nil
}

// ListForUser returns all workspaces the given user has access to.
// isPlatformAdmin should be derived from the JWT role claim ("administrator") by the caller.
// When true, all workspaces are returned regardless of Permify tuples.
func (s *WorkspaceService) ListForUser(ctx context.Context, tenantID, userID string, authzClient AuthzWriter, isPlatformAdmin bool) ([]*workspacemod.Workspace, error) {
	ctx, end, log := traceutil.StartLite(ctx,
		"github.com/hotkhwan/gateway-api/workspacesvc",
		"workspacesvc.ListForUser",
		"workspacesvc", "ListForUser",
	)
	defer end()

	if isPlatformAdmin {
		log.Debug().Str("userID", userID).Msg("[workspacesvc] platform admin — loading all workspaces")
		return s.repo.ListAll(ctx)
	}

	type subjectLister interface {
		ListRelationshipsBySubject(ctx context.Context, tenantId string, subjectType string, subjectId string) ([]authzgw.Relationship, error)
	}

	sl, ok := authzClient.(subjectLister)
	if !ok {
		return nil, fmt.Errorf("workspacesvc: authz client does not support ListRelationshipsBySubject")
	}

	rels, err := sl.ListRelationshipsBySubject(ctx, tenantID, "user", userID)
	if err != nil {
		log.Error().Err(err).Msg("[workspacesvc] list workspace relationships by subject failed")
		return nil, fmt.Errorf("workspacesvc: list relationships: %w", err)
	}

	// Collect workspace IDs where user has any direct role
	seen := make(map[string]struct{})
	for _, r := range rels {
		if r.Entity.Type == "workspace" {
			seen[r.Entity.ID] = struct{}{}
		}
	}

	ids := make([]string, 0, len(seen))
	for id := range seen {
		ids = append(ids, id)
	}

	log.Debug().
		Str("tenantID", tenantID).
		Str("userID", userID).
		Strs("ids", ids).
		Msg("[workspacesvc] workspace IDs from tuple lookup")

	return s.repo.ListByIDs(ctx, ids)
}

// GetByID returns a single workspace by workspaceId.
func (s *WorkspaceService) GetByID(ctx context.Context, workspaceID string) (*workspacemod.Workspace, error) {
	ctx, end, _ := traceutil.StartLite(ctx,
		"github.com/hotkhwan/gateway-api/workspacesvc",
		"workspacesvc.GetByID",
		"workspacesvc", "GetByID",
	)
	defer end()
	return s.repo.FindByWorkspaceID(ctx, workspaceID)
}

// UpdateName renames a workspace. Only allowed for owners/admins (enforced at middleware level).
func (s *WorkspaceService) UpdateName(ctx context.Context, workspaceID, name string) error {
	ctx, end, _ := traceutil.StartLite(ctx,
		"github.com/hotkhwan/gateway-api/workspacesvc",
		"workspacesvc.UpdateName",
		"workspacesvc", "UpdateName",
	)
	defer end()
	return s.repo.UpdateName(ctx, workspaceID, name)
}

// DeleteStandalone removes a standalone workspace. Standalone = no klynxOrgId.
// Klynx-provisioned workspaces should be suspended via SuspendFromOrg instead.
func (s *WorkspaceService) DeleteStandalone(ctx context.Context, workspaceID string) error {
	ctx, end, log := traceutil.StartLite(ctx,
		"github.com/hotkhwan/gateway-api/workspacesvc",
		"workspacesvc.DeleteStandalone",
		"workspacesvc", "DeleteStandalone",
	)
	defer end()

	ws, err := s.repo.FindByWorkspaceID(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("workspacesvc: workspace not found: %w", err)
	}
	if ws.KlynxOrgID != "" {
		return fmt.Errorf("workspacesvc: cannot delete klynx-provisioned workspace — use suspend instead")
	}

	if err := s.repo.Delete(ctx, workspaceID); err != nil {
		return fmt.Errorf("workspacesvc: delete workspace: %w", err)
	}

	log.Info().Str("workspaceId", workspaceID).Msg("[workspacesvc] standalone workspace deleted")
	return nil
}

// SuspendFromOrg suspends the workspace linked to a deleted klynx org.
func (s *WorkspaceService) SuspendFromOrg(ctx context.Context, ev eventschema.OrgDeletedEvent) error {
	ctx, end, log := traceutil.StartLite(ctx,
		"github.com/hotkhwan/gateway-api/workspacesvc",
		"workspacesvc.SuspendFromOrg",
		"workspacesvc", "SuspendFromOrg",
	)
	defer end()

	ws, err := s.repo.FindByKlynxOrgID(ctx, ev.OrgID)
	if err != nil {
		log.Warn().Str("klynxOrgId", ev.OrgID).Err(err).Msg("[workspacesvc] workspace not found for suspension — skipping")
		return nil
	}

	if err := s.repo.UpdateStatus(ctx, ws.WorkspaceID, workspacemod.WorkspaceSuspended); err != nil {
		return fmt.Errorf("workspacesvc: suspend workspace: %w", err)
	}

	log.Info().Str("workspaceId", ws.WorkspaceID).Str("klynxOrgId", ev.OrgID).Msg("[workspacesvc] workspace suspended")
	return nil
}

// IngestConfigView is the response DTO for workspace ingest config (no raw key).
type IngestConfigView struct {
	IngestEndpoint     string          `json:"ingestEndpoint"`
	IngestSecretMasked string          `json:"ingestSecretMasked"`
	SignatureRequired  bool            `json:"signatureRequired"`
	RateLimit          RateLimitConfig `json:"rateLimit"`
}

type RateLimitConfig struct {
	PerSecond int `json:"perSecond"`
	Burst     int `json:"burst"`
}

func resolveRateLimit(cfg workspacemod.WorkspaceIngestConfig) RateLimitConfig {
	perSec := cfg.RateLimitPerSec
	if perSec <= 0 {
		perSec = 10
	}
	burst := cfg.RateLimitBurst
	if burst <= 0 {
		burst = 20
	}
	return RateLimitConfig{PerSecond: perSec, Burst: burst}
}

func maskSecret(s string) string {
	if len(s) <= 8 {
		return s
	}
	return s[:4] + "..." + s[len(s)-4:]
}

func generateIngestKey() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// GetIngestConfig returns the workspace-scoped ingest config.
// Generates and persists an ingest key if one does not exist yet.
func (s *WorkspaceService) GetIngestConfig(ctx context.Context, workspaceID string) (*IngestConfigView, error) {
	ctx, end, log := traceutil.StartLite(ctx,
		"github.com/hotkhwan/gateway-api/workspacesvc",
		"workspacesvc.GetIngestConfig",
		"workspacesvc", "GetIngestConfig",
	)
	defer end()

	ws, err := s.repo.FindByWorkspaceID(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("workspacesvc: workspace not found: %w", err)
	}

	// Auto-provision ingest key on first access.
	if ws.IngestConfig.IngestKey == "" {
		key, err := generateIngestKey()
		if err != nil {
			return nil, fmt.Errorf("workspacesvc: generate ingest key: %w", err)
		}
		if err := s.repo.UpdateIngestKey(ctx, workspaceID, key); err != nil {
			return nil, fmt.Errorf("workspacesvc: persist ingest key: %w", err)
		}
		ws.IngestConfig.IngestKey = key
		log.Info().Str("workspaceId", workspaceID).Msg("[workspacesvc] ingest key auto-provisioned")
	}

	return &IngestConfigView{
		IngestEndpoint:     ws.EventURI,
		IngestSecretMasked: maskSecret(ws.IngestConfig.IngestKey),
		SignatureRequired:  ws.IngestConfig.SignatureRequired,
		RateLimit:          resolveRateLimit(ws.IngestConfig),
	}, nil
}

// RotateIngestSecret generates a new ingest key and returns the masked version.
func (s *WorkspaceService) RotateIngestSecret(ctx context.Context, workspaceID string) (*IngestConfigView, error) {
	ctx, end, log := traceutil.StartLite(ctx,
		"github.com/hotkhwan/gateway-api/workspacesvc",
		"workspacesvc.RotateIngestSecret",
		"workspacesvc", "RotateIngestSecret",
	)
	defer end()

	ws, err := s.repo.FindByWorkspaceID(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("workspacesvc: workspace not found: %w", err)
	}

	key, err := generateIngestKey()
	if err != nil {
		return nil, fmt.Errorf("workspacesvc: generate ingest key: %w", err)
	}
	if err := s.repo.UpdateIngestKey(ctx, workspaceID, key); err != nil {
		return nil, fmt.Errorf("workspacesvc: persist ingest key: %w", err)
	}

	log.Info().Str("workspaceId", workspaceID).Msg("[workspacesvc] ingest key rotated")

	return &IngestConfigView{
		IngestEndpoint:     ws.EventURI,
		IngestSecretMasked: maskSecret(key),
		SignatureRequired:  ws.IngestConfig.SignatureRequired,
		RateLimit:          resolveRateLimit(ws.IngestConfig),
	}, nil
}

func (s *WorkspaceService) publishProvisioned(ctx context.Context, ws *workspacemod.Workspace) error {
	topic := os.Getenv("KAFKA_TOPIC_WORKSPACE_PROVISIONED")
	if topic == "" {
		topic = "gw.workspace.provisioned.v1"
	}

	ev := eventschema.WorkspaceProvisionedEvent{
		WorkspaceID:    ws.WorkspaceID,
		KlynxOrgID:     ws.KlynxOrgID,
		TenantID:       ws.TenantID,
		EventIngestURI: ws.EventURI,
		ProvisionedAt:  time.Now().UTC(),
	}

	headers := map[string]string{}
	traceutil.InjectHeaders(ctx, headers)

	if err := kafkautil.PublishEventTo(ctx, topic, ws.WorkspaceID, ev, headers); err != nil {
		return fmt.Errorf("workspacesvc: publish provisioned: %w", err)
	}
	return nil
}
