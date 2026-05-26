// internal/kafka/deliverycons/schemaadapter.go
package deliverycons

import (
	"github.com/hotkhwan/gateway-api/internal/eventschema"
	"github.com/hotkhwan/gateway-api/models/ingestmod"
)

// FromEventSchema adapts the canonical cross-service eventschema.NormalizedEvent
// shape (carried on gw.events.normalized.v1) into the internal
// ingestmod.NormalizedEvent shape that existing delivery helpers operate on.
//
// Introduced by the Wide slice (delivery-topic-unification) so the v2
// delivery consumer can subscribe to gw.events.normalized.v1 directly
// without refactoring classification.Apply / renderContext / DLQ insert
// / match bag builders. Pure data transform; no I/O.
//
// Field mapping (eventschema.NormalizedEvent → ingestmod.NormalizedEvent):
//
//	EventID                → EventId
//	OrgID                  → TenantId
//	SourceType             → EventType
//	SourceCategory         → EventCategory
//	SourceAction           → EventAction
//	SourceFamily           → Source.DeviceType (kept per narrow-v2 convention);
//	                         also mirrored to Source.Vendor when Vendor is unset
//	WorkspaceID            → Source.WorkspaceId (ingestmod has no root workspaceId)
//	OccurredAt             → OccurredAt
//	ReceivedAt             → Meta.NormalizedAt
//	SchemaVersion          → Meta.SchemaVersion
//	TemplateID             → Meta.TemplateId
//	TraceID                → Meta.TraceId
//
//	Source.DeviceID        → Source.DeviceId
//	Source.DeviceMgmtID    → Source.DeviceMgmtId  (new ingestmod field)
//	Source.SN              → Source.SN            (new ingestmod field)
//	Source.EdgeName        → Source.EdgeName      (new ingestmod field)
//	Source.OrgID           → Source.OrgId         (new ingestmod field)
//	Source.WorkspaceID     → Source.WorkspaceId
//	Source.SourceType      → (duplicates EventType; not stored again)
//	Source.SourceFamily    → (duplicates Source.DeviceType; not stored again)
//
//	Location.*             → Location.* (Lat, Lng, Site, Zone)
//	Geo.*                  → Geo.* (CountryCode, AdminLevel, AdminName, AdminCode, IdScheme)
//	GeoCell.*              → GeoCell.* (Scheme, Precision, Cell)
//	ByAdminArea            → ByAdminArea
//	Payload                → Payload
//	BinaryRefs[i]          → BinaryRefs[i] (FieldName left empty — no counterpart on eventschema)
//	RawPayloadRef          → (not carried on ingestmod; dropped)
func FromEventSchema(src *eventschema.NormalizedEvent) *ingestmod.NormalizedEvent {
	if src == nil {
		return nil
	}

	out := &ingestmod.NormalizedEvent{
		EventId:       src.EventID,
		TenantId:      src.OrgID,
		EventType:     src.SourceType,
		EventCategory: src.SourceCategory,
		EventAction:   src.SourceAction,
		// Severity + EventClass — producer-stamped via normalizedcons.buildBridgeEvent
		// (Layer C, klynx-api docs/contracts/event-severity-forwarding.md §6).
		// When non-empty the delivery consumer reuses the value verbatim and
		// filter.go skips re-classification; the existing fallback rule re-run
		// in filter.go covers empty wire severity (legacy or no rule matched).
		EventSeverity: src.Severity,
		EventClass:    src.EventClass,
		OccurredAt:    src.OccurredAt,
		Payload:       src.Payload,
		Meta: ingestmod.NormalizationMeta{
			SchemaVersion: src.SchemaVersion,
			TraceId:       src.TraceID,
			TemplateId:    src.TemplateID,
			NormalizedAt:  src.ReceivedAt,
		},
	}

	// Source — merge root SourceFamily + nested Source block.
	out.Source = ingestmod.SourceInfo{
		DeviceType:  src.SourceFamily,
		Vendor:      src.SourceFamily, // convenience alias; legacy code sometimes reads Vendor
		WorkspaceId: src.WorkspaceID,
	}
	if src.Source != nil {
		out.Source.DeviceId = src.Source.DeviceID
		out.Source.DeviceMgmtId = src.Source.DeviceMgmtID
		out.Source.SN = src.Source.SN
		out.Source.EdgeName = src.Source.EdgeName
		out.Source.OrgId = src.Source.OrgID
		// Prefer nested workspaceId when both present; they should agree.
		if src.Source.WorkspaceID != "" {
			out.Source.WorkspaceId = src.Source.WorkspaceID
		}
	}

	if src.Location != nil {
		out.Location = ingestmod.LocationInfo{
			Lat:  src.Location.Lat,
			Lng:  src.Location.Lng,
			Site: src.Location.Site,
			Zone: src.Location.Zone,
		}
	}
	if src.Geo != nil {
		out.Geo = ingestmod.GeoInfo{
			CountryCode: src.Geo.CountryCode,
			AdminLevel:  src.Geo.AdminLevel,
			AdminName:   src.Geo.AdminName,
			AdminCode:   src.Geo.AdminCode,
			IdScheme:    src.Geo.IdScheme,
		}
	}
	if src.GeoCell != nil {
		out.GeoCell = ingestmod.GeoCellInfo{
			Scheme:    src.GeoCell.Scheme,
			Precision: src.GeoCell.Precision,
			Cell:      src.GeoCell.Cell,
		}
	}
	if src.ByAdminArea != nil {
		out.ByAdminArea = ingestmod.ByAdminAreaInfo(src.ByAdminArea)
	}
	if len(src.BinaryRefs) > 0 {
		out.BinaryRefs = make([]ingestmod.BinaryRef, 0, len(src.BinaryRefs))
		for _, b := range src.BinaryRefs {
			out.BinaryRefs = append(out.BinaryRefs, ingestmod.BinaryRef{
				ObjectId:    b.ObjectID,
				Bucket:      b.Bucket,
				ContentType: b.ContentType,
				Kind:        b.Kind,
				Role:        b.Role,
				SourceIndex: b.SourceIndex,
			})
		}
	}

	return out
}
