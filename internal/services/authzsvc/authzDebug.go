// internal/services/authzsvc/authzDebug.gos
package authzsvc

import (
	"context"
	"errors"
	"time"

	"github.com/hotkhwan/gateway-api/config"
	"github.com/hotkhwan/gateway-api/internal/gateways/authzgw"
	"github.com/hotkhwan/gateway-api/internal/repo/authzrepo"
	"github.com/hotkhwan/gateway-api/utils/traceutil"
)

type PermifyTupleFilter struct {
	EntityType      string
	EntityId        string
	Relation        string
	SubjectType     string
	SubjectId       string
	PageSize        int
	ContinuousToken string
}

type PermifyTupleRow struct {
	EntityType  string `json:"entityType"`
	EntityId    string `json:"entityId"`
	Relation    string `json:"relation"`
	SubjectType string `json:"subjectType"`
	SubjectId   string `json:"subjectId"`
}

type PermifyTupleListResult struct {
	Tuples          []PermifyTupleRow `json:"tuples"`
	ContinuousToken string            `json:"continuousToken"`
	PageSize        int               `json:"pageSize"`
}

type ResetTenantResult struct {
	TenantId string         `json:"tenantId"`
	Deleted  map[string]int `json:"deleted"`
}

func ListPermifyTuples(ctx context.Context, tenantId string, filter PermifyTupleFilter) (*PermifyTupleListResult, error) {
	ctx, end, log := traceutil.StartLite(
		ctx,
		"github.com/hotkhwan/gateway-api/authzsvc",
		"authz.ListPermifyTuples",
		"authzsvc", "ListPermifyTuples",
	)
	defer end()

	if tenantId == "" {
		tenantId = config.PermifyTenantID
	}
	if filter.PageSize <= 0 {
		filter.PageSize = 50
	}
	if filter.PageSize > 200 {
		filter.PageSize = 200
	}

	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()

	client := authzgw.NewPermifyRestDebugClient()
	res, err := client.ReadTuples(ctx, tenantId, authzgw.ReadTuplesRequest{
		EntityType:      filter.EntityType,
		EntityId:        filter.EntityId,
		Relation:        filter.Relation,
		SubjectType:     filter.SubjectType,
		SubjectId:       filter.SubjectId,
		PageSize:        filter.PageSize,
		ContinuousToken: filter.ContinuousToken,
		SchemaVersion:   "latest",
		Depth:           50,
	})
	if err != nil {
		log.Error().Err(err).Msg("permify read tuples failed")
		return nil, err
	}

	out := &PermifyTupleListResult{
		Tuples:          make([]PermifyTupleRow, 0, len(res.Tuples)),
		ContinuousToken: res.ContinuousToken,
		PageSize:        filter.PageSize,
	}

	for _, t := range res.Tuples {
		out.Tuples = append(out.Tuples, PermifyTupleRow{
			EntityType:  t.Entity.Type,
			EntityId:    t.Entity.Id,
			Relation:    t.Relation,
			SubjectType: t.Subject.Type,
			SubjectId:   t.Subject.Id,
		})
	}

	return out, nil
}

func FactoryResetPermifyTuples(ctx context.Context, tenantId string, entityType string) (int, error) {
	ctx, end, log := traceutil.StartLite(
		ctx,
		"github.com/hotkhwan/gateway-api/authzsvc",
		"authz.FactoryResetPermifyTuples",
		"authzsvc", "FactoryResetPermifyTuples",
	)
	defer end()

	if tenantId == "" {
		return 0, errors.New("tenantId is required")
	}
	if entityType == "" {
		return 0, errors.New("entityType is required")
	}

	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	client := authzgw.NewPermifyRestDebugClient()

	// 1) read count ก่อน (optional แต่ช่วยคืน deleted)
	readRes, err := client.ReadTuples(ctx, tenantId, authzgw.ReadTuplesRequest{
		EntityType:    entityType,
		PageSize:      200,
		SchemaVersion: "latest",
		Depth:         50,
	})
	if err != nil {
		log.Error().Err(err).Msg("permify read tuples failed")
		return 0, err
	}

	// 2) delete by filter (ถูก spec)
	if err := client.DeleteTuples(ctx, tenantId, authzgw.DeleteTuplesRequest{
		EntityType:    entityType,
		SchemaVersion: "latest",
		Depth:         50,
	}); err != nil {
		log.Error().Err(err).Msg("permify delete tuples failed")
		return 0, err
	}

	// หมายเหตุ: Permify delete API ส่วนใหญ่ไม่คืนจำนวนที่ลบ เราเลยประมาณจาก page แรก
	// ถ้าคุณอยากแม่น: loop read ทั้งหมดเพื่อ count ก่อนค่อย delete (แต่จะช้า)
	deleted := len(readRes.Tuples)
	return deleted, nil
}

type ProveUserOrgsResult struct {
	TenantId       string   `json:"tenantId"`
	UserId         string   `json:"userId"`
	PermifyOrgIds  []string `json:"permifyOrgIds"`
	MongoOrgIds    []string `json:"mongoOrgIds"`
	MissingInMongo []string `json:"missingInMongo"`
}

