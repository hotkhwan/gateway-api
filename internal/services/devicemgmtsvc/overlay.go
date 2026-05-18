// internal/services/devicemgmtsvc/overlay.go
package devicemgmtsvc

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"time"

	"github.com/hotkhwan/gateway-api/internal/repo/cachedevicemgmt"
	"github.com/hotkhwan/gateway-api/models/ingestmod"
	"github.com/hotkhwan/gateway-api/utils/traceutil"
	"go.mongodb.org/mongo-driver/bson"
)

// Per klynx-api/docs/contracts/camera-gw-managed-overlay.md §8.3 — strict whitelists.
// acceptedOverlayFields are the only field names this endpoint will persist.
var acceptedOverlayFields = map[string]struct{}{
	"name":        {},
	"description": {},
	"lat":         {},
	"lng":         {},
	"site":        {},
	"zone":        {},
	"serialNo":    {},
}

// rejectNotAcceptedFields are klynx-canonical fields that the contract explicitly
// rejects: stream creds + brand/ip/district/location/type. Including any of
// these surfaces `FIELD_NOT_ACCEPTED` so klynx cannot drift the surface later
// without a contract update.
var rejectNotAcceptedFields = map[string]struct{}{
	"url":            {},
	"user":           {},
	"password":       {},
	"streamUrl":      {},
	"streamUser":     {},
	"streamPassword": {},
	"brand":          {},
	"ip":             {},
	"district":       {},
	"location":       {},
	"type":           {},
}

// rejectReadonlyFields are system / path-param fields the caller must not
// include in the body. `lastOutboundHash` is system-managed.
var rejectReadonlyFields = map[string]struct{}{
	"deviceMgmtId":     {},
	"tenantId":         {},
	"workspaceId":      {},
	"sourceFamily":     {},
	"entityType":       {},
	"entityId":         {},
	"deviceId":         {},
	"createdAt":        {},
	"updatedAt":        {},
	"lastOutboundHash": {},
}

// metadataOnlyFields are accepted in the body for logging/observability but
// not persisted. Per §8.3 stub note.
var metadataOnlyFields = map[string]struct{}{
	"klynxEditAt": {},
}

// IfMatchStatus reports the outcome of the optional If-Match header for the
// observability response header `X-If-Match-Status`. v1 semantics per §8.7:
// purely informational — never gates the write, never returns 409.
type IfMatchStatus string

const (
	IfMatchAbsent     IfMatchStatus = "absent"
	IfMatchMatched    IfMatchStatus = "matched"
	IfMatchMismatched IfMatchStatus = "mismatched"
)

// OverlayValidationError carries the offending field names so the controller
// can populate `details.fields` per the contract error envelope.
type OverlayValidationError struct {
	Code   string   // FIELD_NOT_ACCEPTED | FIELD_READONLY | FIELD_UNKNOWN | VALIDATION_FAILED
	Fields []string // affected field names
	Reason string   // human-readable detail (e.g. "lat must be a number")
}

func (e *OverlayValidationError) Error() string {
	if e.Reason != "" {
		return e.Code + ": " + e.Reason
	}
	return e.Code
}

