// internal/services/bindingsvc/service_test.go
package bindingsvc

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/hotkhwan/gateway-api/models/gmod"
	"github.com/hotkhwan/gateway-api/models/ingestmod"
	"github.com/redis/go-redis/v9"
	"go.mongodb.org/mongo-driver/bson"
)

// ============================================================
// stubRepo — implements bindingRepoI
// ============================================================

type stubRepo struct {
	// Insert
	insertErr error

	// FindByID — returns same value on every call (good enough for unit tests)
	findResult *ingestmod.TemplateDeliveryBinding
	findErr    error

	// FindByWorkspaceAndStage
	byStageResult []ingestmod.TemplateDeliveryBinding
	byStageErr    error

	// FindAllByStage
	allByStageResult []ingestmod.TemplateDeliveryBinding
	allByStageErr    error

	// Update / Delete
	updateErr error
	deleteErr error
}

func (r *stubRepo) Insert(_ context.Context, b *ingestmod.TemplateDeliveryBinding) error {
	if r.insertErr != nil {
		return r.insertErr
	}
	b.ID = "stub-binding-id"
	return nil
}

func (r *stubRepo) FindByID(_ context.Context, _, _ string) (*ingestmod.TemplateDeliveryBinding, error) {
	return r.findResult, r.findErr
}

func (r *stubRepo) FindByWorkspaceAndStage(_ context.Context, _, _ string) ([]ingestmod.TemplateDeliveryBinding, error) {
	return r.byStageResult, r.byStageErr
}

func (r *stubRepo) FindAllByStage(_ context.Context, _ string) ([]ingestmod.TemplateDeliveryBinding, error) {
	return r.allByStageResult, r.allByStageErr
}

func (r *stubRepo) List(_ context.Context, _ string, _, _ int) ([]ingestmod.TemplateDeliveryBinding, *gmod.Pagination, error) {
	return nil, nil, nil
}

func (r *stubRepo) Update(_ context.Context, _, _ string, _ bson.M) error {
	return r.updateErr
}

func (r *stubRepo) Delete(_ context.Context, _, _ string) error {
	return r.deleteErr
}

// ============================================================
// spyRedis — implements redisI; records SET / DEL calls
// ============================================================

type spyRedis struct {
	data     map[string][]byte
	setCalls []string // keys passed to Set
	delCalls []string // keys passed to Del
}

func newSpyRedis() *spyRedis {
	return &spyRedis{data: map[string][]byte{}}
}

func (s *spyRedis) Get(ctx context.Context, key string) *redis.StringCmd {
	cmd := redis.NewStringCmd(ctx, "get", key)
	v, ok := s.data[key]
	if !ok {
		cmd.SetErr(redis.Nil)
	} else {
		cmd.SetVal(string(v))
	}
	return cmd
}

func (s *spyRedis) Set(ctx context.Context, key string, value interface{}, _ time.Duration) *redis.StatusCmd {
	s.setCalls = append(s.setCalls, key)
	if b, ok := value.([]byte); ok {
		s.data[key] = b
	}
	return redis.NewStatusCmd(ctx, "set", key)
}

func (s *spyRedis) Del(ctx context.Context, keys ...string) *redis.IntCmd {
	s.delCalls = append(s.delCalls, keys...)
	for _, k := range keys {
		delete(s.data, k)
	}
	cmd := redis.NewIntCmd(ctx, "del")
	cmd.SetVal(int64(len(keys)))
	return cmd
}

// ============================================================
// Helpers
// ============================================================

func newSvc(repo bindingRepoI, r *spyRedis) *BindingService {
	return &BindingService{repo: repo, redis: r}
}

func realtimeBinding(wsId string) *ingestmod.TemplateDeliveryBinding {
	return &ingestmod.TemplateDeliveryBinding{
		ID:            "b1",
		WorkspaceID:   wsId,
		TemplateID:    "tpl1",
		TargetID:      "tgt1",
		DispatchStage: ingestmod.DispatchStageRealtime,
		Enabled:       true,
	}
}

func normalizeBinding(wsId string) *ingestmod.TemplateDeliveryBinding {
	return &ingestmod.TemplateDeliveryBinding{
		ID:            "b2",
		WorkspaceID:   wsId,
		TemplateID:    "tpl1",
		TargetID:      "tgt1",
		DispatchStage: ingestmod.DispatchStageNormalize,
		Enabled:       true,
	}
}

func containsKey(calls []string, key string) bool {
	for _, k := range calls {
		if k == key {
			return true
		}
	}
	return false
}

