# PLAN_INGEST.md — Frontend Plan: Ingest + Delivery + Subscription

> Scope: Frontend (portal) pages สำหรับระบบ Ingest, Delivery, DLQ, Subscription
> อ้างอิง: `baseline/DELIVERY_NOTIFY_PLAN.md`, `baseline/README.md` (BE), `portal/baseline/README.md` (FE)
> Updated: 2026-03-07

---

## 1) Current State (Baseline FE)

### Existing Pages

| Route | Status | Description |
|---|---|---|
| `ingest/management` | Done | Event management: list, view, approve, reject, bulk ops, create template from event |
| `ingest/details` | Done | Approved events list + detail view |
| `ingest/mappingTemplates` | Done | Mapping template CRUD (name, match, field mappings) |
| `subscription` | Done | Plan cards (hardcoded PLANS), enterprise activation |
| `orgs/targets` | Done | Delivery targets CRUD (webhook, LINE, Discord, Telegram) |

### Existing API Libraries

| File | Functions |
|---|---|
| `lib/api/ingest.ts` | Events CRUD, bulk ops, templates CRUD, dashboard stats |
| `lib/api/target.ts` | Delivery targets CRUD (`/api/v1/targets`) |
| `lib/api/subscription.ts` | bootstrap, getSubscription, changePlan, activateEnterprise (hardcoded PLANS) |

### Existing Types

| File | Key Types |
|---|---|
| `lib/types/org.ts` | `DeliveryTarget`, `WebhookConfig`, `LineConfig`, `TelegramConfig`, `DiscordConfig` |
| `lib/api/ingest.ts` | `PendingEvent`, `MappingTemplate`, `FieldMapping`, `MatchRule` (inline) |
| `lib/api/subscription.ts` | `Subscription`, `PlanInfo`, `PlanId` (inline) |

### Existing Backend API Endpoints

| Method | Path | Controller | Status |
|---|---|---|---|
| GET/POST | `/api/v1/targets` | TargetController | Done |
| GET/PATCH/DELETE | `/api/v1/targets/:id` | TargetController | Done |
| GET | `/api/v1/subscriptions/packages` | SubscriptionController | Done |
| GET | `/api/v1/subscriptions/current` | SubscriptionController | Done |
| POST | `/api/v1/subscriptions/bootstrap` | SubscriptionController | Done |
| GET | `/api/v1/subscriptions/me` | SubscriptionController | Done |
| PATCH | `/api/v1/subscriptions/plan` | SubscriptionController | Done |
| POST | `/api/v1/subscriptions/enterprise/activate` | SubscriptionController | Done |
| GET/POST | `/api/v1/ingest/management` | EventManagementController | Done |
| GET/PATCH/DELETE | `/api/v1/ingest/management/:eventId` | EventManagementController | Done |
| POST | `/api/v1/ingest/management/:eventId/approve` | EventManagementController | Done |
| POST | `/api/v1/ingest/management/:eventId/reject` | EventManagementController | Done |
| POST | `/api/v1/ingest/management/bulk/*` | BulkController | Done |
| GET/POST | `/api/v1/ingest/mappingTemplates` | TemplateController | Done |
| GET/PATCH/DELETE | `/api/v1/ingest/mappingTemplates/:id` | TemplateController | Done |
| GET | `/api/v1/ingest/dlq` | DLQController | Done |
| GET | `/api/v1/ingest/dlq/stats` | DLQController | Done |
| GET | `/api/v1/ingest/dlq/:id` | DLQController | Done |
| POST | `/api/v1/ingest/dlq/:id/retry` | DLQController | Done |
| POST | `/api/v1/ingest/dlq/:id/replay` | DLQController | Done |
| POST | `/api/v1/ingest/dlq/:id/abandon` | DLQController | Done |
| GET | `/api/v1/ingest/details` | EventDetailsController | Done |
| GET | `/api/v1/ingest/details/:eventId` | EventDetailsController | Done |

---

## 2) Target State

### New/Updated Pages

