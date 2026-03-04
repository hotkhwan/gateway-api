// internal/services/kctrlsvc/sop.go
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
)

func AppendAlarmSop(ctx context.Context, alarmId string, req kctrlmod.SopStepRequest, c *fiber.Ctx) error {
	traceCtx, end, _ := traceutil.StartLite(ctx, "github.com/hotkhwan/gateway-api/kctrlsvc", "alarms.AppendAlarmSop", "kctrlsvc", "AppendAlarmSop")
	defer end()

	cctx, cancel := context.WithTimeout(traceCtx, 5*time.Second)
	defer cancel()

	objId, err := primitive.ObjectIDFromHex(alarmId)
	if err != nil {
		return err
	}

	req.Type = strings.TrimSpace(req.Type)
	req.Description = strings.TrimSpace(req.Description)

	if req.Type == "" {
		return errors.New("type is required")
	}
	if req.Description == "" {
		return errors.New("description is required")
	}

	stepStatus := strings.TrimSpace(req.StepStatus)
	if stepStatus == "" {
		stepStatus = "inProgress"
	}

	switch stepStatus {
	case "inProgress", "done", "failed":
	default:
		return errors.New("invalid stepStatus")
	}

	now := time.Now()
	actorId, actorName, actorIp := getActorFromCtx(c)

	step := kctrlmod.SopStep{
		StepId:      primitive.NewObjectID(),
		Type:        req.Type,
		Description: req.Description,
		Detail:      req.Detail,
		StepStatus:  stepStatus,
		CreatedAt:   now,
		ActorId:     actorId,
		ActorName:   actorName,
		ActorIp:     actorIp,
	}

	db := config.MongoClient.Database(os.Getenv("MONGO_DB"))
	almColl := db.Collection("kcontrol_alarms")

	update := bson.M{
		"$set": bson.M{
			"updatedAt": now,
		},
		"$push": bson.M{
			"sop": bson.M{
				"$each":  []any{step},
				"$slice": -10,
			},
		},
	}

	res, err := almColl.UpdateOne(
		cctx,
		bson.M{"_id": objId, "isDeleted": bson.M{"$ne": true}},
		update,
	)
	if err != nil {
		return err
	}
	if res.MatchedCount == 0 {
		return errors.New("alarm not found")
	}

	return nil
}
