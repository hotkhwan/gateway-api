// internal/services/authzsvc/publisher.go
package authzsvc

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/hotkhwan/gateway-api/internal/kafka"
	"github.com/hotkhwan/gateway-api/models/authzmod"
	"github.com/hotkhwan/gateway-api/utils/traceutil"
)

// PublishAddUserToGroup → ใช้ Kafka Event (REST Compatible)
func PublishAddUserToGroup(ctx context.Context, userID, groupID string) error {
	ctx, end, log := traceutil.StartLite(
		ctx,
		"github.com/hotkhwan/gateway-api/authzsvc",
		"authorization.PublishAddUserToGroup",
		"authzsvc", "PublishAddUserToGroup",
	)
	defer end()

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	evt := authzmod.RelationshipEvent{
		Type:      "group_member_added",
		GroupID:   groupID,
		UserID:    userID,
		Timestamp: time.Now().UnixMilli(),
	}

	topic := os.Getenv("KAFKA_AUTHZ_TOPIC")
	if topic == "" {
		topic = "authz.relationship.updated"
	}

	headers := map[string]string{
		"event":           "authz.group_member.added",
		"schema":          "authz/relationship/1",
		"idempotency_key": fmt.Sprintf("authz.group_member.added:%s:%s", groupID, userID),
		// "source" / "trace_id" จะถูกเติมโดย wrapper จาก ENV/ctx ถ้ายังไม่มี
	}

	key := groupID // คง order ต่อ group
	log.Debug().Interface("event", evt).Str("topic", topic).Str("key", key).Msg("📤 Publishing group_member_added")
	return kafka.PublishEventTo(ctx, topic, key, evt, headers)
}

// PublishResourceRelocated → ใช้ Kafka Event (REST Compatible)
func PublishResourceRelocated(ctx context.Context, resourceID, newGroupID string) error {
	ctx, end, log := traceutil.StartLite(
		ctx,
		"github.com/hotkhwan/gateway-api/authzsvc",
		"authorization.PublishResourceRelocated",
		"authzsvc", "PublishResourceRelocated",
	)
	defer end()

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	evt := authzmod.RelationshipEvent{
		Type:       "resource_relocated",
		ResourceID: resourceID,
		NewGroupID: newGroupID,
		Timestamp:  time.Now().UnixMilli(),
	}

	topic := os.Getenv("KAFKA_AUTHZ_TOPIC")
	if topic == "" {
		topic = "authz.relationship.updated"
	}

	headers := map[string]string{
		"event":           "authz.resource.relocated",
		"schema":          "authz/relationship/1",
		"idempotency_key": fmt.Sprintf("authz.resource.relocated:%s:%s", resourceID, newGroupID),
	}

	key := resourceID // คง order ต่อ resource
	log.Debug().Interface("event", evt).Str("topic", topic).Str("key", key).Msg("📤 Publishing resource_relocated")
	return kafka.PublishEventTo(ctx, topic, key, evt, headers)
}
