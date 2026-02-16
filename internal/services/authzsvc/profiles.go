// internal/services/authzsvc/profiles.go
package authzsvc

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/hotkhwan/gateway-api/internal/logger"
	"github.com/hotkhwan/gateway-api/internal/repo/authzrepo"
	"github.com/hotkhwan/gateway-api/models/authzmod"
	"github.com/hotkhwan/gateway-api/utils/traceutil"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// ===============================
//
//	ProfileService – High level API
//
// ===============================
//
// collections:
//
//   - profiles
//     = desired config (สิ่งที่ UI แก้ไขอยู่ตอนนี้)
//   - profile_versions
//     = history / rollback / audit trail
//
// Permify = source of truth ของ “effective permissions ตอนนี้”
// Profiles  = source of truth ของ “desired config จาก UI”
// ===============================
type ProfileService struct {
	profileRepo   authzrepo.ProfileRepo
	versionRepo   authzrepo.ProfileVersionRepo
	idemRepo      authzrepo.IdempotencyRepo
	auditRepo     authzrepo.AuditLogRepo
	effTupleRepo  authzrepo.ProfileEffectiveTupleRepo
	permifyClient *PermifyClient
}

func NewProfileService(
	profileRepo authzrepo.ProfileRepo,
	versionRepo authzrepo.ProfileVersionRepo,
	idemRepo authzrepo.IdempotencyRepo,
	auditRepo authzrepo.AuditLogRepo,
	effTupleRepo authzrepo.ProfileEffectiveTupleRepo,
) *ProfileService {
	return &ProfileService{
		profileRepo:   profileRepo,
		versionRepo:   versionRepo,
		idemRepo:      idemRepo,
		auditRepo:     auditRepo,
		effTupleRepo:  effTupleRepo,
		permifyClient: NewPermifyClient(),
	}
}

// ===============================
//  CRUD พื้นฐานบน profiles
// ===============================

// CreateProfile สร้าง profile ใหม่ใน MongoDB
// profiles = desired config
func (s *ProfileService) CreateProfile(ctx context.Context, p *authzmod.Profile) error {
	ctx, end, log := traceutil.StartLite(
		ctx,
		"github.com/hotkhwan/gateway-api/authzsvc",
		"authorization.CreateProfile",
		"authzsvc", "CreateProfile",
	)
	defer end()

	if p == nil {
		return errors.New("profile is nil")
	}
	if p.Code == "" {
		return errors.New("profile code is required")
	}

	log.Info().Str("code", p.Code).Msg("creating profile")
	if err := s.profileRepo.Create(ctx, p); err != nil {
		log.Error().Err(err).Str("code", p.Code).Msg("failed to create profile")
		return err
	}
	return nil
}

func (s *ProfileService) DeleteProfile(ctx context.Context, code string) error {
	ctx, end, log := traceutil.StartLite(
		ctx,
		"github.com/hotkhwan/gateway-api/authzsvc",
		"authorization.DeleteProfile",
		"authzsvc", "DeleteProfile",
	)
	defer end()

	if code == "" {
		return errors.New("profile code is required")
	}

	log.Info().Str("code", code).Msg("deleting profile")
	if err := s.profileRepo.Delete(ctx, code); err != nil {
		log.Error().Err(err).Str("code", code).Msg("failed to delete profile")
		return err
	}
	return nil
}

// ListProfiles คืน list profiles ตาม filter + options
func (s *ProfileService) ListProfiles(
	ctx context.Context,
	filter bson.M,
	opts *options.FindOptions,
) ([]*authzmod.Profile, error) {
	ctx, end, _ := traceutil.StartLite(
		ctx,
		"github.com/hotkhwan/gateway-api/authzsvc",
		"authorization.ListProfiles",
		"authzsvc", "ListProfiles",
	)
	defer end()

	if filter == nil {
		filter = bson.M{}
	}
	if opts == nil {
		opts = options.Find()
	}

	return s.profileRepo.List(ctx, filter, opts)
}

// CountProfiles นับจำนวน profile ตาม filter
func (s *ProfileService) CountProfiles(
	ctx context.Context,
	filter bson.M,
) (int64, error) {
	ctx, end, _ := traceutil.StartLite(
		ctx,
		"github.com/hotkhwan/gateway-api/authzsvc",
		"authorization.CountProfiles",
		"authzsvc", "CountProfiles",
	)
	defer end()

	if filter == nil {
		filter = bson.M{}
	}
	return s.profileRepo.Count(ctx, filter)
}