| Route | Action | Description |
|---|---|---|
| `ingest/mappingTemplates` | **Update** | Add delivery targets binding, messageTemplates, classificationRules, DLQ config, defaultLocale |
| `ingest/management` | **Update** | When event has template match -> link to edit existing template (not always create new) |
| `delivery/targets` | **New** | Delivery target management with quota display from subscription |
| `delivery/templates` | **New** | Message template management (per-channel, per-locale, preview) |
| `delivery/dlq` | **New** | DLQ list with filters (status, stage, channel, date range) |
| `delivery/dlq/[id]` | **New** | DLQ detail: payload view, error stack, retry/replay/abandon actions |
| `subscriptions/packages` | **New** | Subscription packages from API (replace hardcoded PLANS) |
| `subscriptions/current` | **New** | Current subscription + usage/quota overview |

### Empty directories already created (from prior session):

```
src/routes/(app)/delivery/targets/        (empty)
src/routes/(app)/delivery/templates/      (empty)
src/routes/(app)/delivery/dlq/            (empty)
src/routes/(app)/delivery/dlq/[id]/       (empty)
src/routes/(app)/subscriptions/packages/  (empty)
src/routes/(app)/subscriptions/current/   (empty)
```

---

## 3) Implementation Plan (PRs)

### PR-FE-1: Types + API Libraries + i18n + Sidebar

**Files to create:**

| File | Description |
|---|---|
| `i18n/en/delivery.json` | Delivery i18n keys (targets, templates, DLQ, channels, filters, quota) |
| `i18n/th/delivery.json` | Thai translations |
| `i18n/en/subscription.json` | Subscription i18n keys (packages, current, usage, plans) |
| `i18n/th/subscription.json` | Thai translations |

**Files to update:**

| File | Changes |
|---|---|
| `lib/types/org.ts` | Update `LineConfig.to` from `string` to `string[]`, add `channelAccessToken` field |
| `lib/api/ingest.ts` | Update `MappingTemplate` type: add `deliveryTargets`, `messageTemplates`, `classificationRules`, `dlq`, `defaultLocale` fields. Update `updateTemplate()` to accept new fields. |
| `lib/api/ingest.ts` | Add DLQ API functions: `listDlq()`, `getDlqStats()`, `getDlqDetail()`, `retryDlq()`, `replayDlq()`, `abandonDlq()` |
| `lib/api/subscription.ts` | Add `listPackages()` to call `GET /subscriptions/packages`. Replace hardcoded `PLANS` with API-driven data. Add `getCurrentSubscription()` for effective subscription. |
| `lib/api/target.ts` | Update API path from `/targets` to `/v1/targets` if needed. No major changes — existing CRUD is correct. |
| `lib/stores/appSidebarMenus.ts` | Add "Delivery" section (Targets, Templates, DLQ) and update Subscription entry |

**Sidebar structure (target):**

```
Events (existing)
  - Management
  - Details
  - Mapping Templates

Delivery (new section)
  - Targets
  - Message Templates
  - Dead Letter Queue

Tenancy (existing)
  - Users
  - Units
  - Targets            <-- rename to "Org Targets" or keep but clarify
  - Permissions
  - Access

Subscription (existing → update)
  - Packages           <-- from API, not hardcoded
  - Current Plan       <-- effective subscription
```

**Note:** `orgs/targets` page already exists for delivery target CRUD. Decision: keep `orgs/targets` as the primary target management page, OR move to `delivery/targets`. Recommend: move all delivery-related pages under `delivery/` for clearer grouping. The `delivery/targets` page replaces `orgs/targets` and adds quota display.

---

### PR-FE-2: Mapping Template Enhancement

**Update `ingest/mappingTemplates/+page.svelte`:**

Current template form has: name, match rule, field mappings only.

Add to template edit/create form:

1. **`defaultLocale`** — Select: `th`, `en`, etc.

2. **`deliveryTargets[]`** — Section to bind delivery targets
   - Select target from existing targets (loaded via `listTargets()`)
   - Per-target config:
     - `filter[]` — PayloadCondition array (field, operator, values)
     - `eventClasses[]` — Whitelist (checkboxes/tags)
     - `eventSeverities[]` — Whitelist (checkboxes/tags)
     - `messageTemplateKey` — Select from messageTemplates keys

3. **`classificationRules[]`** — Ordered rules
   - Each rule: name, order, when (PayloadCondition[]), set (eventClass, eventSeverity)
   - Visual: drag-to-reorder or numeric order input
   - "First match wins" indicator

4. **`messageTemplates[]`** — Message template definitions
   - Each: key, channelType, locale, title, body, extras
   - Key is a string identifier used by deliveryTargets.messageTemplateKey
   - Channel types: line, discord, telegram
   - Body: textarea with placeholder reference panel

