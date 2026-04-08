// internal/gateways/authzgw/ingestAuthz.go
package authzgw

import (
	"context"
	"os"
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
	client        Client
	tenantId      string
	schemaVersion string
}

// NewIngestAuthzGateway creates an IngestAuthzGateway backed by the existing Permify client.
// tenantId should be "phibek" (from PERMIFY_TENANT_ID env var).
// schemaVersion is read from PERMIFY_SCHEMA_VERSION env var.
func NewIngestAuthzGateway(client Client) IngestAuthzGateway {
	return &permifyIngestAuthzGateway{
		client:        client,
		tenantId:      os.Getenv("PERMIFY_TENANT_ID"),
		schemaVersion: os.Getenv("PERMIFY_SCHEMA_VERSION"),
	}
}

// CanIngest checks whether a source (sourceId) is permitted to ingest events
// into the given workspace (workspaceId).
func (g *permifyIngestAuthzGateway) CanIngest(ctx context.Context, workspaceId, sourceId string) (bool, error) {
	return g.client.CheckPermissionWithSchemaVersion(
		ctx,
		g.tenantId,
		g.schemaVersion,
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
		g.schemaVersion,
		"workspace",
		workspaceId,
		"view",
		"asset",
		assetId,
	)
}
