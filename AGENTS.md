# Architecture Review Workflow

This repository uses a plan-review-implement workflow for cross-repo changes.

## Roles

### Claude

Claude is the primary planner and implementer.

Responsibilities:
- analyze current state and cross-repo impact
- create or update `docs/plan/<name>.md`
- create or update `docs/contracts/<name>.md` or `openapi/<name>.yaml`
- revise the plan based on review feedback
- implement only after the plan is ready
- validate and test after implementation

### Codex

Codex acts as architecture manager and review gate before implementation.

Responsibilities:
- review the plan and contract before implementation starts
- identify architecture drift, contract gaps, rollout risks, and missing validation
- force clarification on ownership, system-of-record, sync authority, and compatibility
- return blocking issues and high-risk assumptions when the plan is not ready
- approve the plan when it is good enough to implement and validate

Target quality bar:
- the plan does not need to be perfect
- the plan should be at least 80% ready before implementation starts

## Standard Context

These defaults apply unless the plan explicitly justifies an override.

- Feature owner backend: default = `klynx-api`
- Events system of record: `gateway-api`
- Kafka topic for normalized events: `gw.events.normalized.v1`
- Event producer: `gateway-api`
- Event consumer: `klynx-api`
- Device/camera identity and sync state source of truth: `gateway-api/device_management`
- `klynx/camera` is a projection and consumer model for Klynx workflows
- Frontend must follow documented backend contracts and must not invent **any** schema — REST request/response/error, Kafka event, MQTT topic+payload, Redis-visible behavior (TTL / invalidation timing / stale-read window), permission rule, auth flow, sync rule. Network traces, screenshots, and BE source code are not contracts.

## Contract Authority Model

The platform runs hub-and-spoke for cross-repo contracts:

- **Hub (cross-repo / cross-service flow contracts):** `klynx-api/docs/contracts/<name>.md` is canonical. It covers REST + Kafka + MQTT + Redis + sync + cache + rollout for the flow, grouped by domain or flow (not per endpoint or per topic). See `klynx-api/docs/contracts/README.md` for grouping rules.
- **`gateway-api`-owned domains (this repo's SoR):** events canonical detail and `device_management` identity / sync state are owned here. Cross-repo flows that touch these domains must mirror or link the gateway-api SoR; klynx-api hub contracts that consume these domains reference `gateway-api` as the canonical writer.
- **OpenAPI / Swagger (`docs/swagger.yaml`):** is the REST schema *subset* of a contract — not the full contract. Async surfaces (Kafka topics, MQTT topics, Redis-visible behavior) and cross-repo behavior (write authority, field ownership, rollout, replay) live in the contract `.md` only. `docs/swagger.yaml` is regenerated from the `swag` annotations on controllers; it must stay aligned with the REST surfaces declared in any consuming contract.
- **`gateway-api/docs/contracts/<name>.md`:** reserved for `gateway-api`-owned cross-repo behavior that has no klynx-api hub equivalent (e.g. when `gateway-api` ships a flow consumed only by `gateway-portal` or by 3rd-party integrators). Use `klynx-api/docs/contracts/TEMPLATE.md` as the skeleton; keep the same domain-or-flow grouping rule.

## Cross-Repo Paths

- BE1 (`klynx-api`): `/home/klynx/klynx-api`
- BE2 (`gateway-api`): `/home/phibek/gateway-api`
- FE1 (`KLynx-Platform`): `/home/klynx/klynx`
- FE2 (`gateway-Platform`): `/home/phibek/gateway-portal`
- FE3 (`phibek`): `/home/phibek/app`

## Required Workflow

### 1. Plan First

For any feature, bug, flow change, event change, sync change, deploy change, or cross-repo work:

- do not implement first
- create or update the plan artifact in `docs/plan/`
- create or update the shared contract artifact in `docs/contracts/` when API, event, sync, or FE integration is affected

### 2. Review Before Implementation

Codex reviews:
- architecture direction
- feature owner
- domain system of record
- producer / consumer / topic mapping
- canonical store / projection store
- request / response / error contract
- sync ownership, field ownership, write authority, idempotency
- cross-repo rollout order
- scope, risks, rollback, validation, testing

If the plan is not ready, the verdict must be:

`Plan requires revision before implementation.`

If the plan is ready, the verdict must be:

`Plan is ready for Claude to implement and validate.`

### 3. Revise Until Ready

Claude revises the plan and contract using Codex feedback until the review gate is clear.

### 4. Implement After Approval

Implementation order:
1. owner backend first
2. upstream or peer backend changes next
3. update contract/docs to match actual implementation
4. FE changes after backend contract is stable
5. validate and test

## Review Rules

### Feature Ownership

- feature owner backend must be declared for every cross-repo change
- if the owner is not `klynx-api`, the plan must explain why

### System of Record

- system of record must be declared per domain
- do not collapse ownership into one global statement

Required defaults:
- events canonical detail: `gateway-api`
- Klynx event projection: `klynx-api/event_refs`
- device/camera identity and sync state: `gateway-api/device_management`
- Klynx camera data: projection for Klynx workflows

### Event Flow

If the change touches events, the plan and contract must state:
- topic names
- producers
- consumers
- canonical store
- projection store
- replay or re-sync behavior
- idempotency or duplicate handling

### Device/Camera Sync

If the change touches device/camera sync, the plan and contract must state:
- write authority
- field ownership
- allowed initiators
- conflict resolution
- echo-loop prevention when relevant

The safe default is:
- Klynx may initiate change
- `gateway-api/device_management` persists first
- downstream sync updates Klynx projection afterward

### Frontend Contract Rule

- frontend must not guess schema across **any** surface — REST request/response/error, Kafka event, MQTT topic + payload, Redis-visible behavior (TTL / invalidation timing / stale-read), permission, auth, or sync. Network traces, screenshots, and BE source code are not contracts.
- frontend should implement only against the shared backend contract:
  - cross-repo / hub flows: `klynx-api/docs/contracts/<name>.md` (with `klynx-api/openapi/<name>.yaml` as REST subset when linked from the `.md`)
  - `gateway-api`-direct REST: `gateway-api/docs/swagger.yaml` (REST subset) plus any `gateway-api/docs/contracts/<name>.md` that exists for cross-repo behavior
- frontend plan / PR description must cite the exact contract file AND section (e.g. `klynx-api/docs/contracts/gateway-klynx-realtime.md §7.1`, not just "see contract")
- if the FE needs behavior the contract does not cover (a new endpoint, new MQTT topic, different Redis TTL / invalidation, different permission rule, etc.) — stop and request a backend contract update first
- the plan must contain enough detail that FE1 (`klynx`) and FE2 (`gateway-portal`) can implement without reverse-engineering backend code, without watching network traces, and without inferring behavior from screenshots

## Minimum Plan Contents

Every cross-repo plan should contain:
- scope
- current state
- ownership model
- contract summary
- field ownership and sync rules when applicable
- cross-repo impact
- rollout order
- rollback strategy
- decision points
- validation checklist

Use `docs/plan/TEMPLATE.md`.

## Prompt Scaffolding

Codex prompt scaffolding lives under `.codex/`.

- `.codex/prompts/review-plan.md`
- `.codex/prompts/revise-plan.md`
- `.codex/prompts/implement-approved-plan.md`
- `.codex/prompts/start-cross-repo-plan.md`
