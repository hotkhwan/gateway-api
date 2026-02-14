// internal/services/systemsvc/edgesvc/update.go
package edgesvc

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"klynx/config"
	"klynx/internal/crypto/secretbox"
	"klynx/internal/logger"
	"klynx/models/systemmod"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func UpdateEdge(ctx context.Context, id string, req systemmod.EdgeUpdateReq) error {
	log := logger.FromCtx(ctx, "edgesvc", "UpdateEdge")

	oid, err := primitive.ObjectIDFromHex(strings.TrimSpace(id))
	if err != nil {
		return errors.New("invalid id")
	}

	// ✅ ไม่ให้ update ตัวที่ถูกลบไปแล้ว
	filter := bson.M{"_id": oid, "isDeleted": bson.M{"$ne": true}}

	set := bson.M{
		"updatedAt": time.Now().UTC(),
	}
	unset := bson.M{} // เผื่ออนาคตใช้ล้าง field

	// plain fields
	if req.Username != nil {
		set["username"] = strings.TrimSpace(*req.Username)
	}
	if req.Name != nil {
		set["name"] = strings.TrimSpace(*req.Name)
	}
	if req.URL != nil {
		set["url"] = strings.TrimSpace(*req.URL)
	}
	if req.TLS != nil {
		set["tls"] = *req.TLS
	}
	if req.APIKey != nil {
		set["apiKey"] = *req.APIKey
	}

	// secrets -> encrypt
	needKR := false
	if req.Password != nil && strings.TrimSpace(*req.Password) != "" {
		needKR = true
	}
	if req.APISecret != nil && strings.TrimSpace(*req.APISecret) != "" {
		needKR = true
	}

	var kr *secretbox.JSONKeyring
	if needKR {
		kr, err = secretbox.LoadKeyringFromEnv()
		if err != nil {
			log.Error().Err(err).Msg("load MASTER_KEYRING_JSON failed")
			return err
		}
	}

	if req.Password != nil {
		p := strings.TrimSpace(*req.Password)
		if p == "" {
			// ถ้าส่ง password="" ถือว่าต้องการล้าง
			unset["passEnc"] = 1
		} else {
			enc, e := secretbox.EncryptString(kr, p)
			if e != nil {
				log.Error().Err(e).Msg("encrypt password failed")
				return fmt.Errorf("encrypt password failed: %w", e)
			}
			set["passEnc"] = enc
		}
	}

	if req.APISecret != nil {
		s := strings.TrimSpace(*req.APISecret)
		if s == "" {
			unset["apiSecretEnc"] = 1
		} else {
			enc, e := secretbox.EncryptString(kr, s)
			if e != nil {
				log.Error().Err(e).Msg("encrypt apiSecret failed")
				return fmt.Errorf("encrypt apiSecret failed: %w", e)
			}
			set["apiSecretEnc"] = enc
		}
	}

	update := bson.M{"$set": set}
	if len(unset) > 0 {
		update["$unset"] = unset
	}

	res, err := config.DB.Collection(systemEdgeColl).UpdateOne(ctx, filter, update)
	if err != nil {
		log.Error().Err(err).Msg("update edge failed")
		return err
	}
	if res.MatchedCount == 0 {
		return errors.New("edge not found")
	}

	log.Info().Str("id", oid.Hex()).Msg("edge updated")
	return nil
}
