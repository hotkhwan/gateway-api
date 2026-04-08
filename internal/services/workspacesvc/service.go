// internal/services/workspacesvc/service.go
package workspacesvc

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/hotkhwan/gateway-api/internal/eventschema"
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
// Writes Permify owner tuple if authz is configured, then publishes phibek.workspace.provisioned.v1.
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

	// P-W7: write Permify workspace owner tuple (non-fatal if authz not configured)
	if s.authz != nil && ev.CreatedBy != "" {
		permifyTenant := os.Getenv("PERMIFY_TENANT_ID")
		if permifyTenant == "" {
			permifyTenant = "phibek"
		}
		tuples := []map[string]interface{}{
			{
				"entity":   map[string]interface{}{"type": "workspace", "id": workspaceID},
				"relation": "owner",
				"subject":  map[string]interface{}{"type": "user", "id": ev.CreatedBy},
			},
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

func (s *WorkspaceService) publishProvisioned(ctx context.Context, ws *workspacemod.Workspace) error {
	topic := os.Getenv("KAFKA_TOPIC_WORKSPACE_PROVISIONED")
	if topic == "" {
		topic = "phibek.workspace.provisioned.v1"
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