// UpdateProfile แก้ไข profile ตาม code (ใช้จาก PATCH /authz/profiles/:code)
func (s *ProfileService) UpdateProfile(
	ctx context.Context,
	code string,
	updates bson.M,
) error {
	ctx, end, log := traceutil.StartLite(
		ctx,
		"github.com/hotkhwan/gateway-api/authzsvc",
		"authorization.UpdateProfile",
		"authzsvc", "UpdateProfile",
	)
	defer end()

	if code == "" {
		return errors.New("profile code is required")
	}
	if updates == nil {
		updates = bson.M{}
	}

	log.Info().Str("code", code).Interface("updates", updates).Msg("updating profile")
	return s.profileRepo.Update(ctx, code, updates)
}

// ===============================
//  Versioning – profile_versions
// ===============================
//
// profiles           → desired config ปัจจุบัน
// profile_versions   → history / snapshot (ใช้ show history / rollback / diff)
// ===============================

// PublishVersion สร้าง snapshot ใหม่ใน profile_versions จาก profile ปัจจุบัน
// ใช้โดย POST /authz/profiles/{code}/publish
func (s *ProfileService) PublishVersion(
	ctx context.Context,
	code string,
) (*authzmod.ProfileVersion, error) {
	ctx, end, log := traceutil.StartLite(
		ctx,
		"github.com/hotkhwan/gateway-api/authzsvc",
		"authorization.PublishVersion",
		"authzsvc", "PublishVersion",
	)
	defer end()

	if code == "" {
		return nil, errors.New("profile code is required")
	}

	profile, err := s.profileRepo.FindByCode(ctx, code)
	if err != nil {
		log.Error().Err(err).Str("code", code).Msg("failed to load profile for publish")
		return nil, err
	}
	if profile == nil {
		return nil, fmt.Errorf("profile not found: %s", code)
	}

	var nextVersion int = 1
	if s.versionRepo != nil {
		if latest, err := s.versionRepo.FindLatest(ctx, code); err == nil && latest != nil {
			if latest.Version >= nextVersion {
				nextVersion = latest.Version + 1
			}
		}
	}

	v := &authzmod.ProfileVersion{
		ProfileCode: code,
		Version:     nextVersion,
		Items:       profile.Items,
		CreatedAt:   time.Now().UTC(),
	}

	log.Info().
		Str("code", code).
		Int("version", nextVersion).
		Int("items", len(profile.Items)).
		Msg("publishing profile version")

	if err := s.versionRepo.Create(ctx, v); err != nil {
		log.Error().Err(err).Str("code", code).Int("version", nextVersion).Msg("failed to create profile version")
		return nil, err
	}
	return v, nil
}

// AddVersionNote เพิ่ม / แก้ note ของ version ใน profile_versions
func (s *ProfileService) AddVersionNote(
	ctx context.Context,
	code string,
	version int,
	note string,
) error {
	ctx, end, log := traceutil.StartLite(
		ctx,
		"github.com/hotkhwan/gateway-api/authzsvc",
		"authorization.AddVersionNote",
		"authzsvc", "AddVersionNote",
	)
	defer end()

	if s.versionRepo == nil {
		return errors.New("version repository is not configured")
	}

	log.Info().Str("code", code).Int("version", version).Msg("updating version note")
	return s.versionRepo.UpdateNote(ctx, code, version, note)
}

// ===============================
//  Plan / Apply – Declarative sync
// ===============================
//
// PlanChanges:
//   desired = profile.Items (ใน Mongo)
//   actual  = tuples ใน Permify (อ่านผ่าน /data/relationships/read เฉพาะ scope นี้)
//
//   ทำ diff:
//     - อยู่ใน desired แต่ไม่อยู่ใน actual → create
//     - อยู่ใน actual แต่ไม่อยู่ใน desired → delete
//
// ApplyChanges:
//   - เขียน create ผ่าน writeDataREST()
//   - ลบ delete ผ่าน deleteTupleREST()
//   - ถ้ามี Idempotency-Key → ใช้ idempotencyRepo เพื่อกันยิงซ้ำ
//
// NOTE:
//   ตอนนี้ plan/diff ทำระดับ "หนึ่ง profile ต่อครั้ง"
//   ถ้า UI เลือกให้ subject/resource เดียวกันอยู่หลาย profile คุณกำลัง share tuple กันเอง
//   behaviour จะเป็น “โปรไฟล์ล่าสุดที่ apply อาจลบของ profile อื่นได้”
//   ⇒ ถ้าจะกันไม่ให้ลบของคนอื่น ต้องเพิ่ม profile_effective_tuples + refCount ภายหลัง
// ===============================

