// internal/grpc/cameraoverlaygrpc/server.go
package cameraoverlaygrpc

import (
	"context"
	"errors"
	"time"

	"github.com/hotkhwan/gateway-api/internal/logger"
	"github.com/hotkhwan/gateway-api/internal/services/devicemgmtsvc"
	"github.com/hotkhwan/gateway-api/models/ingestmod"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ─────────────────────────────────────────────────────────────────────────────
// CameraOverlayService — klynx-initiated camera overlay PATCH over gRPC.
//
// Pairs with the legacy HTTP endpoint at
//   PATCH /api/v1/admin/device-management/cameras/{gwDeviceMgmtId}
// but goes through the shared-secret gRPC channel so klynx-api and gw-api
// do NOT need to share a Keycloak realm. See contracts/
// camera-gw-managed-overlay.md §8 for the field-level rules; this server is
// transport only and delegates straight to devicemgmtsvc.ApplyKlynxOverlay.
// ─────────────────────────────────────────────────────────────────────────────

// overlaySvc is the narrow interface the server needs — same signature as
// the HTTP controller uses, so tests can stub it without standing up Mongo.
type overlaySvc interface {
	ApplyKlynxOverlay(
		ctx context.Context,
		tenantId, workspaceId, deviceMgmtId string,
		body map[string]any,
		ifMatch string,
	) (*ingestmod.DeviceManagement, devicemgmtsvc.IfMatchStatus, error)
}

// WorkspaceLookup resolves a workspaceId → tenantId. Mirrors targetgrpc's
// adapter so klynx can never inject a mismatched (tenant, workspace) pair.
type WorkspaceLookup interface {
	GetByID(ctx context.Context, workspaceID string) (*WorkspaceLookupResult, error)
}

// WorkspaceLookupResult carries just the field this service needs.
type WorkspaceLookupResult struct {
	TenantID string
}

// CameraOverlayServiceServer implements phibek.cameraoverlay.v1.CameraOverlayService.
type CameraOverlayServiceServer struct {
	svc      overlaySvc
	wsLookup WorkspaceLookup
}

// NewCameraOverlayServiceServer wires the service + workspace lookup.
func NewCameraOverlayServiceServer(svc *devicemgmtsvc.DeviceManagementService, wsLookup WorkspaceLookup) *CameraOverlayServiceServer {
	if svc == nil {
		panic("cameraoverlaygrpc: DeviceManagementService required")
	}
	if wsLookup == nil {
		panic("cameraoverlaygrpc: WorkspaceLookup required")
	}
	return &CameraOverlayServiceServer{svc: svc, wsLookup: wsLookup}
}

// ServiceDesc returns the grpc.ServiceDesc for registration on the shared
// gRPC server (workspacegrpc.Start).
func (s *CameraOverlayServiceServer) ServiceDesc() *grpc.ServiceDesc {
	return &grpc.ServiceDesc{
		ServiceName: "phibek.cameraoverlay.v1.CameraOverlayService",
		HandlerType: (*interface{})(nil),
		Methods: []grpc.MethodDesc{
			{MethodName: "ApplyOverlay", Handler: s.applyOverlayHandler()},
		},
		Streams: []grpc.StreamDesc{},
	}
}

// resolveTenant looks up the workspace's tenantId. Missing workspace → NotFound.
func (s *CameraOverlayServiceServer) resolveTenant(ctx context.Context, workspaceID string) (string, error) {
	if workspaceID == "" {
		return "", status.Error(codes.InvalidArgument, "workspaceId required")
	}
	ws, err := s.wsLookup.GetByID(ctx, workspaceID)
	if err != nil {
		return "", status.Error(codes.NotFound, "workspace not found")
	}
	if ws == nil || ws.TenantID == "" {
		return "", status.Error(codes.NotFound, "workspace not found")
	}
	return ws.TenantID, nil
}

// applyOverlayHandler decodes the request, resolves tenant, and delegates to
// devicemgmtsvc.ApplyKlynxOverlay. Service errors are mapped to gRPC codes:
//   *OverlayValidationError → InvalidArgument (carries field list in message)
//   ErrNotFound             → NotFound
//   other                   → Internal
func (s *CameraOverlayServiceServer) applyOverlayHandler() func(srv any, ctx context.Context, dec func(any) error, interceptor grpc.UnaryServerInterceptor) (any, error) {
	return func(srv any, ctx context.Context, dec func(any) error, _ grpc.UnaryServerInterceptor) (any, error) {
		log := logger.FromCtx(ctx, "cameraoverlaygrpc", "ApplyOverlay")

		var req ApplyOverlayRequest
		if err := dec(&req); err != nil {
			return nil, err
		}
		if req.DeviceMgmtID == "" {
			return nil, status.Error(codes.InvalidArgument, "deviceMgmtId required")
		}
		if len(req.Fields) == 0 {
			return nil, status.Error(codes.InvalidArgument, "fields required")
		}

		tenantID, err := s.resolveTenant(ctx, req.WorkspaceID)
		if err != nil {
			return nil, err
		}

		updated, ifMatchStatus, svcErr := s.svc.ApplyKlynxOverlay(ctx, tenantID, req.WorkspaceID, req.DeviceMgmtID, req.Fields, req.IfMatch)
		if svcErr != nil {
			// Validation errors → InvalidArgument with the offending field list
			// embedded in the message (klynx logs the message verbatim per
			// camera-gw-managed-overlay.md §8.4 error contract).
			var verr *devicemgmtsvc.OverlayValidationError
			if errors.As(svcErr, &verr) {
				log.Warn().
					Str("code", verr.Code).
					Strs("fields", verr.Fields).
					Str("deviceMgmtId", req.DeviceMgmtID).
					Msg("cameraoverlaygrpc: validation rejected")
				return nil, status.Error(codes.InvalidArgument, verr.Code)
			}
			if errors.Is(svcErr, devicemgmtsvc.ErrNotFound) {
				return nil, status.Error(codes.NotFound, "DEVICE_NOT_FOUND")
			}
			log.Error().Err(svcErr).Str("deviceMgmtId", req.DeviceMgmtID).Msg("cameraoverlaygrpc: ApplyKlynxOverlay failed")
			return nil, status.Error(codes.Internal, "internal error")
		}

		log.Info().
			Str("deviceMgmtId", req.DeviceMgmtID).
			Str("workspaceId", req.WorkspaceID).
			Str("callerUserId", req.CallerUserID).
			Str("ifMatchStatus", string(ifMatchStatus)).
			Int("fieldCount", len(req.Fields)).
			Msg("cameraoverlaygrpc: overlay applied")

		return &ApplyOverlayResponse{
			DeviceMgmtID:     updated.DeviceMgmtId,
			LastOutboundHash: updated.LastOutboundHash,
			IfMatchStatus:    string(ifMatchStatus),
			UpdatedAt:        updated.UpdatedAt.UTC().Format(time.RFC3339Nano),
		}, nil
	}
}
