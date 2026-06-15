# Plan — stamp `severity` on the normalize/producer path (blacklist = high)

**Owner backend:** gateway-api (events/severity canonical owner)
**Hub contract:** `klynx-api/docs/contracts/event-severity-forwarding.md` (authority)
**Status:** Designed; review-gate PASSED (Claude-held); implementation pending greenlight.
**Date:** 2026-06-15

## User-visible Problem

On `app.aisom.cloud`, AIBOX watchlist **Blacklist** captures
(`payload.listType==3`) should read as **severe** ("เหตุการณ์รุนแรง") on
`/intDash` + `/biDash`. The intDash "เหตุการณ์รุนแรง" KPI (`severity=high` count)
shows **0** because `gw.events.normalized.v1` never carries `severity`.

## Root cause (verified in code)

The severity classification mechanism is fully built but wired only on the
**delivery** path, not the **producer** path that feeds klynx:

- `eventschema.NormalizedEvent.severity` exists; `buildBridgeEvent` forwards
  `event.EventSeverity` ([normalizedcons/consumer.go:529](../../internal/kafka/normalizedcons/consumer.go)).
- BUT `event.EventSeverity` is never populated on the normalize path —
  `applyClassificationRules` is called **only** in
  [deliverycons/dispatch.go:95](../../internal/kafka/deliverycons/dispatch.go)
  (HTTP/notification delivery). The comment in `buildBridgeEvent` ("populated by
  normalizesvc.classification before this build site runs") is aspirational —
  no such call exists.
- Result: every `gw.events.normalized.v1` event ships with `severity` omitted →
  klynx projects `severity=""` → intDash KPI = 0, all badges gray.

This matches `event-severity-forwarding.md` header status ("no implementation yet").

## Decision (user, 2026-06-15): seed default in code **+** template override

Stamp a **watchlist default** (`listType 3→high, 2→medium`, else unset) on the
normalize path so it works platform-wide with zero config, AND apply the matched
template's `ClassificationRules` first so an admin can override per-template via
the existing gateway-portal templates UI.

## Design (small, contained)

`event.Payload` on the normalize path is `normalizedFields` (flat `listType`,
identical to what klynx receives — verified consumer.go:189), so rules use
`field: "listType"`.

### 1. New shared package `internal/services/classifysvc`

Move the 3 private classifier funcs out of `deliverycons/filter.go` so both
consumers share one implementation:

- `MatchesFilter(payload, []PayloadCondition) bool` — moved verbatim.
- `ApplyClassificationRules(event *ingestmod.NormalizedEvent, rules, withDefaults bool)`
  — moved; `withDefaults` gates the legacy `unknown`/`none` defaults.
  - delivery path → `withDefaults=true` (byte-for-byte legacy behavior).
  - normalize path → `withDefaults=false` (leave `""` when no rule matches →
    compact wire, klynx maps `""→none`).
- `WatchlistSeverityDefault(payload) string` — `3→"high"`, `2→"medium"`, else
  `""`. Tolerates flat `listType` OR nested `eventAttribute.listType`.
- `getNestedValue` — moved (unexported).

### 2. `deliverycons/filter.go` — thin delegators (no caller changes)

```go
func matchesFilter(p map[string]any, c []ingestmod.PayloadCondition) bool {
    return classifysvc.MatchesFilter(p, c)
}
func applyClassificationRules(e *ingestmod.NormalizedEvent, r []ingestmod.ClassificationRule) {
    classifysvc.ApplyClassificationRules(e, r, true) // withDefaults — legacy delivery behavior
}
```
`dispatch.go:95`/`:116` callers unchanged. Delivery behavior identical.

### 3. `normalizedcons/consumer.go` — stamp before `buildBridgeEvent` (~line 305)

```go
// Layer C — stamp severity/eventClass on the canonical path so klynx consumers
// (intDash/biDash) get them. Template ClassificationRules win; a watchlist
// default fills blacklist→high when no rule set severity. event-severity-forwarding.md.
if templateId != "" {
    if tmpl, err := deps.TemplateRepo.FindById(ctx, workspaceId, templateId); err == nil && tmpl != nil {
        classifysvc.ApplyClassificationRules(event, tmpl.ClassificationRules, false)
    }
}
if event.EventSeverity == "" {
    event.EventSeverity = classifysvc.WatchlistSeverityDefault(event.Payload)
}
```

`event` is `*ingestmod.NormalizedEvent`; `workspaceId`/`templateId` in scope;
template is Redis-cached (same read delivery already does). No new deps.

## Hub contract delta (apply to klynx-api)

`klynx-api/docs/contracts/event-severity-forwarding.md`:
- Flip header status from "no implementation yet" → producer-path classification
  shipped (gateway-api 3.x).
- Add §6.2 "Normalize-path classification (producer stamping)": gateway-api
  applies `ClassificationRules` + watchlist default on `normalizedcons` before
  forwarding; rule-match-only (no forced defaults) so `severity` stays `""` when
  unset; watchlist default `3→high, 2→medium`; precedence = template rule >
  default. No klynx config UI — rules live on the gateway mapping template
  (gateway-portal). klynx already projects `severity` + filters `?severity=`.

## Files To Change

- NEW `internal/services/classifysvc/classify.go` (+ `_test.go`).
- `internal/kafka/deliverycons/filter.go` — delegate to classifysvc.
- `internal/kafka/normalizedcons/consumer.go` — stamp before `buildBridgeEvent`.
- `version.go` + `CHANGELOG.md` (minor bump).
- klynx-api hub contract (separate small PR).

## Files Not To Touch

- klynx-api projection/REST/WSS — already consumes `severity`; no change.
- `deliverycons/dispatch.go` callers — unchanged (thin wrappers preserve them).

## Risk

Low–medium. The only behavior change to the existing delivery path is the
refactor-with-delegation (kept byte-identical via `withDefaults=true` + thin
wrappers) — covered by moving the existing `deliverycons` filter tests + asserting
parity. New normalize-path stamping is additive (klynx already tolerated empty
severity). Watchlist default could surprise if an org expected blacklist ≠ high —
overridable per template, and documented.

## Test Plan

- `classifysvc` unit tests: MatchesFilter (eq/in), ApplyClassificationRules
  (order, first-match, withDefaults on/off), WatchlistSeverityDefault (3/2/1/0/
  nested/absent).
- deliverycons parity: existing filter/classification tests still green.
- normalizedcons: a `listType==3` event → `buildBridgeEvent` output has
  `severity:"high"`; a `listType==0` event → `severity` unset; a template rule
  setting `severity` overrides the default.
- `go build ./... && go vet ./... && go test ./...`.

## Rollback Plan

Revert the 3 code edits + version/changelog. classifysvc is new (drop it);
deliverycons reverts to its in-file funcs. No schema/topic/migration change — the
wire field is additive and already in `eventschema.NormalizedEvent`.

## Review Gate (Claude-held — Codex offline)

- **Ownership/SoR:** severity classification is gateway-api's per
  `event-severity-forwarding.md`; this completes the producer-side stamping the
  contract always intended. klynx unchanged (consumer). ✔
- **Write authority:** no new store/topic/collection; stamps an existing wire
  field from existing template config. ✔
- **Contract completeness:** hub contract §6.2 added; precedence + default +
  no-klynx-UI documented; rule field (`listType`) + payload shape pinned. ✔
- **Delivery-path safety:** refactor preserves legacy behavior via
  `withDefaults=true` + thin wrappers; parity asserted by reused tests. ✔
- **Tenant isolation:** template loaded by `(workspaceId, templateId)` — same
  scoping as delivery; no cross-tenant read. ✔
- **Rollout/compat:** additive wire field (omitempty); pre-feature klynx
  consumers already mapped `""→none`. gateway-api ships; klynx already ready. ✔

**Verdict:** Plan is ready for Claude to implement and validate.