5. **`dlq`** config — DLQ settings
   - `enabled` — Toggle
   - `maxRetries` — Number input
   - `retryTimeoutSeconds` — Number input

**New types to add to `lib/api/ingest.ts`:**

```typescript
interface PayloadCondition {
  field: string
  operator: 'eq' | 'in'
  values: string[]
}

interface ClassificationSet {
  eventClass?: string
  eventSeverity?: string
}

interface ClassificationRule {
  name: string
  when: PayloadCondition[]
  set: ClassificationSet
  order?: number
}

interface TemplateDeliveryTarget {
  targetId: string
  filter?: PayloadCondition[]
  eventClasses?: string[]
  eventSeverities?: string[]
  messageTemplateKey?: string
}

interface MessageTemplate {
  key?: string
  channelType: string
  locale: string
  title: string
  body: string
  extras?: Record<string, string>
}

interface DLQConfig {
  enabled: boolean
  maxRetries: number
  retryTimeoutSeconds: number
}

// Updated MappingTemplate
interface MappingTemplate {
  templateId: string
  orgId?: string
  name: string
  match?: MatchRule
  mappings: FieldMapping[]
  defaultLocale?: string
  deliveryTargets?: TemplateDeliveryTarget[]
  classificationRules?: ClassificationRule[]
  messageTemplates?: MessageTemplate[]
  dlq?: DLQConfig
  createdAt: string
  updatedAt: string
}
```

---

### PR-FE-3: Event Management Enhancement

**Update `ingest/management/+page.svelte`:**

Current: When viewing an event, user can "Create Template from Event" (always creates new).

Change: When an event already has `templateId` (matched via fingerprint), show:
- **"Edit Template"** button instead of "Create Template"
- Links to the existing template in mappingTemplates page
- Shows template name if available

Flow:
1. Event view modal checks `viewEvent.templateId` (from rawBody or meta)
2. If `templateId` exists:
   - Show "Linked Template: {name}" with edit link
   - "Edit Template" button opens mappingTemplates page or inline editor
3. If no template:
   - Show existing "Create Template from Event" flow (unchanged)

**API change needed:**
- `PendingEvent` type needs `templateId?: string` and `templateName?: string` fields
- Backend should return these if the event was template-matched

---

### PR-FE-4: Delivery Targets Page

**Create `delivery/targets/+page.svelte`:**

This replaces/enhances `orgs/targets`. Key additions:

1. **Quota display** at top:
   ```
   Webhook: 1/2 used | LINE: 0/1 | Discord: 0/1 | Telegram: 0/1
   Message channels total: 0/3
   ```
   - Load from `GET /subscriptions/current` for limits
   - Count from `listTargets()` for usage

2. **Target list table:**
   - Columns: Name, Type (badge), Status (active/inactive toggle), Endpoint, Created
   - Actions: Edit, Delete, Test

3. **Create/Edit modal:**
   - Name
   - Type (webhook/line/discord/telegram) — disabled channels show "Not available on current plan"
   - Config fields based on type:
     - Webhook: URL, headers, signing secret, timeout
     - LINE: channelAccessToken/Ref, to[] (broadcast/push/multicast toggle)
     - Discord: webhook URL
     - Telegram: bot token, chat ID
   - Enabled toggle

4. **Test button:** Sends test delivery to target via `POST /targets/:id/test` (if endpoint exists)

**Reuse:** Migrate logic from existing `orgs/targets/+page.svelte`.

---

### PR-FE-5: DLQ Page

**Create `delivery/dlq/+page.svelte`:**

1. **Stats bar** at top: pending count, retrying count, resolved count, abandoned count
   - Load from `GET /ingest/dlq/stats`

2. **Filter bar:**
   - Status: all | pending | retrying | resolved | abandoned
   - Stage: all | deliver | normalize
   - Channel: all | webhook | line | discord | telegram
   - Date range: from/to
   - Event ID search

3. **Table:**
   - Columns: Event ID (truncated), Stage, Target, Channel, Status (badge), Retry Count, Created
   - Click row -> navigate to detail page

4. **Bulk actions:** (future, not in baseline)

**Create `delivery/dlq/[id]/+page.svelte`:**

