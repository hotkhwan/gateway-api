// internal/services/systemsvc/edgesvc/create.go
package edgesvc

import (
	"context"
	"fmt"
	"strings"
	"time"

	"klynx/internal/crypto/secretbox"
	"klynx/internal/logger"
	"klynx/internal/repo/stomongo"
	"klynx/models/systemmod"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

const systemEdgeColl = "system_edges"

func CreateEdge(ctx context.Context, edgeType string, req systemmod.EdgeCreateReq) (primitive.ObjectID, error) {
	log := logger.FromCtx(ctx, "edgesvc", "CreateEdge")

	et, err := normalizeEdgeType(edgeType)
	if err != nil {
		return primitive.NilObjectID, err
	}

	now := time.Now().UTC()

	kr, err := secretbox.LoadKeyringFromEnv()
	if err != nil {
		log.Error().Err(err).Msg("load MASTER_KEYRING_JSON failed")
		return primitive.NilObjectID, err
	}

	var passEnc, apiSecretEnc *secretbox.EncBlob

	if strings.TrimSpace(req.Password) != "" {
		passEnc, err = secretbox.EncryptString(kr, req.Password)
		if err != nil {
			log.Error().Err(err).Msg("encrypt password failed")
			return primitive.NilObjectID, fmt.Errorf("encrypt password failed: %w", err)
		}
	}

	if strings.TrimSpace(req.APISecret) != "" {
		apiSecretEnc, err = secretbox.EncryptString(kr, req.APISecret)
		if err != nil {
			log.Error().Err(err).Msg("encrypt apiSecret failed")
			return primitive.NilObjectID, fmt.Errorf("encrypt apiSecret failed: %w", err)
		}
	}

	doc := systemmod.EdgeDoc{
		Type:     et,
		Username: strings.TrimSpace(req.Username),
		Name:     strings.TrimSpace(req.Name),
		URL:      strings.TrimSpace(req.URL),
		TLS:      req.TLS,

		PassEnc:      passEnc,
		APIKey:       req.APIKey,
		APISecretEnc: apiSecretEnc,

		CreatedAt: now,
		UpdatedAt: now,
	}

	oid, err := stomongo.InsertOne(ctx, systemEdgeColl, doc)
	if err != nil {
		log.Error().Err(err).Msg("insert edge failed")
		return primitive.NilObjectID, err
	}

	log.Info().Str("edgeType", string(et)).Str("id", oid.Hex()).Msg("edge created")
	return oid, nil
}
