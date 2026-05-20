// internal/grpc/workspacegrpc/server.go
package workspacegrpc

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"

	"github.com/hotkhwan/gateway-api/internal/eventschema"
	"github.com/hotkhwan/gateway-api/internal/grpc/cameraoverlaygrpc"
	"github.com/hotkhwan/gateway-api/internal/grpc/eventservice"
	"github.com/hotkhwan/gateway-api/internal/grpc/targetgrpc"
	"github.com/hotkhwan/gateway-api/internal/logger"
	"github.com/hotkhwan/gateway-api/internal/services/devicemgmtsvc"
	"github.com/hotkhwan/gateway-api/internal/services/targetsvc"
	"github.com/hotkhwan/gateway-api/internal/services/workspacesvc"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/encoding"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// jsonCodec encodes gRPC messages as JSON — must match klynx-api's codec name.
type jsonCodec struct{}

func (jsonCodec) Marshal(v any) ([]byte, error)   { return json.Marshal(v) }
func (jsonCodec) Unmarshal(b []byte, v any) error { return json.Unmarshal(b, v) }
func (jsonCodec) Name() string                    { return "klynx-json" }

func init() {
	encoding.RegisterCodec(jsonCodec{})
}

// ProvisionRequest mirrors phibekgw.ProvisionWorkspaceRequest in klynx-api.
type ProvisionRequest struct {
	KlynxOrgID string `json:"klynxOrgId"`
	TenantID   string `json:"tenantId"`
	Name       string `json:"name"`
	CreatedBy  string `json:"createdBy"`
}

// ProvisionResponse mirrors phibekgw.ProvisionWorkspaceResponse in klynx-api.
type ProvisionResponse struct {
	WorkspaceID    string `json:"workspaceId"`
	EventIngestURI string `json:"eventIngestUri"`
}

// RegisterDeliveryTargetRequest is sent by klynx to register the mode=klynx system target.
// WorkspaceID is the gw workspace (= gw orgId). TenantID is resolved server-side from
// the workspace record — the field is present for forward-compatibility only.
type RegisterDeliveryTargetRequest struct {
	WorkspaceID string `json:"workspaceId"`
}

// RegisterDeliveryTargetResponse is returned after delivery target registration.
// TargetID is empty when the target already existed (idempotent).
type RegisterDeliveryTargetResponse struct {
	TargetID string `json:"targetId"`
}

// workspaceServer handles WorkspaceService RPCs.
type workspaceServer struct {
	svc       *workspacesvc.WorkspaceService
	targetSvc *targetsvc.TargetService
}

// workspaceLookupAdapter bridges workspacesvc.WorkspaceService to the
// targetgrpc.WorkspaceLookup interface. The adapter lives here (not in
// targetgrpc) so targetgrpc has no dependency on workspacesvc and can
// be tested with a lightweight stub.
type workspaceLookupAdapter struct {
	svc *workspacesvc.WorkspaceService
}

func (a workspaceLookupAdapter) GetByID(ctx context.Context, workspaceID string) (*targetgrpc.WorkspaceLookupResult, error) {
	ws, err := a.svc.GetByID(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	if ws == nil {
		return nil, nil
	}
	return &targetgrpc.WorkspaceLookupResult{TenantID: ws.TenantID}, nil
}

// cameraOverlayLookupAdapter bridges workspacesvc to cameraoverlaygrpc.WorkspaceLookup.
// Mirrors workspaceLookupAdapter — the two interfaces are intentionally
// duplicated so each gRPC sub-package stays independent of workspacesvc.
type cameraOverlayLookupAdapter struct {
	svc *workspacesvc.WorkspaceService
}

func (a cameraOverlayLookupAdapter) GetByID(ctx context.Context, workspaceID string) (*cameraoverlaygrpc.WorkspaceLookupResult, error) {
	ws, err := a.svc.GetByID(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	if ws == nil {
		return nil, nil
	}
	return &cameraoverlaygrpc.WorkspaceLookupResult{TenantID: ws.TenantID}, nil
}

// sharedSecretInterceptor validates x-gw-token metadata.
// Bypassed when GRPC_SHARED_SECRET is empty (dev mode).
func sharedSecretInterceptor(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
	secret := os.Getenv("GRPC_SHARED_SECRET")
	if secret == "" {
		return handler(ctx, req) // dev mode — skip auth
	}
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "missing metadata")
	}
	tokens := md.Get("x-gw-token")
	if len(tokens) == 0 || tokens[0] != secret {
		return nil, status.Error(codes.Unauthenticated, "invalid gw token")
	}
	return handler(ctx, req)
}

