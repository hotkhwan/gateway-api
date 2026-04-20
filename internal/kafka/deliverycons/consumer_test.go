package deliverycons

import (
	"encoding/json"
	"testing"

	"github.com/hotkhwan/gateway-api/models/ingestmod"
	"github.com/segmentio/kafka-go"
)

func TestResolveNormalizedEventFields_FromHeaders(t *testing.T) {
	t.Parallel()

	event := &ingestmod.NormalizedEvent{
		EventId: "evt-1",
	}
	msg := kafka.Message{
		Value: []byte(`{"eventId":"evt-1","source":{}}`),
		Headers: []kafka.Header{
			{Key: "templateId", Value: []byte("auto:aibox.generalDetect.v1")},
			{Key: "workspaceId", Value: []byte("ws-1")},
			{Key: "tenantId", Value: []byte("tenant-1")},
		},
	}

	sources := resolveNormalizedEventFields(event, msg)

	if got := event.Meta.TemplateId; got != "auto:aibox.generalDetect.v1" {
		t.Fatalf("templateId = %q, want %q", got, "auto:aibox.generalDetect.v1")
	}
	if got := event.Source.WorkspaceId; got != "ws-1" {
		t.Fatalf("workspaceId = %q, want %q", got, "ws-1")
	}
	if got := event.TenantId; got != "tenant-1" {
		t.Fatalf("tenantId = %q, want %q", got, "tenant-1")
	}
	if sources.TemplateID != "header" {
		t.Fatalf("template source = %q, want %q", sources.TemplateID, "header")
	}
	if sources.WorkspaceID != "header" {
		t.Fatalf("workspace source = %q, want %q", sources.WorkspaceID, "header")
	}
	if sources.TenantID != "header" {
		t.Fatalf("tenant source = %q, want %q", sources.TenantID, "header")
	}
}

func TestResolveNormalizedEventFields_PrefersJSONBeforeHeaders(t *testing.T) {
	t.Parallel()

	body, err := json.Marshal(map[string]any{
		"eventId":     "evt-2",
		"templateId":  "template-json",
		"tenantId":    "tenant-json",
		"workspaceId": "ws-root",
		"source": map[string]any{
			// orgId is intentionally present to assert we do NOT resolve
			// workspaceId from source.orgId (klynxOrgId ≠ phibek workspaceId).
			"orgId": "klynx-org-not-workspace",
		},
	})
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}

	event := &ingestmod.NormalizedEvent{EventId: "evt-2"}
	msg := kafka.Message{
		Value: body,
		Headers: []kafka.Header{
			{Key: "templateId", Value: []byte("template-header")},
			{Key: "workspaceId", Value: []byte("ws-header")},
			{Key: "tenantId", Value: []byte("tenant-header")},
		},
	}

	sources := resolveNormalizedEventFields(event, msg)

	if got := event.Meta.TemplateId; got != "template-json" {
		t.Fatalf("templateId = %q, want %q", got, "template-json")
	}
	if got := event.Source.WorkspaceId; got != "ws-root" {
		t.Fatalf("workspaceId = %q, want %q", got, "ws-root")
	}
	if got := event.TenantId; got != "tenant-json" {
		t.Fatalf("tenantId = %q, want %q", got, "tenant-json")
	}
	if sources.TemplateID != "rootJson" {
		t.Fatalf("template source = %q, want %q", sources.TemplateID, "rootJson")
	}
	if sources.WorkspaceID != "rootJson" {
		t.Fatalf("workspace source = %q, want %q", sources.WorkspaceID, "rootJson")
	}
	if sources.TenantID != "rootJson" {
		t.Fatalf("tenant source = %q, want %q", sources.TenantID, "rootJson")
	}
}

// TestResolveNormalizedEventFields_WorkspaceIdFromHeaderWhenOnlyOrgIdInPayload
// guards the klynx-api minimal republish case: source.workspaceId is missing
// and only source.orgId + root orgId are present (klynx orgId, not workspaceId).
// The delivery consumer must fall through to the Kafka header "workspaceId"
// which upstream publishers set explicitly to the authoritative phibek workspaceId.
func TestResolveNormalizedEventFields_WorkspaceIdFromHeaderWhenOnlyOrgIdInPayload(t *testing.T) {
	t.Parallel()

	body, err := json.Marshal(map[string]any{
		"eventId":  "evt-4",
		"tenantId": "klynx-org-id",
		"orgId":    "klynx-org-id",
		"source": map[string]any{
			"deviceId": "51",
			"orgId":    "klynx-org-id",
		},
	})
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}

	event := &ingestmod.NormalizedEvent{EventId: "evt-4"}
	msg := kafka.Message{
		Value: body,
		Headers: []kafka.Header{
			{Key: "workspaceId", Value: []byte("phibek-workspace-id")},
		},
	}

	sources := resolveNormalizedEventFields(event, msg)

	if got := event.Source.WorkspaceId; got != "phibek-workspace-id" {
		t.Fatalf("workspaceId = %q, want %q (must not fall back to orgId)", got, "phibek-workspace-id")
	}
	if sources.WorkspaceID != "header" {
		t.Fatalf("workspace source = %q, want %q", sources.WorkspaceID, "header")
	}
}

func TestResolveNormalizedEventFields_PreservesPayloadValues(t *testing.T) {
	t.Parallel()

	event := &ingestmod.NormalizedEvent{
		EventId:  "evt-3",
		TenantId: "tenant-payload",
		Source: ingestmod.SourceInfo{
			WorkspaceId: "ws-payload",
		},
		Meta: ingestmod.NormalizationMeta{
			TemplateId: "template-payload",
		},
	}
	msg := kafka.Message{
		Value: []byte(`{"eventId":"evt-3","tenantId":"tenant-json","workspaceId":"ws-json","templateId":"template-json"}`),
		Headers: []kafka.Header{
			{Key: "templateId", Value: []byte("template-header")},
			{Key: "workspaceId", Value: []byte("ws-header")},
			{Key: "tenantId", Value: []byte("tenant-header")},
		},
	}

	sources := resolveNormalizedEventFields(event, msg)

	if got := event.Meta.TemplateId; got != "template-payload" {
		t.Fatalf("templateId = %q, want %q", got, "template-payload")
	}
	if got := event.Source.WorkspaceId; got != "ws-payload" {
		t.Fatalf("workspaceId = %q, want %q", got, "ws-payload")
	}
	if got := event.TenantId; got != "tenant-payload" {
		t.Fatalf("tenantId = %q, want %q", got, "tenant-payload")
	}
	if sources.TemplateID != "meta" {
		t.Fatalf("template source = %q, want %q", sources.TemplateID, "meta")
	}
	if sources.WorkspaceID != "payload" {
		t.Fatalf("workspace source = %q, want %q", sources.WorkspaceID, "payload")
	}
	if sources.TenantID != "payload" {
		t.Fatalf("tenant source = %q, want %q", sources.TenantID, "payload")
	}
}