// PlanChanges คำนวณแผน create/delete สำหรับ profile ที่ระบุ
// desired = profile.Items (สิ่งที่ UI ต้องการให้ profile นี้มี)
// owned   = tuple keys ที่ profile นี้ own อยู่ใน Mongo (profile_effective_tuples)
// diff:
//   - create = desired - owned
//   - delete = owned - desired  (delete = profile นี้ขอ detach; จะลบจริงไหมไปตัดสินใจใน ApplyChanges)
func (s *ProfileService) PlanChanges(
	ctx context.Context,
	code string,
	versionArg int,
) (*TuplePlan, error) {
	ctx, end, log := traceutil.StartLite(
		ctx,
		"github.com/hotkhwan/gateway-api/authzsvc",
		"authorization.PlanChanges",
		"authzsvc", "PlanChanges",
	)
	defer end()

	if code == "" {
		return nil, errors.New("profile code is required")
	}

	// 1) โหลด profile ปัจจุบัน (desired)
	profile, err := s.profileRepo.FindByCode(ctx, code)
	if err != nil {
		log.Error().Err(err).Str("code", code).Msg("failed to load profile for plan")
		return nil, err
	}
	if profile == nil {
		return nil, fmt.Errorf("profile not found: %s", code)
	}

	log = logger.FromCtx(ctx, "authzsvc", "PlanChanges")
	log.Info().
		Str("code", code).
		Int("itemCount", len(profile.Items)).
		Msg("building desired/owned tuple sets for plan")

	// 2) desired tuples จาก profile.Items
	//    BuildDesiredTuplesFromItems คืนค่าเป็น set (map[string]Relationship)
	desiredSetRaw := BuildDesiredTuplesFromItems(profile.Items)

	// แปลง desiredSetRaw → map[TupleKey]tupleBody
	desiredSet := make(map[authzmod.TupleKey]map[string]interface{})

	for _, rel := range desiredSetRaw {
		// rel = authzsvc.Relationship (struct)
		entType := rel.Entity.Type
		entID := rel.Entity.ID
		subType := rel.Subject.Type
		subID := rel.Subject.ID
		relation := rel.Relation

		if entType == "" || entID == "" || subType == "" || subID == "" || relation == "" {
			log.Warn().Interface("relationship", rel).Msg("skip invalid desired relationship")
			continue
		}

		key := authzmod.TupleKey{
			EntityType:  entType,
			EntityID:    entID,
			Relation:    relation,
			SubjectType: subType,
			SubjectID:   subID,
		}

		// แปลงเป็น tuple body แบบ map[string]interface{} ให้ ApplyChanges ใช้ได้เหมือนเดิม
		tuple := map[string]interface{}{
			"entity": map[string]interface{}{
				"type": entType,
				"id":   entID,
			},
			"relation": relation,
			"subject": map[string]interface{}{
				"type": subType,
				"id":   subID,
			},
		}

		desiredSet[key] = tuple
	}

	// 3) owned tuples ของ profile นี้จาก Mongo (profile_effective_tuples)
	ownedKeys := []authzmod.TupleKey{}
	if s.effTupleRepo != nil {
		if keys, err := s.effTupleRepo.ListByProfile(ctx, code); err != nil {
			log.Error().
				Err(err).
				Str("code", code).
				Msg("failed to load owned tuples from profile_effective_tuples")
		} else {
			ownedKeys = keys
		}
	}

	ownedSet := make(map[authzmod.TupleKey]struct{}, len(ownedKeys))
	for _, k := range ownedKeys {
		ownedSet[k] = struct{}{}
	}

	log.Debug().
		Str("code", code).
		Int("desiredCount", len(desiredSet)).
		Int("ownedCount", len(ownedKeys)).
		Msg("computed desired/owned sets for profile")

	// 4) diff → create/delete
	var changes []TupleChange
	stats := map[string]int{"create": 0, "delete": 0}

	// 4.1 create = desired - owned
	for key, tuple := range desiredSet {
		if _, ok := ownedSet[key]; !ok {
			changes = append(changes, TupleChange{
				Action: "create",
				Tuple:  tuple,
			})
			stats["create"]++
		}
	}

	// 4.2 delete = owned - desired
	for _, key := range ownedKeys {
		if _, ok := desiredSet[key]; ok {
			// profile นี้ยังต้องการ tuple นี้อยู่ → ไม่ลบ
			continue
		}

		// สร้าง tuple body กลับขึ้นมา เพื่อส่งต่อไปยัง ApplyChanges → DetachProfile + deleteTupleREST
		tuple := map[string]interface{}{
			"entity": map[string]interface{}{
				"type": key.EntityType,
				"id":   key.EntityID,
			},
			"relation": key.Relation,
			"subject": map[string]interface{}{
				"type": key.SubjectType,
				"id":   key.SubjectID,
			},
		}

		changes = append(changes, TupleChange{
			Action: "delete",
			Tuple:  tuple,
		})
		stats["delete"]++
	}

	// 5) เลือก version ให้ตรงกับ history
	finalVersion := versionArg
	if finalVersion <= 0 && s.versionRepo != nil {
		if latest, err := s.versionRepo.FindLatest(ctx, code); err == nil && latest != nil {
			finalVersion = latest.Version
		}
	}

	plan := &TuplePlan{
		ProfileCode: code,
		Version:     finalVersion,
		Changes:     changes,
		Stats:       stats,
	}

	log.Debug().
		Str("code", code).
		Int("version", plan.Version).
		Int("create", plan.Stats["create"]).
		Int("delete", plan.Stats["delete"]).
		Msg("profile plan computed (from desired vs owned)")

	return plan, nil
}