// ============================================================
// Create — dispatchStage Redis behaviour
// ============================================================

func TestCreate_DispatchStage_RedisCache(t *testing.T) {
	t.Parallel()

	const wsId = "ws-abc"
	cacheKey := realtimeCacheKey(wsId)

	tests := []struct {
		name          string
		stage         string
		wantSetCalled bool
	}{
		{
			name:          "realtime → Redis SET called",
			stage:         ingestmod.DispatchStageRealtime,
			wantSetCalled: true,
		},
		{
			name:          "normalize → Redis NOT touched",
			stage:         ingestmod.DispatchStageNormalize,
			wantSetCalled: false,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			rdb := newSpyRedis()
			repo := &stubRepo{
				byStageResult: []ingestmod.TemplateDeliveryBinding{}, // warmRealtimeCache query
			}
			svc := newSvc(repo, rdb)

			_, err := svc.Create(context.Background(), CreateBindingInput{
				WorkspaceID:   wsId,
				TemplateID:    "tpl1",
				TargetID:      "tgt1",
				DispatchStage: tc.stage,
				Enabled:       true,
			})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			setCalled := containsKey(rdb.setCalls, cacheKey)
			if setCalled != tc.wantSetCalled {
				t.Errorf("Redis SET called=%v, want %v (key=%s)", setCalled, tc.wantSetCalled, cacheKey)
			}
			// DEL must never be called on Create
			if containsKey(rdb.delCalls, cacheKey) {
				t.Errorf("Redis DEL must not be called on Create, but was (key=%s)", cacheKey)
			}
		})
	}
}

// ============================================================
// Create — Redis key format
// ============================================================

func TestCreate_Realtime_CacheKeyFormat(t *testing.T) {
	t.Parallel()

	const wsId = "workspace-xyz"
	rdb := newSpyRedis()
	repo := &stubRepo{byStageResult: nil}
	svc := newSvc(repo, rdb)

	_, err := svc.Create(context.Background(), CreateBindingInput{
		WorkspaceID:   wsId,
		TemplateID:    "t",
		TargetID:      "g",
		DispatchStage: ingestmod.DispatchStageRealtime,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	wantKey := "realtime_bindings:" + wsId
	if !containsKey(rdb.setCalls, wantKey) {
		t.Errorf("expected Redis SET with key %q, got setCalls=%v", wantKey, rdb.setCalls)
	}
}

// ============================================================
// Update — dispatchStage realtime → Redis SET called
// ============================================================

func TestUpdate_Realtime_WarmsCacheOnStageChange(t *testing.T) {
	t.Parallel()

	const wsId = "ws-update"
	cacheKey := realtimeCacheKey(wsId)

	tests := []struct {
		name          string
		existing      *ingestmod.TemplateDeliveryBinding
		newStage      *string
		wantSetCalled bool
	}{
		{
			name:          "existing=realtime, no stage change → SET called",
			existing:      realtimeBinding(wsId),
			newStage:      nil,
			wantSetCalled: true,
		},
		{
			name:     "existing=normalize, change to realtime → SET called",
			existing: normalizeBinding(wsId),
			newStage: strPtr(ingestmod.DispatchStageRealtime),
			wantSetCalled: true,
		},
		{
			name:          "existing=normalize, no stage change → NOT SET",
			existing:      normalizeBinding(wsId),
			newStage:      nil,
			wantSetCalled: false,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			rdb := newSpyRedis()
			repo := &stubRepo{
				findResult:    tc.existing,
				byStageResult: []ingestmod.TemplateDeliveryBinding{},
			}
			svc := newSvc(repo, rdb)

			enabled := true
			_, err := svc.Update(context.Background(), UpdateBindingInput{
				WorkspaceID:   wsId,
				ID:            tc.existing.ID,
				DispatchStage: tc.newStage,
				Enabled:       &enabled,
			})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			setCalled := containsKey(rdb.setCalls, cacheKey)
			if setCalled != tc.wantSetCalled {
				t.Errorf("Redis SET called=%v, want %v", setCalled, tc.wantSetCalled)
			}
		})
	}
}

// ============================================================
// Delete — realtime binding → Redis DEL called
// ============================================================

func TestDelete_Realtime_InvalidatesCache(t *testing.T) {
	t.Parallel()

	const wsId = "ws-del"
	cacheKey := realtimeCacheKey(wsId)

	tests := []struct {
		name          string
		existing      *ingestmod.TemplateDeliveryBinding
		wantDelCalled bool
	}{
		{
			name:          "realtime binding → Redis DEL called",
			existing:      realtimeBinding(wsId),
			wantDelCalled: true,
		},
		{
			name:          "normalize binding → Redis NOT touched",
			existing:      normalizeBinding(wsId),
			wantDelCalled: false,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			rdb := newSpyRedis()
			repo := &stubRepo{findResult: tc.existing}
			svc := newSvc(repo, rdb)

			if err := svc.Delete(context.Background(), wsId, tc.existing.ID); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			delCalled := containsKey(rdb.delCalls, cacheKey)
			if delCalled != tc.wantDelCalled {
				t.Errorf("Redis DEL called=%v, want %v (key=%s)", delCalled, tc.wantDelCalled, cacheKey)
			}
		})
	}
}

