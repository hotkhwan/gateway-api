// internal/services/kctrlsvc/control.go
package kctrlsvc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"

	"klynx/config"
	"klynx/internal/mqtt/kcontrolmsg"
	"klynx/models/kctrlmod"
	"klynx/utils/traceutil"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func SendMessage(ctx context.Context, msg kctrlmod.ControlMessage) error {
	_, end, log := traceutil.StartLite(ctx, "klynx/kctrlsvc", "alarms.SendMessage", "kctrlsvc", "SendMessage")
	defer end()

	if msg.Topic == "" || msg.HwID == nil {
		return errors.New("missing topic or hwId")
	}
	if msg.Timestamp.IsZero() {
		msg.Timestamp = time.Now()
	}

	// validate
	validOutputs := map[string]bool{"OUT_1": true, "OUT_2": true}
	if msg.SetOutput != "" && !validOutputs[msg.SetOutput] {
		return fmt.Errorf("invalid setOutput: %s (allowed: OUT_1, OUT_2)", msg.SetOutput)
	}
	validTypes := map[string]bool{"ONCE": true, "HIGH": true, "LOW": true}
	if msg.Type != "" && !validTypes[msg.Type] {
		return fmt.Errorf("invalid type: %s (allowed: ONCE, HIGH, LOW)", msg.Type)
	}
	if msg.SetAlarmInterval < 0 || msg.SetHealthInterval < 0 {
		return fmt.Errorf("intervals must be >= 0")
	}

	// payload เฉพาะคีย์ที่มีค่า
	m := map[string]any{
		"hwId": msg.HwID,
	}
	if msg.Message != "" {
		m["message"] = msg.Message
	}
	if msg.SetHealthInterval > 0 {
		m["setHealthInterval"] = msg.SetHealthInterval
	}
	if msg.SetAlarmInterval > 0 {
		m["setAlarmInterval"] = msg.SetAlarmInterval
	}
	if msg.IntervalIN1 > 0 {
		m["interval_IN_1"] = msg.IntervalIN1
	}
	if msg.IntervalIN2 > 0 {
		m["interval_IN_2"] = msg.IntervalIN2
	}
	if msg.IntervalIN3 > 0 {
		m["interval_IN_3"] = msg.IntervalIN3
	}
	if msg.IntervalIN4 > 0 {
		m["interval_IN_4"] = msg.IntervalIN4
	}
	if msg.SetOutput != "" {
		m["setOutput"] = msg.SetOutput
	}
	if msg.Type != "" {
		m["type"] = msg.Type
	}

	// setMqtt + persist
	if msg.SetMqtt {
		m["setMqtt"] = true
		if msg.Host != "" {
			m["host"] = msg.Host
		}
		if msg.Port > 0 {
			m["port"] = msg.Port
		}
		if msg.User != "" {
			m["user"] = msg.User
		}
		if msg.Pass != "" {
			m["pass"] = msg.Pass
		}
		if msg.Pem != "" {
			m["pem"] = msg.Pem
		}
		if msg.Persist != nil {
			m["persist"] = *msg.Persist
		}
	}

	if msg.SetOta {
		m["setOta"] = true
		if msg.Url != "" {
			m["url"] = msg.Url
		}
	}

	if msg.ClearConfig {
		m["clearConfig"] = true
	}

	payload, err := json.Marshal(m)
	if err != nil {
		return err
	}

	log.Debug().
		Str("topic", msg.Topic).
		Interface("hwId", msg.HwID).
		RawJSON("payload", payload).
		Msg("📤 [Kcontrol] Publishing control message to MQTT")

	return kcontrolmsg.PublishToMQTT(msg.Topic, string(payload))
}

func AckDeviceAlarm(ctx context.Context, id string, payload kctrlmod.EventMessage) error {
	traceCtx, end, log := traceutil.StartLite(ctx, "klynx/kctrlsvc", "alarms.AckDeviceAlarm", "kctrlsvc", "AckDeviceAlarm")
	defer end()
	cctx, cancel := context.WithTimeout(traceCtx, 5*time.Second)
	defer cancel()

	if payload.HwID == "" {
		return errors.New("missing hwId")
	}

	// payload สำหรับ MQTT
	msg := map[string]interface{}{
		"hwId":    payload.HwID,
		"message": "off",
	}
	eventBytes, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	// ส่งไป MQTT topic
	if err := kcontrolmsg.PublishToMQTT("kcontrol/alarms", string(eventBytes)); err != nil {
		log.Error().
			Err(err).
			Str("topic", "kcontrol.alarms").
			Str("hwId", payload.HwID).
			Msg("❌ MQTT publish failed")
		return err
	}
	log.Debug().
		Str("topic", "kcontrol.alarms").
		Str("hwId", payload.HwID).
		RawJSON("payload", eventBytes).
		Msg("📡 MQTT alarm ack sent")

	objID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return err
	}

	// Update MongoDB
	coll := config.MongoClient.Database(os.Getenv("MONGO_DB")).Collection("kcontrol")

	update := bson.M{
		"$set": bson.M{
			"alarm":          false,
			"dateTimeUpdate": time.Now(),
		},
	}
	if _, err := coll.UpdateOne(cctx, bson.M{"_id": objID}, update); err != nil {
		log.Error().
			Err(err).
			Str("mongo_id", id).
			Msg("❌ MongoDB update failed")
		return err
	}

	log.Debug().
		Str("mongo_id", id).
		Msg("✅ MongoDB alarm ack updated")
	return nil
}
