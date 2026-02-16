// internal/services/kctrlsvc/alarmResolved
package kctrlsvc

import (
	"context"
	"errors"
	"os"
	"strings"
	"time"

	"github.com/hotkhwan/gateway-api/config"
	"github.com/hotkhwan/gateway-api/models/kctrlmod"
	"github.com/hotkhwan/gateway-api/utils/traceutil"

	"github.com/gofiber/fiber/v2"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

func ResolvedAlarm(ctx context.Context, alarmId string, req kctrlmod.ResolvedAlarmRequest, c *fiber.Ctx) (kctrlmod.ResolvedAlarmResult, error) {
	traceCtx, end, log := traceutil.StartLite(ctx, "github.com/hotkhwan/gateway-api/kctrlsvc", "alarms.ResolvedAlarm", "kctrlsvc", "ResolvedAlarm")
	defer end()

	cctx, cancel := context.WithTimeout(traceCtx, 5*time.Second)
	defer cancel()

	objId, err := primitive.ObjectIDFromHex(alarmId)
	if err != nil {
		return kctrlmod.ResolvedAlarmResult{}, err
	}

	db := config.MongoClient.Database(os.Getenv("MONGO_DB"))
	almColl := db.Collection("kcontrol_alarms")

	// โหลด alarm เพื่อ:
	// - เช็ค exists
	// - ดึง sop ที่ inProgress เพื่อ auto-close (optional)
	var alarm bson.M
	if err := almColl.FindOne(cctx, bson.M{"_id": objId, "isDeleted": bson.M{"$ne": true}}).Decode(&alarm); err != nil {
		if err == mongo.ErrNoDocuments {
			return kctrlmod.ResolvedAlarmResult{}, errors.New("alarm not found")
		}
		return kctrlmod.ResolvedAlarmResult{}, err
	}

	if status, _ := alarm["status"].(string); status == "resolved" {
		return kctrlmod.ResolvedAlarmResult{}, errors.New("alarm already resolved")
	}

	now := time.Now()
	actorId, actorName, actorIp := getActorFromCtx(c)

	description := strings.TrimSpace(req.Description)
	if description == "" {
		description = "ปิดงาน"
	}

	autoClose := true
	if req.AutoCloseOpenSteps != nil {
		autoClose = *req.AutoCloseOpenSteps
	}

	// 1) สร้าง resolve step (done)
	resolveStep := kctrlmod.SopStep{
		StepId:      primitive.NewObjectID(),
		Type:        "resolve",
		Description: description,
		Detail:      req.Detail,
		StepStatus:  "done",
		CreatedAt:   now,
		ActorId:     actorId,
		ActorName:   actorName,
		ActorIp:     actorIp,
	}

	// 2) (optional) auto-close inProgress steps ด้วย "update" events (append-only)
	updateSteps := make([]any, 0, 4)

	if autoClose {
		if sopAny, ok := alarm["sop"].(primitive.A); ok {
			for _, s := range sopAny {
				stepDoc, ok := s.(bson.M)
				if !ok {
					continue
				}
				stepStatus, _ := stepDoc["stepStatus"].(string)
				if stepStatus != "inProgress" {
					continue
				}

				stepIdVal, ok := stepDoc["stepId"].(primitive.ObjectID)
				if !ok {
					continue
				}

				typ, _ := stepDoc["type"].(string)

				u := kctrlmod.SopStep{
					StepId:      primitive.NewObjectID(),
					Type:        "update",
					Description: "auto close step",
					Detail: map[string]any{
						"targetStepId":  stepIdVal.Hex(),
						"targetType":    typ,
						"newStepStatus": "done",
						"note":          "auto closed by resolved",
					},
					StepStatus: "done",
					CreatedAt:  now,
					ActorId:    actorId,
					ActorName:  actorName,
					ActorIp:    actorIp,
				}
				updateSteps = append(updateSteps, u)
			}
		}
	}

	// push order: updateSteps... then resolveStep (หรือกลับกันก็ได้)
	each := make([]any, 0, len(updateSteps)+1)
	each = append(each, updateSteps...)
	each = append(each, resolveStep)

	update := bson.M{
		"$set": bson.M{
			"status":     "resolved",
			"resolvedAt": now,
			"resolvedBy": bson.M{
				"actorId":   actorId,
				"actorName": actorName,
				"actorIp":   actorIp,
			},
			"updatedAt": now,
		},
		"$push": bson.M{
			"sop": bson.M{
				"$each":  each,
				"$slice": -10,
			},
		},
	}

	// guard: กัน concurrent resolve ซ้ำ
	filter := bson.M{
		"_id":       objId,
		"isDeleted": bson.M{"$ne": true},
		"status":    bson.M{"$ne": "resolved"},
	}

	res, err := almColl.UpdateOne(cctx, filter, update)
	if err != nil {
		log.Error().Err(err).Msg("resolved update failed")
		return kctrlmod.ResolvedAlarmResult{}, err
	}
	if res.MatchedCount == 0 {
		return kctrlmod.ResolvedAlarmResult{}, errors.New("alarm not found or already resolved")
	}

	return kctrlmod.ResolvedAlarmResult{
		AlarmId:    alarmId,
		ResolvedAt: now,
		Status:     "resolved",
	}, nil
}