// ApplyKlynxOverlay applies a klynx-initiated PATCH to a gw-managed
// device_management record per klynx-api/docs/contracts/camera-gw-managed-overlay.md §8.
//
// body MUST be the decoded JSON object from the request (map keys preserved).
// ifMatch is the value of the optional If-Match header (empty string = absent).
//
// Returns the updated document, the IfMatchStatus observability value, and an
// error. The error is one of:
//   - *OverlayValidationError — caller maps to 400 with the appropriate code
//   - ErrNotFound             — caller maps to 404 DEVICE_NOT_FOUND
//   - any other error         — caller maps to 500 INTERNAL_ERROR
//
// `If-Match` is replay-only in v1 (§8.7): it never gates the write. The
// status is recorded for the response header and a structured log line; the
// write proceeds regardless.
func (s *DeviceManagementService) ApplyKlynxOverlay(
	ctx context.Context,
	tenantId, workspaceId, deviceMgmtId string,
	body map[string]any,
	ifMatch string,
) (*ingestmod.DeviceManagement, IfMatchStatus, error) {
	ctx, end, log := traceutil.StartLite(ctx, "gateway.devicemgmtsvc", "ApplyKlynxOverlay", "devicemgmtsvc", "ApplyKlynxOverlay")
	defer end()

	// 1) Validate body against whitelists. Each rejection path is non-retriable
	//    on the klynx side (§8.4), so collect every offending field in one pass
	//    rather than failing on the first one — gives klynx a clean error list
	//    to log and fix.
	var notAccepted, readonly, unknown, validation []string

	accepted := make(map[string]any, len(body))
	for k, v := range body {
		switch {
		case isAccepted(k):
			if err := validateAcceptedField(k, v); err != nil {
				validation = append(validation, k)
				continue
			}
			accepted[k] = v
		case isMetadataOnly(k):
			// klynxEditAt — log but do not persist
			continue
		case isNotAccepted(k):
			notAccepted = append(notAccepted, k)
		case isReadonly(k):
			readonly = append(readonly, k)
		default:
			unknown = append(unknown, k)
		}
	}

	// Order of error precedence — most semantically meaningful first.
	if len(notAccepted) > 0 {
		sort.Strings(notAccepted)
		return nil, IfMatchAbsent, &OverlayValidationError{Code: "FIELD_NOT_ACCEPTED", Fields: notAccepted}
	}
	if len(readonly) > 0 {
		sort.Strings(readonly)
		return nil, IfMatchAbsent, &OverlayValidationError{Code: "FIELD_READONLY", Fields: readonly}
	}
	if len(unknown) > 0 {
		sort.Strings(unknown)
		return nil, IfMatchAbsent, &OverlayValidationError{Code: "FIELD_UNKNOWN", Fields: unknown}
	}
	if len(validation) > 0 {
		sort.Strings(validation)
		return nil, IfMatchAbsent, &OverlayValidationError{Code: "VALIDATION_FAILED", Fields: validation, Reason: "type or null on non-nullable field"}
	}

	// 2) Load the existing record so we can (a) emit 404 if it does not exist,
	//    (b) compute the If-Match status against the current hash, and (c) have
	//    a baseline doc to publish from after the persist.
	existing, err := s.repo.FindById(ctx, tenantId, workspaceId, deviceMgmtId)
	if err != nil {
		return nil, IfMatchAbsent, err
	}

	// 3) Compute the new lastOutboundHash from the canonical accepted-fields
	//    payload (alphabetically-sorted keys, no whitespace). Per §8.4 this is
	//    what klynx stores for the next call.
	newHash, err := canonicalHash(accepted)
	if err != nil {
		return nil, IfMatchAbsent, err
	}

	// 4) Resolve If-Match observability (§8.7).
	ifMatchStatus := classifyIfMatch(ifMatch, existing.LastOutboundHash)

	// 5) Persist accepted fields + hash + updatedAt. Existing devicemgmtsvc.Update
	//    does not bump lastOutboundHash on its own (admin UI / bulk import /
	//    ingest auto-upsert all bypass this endpoint — §8.7 documents this as
	//    the reason If-Match is replay-only in v1).
	setFields := bson.M{}
	for k, v := range accepted {
		setFields[k] = v
	}
	setFields["lastOutboundHash"] = newHash
	setFields["updatedAt"] = time.Now().UTC()

	if err := s.repo.Update(ctx, tenantId, workspaceId, deviceMgmtId, setFields); err != nil {
		return nil, ifMatchStatus, err
	}

	// 6) Read back so we return the fully-current document. FindById is cheap
	//    and avoids a stale in-memory mutation.
	updated, err := s.repo.FindById(ctx, tenantId, workspaceId, deviceMgmtId)
	if err != nil {
		return nil, ifMatchStatus, err
	}

	// 7) Invalidate cache + emit notification (§8.5). The publisher payload is
	//    intentionally slim — klynx-side gwdevicecons treats it as
	//    invalidation-only per the contract, not as state-bearing.
	cachedevicemgmt.Invalidate(ctx, tenantId, workspaceId, updated.SourceFamily, updated.EntityType, updated.EntityId)
	publishDevicesChanged(ctx, updated, "update")

	log.Info().
		Str("deviceMgmtId", deviceMgmtId).
		Str("workspaceId", workspaceId).
		Int("acceptedFields", len(accepted)).
		Str("ifMatchStatus", string(ifMatchStatus)).
		Msg("klynx overlay applied")

	return updated, ifMatchStatus, nil
}

// classifyIfMatch implements the replay-only If-Match observability per §8.7.
// Returns "absent" when caller did not provide the header; "matched" when the
// supplied hash equals the current document's hash; "mismatched" otherwise.
// In all cases the write proceeds — this is informational only.
func classifyIfMatch(supplied, current string) IfMatchStatus {
	if supplied == "" {
		return IfMatchAbsent
	}
	if supplied == current {
		return IfMatchMatched
	}
	return IfMatchMismatched
}

// canonicalHash returns sha256-hex of the JSON-encoded accepted-fields payload
// with deterministic key ordering. Sort the keys explicitly so two semantically
// equal payloads always hash the same regardless of map iteration order or the
// caller's body encoding.
func canonicalHash(accepted map[string]any) (string, error) {
	keys := make([]string, 0, len(accepted))
	for k := range accepted {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// Use json.Marshal on a small slice of [key, value] pairs to keep ordering
	// deterministic. Marshalling a map directly would lose the ordering.
	type kv struct {
		K string
		V any
	}
	pairs := make([]kv, len(keys))
	for i, k := range keys {
		pairs[i] = kv{K: k, V: accepted[k]}
	}
	b, err := json.Marshal(pairs)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}

func isAccepted(k string) bool      { _, ok := acceptedOverlayFields[k]; return ok }
func isNotAccepted(k string) bool   { _, ok := rejectNotAcceptedFields[k]; return ok }
func isReadonly(k string) bool      { _, ok := rejectReadonlyFields[k]; return ok }
func isMetadataOnly(k string) bool  { _, ok := metadataOnlyFields[k]; return ok }

// validateAcceptedField enforces type contracts on accepted-field values.
// Per §8.3: absent = no change; "" = set empty (string fields); null = 400.
// Wrong type = 400.
func validateAcceptedField(name string, v any) error {
	if v == nil {
		return errors.New("null is not allowed; omit the field to keep current value")
	}
	switch name {
	case "name", "description", "site", "zone", "serialNo":
		_, ok := v.(string)
		if !ok {
			return errors.New("must be a string")
		}
	case "lat", "lng":
		switch v.(type) {
		case float64, float32, int, int32, int64:
			// json.Unmarshal will give float64 for numbers; allow ints too.
		default:
			return errors.New("must be a number")
		}
	}
	return nil
}
