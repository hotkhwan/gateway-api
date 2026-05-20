// internal/grpc/cameraoverlaygrpc/types.go
package cameraoverlaygrpc

// ─────────────────────────────────────────────────────────────────────────────
// Service descriptor: phibek.cameraoverlay.v1.CameraOverlayService
//
// Auth: shared-secret interceptor on the containing gRPC server
// (GRPC_SHARED_SECRET via x-gw-token) — same gate as workspace + target
// services. klynx-api is the authorized caller; per-operator audit context
// is forwarded as CallerUserID for log enrichment only (not verified — see
// docs/contracts/camera-gw-managed-overlay.md §8.2).
// ─────────────────────────────────────────────────────────────────────────────

// ApplyOverlayRequest is the klynx → gw push payload for camera overlay edits.
// klynx-api owns the enriched fields (name/description/lat/lng) per the
// camera-gw-managed-overlay.md §2 SoR table; this RPC carries them down to
// gw-api so device_management stays in sync.
type ApplyOverlayRequest struct {
	WorkspaceID  string         `json:"workspaceId"`
	DeviceMgmtID string         `json:"deviceMgmtId"`
	// Fields holds the accepted-field map (name/description/lat/lng in v1).
	// Validation + readonly checks happen server-side in devicemgmtsvc.ApplyKlynxOverlay.
	Fields map[string]any `json:"fields"`
	// IfMatch is the previous lastOutboundHash — replay-only observability per §8.7,
	// never gates the write.
	IfMatch string `json:"ifMatch,omitempty"`
	// CallerUserID is the klynx user that initiated the edit. Forwarded for audit
	// log enrichment only — this RPC's auth gate is the shared secret, so the
	// userId is not cryptographically verified. klynx may claim any value here.
	CallerUserID string `json:"callerUserId,omitempty"`
}

// ApplyOverlayResponse returns the updated identifier + hash + If-Match status
// so klynx can store the new lastOutboundHash for the next round-trip.
type ApplyOverlayResponse struct {
	DeviceMgmtID     string `json:"deviceMgmtId"`
	LastOutboundHash string `json:"lastOutboundHash"`
	// IfMatchStatus is "absent" | "matched" | "mismatched" per §8.7.
	IfMatchStatus string `json:"ifMatchStatus"`
	UpdatedAt     string `json:"updatedAt"`
}

// ApplyOverlayValidationDetail describes which fields caused a validation
// failure. Returned inside the gRPC status detail (status.WithDetails) so the
// klynx side can pass them through to logs. Plain JSON wire shape; matches the
// HTTP error body produced by controllers/adminapi/cameraOverlayInbound.go.
type ApplyOverlayValidationDetail struct {
	Code   string   `json:"code"`
	Fields []string `json:"fields"`
	Reason string   `json:"reason,omitempty"`
}
