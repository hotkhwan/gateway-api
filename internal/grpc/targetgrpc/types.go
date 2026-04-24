// internal/grpc/targetgrpc/types.go
package targetgrpc

import "github.com/hotkhwan/gateway-api/models/authzmod"

// ─────────────────────────────────────────────────────────────────────────────
// Service descriptor: phibek.target.v1.TargetService
//
// Shared-secret interceptor on the containing gRPC server (GRPC_SHARED_SECRET
// via x-gw-token) is the only authentication. klynx-api performs the klynx
// user / klynx-org ↔ phibek-workspace authorization before forwarding, so
// gRPC handlers bypass the per-user Permify check on the gw side and treat
// every request as a platform-authority call (see §4 Ownership Model in
// docs/plan/target-provisioning-cross-repo.md).
// ─────────────────────────────────────────────────────────────────────────────

// DeliveryTargetView is the wire shape of a delivery target returned by
// Get / List / Create / Update. Secret config fields (access tokens, signing
// secrets, bot tokens) are replaced with empty strings before transit — see
// redact.go. JSON field names mirror authzmod.DeliveryTarget for FE parity.
type DeliveryTargetView struct {
	TargetID    string             `json:"id"`
	WorkspaceID string             `json:"workspaceId"`
	TenantID    string             `json:"tenantId"`
	Name        string             `json:"name"`
	Type        string             `json:"type"` // webhook | line | telegram | discord
	Mode        string             `json:"mode,omitempty"`
	Enabled     bool               `json:"enabled"`
	Config      TargetConfigView   `json:"config"`
	CreatedBy   string             `json:"createdBy"`
	CreatedAt   string             `json:"createdAt"` // RFC3339 UTC
	UpdatedAt   string             `json:"updatedAt"` // RFC3339 UTC
}

// TargetConfigView is the redacted wire shape. Boolean/non-secret fields
// round-trip unchanged; secret fields carry a sentinel when present so the
// FE can render "●●●●● (set)" without seeing the value.
type TargetConfigView struct {
	// webhook + discord
	URL            string            `json:"url,omitempty"`
	Headers        map[string]string `json:"headers,omitempty"`
	SigningEnabled bool              `json:"signingEnabled"`
	SigningSecretSet bool            `json:"signingSecretSet"`
	TimeoutMs      int               `json:"timeoutMs,omitempty"`

	// line
	ChannelAccessTokenSet    bool     `json:"channelAccessTokenSet"`
	ChannelAccessTokenRefSet bool     `json:"channelAccessTokenRefSet"`
	To                       []string `json:"to,omitempty"`

	// telegram
	BotTokenSet bool   `json:"botTokenSet"`
	ChatID      string `json:"chatId,omitempty"`
}

// ─── Create ──────────────────────────────────────────────────────────────────

type CreateTargetRequest struct {
	WorkspaceID string                `json:"workspaceId"`
	// CallerUserID is the originating klynx user, forwarded for audit (CreatedBy).
	CallerUserID string               `json:"callerUserId"`
	Name         string               `json:"name"`
	Type         string               `json:"type"`
	Mode         string               `json:"mode,omitempty"`
	Enabled      *bool                `json:"enabled,omitempty"`
	Config       authzmod.TargetConfig `json:"config"`
}

type CreateTargetResponse struct {
	Target *DeliveryTargetView `json:"target"`
}

// ─── List ────────────────────────────────────────────────────────────────────

type ListTargetsRequest struct {
	WorkspaceID string `json:"workspaceId"`
	Search      string `json:"search,omitempty"`
	Page        int    `json:"page,omitempty"`
	PerPage     int    `json:"perPage,omitempty"`
	SortField   string `json:"sortField,omitempty"`
	SortOrder   string `json:"sortOrder,omitempty"`
}

type ListTargetsResponse struct {
	Items        []*DeliveryTargetView `json:"items"`
	TotalRecords int64                 `json:"totalRecords"`
	Page         int                   `json:"page"`
	PerPage      int                   `json:"perPage"`
}

// ─── Get ─────────────────────────────────────────────────────────────────────

type GetTargetRequest struct {
	WorkspaceID string `json:"workspaceId"`
	TargetID    string `json:"targetId"`
}

type GetTargetResponse struct {
	Target *DeliveryTargetView `json:"target"`
}

// ─── Update ──────────────────────────────────────────────────────────────────

type UpdateTargetRequest struct {
	WorkspaceID  string                 `json:"workspaceId"`
	TargetID     string                 `json:"targetId"`
	CallerUserID string                 `json:"callerUserId"`
	Name         *string                `json:"name,omitempty"`
	Enabled      *bool                  `json:"enabled,omitempty"`
	Config       *authzmod.TargetConfig `json:"config,omitempty"`
}

type UpdateTargetResponse struct {
	Target *DeliveryTargetView `json:"target"`
}

// ─── Delete ──────────────────────────────────────────────────────────────────

type DeleteTargetRequest struct {
	WorkspaceID  string `json:"workspaceId"`
	TargetID     string `json:"targetId"`
	CallerUserID string `json:"callerUserId"`
}

type DeleteTargetResponse struct {
	// TemplatesInUse is populated when the target cannot be deleted because
	// one or more mapping templates still reference it. The caller surfaces
	// this as a 409/FailedPrecondition to the UI with the template names so
	// the user can unlink before retrying.
	TemplatesInUse []string `json:"templatesInUse,omitempty"`
}