// ============================================================
// DeleteWorkspace / InvalidateRealtimeCache — must DEL cache
// ============================================================

func TestInvalidateRealtimeCache_DeletesRedisKey(t *testing.T) {
	t.Parallel()

	const wsId = "ws-workspace-delete"
	cacheKey := realtimeCacheKey(wsId)

	rdb := newSpyRedis()
	// Pre-populate cache to confirm it is cleared
	rdb.data[cacheKey] = []byte(`[]`)

	svc := newSvc(&stubRepo{}, rdb)
	svc.InvalidateRealtimeCache(context.Background(), wsId)

	if !containsKey(rdb.delCalls, cacheKey) {
		t.Errorf("expected Redis DEL %q after workspace invalidation, delCalls=%v", cacheKey, rdb.delCalls)
	}
	if _, exists := rdb.data[cacheKey]; exists {
		t.Errorf("expected key %q to be removed from Redis, but it still exists", cacheKey)
	}
}

// ============================================================
// Realtime cache — warm writes JSON, read round-trips correctly
// ============================================================

func TestWarmAndGet_RealtimeCache_RoundTrip(t *testing.T) {
	t.Parallel()

	const wsId = "ws-roundtrip"
	cacheKey := realtimeCacheKey(wsId)

	bindings := []ingestmod.TemplateDeliveryBinding{
		*realtimeBinding(wsId),
	}
	rdb := newSpyRedis()
	repo := &stubRepo{byStageResult: bindings}
	svc := newSvc(repo, rdb)

	// Warm cache
	svc.warmRealtimeCache(context.Background(), wsId)

	if !containsKey(rdb.setCalls, cacheKey) {
		t.Fatalf("warmRealtimeCache did not SET key %q", cacheKey)
	}

	// Read back
	got, ok := svc.GetRealtimeBindings(context.Background(), wsId)
	if !ok {
		t.Fatal("GetRealtimeBindings returned cache miss after warm")
	}
	if len(got) != 1 || got[0].ID != bindings[0].ID {
		t.Errorf("cache round-trip mismatch: got %+v", got)
	}
}

// ============================================================
// GetRealtimeBindings — cache miss returns false
// ============================================================

func TestGetRealtimeBindings_Miss_ReturnsFalse(t *testing.T) {
	t.Parallel()

	rdb := newSpyRedis() // empty
	svc := newSvc(&stubRepo{}, rdb)

	_, ok := svc.GetRealtimeBindings(context.Background(), "ws-miss")
	if ok {
		t.Error("expected cache miss (false), got hit (true)")
	}
}

// ============================================================
// GetRealtimeBindings — corrupt JSON returns false
// ============================================================

func TestGetRealtimeBindings_CorruptJSON_ReturnsFalse(t *testing.T) {
	t.Parallel()

	rdb := newSpyRedis()
	rdb.data["realtime_bindings:ws-corrupt"] = []byte(`not-json`)
	svc := newSvc(&stubRepo{}, rdb)

	_, ok := svc.GetRealtimeBindings(context.Background(), "ws-corrupt")
	if ok {
		t.Error("expected cache miss on corrupt JSON, got hit")
	}
}

// ============================================================
// Create — warm stores valid JSON
// ============================================================

func TestCreate_Realtime_CacheContainsValidJSON(t *testing.T) {
	t.Parallel()

	const wsId = "ws-json"
	cacheKey := realtimeCacheKey(wsId)

	b := *realtimeBinding(wsId)
	rdb := newSpyRedis()
	repo := &stubRepo{byStageResult: []ingestmod.TemplateDeliveryBinding{b}}
	svc := newSvc(repo, rdb)

	svc.Create(context.Background(), CreateBindingInput{ //nolint:errcheck
		WorkspaceID:   wsId,
		TemplateID:    "t",
		TargetID:      "g",
		DispatchStage: ingestmod.DispatchStageRealtime,
	})

	raw, ok := rdb.data[cacheKey]
	if !ok {
		t.Fatal("expected key in Redis after Create realtime, not found")
	}
	var decoded []ingestmod.TemplateDeliveryBinding
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("Redis value is not valid JSON: %v (raw=%s)", err, raw)
	}
}