1. **Header:** Event ID, Status badge, Stage badge
2. **Info table:** eventId, stage, target name, channel, reason, retry count/max, created/updated
3. **Payload panel:** JSON viewer (collapsible, syntax highlighted)
4. **Error stack:** preformatted error text
5. **Action buttons:**
   - **Retry** — `POST /ingest/dlq/:id/retry` — confirm dialog
   - **Replay** — `POST /ingest/dlq/:id/replay` — confirm with warning ("restarts full pipeline")
   - **Abandon** — `POST /ingest/dlq/:id/abandon` — confirm with reason input
6. **Status is resolved/abandoned?** — Show read-only, hide action buttons

---

### PR-FE-6: Subscription Pages (API-driven)

**Create `subscriptions/packages/+page.svelte`:**

1. Load packages from `GET /subscriptions/packages` (no more hardcoded `PLANS`)
2. Display cards with:
   - Plan name, description, price (from `billing.price.display`)
   - Feature list (from `ui.featureList`)
   - Limits summary (orgs, members, events, webhooks, channels)
   - Current plan badge (compare with current subscription)
   - Highlight/theme from `ui.highlight` + `ui.theme`
3. Upgrade button (calls `PATCH /subscriptions/plan`)

**Create `subscriptions/current/+page.svelte`:**

1. Load effective subscription from `GET /subscriptions/current`
2. Display:
   - Current plan name + status badge
   - Billing cycle
   - Period start/end
   - Limits vs usage table:
     ```
     Resource        | Used  | Limit
     Organizations   | 2     | 5
     Team Members    | 12    | 25
     Events/Month    | 5,230 | 100,000
     Webhooks/Org    | 1     | 2
     LINE/Org        | 1     | 1
     Discord/Org     | 0     | 1
     Msg Channels    | 1     | 3
     ```
   - Link to packages page for upgrade

**Update `subscription/+page.svelte`:**
- Redirect to `subscriptions/packages` OR refactor to use API-driven data

---

## 4) i18n Keys Plan

### `i18n/en/delivery.json` (~100 keys)

Groups:
- `delivery*` — General delivery section
- `deliveryTargets*` — Target CRUD, quota, test, config fields
- `deliveryTemplates*` — Message template CRUD, preview, placeholders
- `deliveryDlq*` — DLQ list, detail, retry/replay/abandon, statuses, stages
- `deliveryChannel*` — Channel name labels
- `deliveryFilter*` — Filter operator labels
- `deliveryQuota*` — Quota display labels

### `i18n/en/subscription.json` (~40 keys)

Groups:
- `subscription*` — General subscription section
- `subscriptionPackages*` — Package cards, features, limits
- `subscriptionCurrent*` — Current plan, usage, billing
- `subscriptionPlan*` — Plan names (if not from API)

### Existing keys to update:
- `i18n/en/nav.json` — Add nav keys for Delivery section, update Subscription
- `i18n/en/ingest.json` — Add keys for classification rules, delivery target binding, DLQ config in template

---

## 5) Type System Changes

### `lib/types/org.ts` — Update `LineConfig`

```typescript
// Before
export interface LineConfig {
  channelAccessTokenRef: string
  to: string
}

// After
export interface LineConfig {
  channelAccessToken?: string
  channelAccessTokenRef?: string
  to: string[]  // [] = broadcast, ['U...'] = push, ['U...','U...'] = multicast
}
```

### `lib/api/ingest.ts` — Extend `MappingTemplate`

Add fields: `deliveryTargets`, `messageTemplates`, `classificationRules`, `dlq`, `defaultLocale`

### `lib/api/ingest.ts` — Add DLQ types and API functions

```typescript
interface DlqMessage {
  messageId: string
  eventId: string
  tenantId: string
  orgId: string
  templateId: string
  topic: string
  stage: string  // 'deliver' | 'normalize'
  reason: string
  payload: Record<string, unknown>
  retryCount: number
  maxRetries: number
  retryTimeoutSeconds: number
  status: string  // 'pending' | 'retrying' | 'resolved' | 'abandoned'
  lastErrorAt: string
  createdAt: string
  updatedAt: string
}

interface DlqStats {
  pending: number
  retrying: number
  resolved: number
  abandoned: number
  total: number
}
```

---

## 6) Sidebar Menu Changes