// ApplyChanges นำแผนที่ PlanChanges คืนมาไป apply ลง Permify
// - ใช้ writeDataREST กับ create
// - ใช้ deleteTupleREST กับ delete (เฉพาะกรณี refCount == 0)
// - ใช้ idempotency กันยิงซ้ำ (ถ้ามี key + repo)
func (s *ProfileService) ApplyChanges(
	ctx context.Context,
	plan *TuplePlan,
	idempotencyKey string,
) error {
	ctx, end, log := traceutil.StartLite(
		ctx,
		"github.com/hotkhwan/gateway-api/authzsvc",
		"authorization.ApplyChanges",
		"authzsvc", "ApplyChanges",
	)
	defer end()

	if plan == nil {
		return errors.New("tuple plan is nil")
	}

	log = logger.FromCtx(ctx, "authzsvc", "ApplyChanges")
	log.Debug().
		Str("profileCode", plan.ProfileCode).
		Int("version", plan.Version).
		Interface("stats", plan.Stats).
		Msg("applying profile tuple plan")

	// 1) Idempotency
	if idempotencyKey != "" && s.idemRepo != nil {
		if rec, err := s.idemRepo.Get(ctx, idempotencyKey); err == nil && rec != nil {
			log.Info().
				Str("idempotencyKey", idempotencyKey).
				Msg("idempotent apply detected, skipping re-apply")
			return nil
		}
	}

	var (
		createTuples []map[string]interface{}
		deleteTuples []map[string]interface{}
	)

	for i, ch := range plan.Changes {
		if ch.Tuple == nil {
			log.Warn().Int("index", i).Msg("skip nil tuple in plan")
			continue
		}

		entityRaw, _ := ch.Tuple["entity"].(map[string]interface{})
		subjectRaw, _ := ch.Tuple["subject"].(map[string]interface{})
		relation, _ := ch.Tuple["relation"].(string)

		entType, _ := entityRaw["type"].(string)
		entID, _ := entityRaw["id"].(string)
		subType, _ := subjectRaw["type"].(string)
		subID, _ := subjectRaw["id"].(string)

		if entType == "" || entID == "" || subType == "" || subID == "" || relation == "" {
			log.Warn().
				Int("index", i).
				Interface("tuple", ch.Tuple).
				Msg("skip invalid tuple in plan")
			continue
		}

		key := authzmod.TupleKey{
			EntityType:  entType,
			EntityID:    entID,
			Relation:    relation,
			SubjectType: subType,
			SubjectID:   subID,
		}

		log.Debug().
			Int("index", i).
			Str("action", ch.Action).
			Interface("key", key).
			Msg("processing tuple change")

		switch ch.Action {
		case "create":
			if s.effTupleRepo != nil {
				if err := s.effTupleRepo.AttachProfile(ctx, plan.ProfileCode, key); err != nil {
					log.Error().
						Err(err).
						Str("profileCode", plan.ProfileCode).
						Interface("key", key).
						Msg("failed to attach profile to tuple")
					return err
				}
			}
			createTuples = append(createTuples, ch.Tuple)

		case "delete":
			if s.effTupleRepo == nil {
				log.Debug().
					Str("profileCode", plan.ProfileCode).
					Interface("key", key).
					Msg("no effTupleRepo, delete tuple directly")
				deleteTuples = append(deleteTuples, ch.Tuple)
				continue
			}

			remaining, owned, err := s.effTupleRepo.DetachProfile(ctx, plan.ProfileCode, key)
			if err != nil {
				log.Error().
					Err(err).
					Str("profileCode", plan.ProfileCode).
					Interface("key", key).
					Msg("failed to detach profile from tuple")
				return err
			}

			log.Debug().
				Str("profileCode", plan.ProfileCode).
				Interface("key", key).
				Bool("ownedBefore", owned).
				Strs("remainingProfiles", remaining).
				Msg("detached profile from tuple")

			if !owned {
				log.Debug().
					Str("profileCode", plan.ProfileCode).
					Interface("key", key).
					Msg("profile didn't own this tuple, skip delete in Permify")
				continue
			}

			if len(remaining) == 0 {
				log.Info().
					Str("profileCode", plan.ProfileCode).
					Interface("key", key).
					Msg("no remaining profiles, will delete tuple from Permify")
				deleteTuples = append(deleteTuples, ch.Tuple)
			} else {
				log.Debug().
					Str("profileCode", plan.ProfileCode).
					Interface("key", key).
					Strs("remainingProfiles", remaining).
					Msg("tuple still used by other profiles, skip delete in Permify")
			}

		default:
			log.Warn().
				Str("action", ch.Action).
				Interface("tuple", ch.Tuple).
				Msg("unknown action in tuple plan")
		}
	}

	log.Debug().
		Str("profileCode", plan.ProfileCode).
		Int("createTuples", len(createTuples)).
		Int("deleteTuples", len(deleteTuples)).
		Msg("tuple batches prepared for Permify")

	// 2) create → /data/write
	if err := writeDataREST(ctx, createTuples, nil); err != nil {
		log.Error().Err(err).Msg("failed to write tuples to Permify")
		return err
	}

	// 3) delete → /data/delete ทีละตัว (แต่เดี๋ยวเราจะคุยว่าจะใช้ deleteTupleREST หรือ RevokeResource)
	for _, t := range deleteTuples {
		if err := deleteTupleREST(ctx, t); err != nil {
			log.Warn().
				Err(err).
				Interface("tuple", t).
				Msg("failed to delete tuple from Permify")
			// ไม่ return error เพื่อไม่ให้ทั้ง apply fail ถ้าลบตัวเดียวไม่ได้
		}
	}

	// 4) idempotency
	if idempotencyKey != "" && s.idemRepo != nil {
		_ = s.idemRepo.Put(ctx, idempotencyKey, map[string]interface{}{
			"profileCode": plan.ProfileCode,
			"version":     plan.Version,
			"stats":       plan.Stats,
			"appliedAt":   time.Now().UTC(),
		})
	}

	log.Info().
		Str("profileCode", plan.ProfileCode).
		Int("version", plan.Version).
		Msg("profile plan applied successfully (with refCount)")

	return nil
}

// ===============================
//  Detach / Drift (future work)
// ===============================
//
// - DriftProfile:
//     อ่าน desired จาก profiles
//     อ่าน actual จาก Permify
//     ทำ diff แล้วบอกว่า "มี tuple ไหนเกินมา/ขาดไปจาก profile"
//
// - DetachProfile / ReconcileProfile:
//     ใช้ drift แล้วตัดสินใจว่าจะ:
//       - ลบ tuple เกินออก (force match profile)
//       - หรือแค่โชว์ใน UI ให้คนตัดสินใจ
//
// ตอนนี้ controller ของคุณสำหรับ /drift และ /reconcile ยังตอบ NOT_IMPLEMENTED อยู่
// ถ้าจะทำต่อ เราจะมาเติม service layer ในไฟล์นี้ แล้วให้ handler เรียกครับ
// ===============================