// Start registers the WorkspaceService, EventService, TargetService, and
// CameraOverlayService handlers on GRPC_PORT (default 50051). Blocks until
// ctx is cancelled or a fatal listen error.
func Start(ctx context.Context, svc *workspacesvc.WorkspaceService, targetSvc *targetsvc.TargetService, deviceMgmtSvc *devicemgmtsvc.DeviceManagementService, eventRepo eventservice.EventDetailsRepo) error {
	port := os.Getenv("GRPC_PORT")
	if port == "" {
		port = "50051"
	}

	log := logger.Boot("workspacegrpc", "Start")

	lis, err := net.Listen("tcp", ":"+port)
	if err != nil {
		return fmt.Errorf("workspacegrpc: listen :%s: %w", port, err)
	}

	ws := &workspaceServer{svc: svc, targetSvc: targetSvc}

	// otelgrpc.NewServerHandler() extracts W3C trace context (traceparent/
	// tracestate) from incoming gRPC metadata and starts a child span.
	// Pairs with the otelgrpc.NewClientHandler() on the klynx-api side so
	// trace chains cross the gRPC boundary.
	srv := grpc.NewServer(
		grpc.ForceServerCodec(jsonCodec{}),
		grpc.StatsHandler(otelgrpc.NewServerHandler()),
		grpc.UnaryInterceptor(sharedSecretInterceptor),
	)

	// Register handler for /phibek.workspace.v1.WorkspaceService
	srv.RegisterService(&grpc.ServiceDesc{
		ServiceName: "phibek.workspace.v1.WorkspaceService",
		HandlerType: (*interface{})(nil),
		Methods: []grpc.MethodDesc{
			{
				MethodName: "ProvisionFromOrg",
				Handler:    provisionFromOrgHandler(ws),
			},
			{
				MethodName: "RegisterDeliveryTarget",
				Handler:    registerDeliveryTargetHandler(ws),
			},
		},
		Streams: []grpc.StreamDesc{},
	}, struct{}{})

	// Register handler for /phibek.event.v1.EventService
	if eventRepo != nil {
		esSrv := eventservice.NewEventServiceServer(eventRepo)
		srv.RegisterService(esSrv.ServiceDesc(), struct{}{})
	}

	// Register handler for /phibek.target.v1.TargetService
	// (delivery-target admin CRUD — klynx-api proxy surface, see
	// docs/plan/target-provisioning-cross-repo.md).
	if targetSvc != nil {
		tsSrv := targetgrpc.NewTargetServiceServer(targetSvc, workspaceLookupAdapter{svc: svc})
		srv.RegisterService(tsSrv.ServiceDesc(), struct{}{})
	}

	// Register handler for /phibek.cameraoverlay.v1.CameraOverlayService
	// (klynx-initiated camera overlay PATCH over gRPC — replaces the HTTP
	// path that required Keycloak realm parity. See
	// docs/contracts/camera-gw-managed-overlay.md §8.)
	if deviceMgmtSvc != nil {
		coSrv := cameraoverlaygrpc.NewCameraOverlayServiceServer(deviceMgmtSvc, cameraOverlayLookupAdapter{svc: svc})
		srv.RegisterService(coSrv.ServiceDesc(), struct{}{})
	}

	log.Info().Str("port", port).Msg("✅ phibek gRPC workspace server started")

	go func() {
		<-ctx.Done()
		srv.GracefulStop()
		log.Info().Msg("phibek gRPC workspace server stopped")
	}()

	return srv.Serve(lis)
}

func provisionFromOrgHandler(ws *workspaceServer) func(srv any, ctx context.Context, dec func(any) error, interceptor grpc.UnaryServerInterceptor) (any, error) {
	return func(srv any, ctx context.Context, dec func(any) error, interceptor grpc.UnaryServerInterceptor) (any, error) {
		var req ProvisionRequest
		if err := dec(&req); err != nil {
			return nil, err
		}

		ev := eventschema.OrgCreatedEvent{
			OrgID:     req.KlynxOrgID,
			TenantID:  req.TenantID,
			Name:      req.Name,
			CreatedBy: req.CreatedBy,
		}

		ws2, err := ws.svc.ProvisionFromOrg(ctx, ev)
		if err != nil {
			return nil, err
		}

		return &ProvisionResponse{
			WorkspaceID:    ws2.WorkspaceID,
			EventIngestURI: ws2.EventURI,
		}, nil
	}
}

// registerDeliveryTargetHandler handles WorkspaceService/RegisterDeliveryTarget.
// Resolves tenantId from the workspace record, then delegates to TargetService.RegisterKlynxTarget.
func registerDeliveryTargetHandler(ws *workspaceServer) func(srv any, ctx context.Context, dec func(any) error, interceptor grpc.UnaryServerInterceptor) (any, error) {
	return func(srv any, ctx context.Context, dec func(any) error, interceptor grpc.UnaryServerInterceptor) (any, error) {
		var req RegisterDeliveryTargetRequest
		if err := dec(&req); err != nil {
			return nil, err
		}

		log := logger.Boot("workspacegrpc", "RegisterDeliveryTarget")

		// Resolve tenantId from the workspace record — klynx does not need to send it.
		workspace, err := ws.svc.GetByID(ctx, req.WorkspaceID)
		if err != nil {
			log.Error().Err(err).Str("workspaceId", req.WorkspaceID).Msg("workspacegrpc: workspace lookup failed")
			return nil, fmt.Errorf("workspacegrpc: workspace not found: %w", err)
		}

		targetId, err := ws.targetSvc.RegisterKlynxTarget(ctx, req.WorkspaceID, workspace.TenantID)
		if err != nil {
			log.Error().Err(err).Str("workspaceId", req.WorkspaceID).Msg("workspacegrpc: RegisterDeliveryTarget failed")
			return nil, err
		}

		log.Info().
			Str("workspaceId", req.WorkspaceID).
			Str("targetId", targetId).
			Msg("workspacegrpc: klynx delivery target registered")

		return &RegisterDeliveryTargetResponse{TargetID: targetId}, nil
	}
}
