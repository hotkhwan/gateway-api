// internal/services/devicemgmtsvc/service.go
package devicemgmtsvc

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/hotkhwan/gateway-api/config"
	"github.com/hotkhwan/gateway-api/internal/repo/cachedevicemgmt"
	"github.com/hotkhwan/gateway-api/internal/repo/devicemgmtrepo"
	"github.com/hotkhwan/gateway-api/models/ingestmod"
	"github.com/hotkhwan/gateway-api/utils/traceutil"
	"go.mongodb.org/mongo-driver/bson"
)

type DeviceManagementService struct {
	repo *devicemgmtrepo.DeviceManagementRepo
}

func NewDeviceManagementService(repo *devicemgmtrepo.DeviceManagementRepo) *DeviceManagementService {
	if repo == nil {
		panic("DeviceManagementService: repo required")
	}
	return &DeviceManagementService{repo: repo}
}

func (s *DeviceManagementService) Create(ctx context.Context, d *ingestmod.DeviceManagement) error {
	ctx, end, _ := traceutil.StartLite(ctx, "gateway.devicemgmtsvc", "Create", "devicemgmtsvc", "Create")
	defer end()

	now := time.Now().UTC()
	d.DeviceMgmtId = uuid.NewString()
	d.CreatedAt = now
	d.UpdatedAt = now
	if err := s.repo.Insert(ctx, d); err != nil {
		return err
	}
	publishDevicesChanged(ctx, d, "create")
	return nil
}

func (s *DeviceManagementService) Get(ctx context.Context, tenantId, orgId, deviceMgmtId string) (*ingestmod.DeviceManagement, error) {
	ctx, end, _ := traceutil.StartLite(ctx, "gateway.devicemgmtsvc", "Get", "devicemgmtsvc", "Get")
	defer end()

	return s.repo.FindById(ctx, tenantId, orgId, deviceMgmtId)
}

func (s *DeviceManagementService) List(ctx context.Context, tenantId, orgId string, page, perPage int) ([]*ingestmod.DeviceManagement, error) {
	ctx, end, _ := traceutil.StartLite(ctx, "gateway.devicemgmtsvc", "List", "devicemgmtsvc", "List")
	defer end()

	return s.repo.List(ctx, tenantId, orgId, page, perPage)
}

func (s *DeviceManagementService) Update(ctx context.Context, tenantId, orgId, deviceMgmtId string, update bson.M) error {
	ctx, end, _ := traceutil.StartLite(ctx, "gateway.devicemgmtsvc", "Update", "devicemgmtsvc", "Update")
	defer end()

	update["updatedAt"] = time.Now().UTC()
	if err := s.repo.Update(ctx, tenantId, orgId, deviceMgmtId, update); err != nil {
		return err
	}
	// Invalidate cache — fetch record to get entity info
	d, err := s.repo.FindById(ctx, tenantId, orgId, deviceMgmtId)
	if err == nil && d != nil {
		cachedevicemgmt.Invalidate(ctx, tenantId, orgId, d.SourceFamily, d.EntityType, d.EntityId)
		publishDevicesChanged(ctx, d, "update")
	}
	return nil
}

// Resolve looks up enrichment data for a source entity (hot path, cache-first).
func (s *DeviceManagementService) Resolve(ctx context.Context, tenantId, orgId, sourceFamily, entityType, entityId string) *ingestmod.DeviceManagement {
	if cached := cachedevicemgmt.Get(ctx, tenantId, orgId, sourceFamily, entityType, entityId); cached != nil {
		return cached
	}
	d, err := s.repo.FindByEntity(ctx, tenantId, orgId, sourceFamily, entityType, entityId)
	if err != nil {
		return nil
	}
	cachedevicemgmt.Set(ctx, d)
	return d
}

// publishDevicesChanged fires a non-blocking Kafka publish to gw.devices.changed.v1.
// Non-fatal: errors are logged only.
//
// Wire schema matches klynx-api's `eventbridge.DeviceChangedEvent` consumer
// (dahua-camera-event-ingest.md §6.1): the join field is `remoteDeviceId`
// (= deviceId = camId), `changeType` is the past-tense enum, and `orgId`/
// `gwWorkspaceId` both carry the workspace. The legacy {entityId, action,
// workspaceId} shape left the consumer's RemoteDeviceID empty so SyncFromGW
// early-returned — no projection, no metadata enrichment.
func publishDevicesChanged(_ context.Context, d *ingestmod.DeviceManagement, action string) {
	topic := config.TopicEnv("KAFKA_TOPIC_GW_DEVICES", "gw.devices.changed.v1")
	changeType := mapChangeType(action)
	payload, _ := json.Marshal(devicesChangedPayload(d, changeType))
	headers := map[string]string{"workspaceId": d.WorkspaceId, "changeType": changeType}
	go func() {
		_ = config.SendToKafkaWithCtx(context.Background(), topic, d.WorkspaceId, payload, headers)
	}()
}

// devicesChangedPayload builds the gw.devices.changed.v1 wire payload that the
// klynx-api `eventbridge.DeviceChangedEvent` consumer decodes. Pure + testable.
func devicesChangedPayload(d *ingestmod.DeviceManagement, changeType string) map[string]any {
	// Join key: prefer the canonical DeviceId (= camId for provisioned cameras);
	// fall back to entityId for reactive channel-keyed records that carry no alias.
	remoteDeviceId := d.DeviceId
	if remoteDeviceId == "" {
		remoteDeviceId = d.EntityId
	}
	return map[string]any{
		"eventId":        uuid.NewString(),
		"syncOrigin":     "gw",
		"orgId":          d.WorkspaceId, // klynx camera.orgId == gw workspaceId
		"gwWorkspaceId":  d.WorkspaceId,
		"remoteDeviceId": remoteDeviceId,
		"deviceId":       d.DeviceId,
		"deviceMgmtId":   d.DeviceMgmtId,
		"tenantId":       d.TenantId,
		"sourceFamily":   d.SourceFamily,
		"entityType":     d.EntityType,
		"entityId":       d.EntityId,
		"changeType":     changeType,
		"name":           d.Name,
		"lat":            d.Lat,
		"lng":            d.Lng,
		"status":         true,
		"occurredAt":     time.Now().UTC(),
	}
}

// mapChangeType maps the internal action verb to the klynx-api DeviceChangedEvent
// changeType enum (created | updated | deleted).
func mapChangeType(action string) string {
	switch action {
	case "create", "created":
		return "created"
	case "update", "updated":
		return "updated"
	case "delete", "deleted":
		return "deleted"
	default:
		return action
	}
}