func ProveUserOrgsAgainstMongo(ctx context.Context, tenantId string, userId string) (*ProveUserOrgsResult, error) {
	ctx, end, log := traceutil.StartLite(
		ctx,
		"github.com/hotkhwan/gateway-api/authzsvc",
		"authz.ProveUserOrgsAgainstMongo",
		"authzsvc", "ProveUserOrgsAgainstMongo",
	)
	defer end()

	if tenantId == "" {
		tenantId = config.PermifyTenantID
	}

	// 1) ask permify: list tuples subject=userId relation=member on organization
	client := authzgw.NewPermifyRestDebugClient()

	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()

	readRes, err := client.ReadTuples(ctx, tenantId, authzgw.ReadTuplesRequest{
		EntityType:    "organization",
		Relation:      "member",
		SubjectType:   "user",
		SubjectId:     userId,
		PageSize:      200,
		SchemaVersion: "latest",
		Depth:         50,
	})

	if err != nil {
		log.Error().Err(err).Msg("permify read tuples failed")
		return nil, err
	}

	orgIds := make([]string, 0, len(readRes.Tuples))
	seen := map[string]struct{}{}
	for _, t := range readRes.Tuples {
		if t.Entity.Type != "organization" {
			continue
		}
		if _, ok := seen[t.Entity.Id]; ok {
			continue
		}
		seen[t.Entity.Id] = struct{}{}
		orgIds = append(orgIds, t.Entity.Id)
	}

	// 2) query mongo by ids
	orgRepo := authzrepo.NewOrgRepo(config.DB)
	orgDocs, err := orgRepo.FindByIds(ctx, tenantId, orgIds)
	if err != nil {
		log.Error().Err(err).Msg("mongo find orgs failed")
		return nil, err
	}

	mongoIds := make([]string, 0, len(orgDocs))
	mSeen := map[string]struct{}{}
	for _, o := range orgDocs {
		mongoIds = append(mongoIds, o.OrgId)
		mSeen[o.OrgId] = struct{}{}
	}

	missing := make([]string, 0)
	for _, id := range orgIds {
		if _, ok := mSeen[id]; !ok {
			missing = append(missing, id)
		}
	}

	return &ProveUserOrgsResult{
		TenantId:       tenantId,
		UserId:         userId,
		PermifyOrgIds:  orgIds,
		MongoOrgIds:    mongoIds,
		MissingInMongo: missing,
	}, nil
}

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

	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
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

	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()

	client := authzgw.NewPermifyRestDebugClient()

	deleted := 0
	continuousToken := ""

	// ลบเฉพาะ tuples ของ subject user นี้ (คุณจะขยาย relation/entityType เพิ่มได้)
	for {
		readRes, err := client.ReadTuples(ctx, tenantId, authzgw.ReadTuplesRequest{
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

func ListTuplesByUser(ctx context.Context, tenantId string, userId string) ([]PermifyTupleRow, error) {

	if tenantId == "" {
		tenantId = config.PermifyTenantID
	}

	client := authzgw.NewPermifyRestDebugClient()

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	out := []PermifyTupleRow{}
	continuousToken := ""

	for {
		readRes, err := client.ReadTuples(ctx, tenantId, authzgw.ReadTuplesRequest{
			SubjectType:     "user",
			SubjectId:       userId,
			PageSize:        200,
			ContinuousToken: continuousToken,
			SchemaVersion:   "latest",
			Depth:           50,
		})
		if err != nil {
			return nil, err
		}

		if len(readRes.Tuples) == 0 {
			break
		}

		for _, t := range readRes.Tuples {
			out = append(out, PermifyTupleRow{
				EntityType:  t.Entity.Type,
				EntityId:    t.Entity.Id,
				Relation:    t.Relation,
				SubjectType: t.Subject.Type,
				SubjectId:   t.Subject.Id,
			})
		}

		if readRes.ContinuousToken == "" || readRes.ContinuousToken == continuousToken {
			break
		}

		continuousToken = readRes.ContinuousToken
	}

	return out, nil
}

func ListTuplesByEntityType(ctx context.Context, tenantId string, entityType string) ([]PermifyTupleRow, error) {

	if tenantId == "" {
		tenantId = config.PermifyTenantID
	}

	client := authzgw.NewPermifyRestDebugClient()

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	out := []PermifyTupleRow{}
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
			return nil, err
		}

		if len(readRes.Tuples) == 0 {
			break
		}

		for _, t := range readRes.Tuples {
			out = append(out, PermifyTupleRow{
				EntityType:  t.Entity.Type,
				EntityId:    t.Entity.Id,
				Relation:    t.Relation,
				SubjectType: t.Subject.Type,
				SubjectId:   t.Subject.Id,
			})
		}

		if readRes.ContinuousToken == "" || readRes.ContinuousToken == continuousToken {
			break
		}

		continuousToken = readRes.ContinuousToken
	}

	return out, nil
}

func ResetTenant(ctx context.Context, tenantId string) (*ResetTenantResult, error) {

	if tenantId == "" {
		return nil, errors.New("tenantId is required")
	}

	entityTypes := []string{
		"organization",
		"orgUnit",
		"deviceGroup",
		"device",
		"menu",
		"serviceAccount",
	}

	result := &ResetTenantResult{
		TenantId: tenantId,
		Deleted:  make(map[string]int),
	}

	for _, et := range entityTypes {

		res, err := ResetPermifyTuplesAll(ctx, tenantId, et)
		if err != nil {
			return nil, err
		}

		if res != nil {
			result.Deleted[et] = res.Deleted
		} else {
			result.Deleted[et] = 0
		}
	}

	return result, nil
}

type FactoryBootstrapResult struct {
	OrgId         string `json:"orgId"`
	TupleCount    int    `json:"tupleCount"`
	SchemaVersion string `json:"schemaVersion"`
}
