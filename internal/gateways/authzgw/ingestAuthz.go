// internal/gateways/authzgw/ingestAuthz.go
package authzgw

import (
	"context"
	"os"
	"strings"

	"github.com/hotkhwan/gateway-api/config"
)

// IngestAuthzGateway is the lightweight ingest-scoped authorization interface for phibek.
// It provides only the permission checks needed at the ingest gate:
//   - CanIngest: source credential is allowed to ingest into a workspace
//   - IsAssetOwned: asset belongs to the workspace before processing the event
//
// This is intentionally minimal — phibek does not need full Permify authz management,
// tuple authoring, or the complex permission trees used by klynx-api.
type IngestAuthzGateway interface {
	CanIngest(ctx context.Context, workspaceId, sourceId string) (bool, error)
	IsAssetOwned(ctx context.Context, workspaceId, assetId string) (bool, error)
}

type permifyIngestAuthzGateway struct {
	client   Client
	tenantId string
}

// NewIngestAuthzGateway creates an IngestAuthzGateway backed by the existing Permify client.
// tenantId is read from PERMIFY_TENANT_ID env var (fallback: "klynx").
// schemaVersion is resolved dynamically from config.CurrentSchemaVersion at call time.
func NewIngestAuthzGateway(client Client) IngestAuthzGateway {
	tenantId := os.Getenv("PERMIFY_TENANT_ID")
	if tenantId == "" {
		tenantId = "klynx"
	}
	return &permifyIngestAuthzGateway{
		client:   client,
		tenantId: tenantId,
	}
}

// schemaVersion returns the current schema version at call time.
// Falls back to "latest" when config.CurrentSchemaVersion is not yet initialized.
func (g *permifyIngestAuthzGateway) schemaVersion() string {
	if v := strings.TrimSpace(config.CurrentSchemaVersion); v != "" {
		return v
	}
	return "latest"
}

// CanIngest checks whether a source (sourceId) is permitted to ingest events
// into the given workspace (workspaceId).
func (g *permifyIngestAuthzGateway) CanIngest(ctx context.Context, workspaceId, sourceId string) (bool, error) {
	return g.client.CheckPermissionWithSchemaVersion(
		ctx,
		g.tenantId,
		g.schemaVersion(),
		"workspace",
		workspaceId,
		"ingest",
		"source",
		sourceId,
	)
}

// IsAssetOwned checks whether an asset (assetId) is owned by the given workspace (workspaceId).
func (g *permifyIngestAuthzGateway) IsAssetOwned(ctx context.Context, workspaceId, assetId string) (bool, error) {
	return g.client.CheckPermissionWithSchemaVersion(
		ctx,
		g.tenantId,
		g.schemaVersion(),
		"workspace",
		workspaceId,
		"view",
		"asset",
		assetId,
	)
}
