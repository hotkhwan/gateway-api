// internal/grpc/eventservice/server.go
package eventservice

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/hotkhwan/gateway-api/internal/logger"
	"github.com/hotkhwan/gateway-api/internal/repo/ingestdetailsrepo"
	"github.com/hotkhwan/gateway-api/models/ingestmod"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ---- Request / Response shapes (JSON-over-gRPC, matches klynx EventDetailDTO) ----

type GetEventRequest struct {
	EventID     string `json:"eventId"`
	WorkspaceID string `json:"workspaceId"`
}

type BatchGetEventsRequest struct {
	EventIDs    []string `json:"eventIds"`
	WorkspaceID string   `json:"workspaceId"`
}

type BatchGetEventsResponse struct {
	Events   []*EventResponse `json:"events"`
	NotFound []string         `json:"notFound,omitempty"`
}

// EventResponse mirrors klynx eventsvc.EventDetailDTO — field names must match exactly.
type EventResponse struct {
	EventID     string             `json:"eventId"`
	EventType   string             `json:"eventType"`
	OrgID       string             `json:"orgId,omitempty"`
	OccurredAt  time.Time          `json:"occurredAt"`
	Source      EventSourceInfo    `json:"source"`
	Location    *EventLocationInfo `json:"location,omitempty"`
	Geo         *EventGeoInfo      `json:"geo,omitempty"`
	GeoCell     *EventGeoCellInfo  `json:"geoCell,omitempty"`
	ByAdminArea map[string]any     `json:"byAdminArea,omitempty"`
	Payload     map[string]any     `json:"payload,omitempty"`
	BinaryRefs  []EventBinaryRef   `json:"binaryRefs,omitempty"`
	Meta        EventMeta          `json:"meta"`
}

type EventSourceInfo struct {
	DeviceID   string `json:"deviceId"`
	DeviceName string `json:"deviceName,omitempty"`
	DeviceType string `json:"deviceType,omitempty"`
	Vendor     string `json:"vendor,omitempty"`
	OrgID      string `json:"orgId,omitempty"`
}

type EventLocationInfo struct {
	Lat  float64 `json:"lat"`
	Lng  float64 `json:"lng"`
	Site string  `json:"site,omitempty"`
	Zone string  `json:"zone,omitempty"`
}

type EventGeoInfo struct {
	CountryCode string `json:"countryCode,omitempty"`
	AdminLevel  int    `json:"adminLevel,omitempty"`
	AdminName   string `json:"adminName,omitempty"`
	AdminCode   string `json:"adminCode,omitempty"`
	IdScheme    string `json:"idScheme,omitempty"`
}

type EventGeoCellInfo struct {
	Scheme    string `json:"scheme,omitempty"`
	Precision int    `json:"precision,omitempty"`
	Cell      string `json:"cell,omitempty"`
}

type EventBinaryRef struct {
	ObjectId    string `json:"objectId"`
	Bucket      string `json:"bucket,omitempty"`
	ContentType string `json:"contentType,omitempty"`
	FieldName   string `json:"fieldName,omitempty"`
	Kind        string `json:"kind,omitempty"`
	Role        string `json:"role,omitempty"`
	SourceIndex int    `json:"sourceIndex"`
}

type EventMeta struct {
	SchemaVersion string    `json:"schemaVersion,omitempty"`
	NormalizedAt  time.Time `json:"normalizedAt,omitempty"`
	TemplateID    string    `json:"templateId,omitempty"`
}

// ---- Repo interface (DI boundary) ----

type EventDetailsRepo interface {
	FindNormalizedByEventID(ctx context.Context, workspaceId, eventId string) (*ingestmod.NormalizedEvent, error)
	FindNormalizedByEventIDs(ctx context.Context, workspaceId string, eventIds []string) ([]*ingestmod.NormalizedEvent, error)
}

// ---- Server ----

type EventServiceServer struct {
	repo EventDetailsRepo
}

func NewEventServiceServer(repo EventDetailsRepo) *EventServiceServer {
	if repo == nil {
		panic("eventservice: repo required")
	}
	return &EventServiceServer{repo: repo}
}

// ServiceDesc returns the grpc.ServiceDesc for registration on an existing gRPC server.
func (s *EventServiceServer) ServiceDesc() *grpc.ServiceDesc {
	return &grpc.ServiceDesc{
		ServiceName: "phibek.event.v1.EventService",
		HandlerType: (*interface{})(nil),
		Methods: []grpc.MethodDesc{
			{
				MethodName: "GetEvent",
				Handler:    s.getEventHandler(),
			},
			{
				MethodName: "BatchGetEvents",
				Handler:    s.batchGetEventsHandler(),
			},
		},
		Streams: []grpc.StreamDesc{},
	}
}

