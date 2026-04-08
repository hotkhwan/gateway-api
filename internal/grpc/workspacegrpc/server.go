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

// workspaceServer handles the WorkspaceService/ProvisionFromOrg RPC.
type workspaceServer struct {
	svc *workspacesvc.WorkspaceService
}

// Start registers the WorkspaceService handler and serves on GRPC_PORT (default 50051).
// Blocks until ctx is cancelled or a fatal listen error.
func Start(ctx context.Context, svc *workspacesvc.WorkspaceService) error {
	port := os.Getenv("GRPC_PORT")
	if port == "" {
		port = "50051"
	}

	log := logger.Boot("workspacegrpc", "Start")

	lis, err := net.Listen("tcp", ":"+port)
	if err != nil {
		return fmt.Errorf("workspacegrpc: listen :%s: %w", port, err)
	}

	srv := grpc.NewServer(
		grpc.ForceServerCodec(jsonCodec{}),
	)

	// Register handler for /phibek.workspace.v1.WorkspaceService/ProvisionFromOrg
	srv.RegisterService(&grpc.ServiceDesc{
		ServiceName: "phibek.workspace.v1.WorkspaceService",
		HandlerType: (*interface{})(nil),
		Methods: []grpc.MethodDesc{
			{
				MethodName: "ProvisionFromOrg",
				Handler:    provisionFromOrgHandler(&workspaceServer{svc: svc}),
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