```typescript
// appSidebarMenus.ts — add after Events section
{
  kind: 'link',
  id: 'delivery',
  icon: 'bi bi-send',
  textKey: 'navDelivery',
  children: [
    { id: 'deliveryTargets', url: 'delivery/targets', textKey: 'deliveryTargetsTitle' },
    { id: 'deliveryTemplates', url: 'delivery/templates', textKey: 'deliveryTemplatesTitle' },
    { id: 'deliveryDlq', url: 'delivery/dlq', textKey: 'deliveryDlqTitle' }
  ]
}

// Update subscription entry → expand to children
{
  kind: 'link',
  id: 'subscription',
  icon: 'bi bi-gem',
  textKey: 'navSubscription',
  children: [
    { id: 'subscriptionPackages', url: 'subscriptions/packages', textKey: 'subscriptionPackagesTitle' },
    { id: 'subscriptionCurrent', url: 'subscriptions/current', textKey: 'subscriptionCurrentTitle' }
  ]
}
```

---

## 7) Implementation Order

| Order | PR | Scope | Dependencies |
|---|---|---|---|
| 1 | PR-FE-1 | Types, API libs, i18n, sidebar | None |
| 2 | PR-FE-2 | Mapping template enhancement | PR-FE-1 |
| 3 | PR-FE-3 | Event management enhancement | PR-FE-1, PR-FE-2 |
| 4 | PR-FE-4 | Delivery targets page | PR-FE-1 |
| 5 | PR-FE-5 | DLQ page + detail | PR-FE-1 |
| 6 | PR-FE-6 | Subscription pages (API-driven) | PR-FE-1 |

PR-FE-4, FE-5, FE-6 can be done in parallel after PR-FE-1.

---

## 8) Key Design Decisions

### 8.1 Keep existing `orgs/targets` or move to `delivery/targets`?

**Decision:** Create `delivery/targets` as the primary page. Update sidebar to point there. Mark `orgs/targets` as deprecated or redirect. Delivery targets are part of the delivery pipeline, not org administration.

### 8.2 Template edit: separate pages vs inline in mapping template?

**Decision:** `messageTemplates` and `classificationRules` live inside the `MappingTemplate` document (as arrays). Edit them inline in the mapping template form. No separate CRUD pages for these. The `delivery/templates` page can be a convenience view that lists all templates across all mapping templates, linking back to the parent template for editing.

### 8.3 Subscription: hardcoded vs API-driven?

**Decision:** Move to API-driven (`GET /subscriptions/packages`). Delete hardcoded `PLANS` constant from `subscription.ts`. The API returns localized names, feature lists, limits, and UI hints.

### 8.4 DLQ: under ingest or delivery?

**Decision:** DLQ API is under `/api/v1/ingest/dlq` (existing backend). FE pages are under `delivery/dlq` for user-facing organization. API lib functions go in `lib/api/ingest.ts` (matching backend path).

### 8.5 Event management: create vs edit template?

**Decision:** When event has `templateId` (already matched), show "Edit Template" linking to the existing template. Only show "Create Template from Event" when no template is matched. This prevents duplicate templates and guides operators to maintain existing ones.

---

## 9) Acceptance Criteria

### PR-FE-1
- [ ] i18n keys compiled without errors
- [ ] Sidebar shows Delivery section with 3 children
- [ ] Sidebar shows Subscription with 2 children
- [ ] `bun run check` passes with 0 errors

### PR-FE-2
- [ ] Template form shows deliveryTargets, classificationRules, messageTemplates, DLQ config
- [ ] PATCH template with all new fields works
- [ ] Classification rule order is visual and editable

### PR-FE-3
- [ ] Template-matched events show "Edit Template" with link
- [ ] Unmatched events show "Create Template" (existing flow)

### PR-FE-4
- [ ] Target list loads from API
- [ ] Quota display shows used/limit per channel
- [ ] Create target blocked when quota reached (visual + API error)
- [ ] LINE target supports to[] array + broadcast mode

### PR-FE-5
- [ ] DLQ list with stats bar and filters
- [ ] DLQ detail shows payload, error, actions
- [ ] Retry/Replay/Abandon with confirmation
- [ ] Resolved/Abandoned records are read-only

### PR-FE-6
- [ ] Package cards render from API (not hardcoded)
- [ ] Current subscription shows effective limits vs usage
- [ ] Upgrade flow works via API
