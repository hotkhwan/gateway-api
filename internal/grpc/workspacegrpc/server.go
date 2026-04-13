// internal/grpc/workspacegrpc/server.go
package workspacegrpc

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"

	"github.com/hotkhwan/gateway-api/internal/eventschema"
	"github.com/hotkhwan/gateway-api/internal/logger"
	"github.com/hotkhwan/gateway-api/internal/services/targetsvc"
	"github.com/hotkhwan/gateway-api/internal/services/workspacesvc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/encoding"
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

// Start registers the WorkspaceService handler and serves on GRPC_PORT (default 50051).
// Blocks until ctx is cancelled or a fatal listen error.
func Start(ctx context.Context, svc *workspacesvc.WorkspaceService, targetSvc *targetsvc.TargetService) error {
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

	srv := grpc.NewServer(
		grpc.ForceServerCodec(jsonCodec{}),
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
