# Plan — License EPS soft metric (Phase B / P2 spoke, gateway-api)

**Date:** 2026-06-23
**Owner Backend:** `gateway-api` (events SoR + EPS rolling window) — spoke of the klynx-api license entitlement line
**Hub contract:** [klynx-api/docs/contracts/license-entitlements.md](../../../klynx/klynx-api/docs/contracts/license-entitlements.md) §6 (authority — read first). `maxEventsPerSec` lives on the license; klynx-api supplies the number, gateway-api owns the window + metric.
**Predecessors (shipped):** klynx-api 4.127.0 (P2 runtime guards) + 4.128.0 (P2.1 appliance resolver). EPS was deliberately deferred to this PR (see the P2 plan's F1 gate).
**Status:** Reviewed (2026-06-23) — APPROVED with revisions (see Review Outcome). Implementing.

> This is the **EPS half** of license entitlements P2. It is observe-only: events are the system of record and are **never dropped**. The metric exists so sales/ops can see when a deployment exceeds its licensed rate and upsell.

---

## ✅ Review Outcome (2026-06-23) — APPROVED with revisions

Review-gate verified the plan against the actual repo. Core idea (rolling-window EPS keyed by `WorkspaceID`, observe-only, fail-open, no drop) is sound and **F1 keying is correct**. Three premises were wrong against the codebase and are corrected below; these are the plan-of-record:

1. **Limit source = `entitlementsvc`, NOT a new klynxgw HTTP pull.**
   `entitlementsvc.GetForWorkspace()` already returns `RuntimeEntitlement.MaxEventsPerSecond` (`internal/services/entitlementsvc/types.go:14`), keyed per workspace, Redis-cached (TTL 5m), fail-open, populated by `entitlementcons` from `klynx.entitlement.snapshot.v1`. **klynx already supplies the number.** → Scope #3 drops the `klynxgw/client.go` change AND the cross-repo klynx endpoint. No new HTTP, no new contract surface.
2. **Metric export = Prometheus `/metrics` on a dedicated internal listener.** The repo has **no Prometheus client** (`aimappingsvc/metrics.go` is plain `sync/atomic` + an unwired `GetMetrics()` snapshot — the plan's "promauto + repo registry" premise was false). The **cluster** does run `kube-prometheus-stack` (Prometheus + Grafana + Alertmanager, `monitoring` ns), so we add `prometheus/client_golang` to gateway-api and expose `/metrics` on a **separate internal port** (e.g. `METRICS_ADDR`, default `:9091`) that is **never** mounted under `BASE_PATH` / istio HTTPRoute → satisfies F3 by construction. A `ServiceMonitor` (infra, lives with the gateway-api deploy — NOT in `gateway-portal/deploy/phibek/`, which is the FE portal) targets that port. Alert rules (80/100/120%) stay infra-side.
3. **Counting hook is appliance/default-profile scoped.** `eventbridge.Publish()` (`consumer.go:360-364`) only fires for appliance/default; saasPublic goes to `events.delivery.v1` and is **not** counted. Acceptable — this is the appliance license line (contract §6) — but stated explicitly; the recorder stays profile-agnostic so a saasPublic hook can be added later.

**Minor corrections:**
- **Unlimited/unknown sentinel:** the gateway catalog sets EPS to concrete numbers (freemium 10 / pro 100 / appliance+enterprise 1000); `-1` is used for `MaxAssets`/`MaxSources` etc., **never EPS**. Rule: `MaxEventsPerSecond > 0` → emit `license_eps_limit` + `license_eps_percent`; `<= 0` (unknown/unlimited) → emit `license_eps_current`(+peak) only, **omit limit + percent** (fail-open, no false "exceeded").
- **Empty WorkspaceID:** `publisher.go:53-56` falls back to `EventID` as the partition key when WS is empty, so WS is *not strictly* guaranteed. The recorder **skips** empty-WS events (no `""` / `unknown` bucket) to avoid a junk catch-all series.
- **`customer` label** = `tenantId`, resolved via the existing workspace→tenant resolver (`entitlementsvc.WorkspaceTenantResolver`), cached. No new resolver.

**Verdict:** APPROVED to implement with the above. Do not touch `klynxgw/client.go`; do not add a klynx endpoint.

---

## ⛔ F1 — RESOLVED (org/customer key on the normalized topic)

The P2 plan's F1 gate required confirming `gw.events.normalized.v1` carries a stable org/customer key before per-customer keying.

**Finding (verified in this repo):**
- `internal/eventschema/normalized.go:19-24` — `NormalizedEvent` has `OrgID` (`json:"orgId,omitempty"` — **not guaranteed populated**) and `WorkspaceID` (`json:"workspaceId"` — **always present**).
- `internal/eventbridge/publisher.go:53` — the producer ALWAYS sets the Kafka message key to `event.WorkspaceID`. So **WorkspaceID is guaranteed on every produced event**; OrgID is best-effort.

**Decision:** key the EPS window by **`WorkspaceID`** (guaranteed, the existing partition key). Resolve `WorkspaceID → customerAccount/org` for (a) the per-customer `maxEventsPerSec` limit and (b) the metric labels. Do **not** key by `OrgID` (would silently drop un-orged ingest paths from the count — `ata_events` / `atapi/*` historically lack it; cf. klynx memory `project_edge_ai_summary_report`). On a single-tenant appliance the window is effectively deployment-wide regardless.

---

## User-visible Problem

A license now carries `maxEventsPerSec` (soft). Nothing measures actual event rate against it, so ops can't see a deployment running hot or approaching its licensed ceiling, and the P3 FE "Current / Peak EPS" card has no source.

## Scope (Phase B)

1. **Rolling-window EPS counter** keyed per `WorkspaceID`, hooked on the **producer** path — `internal/eventbridge/publisher.go` `Publish()` (every event that lands on `gw.events.normalized.v1` is counted exactly once, at the single choke point). Windows: **1s / 10s / 1m** (current + short + smoothed peak).
2. **Prometheus export** — add `prometheus/client_golang` and register a custom `prometheus.Collector` (reads the ring buffers out-of-band on scrape) on a dedicated `prometheus.Registry`, served by `promhttp` on a **separate internal listener** (`METRICS_ADDR`, default `:9091`) — never under `BASE_PATH`/istio (F3). Series:
   - `license_eps_current{workspace,customer}` — current 1s rate.
   - `license_eps_peak{workspace,customer}` — smoothed peak (max 10s-avg rate observed).
   - `license_eps_limit{workspace,customer}` — the licensed `maxEventsPerSecond` (emitted only when `> 0`; unknown/unlimited ⇒ omitted).
   - `license_eps_percent{workspace,customer}` — current ÷ limit × 100 (omitted when limit `<= 0`).
   - Thresholds are **alert-only** (Prometheus alert rules, infra-side, not in this PR's code): 80% warn / 100% exceeded / 120% critical. **No drop, no reject.**
3. **Limit source** — reuse `entitlementsvc.GetForWorkspace()` → `RuntimeEntitlement.MaxEventsPerSecond` (already keyed per workspace, Redis-cached TTL 5m, fail-open, fed by `entitlementcons` from `klynx.entitlement.snapshot.v1`). The collector reads it out-of-band on scrape via an injected `LimitResolver` interface (small TTL cache inside the eps pkg). **No klynxgw change, no new klynx endpoint, no cross-repo contract surface.**
4. **WorkspaceID → customer resolution** — `customer` label = `tenantId` via the existing `entitlementsvc.WorkspaceTenantResolver` (`GetTenantIDForWorkspace`), injected as a `CustomerResolver` interface and cached (changes rarely).

## Out of scope (Phase B)
Hard EPS reject, grace period, per-node EPS, dropping/sampling events, the FE card itself (P3). Contract §6 / plan "Could Go Further".

## Files To Change (pointers)
- **New** `internal/metrics/eps/` — `recorder.go` (per-workspace sharded ring buffer, 1s/10s/1m + peak, 1 Hz roller goroutine, idle prune), `collector.go` (`prometheus.Collector` joining limit+customer, fail-open), `resolver.go` (`LimitResolver`/`CustomerResolver` interfaces + small TTL cache), `server.go` (internal `/metrics` listener), tests beside.
- `internal/eventbridge/publisher.go` — call `recorder.Observe(event.WorkspaceID)` in `Publish()` (single hook). Allocation-light, non-blocking, skip empty WS. Injected recorder; nil ⇒ disabled.
- Wiring in `main.go`/container — construct recorder+collector behind `LICENSE_EPS_METRIC_ENABLED` (default off, per Rollback), inject `entitlementsvc` adapters as `LimitResolver`/`CustomerResolver`, start the internal listener.
- **No `klynxgw/client.go` change. No klynx-api PR / endpoint** (limit already arrives via the entitlement snapshot).
- **Infra (separate, not this repo):** a `ServiceMonitor` for the gateway-api `METRICS_ADDR` port + the 80/100/120% alert rules — lives with the gateway-api k8s deploy, NOT `gateway-portal/deploy/phibek/`.

## Security pins (review-gate F3)
- `license_eps_*` series labeled per-customer/workspace MUST stay on the **internal ops scrape path only** — never a tenant-reachable route, or one tenant reads every tenant's EPS via labels.
- The P3 FE "Current / Peak EPS" card is served by an **org-scoped klynx-api API filtered to the caller's customer** — NOT by proxying the raw labeled Prometheus series.

## Risk
- **`Publish()` hot path** — the counter must be O(1), lock-light (per-workspace sharded), and never block/allocate per event. Mitigation: atomic ring-buffer counters; export reads them out-of-band.
- **Unknown/unlimited limit (`MaxEventsPerSecond <= 0`)** — gateway catalog uses concrete EPS numbers (10/100/1000), never `-1` for EPS; but a klynx snapshot may send `<= 0`. In that case omit `license_eps_limit` + `license_eps_percent`, emit `current`(+peak) only. No misleading percent.
- **Limit-read fail-open** — the collector reads the limit from `entitlementsvc` out-of-band on scrape; a Redis miss/error ⇒ treat as unknown (suppress percent/exceeded), never fabricate an "exceeded" alert. The hot path never touches the limit.
- **Cardinality** — labels are `{workspace, customer}` only; never per-device/per-event labels.
- **Single-tenant appliance** — WorkspaceID may be a single value; the window is effectively deployment-wide. Acceptable (contract §6).

## Test Plan
- Rolling-window math (1s/10s/1m) under synthetic event bursts; per-workspace isolation; peak tracking.
- Limit join: licensed vs unlimited vs limit-unknown (fail-open) → correct `license_eps_limit/percent` (or omission).
- Metric NOT exposed on any tenant route (only the internal scrape endpoint).
- `Publish()` overhead micro-benchmark stays negligible with the counter on.
- `gofmt` / `go vet` / `go build ./...` / `go test ./...`.

## Rollback Plan
- Observe-only: removing the exporter + the one `Publish()` hook has zero functional impact on event flow (events never depended on it). Gate the hook behind an env flag (e.g. `LICENSE_EPS_METRIC_ENABLED`, default off) so it can be disabled without a revert. No data migration.

## Review Gate
Run the Claude review-gate (Codex offline) before implementing — focus: F1 key correctness (WorkspaceID vs OrgID), `Publish()` hot-path safety, F3 metric isolation (no tenant-reachable labeled series), fail-open on limit-unknown, cross-repo ownership (klynx supplies the number, gateway owns the window). Verdict line required.
