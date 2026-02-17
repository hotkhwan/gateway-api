// internal/services/authzsvc/authzDebugDelete.gน
package authzsvc

import (
	"context"
	"errors"
	"time"

	"github.com/hotkhwan/gateway-api/config"
	"github.com/hotkhwan/gateway-api/internal/gateways/authzgw"
	"github.com/hotkhwan/gateway-api/utils/traceutil"
)

type ResetAllTuplesResult struct {
	TenantId   string `json:"tenantId"`
	EntityType string `json:"entityType"`
	Deleted    int    `json:"deleted"`
}

type ResetUserTuplesResult struct {
	TenantId string `json:"tenantId"`
	UserId   string `json:"userId"`
	Deleted  int    `json:"deleted"`
}

func ResetPermifyTuplesAll(ctx context.Context, tenantId string, entityType string) (*ResetAllTuplesResult, error) {
	ctx, end, log := traceutil.StartLite(
		ctx,
		"github.com/hotkhwan/gateway-api/authzsvc",
		"authz.ResetPermifyTuplesAll",
		"authzsvc", "ResetPermifyTuplesAll",
	)
	defer end()

	if tenantId == "" {
		return nil, errors.New("tenantId is required")
	}
	if entityType == "" {
		return nil, errors.New("entityType is required")
	}

	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	client := authzgw.NewPermifyRestDebugClient()

	deleted := 0
	continuousToken := ""

	for {
		readRes, err := client.ReadTuples(ctx, tenantId, authzgw.ReadTuplesRequest{
			EntityType:      entityType,
			PageSize:        200,
			ContinuousToken: continuousToken,
			SchemaVersion:   "latest",
			Depth:           50,
		})
		if err != nil {
			log.Error().Err(err).Msg("permify read tuples failed")
			return nil, err
		}
		if len(readRes.Tuples) == 0 {
			break
		}

		// ✅ delete ทีละ tuple (ชัวร์สุด เพราะบางเวอร์ชัน require entity.id)
		for _, t := range readRes.Tuples {
			err := client.DeleteTuples(ctx, tenantId, authzgw.DeleteTuplesRequest{
				EntityType:    t.Entity.Type,
				EntityId:      t.Entity.Id,
				Relation:      t.Relation,
				SubjectType:   t.Subject.Type,
				SubjectId:     t.Subject.Id,
				SchemaVersion: "latest",
				Depth:         50,
			})
			if err != nil {
				log.Error().Err(err).Msg("permify delete tuples failed")
				return nil, err
			}
			deleted++
		}

		if readRes.ContinuousToken == "" || readRes.ContinuousToken == continuousToken {
			break
		}
		continuousToken = readRes.ContinuousToken
	}

	return &ResetAllTuplesResult{
		TenantId:   tenantId,
		EntityType: entityType,
		Deleted:    deleted,
	}, nil
}

func ResetPermifyTuplesByUser(ctx context.Context, tenantId string, userId string) (*ResetUserTuplesResult, error) {
	ctx, end, log := traceutil.StartLite(
		ctx,
		"github.com/hotkhwan/gateway-api/authzsvc",
		"authz.ResetPermifyTuplesByUser",
		"authzsvc", "ResetPermifyTuplesByUser",
	)
	defer end()

	if tenantId == "" {
		tenantId = config.PermifyTenantID
	}
	if userId == "" {
		return nil, errors.New("userId is required")
	}

	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	client := authzgw.NewPermifyRestDebugClient()

	deleted := 0
	continuousToken := ""

	// ✅ อ่านเฉพาะ membership org ของ user แล้วลบทีละ tuple
	for {
		readRes, err := client.ReadTuples(ctx, tenantId, authzgw.ReadTuplesRequest{
			EntityType:      "organization",
			Relation:        "member",
			SubjectType:     "user",
			SubjectId:       userId,
			PageSize:        200,
			ContinuousToken: continuousToken,
			SchemaVersion:   "latest",
			Depth:           50,
		})
		if err != nil {
			log.Error().Err(err).Msg("permify read tuples failed")
			return nil, err
		}

		if len(readRes.Tuples) == 0 {
			break
		}

		for _, t := range readRes.Tuples {
			err := client.DeleteTuples(ctx, tenantId, authzgw.DeleteTuplesRequest{
				EntityType:    t.Entity.Type,
				EntityId:      t.Entity.Id,
				Relation:      t.Relation,
				SubjectType:   t.Subject.Type,
				SubjectId:     t.Subject.Id,
				SchemaVersion: "latest",
				Depth:         50,
			})
			if err != nil {
				log.Error().Err(err).Msg("permify delete tuples failed")
				return nil, err
			}
			deleted++
		}

		if readRes.ContinuousToken == "" || readRes.ContinuousToken == continuousToken {
			break
		}
		continuousToken = readRes.ContinuousToken
	}

	return &ResetUserTuplesResult{
		TenantId: tenantId,
		UserId:   userId,
		Deleted:  deleted,
	}, nil
}