func (s *EventServiceServer) getEventHandler() func(srv any, ctx context.Context, dec func(any) error, interceptor grpc.UnaryServerInterceptor) (any, error) {
	return func(srv any, ctx context.Context, dec func(any) error, interceptor grpc.UnaryServerInterceptor) (any, error) {
		log := logger.FromCtx(ctx, "eventservice", "GetEvent")

		var req GetEventRequest
		if err := dec(&req); err != nil {
			return nil, err
		}
		if req.EventID == "" || req.WorkspaceID == "" {
			return nil, status.Error(codes.InvalidArgument, "eventId and workspaceId required")
		}

		ev, err := s.repo.FindNormalizedByEventID(ctx, req.WorkspaceID, req.EventID)
		if err != nil {
			if errors.Is(err, ingestdetailsrepo.ErrNotFound) {
				return nil, status.Error(codes.NotFound, "event not found")
			}
			log.Error().Err(err).Str("eventId", req.EventID).Msg("GetEvent: repo error")
			return nil, status.Error(codes.Internal, "internal error")
		}

		return toEventResponse(ev), nil
	}
}

func (s *EventServiceServer) batchGetEventsHandler() func(srv any, ctx context.Context, dec func(any) error, interceptor grpc.UnaryServerInterceptor) (any, error) {
	return func(srv any, ctx context.Context, dec func(any) error, interceptor grpc.UnaryServerInterceptor) (any, error) {
		log := logger.FromCtx(ctx, "eventservice", "BatchGetEvents")

		var req BatchGetEventsRequest
		if err := dec(&req); err != nil {
			return nil, err
		}
		if req.WorkspaceID == "" || len(req.EventIDs) == 0 {
			return nil, status.Error(codes.InvalidArgument, "workspaceId and eventIds required")
		}
		if len(req.EventIDs) > 100 {
			return nil, status.Error(codes.InvalidArgument, "max 100 eventIds per batch")
		}

		events, err := s.repo.FindNormalizedByEventIDs(ctx, req.WorkspaceID, req.EventIDs)
		if err != nil {
			log.Error().Err(err).Str("workspaceId", req.WorkspaceID).Msg("BatchGetEvents: repo error")
			return nil, status.Error(codes.Internal, "internal error")
		}

		found := make(map[string]bool, len(events))
		responses := make([]*EventResponse, 0, len(events))
		for _, ev := range events {
			found[ev.EventId] = true
			responses = append(responses, toEventResponse(ev))
		}

		var notFound []string
		for _, id := range req.EventIDs {
			if !found[id] {
				notFound = append(notFound, id)
			}
		}

		return &BatchGetEventsResponse{Events: responses, NotFound: notFound}, nil
	}
}

// toEventResponse maps NormalizedEvent → EventResponse (klynx-facing DTO).
func toEventResponse(ev *ingestmod.NormalizedEvent) *EventResponse {
	resp := &EventResponse{
		EventID:    ev.EventId,
		EventType:  ev.EventType,
		OrgID:      ev.Source.WorkspaceId,
		OccurredAt: ev.OccurredAt,
		Source: EventSourceInfo{
			DeviceID:   ev.Source.DeviceId,
			DeviceName: ev.Source.DeviceName,
			DeviceType: ev.Source.DeviceType,
			Vendor:     ev.Source.Vendor,
			OrgID:      ev.Source.WorkspaceId,
		},
		Meta: EventMeta{
			SchemaVersion: ev.Meta.SchemaVersion,
			NormalizedAt:  ev.Meta.NormalizedAt,
			TemplateID:    ev.Meta.TemplateId,
		},
		Payload:     ev.Payload,
		ByAdminArea: map[string]any(ev.ByAdminArea),
	}

	if ev.Location.Lat != 0 || ev.Location.Lng != 0 {
		resp.Location = &EventLocationInfo{
			Lat:  ev.Location.Lat,
			Lng:  ev.Location.Lng,
			Site: ev.Location.Site,
			Zone: ev.Location.Zone,
		}
	}

	if ev.Geo.CountryCode != "" || ev.Geo.AdminName != "" {
		resp.Geo = &EventGeoInfo{
			CountryCode: ev.Geo.CountryCode,
			AdminLevel:  ev.Geo.AdminLevel,
			AdminName:   ev.Geo.AdminName,
			AdminCode:   ev.Geo.AdminCode,
			IdScheme:    ev.Geo.IdScheme,
		}
	}

	if ev.GeoCell.Cell != "" {
		resp.GeoCell = &EventGeoCellInfo{
			Scheme:    ev.GeoCell.Scheme,
			Precision: ev.GeoCell.Precision,
			Cell:      ev.GeoCell.Cell,
		}
	}

	if len(ev.BinaryRefs) > 0 {
		refs := make([]EventBinaryRef, len(ev.BinaryRefs))
		for i, r := range ev.BinaryRefs {
			refs[i] = EventBinaryRef{
				ObjectId:    r.ObjectId,
				Bucket:      r.Bucket,
				ContentType: r.ContentType,
				FieldName:   r.FieldName,
				Kind:        r.Kind,
				Role:        r.Role,
				SourceIndex: r.SourceIndex,
			}
		}
		resp.BinaryRefs = refs
	}

	return resp
}

// Ensure EventResponse is JSON-serialisable (compile-time check via interface satisfaction).
var _ interface{ MarshalJSON() ([]byte, error) } = (*jsonCheckEventResponse)(nil)

type jsonCheckEventResponse struct{ EventResponse }

func (j *jsonCheckEventResponse) MarshalJSON() ([]byte, error) {
	return json.Marshal(j.EventResponse)
}