// ============================================================
// MatchesFields — pure function, table-driven
// ============================================================

func TestMatchesFields(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		payload     map[string]any
		matchFields map[string]any
		want        bool
	}{
		{
			name:        "empty matchFields → always matches",
			payload:     map[string]any{"eventType": "motion"},
			matchFields: map[string]any{},
			want:        true,
		},
		{
			name:        "nil matchFields → always matches",
			payload:     map[string]any{"eventType": "motion"},
			matchFields: nil,
			want:        true,
		},
		{
			name:        "eventType=motion matches",
			payload:     map[string]any{"eventType": "motion"},
			matchFields: map[string]any{"eventType": "motion"},
			want:        true,
		},
		{
			name:        "eventType=motion does not match eventType=intrusion",
			payload:     map[string]any{"eventType": "intrusion"},
			matchFields: map[string]any{"eventType": "motion"},
			want:        false,
		},
		{
			name:        "field missing in payload → false",
			payload:     map[string]any{"other": "value"},
			matchFields: map[string]any{"eventType": "motion"},
			want:        false,
		},
		{
			name:        "multiple conditions — all match",
			payload:     map[string]any{"eventType": "motion", "zone": "A"},
			matchFields: map[string]any{"eventType": "motion", "zone": "A"},
			want:        true,
		},
		{
			name:        "multiple conditions — one mismatch → false",
			payload:     map[string]any{"eventType": "motion", "zone": "B"},
			matchFields: map[string]any{"eventType": "motion", "zone": "A"},
			want:        false,
		},
		{
			name:        "empty payload, non-empty matchFields → false",
			payload:     map[string]any{},
			matchFields: map[string]any{"eventType": "motion"},
			want:        false,
		},
		{
			name:        "numeric value compared as string",
			payload:     map[string]any{"score": 42},
			matchFields: map[string]any{"score": 42},
			want:        true,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := MatchesFields(tc.payload, tc.matchFields)
			if got != tc.want {
				t.Errorf("MatchesFields(%v, %v) = %v, want %v", tc.payload, tc.matchFields, got, tc.want)
			}
		})
	}
}

// ============================================================
// Create — validation errors
// ============================================================

func TestCreate_Validation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input CreateBindingInput
	}{
		{"empty workspaceId", CreateBindingInput{TemplateID: "t", TargetID: "g", DispatchStage: "realtime"}},
		{"empty templateId", CreateBindingInput{WorkspaceID: "ws", TargetID: "g", DispatchStage: "realtime"}},
		{"empty targetId", CreateBindingInput{WorkspaceID: "ws", TemplateID: "t", DispatchStage: "realtime"}},
		{"invalid stage", CreateBindingInput{WorkspaceID: "ws", TemplateID: "t", TargetID: "g", DispatchStage: "streaming"}},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			svc := newSvc(&stubRepo{}, newSpyRedis())
			_, err := svc.Create(context.Background(), tc.input)
			if err != ErrBadRequest {
				t.Errorf("expected ErrBadRequest, got %v", err)
			}
		})
	}
}

// ============================================================
// realtimeCacheKey — key format contract
// ============================================================

func TestRealtimeCacheKey(t *testing.T) {
	t.Parallel()

	tests := []struct {
		wsId string
		want string
	}{
		{"ws1", "realtime_bindings:ws1"},
		{"workspace-abc-123", "realtime_bindings:workspace-abc-123"},
		{"", "realtime_bindings:"},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.wsId, func(t *testing.T) {
			t.Parallel()
			got := realtimeCacheKey(tc.wsId)
			if got != tc.want {
				t.Errorf("realtimeCacheKey(%q) = %q, want %q", tc.wsId, got, tc.want)
			}
		})
	}
}

// ============================================================
// toString helper
// ============================================================

func TestToString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input any
		want  string
	}{
		{nil, ""},
		{"hello", "hello"},
		{42, "42"},
		{3.14, "3.14"},
		{true, "true"},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(strings.ReplaceAll(tc.want, ".", "_"), func(t *testing.T) {
			t.Parallel()
			got := toString(tc.input)
			if got != tc.want {
				t.Errorf("toString(%v) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

// ============================================================
// Helpers
// ============================================================

func strPtr(s string) *string { return &s }
