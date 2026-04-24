// internal/grpc/targetgrpc/server.go
package targetgrpc

import (
	"context"
	"errors"

	"github.com/hotkhwan/gateway-api/internal/logger"
	"github.com/hotkhwan/gateway-api/internal/repo/ingestrepo"
	"github.com/hotkhwan/gateway-api/internal/services/targetsvc"
	"github.com/hotkhwan/gateway-api/models/authzmod"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ─────────────────────────────────────────────────────────────────────────────
// TargetService — admin delivery-target CRUD over gRPC.
//
// Wire protocol matches the existing WorkspaceService (JSON codec, same
// shared-secret interceptor). Registered alongside WorkspaceService by
// workspacegrpc.Start; no new port, no new auth path.
//
// workspaceLookup resolves a workspace to its tenantId — every request
// carries workspaceId from klynx-api, and gw authoritatively looks up
// tenantId so klynx-api can never inject a mismatched pair.
// ─────────────────────────────────────────────────────────────────────────────

// WorkspaceLookup abstracts the workspacesvc.GetByID call so tests can stub
// it without pulling in the full service.
type WorkspaceLookup interface {
	GetByID(ctx context.Context, workspaceID string) (*WorkspaceLookupResult, error)
}

// WorkspaceLookupResult carries just the fields TargetService needs.
type WorkspaceLookupResult struct {
	TenantID string
}

// TargetService subset used by the gRPC server — matches *targetsvc.TargetService.
type targetServiceI interface {
	Create(ctx context.Context, input targetsvc.CreateTargetInput) (*authzmod.DeliveryTarget, error)
	List(ctx context.Context, input targetsvc.ListTargetInput) ([]authzmod.DeliveryTarget, int64, error)
	GetOne(ctx context.Context, tenantID, workspaceID, userID, targetID string, isAdmin bool) (*authzmod.DeliveryTarget, error)
	Update(ctx context.Context, input targetsvc.UpdateTargetInput) (*authzmod.DeliveryTarget, error)
	Delete(ctx context.Context, tenantID, workspaceID, userID, targetID string, isAdmin bool) error
}

// TargetServiceServer implements phibek.target.v1.TargetService.
type TargetServiceServer struct {
	svc      targetServiceI
	wsLookup WorkspaceLookup
}

// NewTargetServiceServer wires the service + workspace lookup.
func NewTargetServiceServer(svc *targetsvc.TargetService, wsLookup WorkspaceLookup) *TargetServiceServer {
	if svc == nil {
		panic("targetgrpc: TargetService required")
	}
	if wsLookup == nil {
		panic("targetgrpc: WorkspaceLookup required")
	}
	return &TargetServiceServer{svc: svc, wsLookup: wsLookup}
}

// newTargetServerWithI is the testable constructor that accepts the service
// interface directly — used only in server_test.go.
func newTargetServerWithI(svc targetServiceI, wsLookup WorkspaceLookup) *TargetServiceServer {
	return &TargetServiceServer{svc: svc, wsLookup: wsLookup}
}

// ServiceDesc returns the grpc.ServiceDesc for registration on an existing server.
func (s *TargetServiceServer) ServiceDesc() *grpc.ServiceDesc {
	return &grpc.ServiceDesc{
		ServiceName: "phibek.target.v1.TargetService",
		HandlerType: (*interface{})(nil),
		Methods: []grpc.MethodDesc{
			{MethodName: "Create", Handler: s.createHandler()},
			{MethodName: "List", Handler: s.listHandler()},
			{MethodName: "Get", Handler: s.getHandler()},
			{MethodName: "Update", Handler: s.updateHandler()},
			{MethodName: "Delete", Handler: s.deleteHandler()},
		},
		Streams: []grpc.StreamDesc{},
	}
}

// resolveTenant looks up the workspace and returns its tenantId. Missing
// workspace → NotFound (no existence leak via PermissionDenied because the
// shared-secret gate has already authenticated the caller).
func (s *TargetServiceServer) resolveTenant(ctx context.Context, workspaceID string) (string, error) {
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

// mapSvcErr converts targetsvc domain errors to gRPC status codes.
// TargetInUseError is returned verbatim so the caller can extract the
// template list via errors.As on the server side before marshalling.
func mapSvcErr(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, targetsvc.ErrNotFound):
		return status.Error(codes.NotFound, err.Error())
	case errors.Is(err, targetsvc.ErrConflict):
		return status.Error(codes.AlreadyExists, err.Error())
	case errors.Is(err, targetsvc.ErrForbidden):
		return status.Error(codes.PermissionDenied, err.Error())
	case errors.Is(err, targetsvc.ErrUnauthorized):
		return status.Error(codes.Unauthenticated, err.Error())
	case errors.Is(err, targetsvc.ErrBadRequest),
		errors.Is(err, targetsvc.ErrMissingBotToken),
		errors.Is(err, targetsvc.ErrMissingChatId),
		errors.Is(err, targetsvc.ErrMissingChannelToken),
		errors.Is(err, targetsvc.ErrMissingRecipients),
		errors.Is(err, targetsvc.ErrMissingURL),
		errors.Is(err, targetsvc.ErrKlynxModeWithURL),
		errors.Is(err, targetsvc.ErrKlynxModeWithHMAC),
		errors.Is(err, targetsvc.ErrKlynxModeInSaasPublic):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, targetsvc.ErrPlanLimitExceeded):
		return status.Error(codes.ResourceExhausted, err.Error())
	case errors.Is(err, targetsvc.ErrTargetInUse):
		// Special-cased: caller handles TargetInUseError by populating
		// TemplatesInUse and returning FailedPrecondition.
		return status.Error(codes.FailedPrecondition, err.Error())
	default:
		return status.Error(codes.Internal, "internal error")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Handlers
// ─────────────────────────────────────────────────────────────────────────────

func (s *TargetServiceServer) createHandler() func(srv any, ctx context.Context, dec func(any) error, interceptor grpc.UnaryServerInterceptor) (any, error) {
	return func(srv any, ctx context.Context, dec func(any) error, _ grpc.UnaryServerInterceptor) (any, error) {
		log := logger.FromCtx(ctx, "targetgrpc", "Create")

		var req CreateTargetRequest
		if err := dec(&req); err != nil {
			return nil, err
		}
		if req.CallerUserID == "" {
			return nil, status.Error(codes.InvalidArgument, "callerUserId required for audit")
		}

		tenantID, err := s.resolveTenant(ctx, req.WorkspaceID)
		if err != nil {
			return nil, err
		}

		enabled := true
		if req.Enabled != nil {
			enabled = *req.Enabled
		}

		t, err := s.svc.Create(ctx, targetsvc.CreateTargetInput{
			TenantId:        tenantID,
			WorkspaceId:     req.WorkspaceID,
			UserId:          req.CallerUserID,
			Name:            req.Name,
			Type:            req.Type,
			Mode:            req.Mode,
			Enabled:         enabled,
			Config:          req.Config,
			IsPlatformAdmin: true, // klynx-api has already authorized the caller
		})
		if err != nil {
			log.Warn().Err(err).Str("workspaceId", req.WorkspaceID).Msg("targetgrpc: Create failed")
			return nil, mapSvcErr(err)
		}
		return &CreateTargetResponse{Target: toView(t)}, nil
	}
}

func (s *TargetServiceServer) listHandler() func(srv any, ctx context.Context, dec func(any) error, interceptor grpc.UnaryServerInterceptor) (any, error) {
	return func(srv any, ctx context.Context, dec func(any) error, _ grpc.UnaryServerInterceptor) (any, error) {
		log := logger.FromCtx(ctx, "targetgrpc", "List")

		var req ListTargetsRequest
		if err := dec(&req); err != nil {
			return nil, err
		}

		tenantID, err := s.resolveTenant(ctx, req.WorkspaceID)
		if err != nil {
			return nil, err
		}

		page := req.Page
		if page < 1 {
			page = 1
		}
		perPage := req.PerPage
		if perPage < 1 || perPage > 100 {
			perPage = 20
		}

		items, total, err := s.svc.List(ctx, targetsvc.ListTargetInput{
			TenantId:    tenantID,
			WorkspaceId: req.WorkspaceID,
			Search:      req.Search,
			Page:        page,
			PerPage:     perPage,
			SortField:   req.SortField,
			SortOrder:   req.SortOrder,
		})
		if err != nil {
			log.Error().Err(err).Str("workspaceId", req.WorkspaceID).Msg("targetgrpc: List failed")
			return nil, status.Error(codes.Internal, "list failed")
		}

		views := make([]*DeliveryTargetView, 0, len(items))
		for i := range items {
			views = append(views, toView(&items[i]))
		}
		return &ListTargetsResponse{
			Items:        views,
			TotalRecords: total,
			Page:         page,
			PerPage:      perPage,
		}, nil
	}
}

func (s *TargetServiceServer) getHandler() func(srv any, ctx context.Context, dec func(any) error, interceptor grpc.UnaryServerInterceptor) (any, error) {
	return func(srv any, ctx context.Context, dec func(any) error, _ grpc.UnaryServerInterceptor) (any, error) {
		var req GetTargetRequest
		if err := dec(&req); err != nil {
			return nil, err
		}
		if req.TargetID == "" {
			return nil, status.Error(codes.InvalidArgument, "targetId required")
		}

		tenantID, err := s.resolveTenant(ctx, req.WorkspaceID)
		if err != nil {
			return nil, err
		}

		// userId empty is fine because isAdmin=true bypasses the Permify check.
		t, err := s.svc.GetOne(ctx, tenantID, req.WorkspaceID, "", req.TargetID, true)
		if err != nil {
			return nil, mapSvcErr(err)
		}
		return &GetTargetResponse{Target: toView(t)}, nil
	}
}

func (s *TargetServiceServer) updateHandler() func(srv any, ctx context.Context, dec func(any) error, interceptor grpc.UnaryServerInterceptor) (any, error) {
	return func(srv any, ctx context.Context, dec func(any) error, _ grpc.UnaryServerInterceptor) (any, error) {
		log := logger.FromCtx(ctx, "targetgrpc", "Update")

		var req UpdateTargetRequest
		if err := dec(&req); err != nil {
			return nil, err
		}
		if req.TargetID == "" {
			return nil, status.Error(codes.InvalidArgument, "targetId required")
		}
		if req.CallerUserID == "" {
			return nil, status.Error(codes.InvalidArgument, "callerUserId required for audit")
		}

		tenantID, err := s.resolveTenant(ctx, req.WorkspaceID)
		if err != nil {
			return nil, err
		}

		t, err := s.svc.Update(ctx, targetsvc.UpdateTargetInput{
			TenantId:        tenantID,
			WorkspaceId:     req.WorkspaceID,
			TargetId:        req.TargetID,
			UserId:          req.CallerUserID,
			Name:            req.Name,
			Enabled:         req.Enabled,
			Config:          req.Config,
			IsPlatformAdmin: true,
		})
		if err != nil {
			log.Warn().Err(err).Str("targetId", req.TargetID).Msg("targetgrpc: Update failed")
			return nil, mapSvcErr(err)
		}
		return &UpdateTargetResponse{Target: toView(t)}, nil
	}
}

func (s *TargetServiceServer) deleteHandler() func(srv any, ctx context.Context, dec func(any) error, interceptor grpc.UnaryServerInterceptor) (any, error) {
	return func(srv any, ctx context.Context, dec func(any) error, _ grpc.UnaryServerInterceptor) (any, error) {
		log := logger.FromCtx(ctx, "targetgrpc", "Delete")

		var req DeleteTargetRequest
		if err := dec(&req); err != nil {
			return nil, err
		}
		if req.TargetID == "" {
			return nil, status.Error(codes.InvalidArgument, "targetId required")
		}

		tenantID, err := s.resolveTenant(ctx, req.WorkspaceID)
		if err != nil {
			return nil, err
		}

		err = s.svc.Delete(ctx, tenantID, req.WorkspaceID, req.CallerUserID, req.TargetID, true)
		if err == nil {
			return &DeleteTargetResponse{}, nil
		}

		// Special-case: ErrTargetInUse carries the list of blocking templates.
		var inUse *targetsvc.TargetInUseError
		if errors.As(err, &inUse) {
			names := make([]string, 0, len(inUse.Templates))
			for _, ref := range inUse.Templates {
				names = append(names, templateLabel(ref))
			}
			log.Info().Str("targetId", req.TargetID).Int("usingTemplates", len(names)).
				Msg("targetgrpc: Delete blocked by template references")
			return &DeleteTargetResponse{TemplatesInUse: names}, status.Error(codes.FailedPrecondition, err.Error())
		}

		log.Warn().Err(err).Str("targetId", req.TargetID).Msg("targetgrpc: Delete failed")
		return nil, mapSvcErr(err)
	}
}

// templateLabel returns a human-readable identifier for a template reference.
// Falls back to the template ID when the name is missing.
func templateLabel(ref ingestrepo.TemplateUsageRef) string {
	if ref.Name != "" {
		return ref.Name
	}
	return ref.TemplateId
}
