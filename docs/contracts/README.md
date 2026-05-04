# `gateway-api/docs/contracts/` — Reserved for gateway-api-owned cross-repo contracts

This directory is reserved for cross-repo / cross-service integration contracts that are **owned by `gateway-api`** and have no `klynx-api` hub equivalent.

## Authority model — read this first

The platform runs hub-and-spoke for cross-repo contracts. Most contracts live in the **hub**:

- **Hub:** `klynx-api/docs/contracts/<name>.md` — canonical for cross-repo / cross-service flows. Grouped by domain or flow (REST + Kafka + MQTT + Redis + sync + cache + rollout in one file when they share one lifecycle). See `klynx-api/docs/contracts/README.md` for the grouping rule.
- **`gateway-api` SoR:** events canonical detail and `device_management` identity / sync state. Hub contracts that touch these domains reference `gateway-api` as the canonical writer; this directory does **not** duplicate the hub contract — it complements it only when needed.

## When to add a contract here

Add a `gateway-api/docs/contracts/<name>.md` only when **all** are true:

1. The flow is owned end-to-end by `gateway-api` (not driven by a klynx-api feature).
2. The consumer is `gateway-portal`, a 3rd-party integrator, or an internal `gateway-api` operator — not `klynx-api`.
3. The flow is more than REST schema (otherwise the `swag`-generated `docs/swagger.yaml` is enough).

Examples that would belong here:
- `gateway-api` ↔ `gateway-portal` flows that are not part of any klynx-api feature
- 3rd-party webhook / ingest contracts where `gateway-api` is the publisher and the consumer is external
- `device_management` operator-level lifecycle that `gateway-portal` uses directly without going through klynx-api

## When NOT to add a contract here

- Cross-repo flows where `klynx-api` is the feature owner — the contract belongs in `klynx-api/docs/contracts/`. This includes most camera / event / sync flows.
- REST-only surfaces with no async or sync behavior — `docs/swagger.yaml` is sufficient.
- Internal-to-`gateway-api` behavior that no other repo consumes.

## Authoring rules

When you do add a contract here:

1. Use `klynx-api/docs/contracts/TEMPLATE.md` as the skeleton — same template, same domain-or-flow grouping rule, same surface sections (REST / Kafka / MQTT / Redis / Sync).
2. Group surfaces by domain or flow. Do **not** create per-endpoint or per-topic micro-contracts; if a reader has to open more than one file to understand a single user-facing flow, the split was wrong — merge.
3. `docs/swagger.yaml` is the REST *subset*; reference it from `§5. REST Surfaces` instead of duplicating field tables.
4. Field ownership, write authority, and conflict resolution must be explicit when sync or projection is involved.
5. Mark the `Owner Backend` field as `gateway-api` (not the default `klynx-api`) and explain why in the plan.

## See also

- [`../../AGENTS.md`](../../AGENTS.md) — plan-review-implement workflow
- [`../../CLAUDE.md`](../../CLAUDE.md) — `gateway-api` Claude rules + Contract Authority Model section
- `klynx-api/docs/contracts/README.md` — hub grouping rule (canonical)
- `klynx-api/docs/contracts/TEMPLATE.md` — canonical contract skeleton (use this here too)
- [`../swagger.yaml`](../swagger.yaml) — `gateway-api` REST schema (OpenAPI subset)
