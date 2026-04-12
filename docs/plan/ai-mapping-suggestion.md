# Plan: AI Mapping Suggestion — Baseline-Ready

**Date:** 2026-04-12
**Scope:** `gateway-api`
**Status:** Revised after review — approved with required changes incorporated

---

## 1. ปัญหาที่แก้

Mapping template ปัจจุบันต้องการให้ผู้ใช้กำหนดด้วยมือ:

- `matchAll` / `matchAny` rules → ยากถ้าไม่รู้ว่า payload มี field อะไร และยากมากถ้าต้องทำให้ mutually-exclusive กับ template อื่น
- `fieldMappings` + `valueCodes` → ต้องอ่าน vendor documentation เอง
- `finalEventType` → ต้องตั้งชื่อเอง
- Delivery config (webhook, LINE, Discord) → ต้องรู้ว่า binding อยู่ที่ไหน และต้องทำหลายขั้นตอน

---

## 2. Design Principles

| หลักการ | รายละเอียด |
|---|---|
| **User-triggered เท่านั้น** | ไม่เรียก AI ต่อ event ที่รับเข้ามา ห้าม drift ไปเรียกต่อ ingest event เด็ดขาด |
| **System suggestion ก่อน** | รัน `SuggestionFieldMap` matcher ก่อนเสมอ → ส่งเป็น base context ให้ AI ปรับปรุง |
| **Policy-driven merge** | AI ไม่ "ชนะ" system เสมอ — ใช้ merge policy ระบุชัด (ดู §7) |
| **Draft-first** | ทุก AI output เป็น draft เท่านั้น — ต้อง review + explicit deploy แยก |
| **Per-workspace config** | แต่ละ workspace ตั้ง provider/model/key ของตัวเองได้ ไม่มี global AI config |
| **Free tier explicit** | ถ้าไม่มี key ใช้ Gemini free tier — ต้อง mark ชัด mode=freeSharedProvider |
| **API key encrypted at rest** | เก็บผ่าน `internal/crypto/secretbox` เสมอ ห้าม return key กลับ client |
| **Graceful fallback with mode** | AI ล้มเหลว → return system suggestion + `mode=aiFailedFallback` — ต้องชัด ไม่เงียบ |
| **Observability required** | metrics + audit log ทุก request — ไม่มี dark fallback |
| **No secret in prompt/log** | token, API key, credential ที่ user พิมพ์ใน prompt ต้องผ่าน secure handling ก่อนเก็บ |

---

## 3. Supported AI Providers

| Provider | Free Model | Paid Model | JSON Mode | Free Tier |
|---|---|---|---|---|
| **Gemini** | `gemini-2.0-flash-lite` | `gemini-1.5-pro` | `response_mime_type: application/json` | 15 RPM / ไม่ต้อง key |
| **OpenAI** | — | `gpt-4o-mini` | `response_format: {type: json_object}` | ไม่มี |
| **Claude** | — | `claude-haiku-4-5-20251001` | Tool Use (Function Calling) | ไม่มี |

> **Free tier mode** = `freeSharedProvider` — FE ต้องแสดง badge ชัดว่าไม่ใช่ user-owned credential
> **Paid mode** = `workspaceManagedProvider`

---

## 4. Architecture Overview

### 4.1 Feature Breakdown

Plan นี้ครอบ 2 feature หลัก:

```
Feature A: AI Mapping Suggest
  ผู้ใช้กดปุ่ม AI Suggest บน template editor
  → ระบบ generate fieldMappings + matchRules + valueCodes + suggestedEventType

Feature B: Prompt to Config Draft
  ผู้ใช้พิมพ์ข้อความ natural language เช่น
  "ถ้า AIBOX เจอ alarm=blacklist ส่ง webhook + LINE + Discord"
  → ระบบแปลงเป็น draft config → review → save → deploy
```

### 4.2 Package Structure (New)

```
models/
  workspacemod/
    aiConfig.go                ← WorkspaceAIConfig

internal/
  repo/
    aiconfigrepo/
      repo.go                  ← FindByWorkspaceID, Upsert, ClearKey

  gateways/
    aiprovider/
      provider.go              ← AIProvider interface
      capabilities.go          ← provider capabilities (JSON mode, model list)
      factory.go               ← NewProvider(cfg, kr) → AIProvider
      gemini.go
      openai.go
      claude.go

  services/
    aimappingsvc/
      service.go               ← AIMappingService.Suggest()
      prompt.go                ← buildPrompt() + EnumContextResolver
      reducer.go               ← PayloadReducer (sanitize + truncate)
      schema.go                ← AISuggestRawResult + typed response schema
      validate.go              ← ValidateAIOutput() — semantic + business rules
      merge.go                 ← MergeWithPolicy()
      metrics.go               ← aiSuggestRequestsTotal, latency, failures

    aiconfigdraftsvc/
      service.go               ← ConfigDraftService (prompt→draft, refine, validate)
      intentparser.go          ← parseIntent() — NL → draft intent struct
      entityresolver.go        ← resolveEntities() — field path, enum, target type
      missingdetector.go       ← detectMissing() — deterministic schema validation
      dryrun.go                ← DryRun() — simulate match + dispatch

controllers/
  aimappingapi/
    suggest.go                 ← POST .../ai-suggest
    config.go                  ← GET/PUT/DELETE .../ai-config

  aiconfigdraftapi/
    draft.go                   ← POST .../config-drafts/from-prompt
    refine.go                  ← POST .../config-drafts/{id}/refine
    dryrun.go                  ← POST .../config-drafts/{id}/dry-run
    save.go                    ← POST .../config-drafts/{id}/save
    deploy.go                  ← POST .../ingest/configs/{id}/deploy

router/
  aiMapping.go
  aiConfigDraft.go
```

---

## 5. Models

### 5.1 WorkspaceAIConfig

**File:** `models/workspacemod/aiConfig.go`

```go
// WorkspaceAIConfig stores AI suggestion settings per workspace.
// Collection: workspace_ai_configs
type WorkspaceAIConfig struct {
    WorkspaceID            string    `json:"workspaceId"            bson:"workspaceId"`
    Enabled                bool      `json:"enabled"                bson:"enabled"`
    // Provider: "gemini" | "openai" | "claude"
    Provider               string    `json:"provider"               bson:"provider"`
    // Model: e.g. "gemini-2.0-flash-lite", "gpt-4o-mini", "claude-haiku-4-5-20251001"
    Model                  string    `json:"model"                  bson:"model"`
    // EncryptedApiKey is an EncBlob JSON stored via secretbox.
    // Empty = use free/unauthenticated tier (Gemini only).
    EncryptedApiKey        string    `json:"-"                      bson:"encryptedApiKey"`
    // ProviderMode: "freeSharedProvider" | "workspaceManagedProvider"
    ProviderMode           string    `json:"providerMode"           bson:"providerMode"`
    // Limits
    DefaultTimeoutMs       int       `json:"defaultTimeoutMs"       bson:"defaultTimeoutMs"`   // default 30000
    MaxInputBytes          int       `json:"maxInputBytes"          bson:"maxInputBytes"`       // default 8192
    // Audit
    CreatedBy              string    `json:"createdBy"              bson:"createdBy"`
    UpdatedBy              string    `json:"updatedBy"              bson:"updatedBy"`
    LastValidatedAt        *time.Time `json:"lastValidatedAt"       bson:"lastValidatedAt"`
    LastValidationStatus   string    `json:"lastValidationStatus"   bson:"lastValidationStatus"` // "ok" | "fail" | ""
    LastValidationError    string    `json:"lastValidationError"    bson:"lastValidationError"`
    CreatedAt              time.Time `json:"createdAt"              bson:"createdAt"`
    UpdatedAt              time.Time `json:"updatedAt"              bson:"updatedAt"`
}
```

> `EncryptedApiKey` ใช้ `json:"-"` เสมอ — ห้าม serialize กลับ client ทุกกรณี

### 5.2 AISuggestInput / AISuggestResult

**File:** `internal/services/aimappingsvc/service.go`

```go
type AISuggestInput struct {
    OrgID            string
    SourceFamily     string
    SamplePayload    map[string]any
    ExistingMappings []ingestmod.FieldMapping // optional
}

type AISuggestResult struct {
    // Mode บอกว่า suggestion นี้มาจากไหนจริง ๆ
    Mode               string                 `json:"mode"`
    // mode values:
    //   "aiAugmented"       — AI enhance ได้สำเร็จ
    //   "systemOnly"        — AI disabled สำหรับ workspace นี้
    //   "aiFailedFallback"  — AI call fail ได้ system suggestion แทน
    Provider           string                 `json:"provider"`
    Model              string                 `json:"model"`
    PromptVersion      string                 `json:"promptVersion"`
    SchemaVersion      string                 `json:"schemaVersion"`
    SuggestedEventType string                 `json:"suggestedEventType"`
    FieldMappings      []SuggestionFieldMap   `json:"fieldMappings"`
    MatchRules         []MatchRuleSuggestion  `json:"matchRules"`
    // Conflicts: fields ที่ AI และ system ขัดกัน — ไม่ auto-overwrite
    Conflicts          []SuggestConflict      `json:"conflicts,omitempty"`
    Warnings           []string               `json:"warnings,omitempty"`
    Diagnostics        SuggestDiagnostics     `json:"diagnostics,omitempty"`
}

type SuggestionFieldMap struct {
    SourceField     string            `json:"sourceField"`
    TargetField     string            `json:"targetField"`
    ValueCodes      map[string]string `json:"valueCodes,omitempty"`
    // ValueCodeSource: "system" | "aiInferred" — ห้ามถือว่า aiInferred ถูก without review
    ValueCodeSource string            `json:"valueCodeSource,omitempty"`
    // Source: "system" | "ai" | "merged"
    Source          string            `json:"source"`
}

type MatchRuleSuggestion struct {
    FieldPath string `json:"fieldPath"`
    Operator  string `json:"operator"`  // allowlist: "eq" | "exists" | "contains"
    Value     any    `json:"value,omitempty"`
    Required  bool   `json:"required"`
    // Source: "system" | "ai" | "merged"
    Source    string `json:"source"`
    Reason    string `json:"reason,omitempty"`
}

type SuggestConflict struct {
    FieldPath   string `json:"fieldPath"`
    SystemValue any    `json:"systemValue"`
    AIValue     any    `json:"aiValue"`
    Resolution  string `json:"resolution"` // "systemKept" | "aiApplied" | "needsReview"
}

type SuggestDiagnostics struct {
    SystemSuggestionUsed    bool    `json:"systemSuggestionUsed"`
    AIParseSuccess          bool    `json:"aiParseSuccess"`
    AIValidationSuccess     bool    `json:"aiValidationSuccess"`
    ObservedPathsCount      int     `json:"observedPathsCount"`
    AIOutputFieldsCount     int     `json:"aiOutputFieldsCount"`
    PayloadReducedBytes     int     `json:"payloadReducedBytes"`
    AILatencyMs             int64   `json:"aiLatencyMs"`
    EnumContextFieldsUsed   int     `json:"enumContextFieldsUsed"`
}
```

---

## 6. Feature A: AI Mapping Suggest

### 6.1 Service Flow

```
AIMappingService.Suggest(ctx, input):

  1. validate authz — workspace scope
  2. normalize payload (sanitize BSON key chars)
  3. extract observedPaths []string
  4. run SuggestionFieldMap matcher → systemSuggestion
  5. load WorkspaceAIConfig
  6. if !enabled → return mode=systemOnly
  7. reduce payload → reducedPayload (§6.3)
  8. resolve enum context bundle (§6.4)
  9. build prompt (promptVersion stamped)
  10. call AIProvider.Complete(ctx with timeout)
  11. unmarshal → AISuggestRawResult (typed struct, strict)
  12. validate AI output (§6.5)
  13. merge with policy (§7)
  14. compute diagnostics
  15. emit metrics + audit log (redacted — no payload/key)
  16. return AISuggestResult
```

### 6.2 Structured Output Enforcement

แต่ละ provider ต้องใช้ JSON mode ระดับ native:

```
Gemini:  generationConfig.responseMimeType = "application/json"
OpenAI:  response_format = { type: "json_object" }
Claude:  Tool Use (function calling) — define output schema as tool parameters
```

Pipeline หลัง provider return:

```
provider response
  → strict json.Unmarshal → AISuggestRawResult (typed struct)
  → ValidateAIOutput() — schema + semantic + business rules
  → if fail → aiParseFailuresTotal++ → fallback mode
```

ห้าม accept AI output แบบ loose text scraping

### 6.3 Payload Reducer

**File:** `internal/services/aimappingsvc/reducer.go`

Pipeline ก่อนส่งเข้า prompt:

```
1. remove known-noise fields
   เช่น imageBase64, rawFrame, binaryData, debugDump
2. bound array length → เหลือ 2 elements, mark truncated=true
3. bound object depth → ลึกเกิน 5 ชั้นให้ collapse + annotate
4. bound string length → ยาวเกิน 256 chars ให้ truncate + annotate
5. preserve path inventory — path ทุก field ยังต้องอยู่แม้ value ถูก truncate
6. build structural summary
```

Output ที่ส่งเข้า prompt:

```json
{
  "observedPaths": ["alarmType", "eventAttribute.plateLicense", "..."],
  "reducedSamplePayload": { ... },
  "truncationInfo": {
    "arraysTruncated": 2,
    "stringsTruncated": 1,
    "droppedPaths": ["imageBase64", "rawFrameData"]
  }
}
```

Limits:
- `MaxInputBytes` per workspace config (default 8192)
- Hard cap 16 KB — reject ถ้าเกิน หลังจาก reduce แล้ว

### 6.4 Enum Context Resolver

**File:** `internal/services/aimappingsvc/prompt.go`

```go
type EnumContextResolver interface {
    Resolve(sourceFamily string, observedPaths []string) EnumContextBundle
}

type EnumContextBundle struct {
    // เฉพาะ paths ที่พบใน observedPaths เท่านั้น
    FieldDictionaries map[string]map[string]string // path → valueCodes
}
```

Rules:
- inject เฉพาะ enum context ของ field ที่อยู่ใน observedPaths จริง
- max 20 fields per bundle
- max 2 KB ต่อ field dictionary
- ตัด field ที่ไม่อยู่ใน payload ทิ้ง

### 6.5 AI Output Validation

**File:** `internal/services/aimappingsvc/validate.go`

ตรวจ:
- required fields ครบ
- operator อยู่ใน allowlist: `eq | exists | contains`
- source/target paths อยู่ใน observedPaths หรือ known canonical paths
- valueCodes key เป็น string
- normalize eventType เป็น UPPER_SNAKE_CASE
- ไม่ยอมรับ `exists` rule เดี่ยวๆ โดยไม่มี supporting discriminator rule อื่น

```go
type ValidationError struct {
    Kind    string // "schemaError" | "unknownOperator" | "unknownPath" | "weakMatchRule"
    Field   string
    Message string
}
```

### 6.6 Cross-Template Discriminator Check

ก่อน return matchRules:
- โหลด template อื่น ๆ ของ workspace + sourceFamily เดียวกัน
- ลอง evaluate rule ชุดนี้กับ sample payload ของ template อื่น
- ถ้า rule นี้ match template อื่น → เพิ่ม `SuggestConflict` + `warning`

```go
type DiscriminatorCheckResult struct {
    IsUnique        bool
    OverlapsWith    []string // templateIds ที่ overlap
    OverlapReasons  []string
}
```

### 6.7 Metrics

**File:** `internal/services/aimappingsvc/metrics.go`

```
aiSuggestRequestsTotal          {workspace, sourceFamily, provider, mode}
aiSuggestProviderFailuresTotal  {workspace, provider, errorKind}
aiSuggestFallbackTotal          {workspace, reason}
aiSuggestParseFailuresTotal     {workspace, provider}
aiSuggestLatencyMs              {workspace, provider, model}
aiSuggestConflictsTotal         {workspace, sourceFamily}
```

### 6.8 Confidence

ไม่ expose AI self-reported confidence โดยตรง — คำนวณจาก heuristic ฝั่ง service:

```go
func computeConfidence(
    aiOutput AISuggestRawResult,
    systemSuggestion SystemSuggestion,
    observedPaths []string,
    parseSuccess bool,
    validationSuccess bool,
) float64 {
    score := 0.0
    if parseSuccess { score += 0.3 }
    if validationSuccess { score += 0.2 }
    // field coverage: AI fields ที่อยู่ใน observedPaths / total AI fields
    score += 0.3 * fieldCoverageRatio(aiOutput.FieldMappings, observedPaths)
    // matchRule quality: มี discriminator ที่ valid ไหม
    if hasStrongDiscriminator(aiOutput.MatchRules) { score += 0.2 }
    return score
}
```

---

## 7. Merge Policy

**File:** `internal/services/aimappingsvc/merge.go`

ลำดับ priority:

```
Tier 1 — systemLockedFields (AI ห้ามแก้)
  - FieldMappings ที่ system suggestion มีอยู่แล้ว + confidence สูง
  - MatchConditions ที่ validated โดย system matcher

Tier 2 — aiExtendableFields (AI เติมได้)
  - ValueCodes สำหรับ field ที่ system ไม่มี
  - FieldMappings สำหรับ path ที่ system ไม่ cover
  - SuggestedEventType (ถ้า system ไม่มี)

Tier 3 — conflict (ต้อง mark ไม่ auto-overwrite)
  - AI เสนอ TargetPath ต่างจาก system
  - AI เสนอ EventType ขัดกับ existing template
  - AI เสนอ MatchRule ที่ overlap กับ template อื่น
```

```go
type MergePolicy struct {
    // SystemLockedPaths: paths ที่ system ตัดสินใจแล้ว → AI ห้ามแก้
    SystemLockedPaths []string
    // AIExtendablePaths: paths ที่ AI เติมได้ถ้า system ไม่มี
    AIExtendablePaths []string
}
```

Conflict resolution:
- `systemKept` — system value ชนะ
- `aiApplied` — AI value ใช้ (เฉพาะ tier 2 ที่ validate ผ่าน)
- `needsReview` — ขัดกันบน tier 1 → FE ต้องแสดง review UI

---

## 8. Rate Limit / Guard

| Limit | Value | Scope |
|---|---|---|
| per-workspace RPS | 2 req/s | workspace |
| per-user burst | 5 req/min | userId |
| payload size | MaxInputBytes (8192 default) | per request |
| timeout hard cap | 30s | per AI call |
| concurrent requests | 3 | per workspace |
| payload hash cache | 60s TTL | deduplicate identical requests |

---

## 9. AI Config API

```
GET  /workspaces/{workspaceId}/ingest/ai-config
     → WorkspaceAIConfig (EncryptedApiKey omitted, hasApiKey: bool แทน)
     authz: manage_ingest permission

PUT  /workspaces/{workspaceId}/ingest/ai-config
     body: { enabled, provider, model, apiKey?, defaultTimeoutMs?, maxInputBytes? }
     - apiKey encrypted via secretbox.EncryptString(kr, key) ก่อน write
     - apiKey "" → clears key → ProviderMode = freeSharedProvider
     - validate provider + model ใน allowlist
     authz: manage_ingest permission

DELETE /workspaces/{workspaceId}/ingest/ai-config/key
     → clears API key only
     authz: manage_ingest permission

POST /workspaces/{workspaceId}/ingest/ai-config/validate
     → test connection to provider with current config
     → update LastValidatedAt + LastValidationStatus
     authz: manage_ingest permission
```

```
POST /workspaces/{workspaceId}/ingest/templates/ai-suggest
     body: { sourceFamily, samplePayload, existingMappings? }
     → AISuggestResult
     authz: manage_ingest permission
     rate limit: per-workspace + per-user guard
```

---

## 10. Feature B: Prompt to Config Draft

### 10.1 Flow

```
User: "ตั้งค่า event AIBOX ถ้าเจอ alarm = blacklist ให้ส่ง webhook ไปที่ http://...
       + realtime LINE token xxxx + Discord"

POST .../config-drafts/from-prompt
  ↓
1. parseIntent()       → DraftIntent (NL → typed structure)
2. resolveEntities()   → แปล "alarm" → field path, "blacklist" → valuecode, "LINE" → delivery type
3. detectMissing()     → deterministic schema validation (ไม่ให้ AI ตัดสินว่าขาดอะไร)
4. sanitizeSecrets()   → LINE/Discord token → masked, ไม่เก็บ plain ใน draft
5. validateTargetURLs() → SSRF check + scheme allowlist
6. buildDraft()        → ConfigDraft
7. return draft + missingFields + warnings + reviewSummary
```

### 10.2 Intent Struct

```go
type DraftIntent struct {
    SourceFamily   string
    Conditions     []IntentCondition
    DeliveryActions []IntentAction
    RealtimeActions []IntentAction
    RawPrompt      string    // เก็บ redacted version (ตัด token/URL ออก)
    ParsedAt       time.Time
}

type IntentCondition struct {
    RawPhrase   string // "alarm = blacklist"
    FieldHint   string // "alarm"
    Operator    string // "eq"
    ValueHint   string // "blacklist"
    Resolved    bool   // ถ้า resolveEntities() เจอ field จริง
    ResolvedPath string
    ResolvedValue any
}

type IntentAction struct {
    Type      string // "webhook" | "line" | "discord" | "mqtt"
    RawTarget string // masked — ไม่เก็บ URL หรือ token plain
    SecretRef string // ref ไปยัง secure storage หลัง user bind
    Resolved  bool
}
```

### 10.3 Missing Field Detector

Deterministic rules — ไม่ให้ AI ตัดสิน:

```go
// detectMissing ตรวจตาม delivery action type
// webhook:  ต้องมี url (validated) + optional auth
// line:     ต้องมี channelAccessToken หรือ notifyToken (type ชัด)
// discord:  ต้องมี webhookUrl
// mqtt:     ต้องมี topic
// binding:  ต้องมี templateId
func detectMissing(draft *ConfigDraft) []MissingFieldHint
```

```go
type MissingFieldHint struct {
    Field   string // "discordWebhookUrl"
    Reason  string // "Discord delivery target requires a webhook URL"
    ForAction string
}
```

### 10.4 Secret Handling

ข้อความที่ user พิมพ์ใน prompt อาจมี token/credential:

```
Rule:
  ทุก string ที่ detect เป็น token/credential pattern (เช่น LINE, Discord token)
  → NEVER เก็บ plain ใน ConfigDraft หรือ audit log
  → แปลงเป็น secretRef placeholder
  → แจ้ง user ว่าต้อง bind ผ่าน secure settings แยก

Pattern detection:
  LINE: token ยาว 170+ chars เป็น Bearer token
  Discord: webhook URL format
  Generic: anything matching high-entropy string heuristic
```

### 10.5 SSRF / URL Validation

ก่อน save URL ใดก็ตาม:

```go
func validateWebhookURL(raw string) error {
    // allowlist schemes: https only (http ในบาง env)
    // block: localhost, 127.0.0.1, 0.0.0.0, ::1, 169.254.x.x, 10.x, 172.16-31.x, 192.168.x
    // block: file://, ftp://, internal-only TLDs
    // optional: outbound allowlist per workspace config
}
```

### 10.6 Draft Config Shape

```go
type ConfigDraft struct {
    DraftID        string
    WorkspaceID    string
    Status         string    // "incomplete" | "ready" | "reviewed" | "published" | "deployed"
    SourceFamily   string
    MatchConditions []ingestmod.MatchCondition
    DeliveryTargetRefs []DeliveryTargetRef
    RealtimeTargetRefs []RealtimeTargetRef
    MissingFields  []MissingFieldHint
    Warnings       []string
    ReviewSummary  []string  // human-readable summary ของ config ที่จะเกิดขึ้น
    CreatedBy      string
    CreatedAt      time.Time
    UpdatedAt      time.Time
    // prompt raw (redacted — secret และ URL ถูก mask)
    RedactedPrompt string
}
```

### 10.7 Dry Run

ก่อน deploy ต้อง dry-run ได้:

```go
// DryRun ประเมิน ConfigDraft กับ samplePayload
// return: matched, targetsTriggered, warnings
type DryRunResult struct {
    Matched               bool
    WebhookTargetsCount   int
    LineTargetsCount      int
    DiscordTargetsCount   int
    IncompleteTargets     []string  // targets ที่ยัง missing secret
    EvaluationDetails     []string
}
```

### 10.8 API Endpoints

```
POST /workspaces/{workspaceId}/ingest/config-drafts/from-prompt
     body: { prompt: "..." }
     → ConfigDraft (redacted)
     authz: manage_ingest

POST /workspaces/{workspaceId}/ingest/config-drafts/{draftId}/refine
     body: { answers: { "discordWebhookUrl": "https://discord.com/api/webhooks/..." } }
     → ConfigDraft (updated)
     authz: manage_ingest

POST /workspaces/{workspaceId}/ingest/config-drafts/{draftId}/dry-run
     body: { samplePayload: { ... } }
     → DryRunResult
     authz: manage_ingest

POST /workspaces/{workspaceId}/ingest/config-drafts/{draftId}/save
     → ConfigDraft { status: "ready" }
     authz: manage_ingest

POST /workspaces/{workspaceId}/ingest/configs/{configId}/deploy
     → { status: "deployed", deployedAt }
     authz: manage_ingest (separate explicit action)
```

> **save และ deploy ต้องเป็น 2 action แยก** — ห้าม implicit deploy ตอน save

---

## 11. Prompt / Schema Versioning

ทุก AI call ต้อง stamp version:

```go
const (
    AISuggestPromptVersion  = "v1.0"
    AISuggestSchemaVersion  = "v1.0"
    ConfigDraftPromptVersion = "v1.0"
)
```

เก็บไว้ใน:
- `AISuggestResult.PromptVersion` / `.SchemaVersion`
- audit log ของทุก suggestion request

เมื่อ prompt หรือ schema เปลี่ยน → bump version → สามารถ trace regression ได้

---

## 12. Authz

| Endpoint | Permission |
|---|---|
| GET/PUT/DELETE ai-config | `manage_ingest` |
| POST ai-config/validate | `manage_ingest` |
| POST ai-suggest | `manage_ingest` |
| POST config-drafts/from-prompt | `manage_ingest` |
| POST config-drafts/refine | `manage_ingest` |
| POST config-drafts/dry-run | `manage_ingest` |
| POST config-drafts/save | `manage_ingest` |
| POST configs/deploy | `manage_ingest` (explicit deploy) |

---

## 13. Security Rules Summary

| Rule | Detail |
|---|---|
| API key never stored plaintext | `secretbox.EncryptString(kr, key)` ก่อน write ทุกครั้ง |
| API key never returned to client | Response ใช้ `hasApiKey: bool` แทน |
| API key never logged | ห้าม log string ที่ decrypt มาจาก encryptedApiKey |
| Prompt secrets masked | token, URL, credential จาก NL prompt ต้องถูก mask/strip ก่อน log/store |
| SSRF protection | validate webhook URL ก่อน save/dry-run |
| No implicit deploy | save ≠ deploy เสมอ |
| AI output never trusted directly | ผ่าน parse → validate → merge policy ทุกครั้ง |
| AI output redacted in logs | ไม่ log raw AI response — log metadata เท่านั้น |
| `hasApiKey` is sensitive metadata | restrict GET ai-config เฉพาะ `manage_ingest` |

---

## 14. ลำดับ Implement

```
Step 1  models + config bootstrap
        → models/workspacemod/aiConfig.go (WorkspaceAIConfig + audit fields)
        → models/ingestmod/eventTypeRegistry.go (EventTypeDefinition)
        → config/ingest/eventTypeRegistry.json (standard event types)
        → config/prompts/aiSuggest_v1.0.tmpl
        → config/prompts/configDraft_v1.0.tmpl

Step 2  internal/repo/
        → aiconfigrepo/repo.go (FindByWorkspaceID, Upsert, ClearKey)
        → aisuggestauditrepo/repo.go (Save, FindByWorkspace, Prune)
        → index: workspace_ai_configs.workspaceId (unique)
        → index: ai_suggest_audit.expiresAt (TTL)

Step 3  internal/gateways/aiprovider/
        → provider.go (interface + AiSuggestRawResult typed schema)
        → capabilities.go (JSON mode map per provider)
        → circuitbreaker.go (state machine + multi-metric trigger)
        → gemini.go (free tier + paid, JSON mode)
        → openai.go
        → claude.go (Tool Use structured output)
        → factory.go (NewProvider + NewProviderCircuitBreakers)

Step 4  internal/services/aimappingsvc/
        → reducer.go (PayloadReducer)
        → schema.go (AISuggestRawResult)
        → prompt.go (PromptLoader + buildPrompt + EnumContextResolver)
        → validate.go (ValidateAIOutput)
        → merge.go (MergeWithPolicy — 3 tiers)
        → eventtype.go (matchEventType + registry lookup + similarity)
        → dedup.go (requestHash + Redis dedup cache)
        → metrics.go
        → service.go (AIMappingService.Suggest — orchestrate)

Step 5  internal/services/aiconfigdraftsvc/
        → intentparser.go
        → entityresolver.go
        → missingdetector.go (deterministic schema rules)
        → statemachine.go (ValidateTransition)
        → dryrun.go
        → service.go (ConfigDraftService)

Step 6  controllers/aimappingapi/ + controllers/aiconfigdraftapi/
        → handlers with traceutil.StartLite + httputil responses
        → swagger godoc blocks ครบ
        → authz: manage_ingest_config / edit_ingest_template / deploy_ingest_config

Step 7  router/aiMapping.go + router/aiConfigDraft.go
        → routes + AuthBearer + ActiveOrg middleware
        → rate limit middleware (per-workspace + per-user)

Step 8  internal/app/container.go
        → load AIFeatureFlags from env
        → wire AIProviderCircuitBreakers
        → wire AIMappingService + ConfigDraftService deps

Step 9  ทดสอบ (ดู §15)
```

---

## 15. Test Plan

| Category | Test Cases |
|---|---|
| Config CRUD | create, update, clear key, get (hasApiKey only) |
| Authz | unauthorized access, wrong workspace |
| Encrypt/Decrypt | round-trip, key rotation, wrong KID |
| Provider factory | each provider init, missing key error, free tier init |
| Reducer | array truncation, deep object collapse, string truncation, path preservation, noise removal |
| Prompt builder | enum context inject only relevant paths, max size respected |
| AI Output validation | missing fields, unknown operator, unknown path, weak rule (exists alone), type mismatch |
| Parse errors | malformed JSON, partial JSON, extra junk fields, nested type mismatch |
| Merge policy | system locked fields preserved, AI fills tier 2, conflict marked not overwritten |
| Cross-template discriminator | overlap detected, unique match passes |
| Fallback | AI timeout → mode=aiFailedFallback + system result, AI parse fail → fallback |
| Rate limit | burst exceeded → 429, concurrent limit, cache dedup |
| Intent parser | NL → DraftIntent, entity resolution, missing field detection |
| Secret handling | token in prompt → masked in draft, not in log |
| SSRF | private IP block, localhost block, non-https block |
| Dry run | matched=true, targetsTriggered counts, incomplete targets flagged |
| Save/Deploy separation | save does not deploy, deploy requires saved config |
| Metrics | all counters incremented correctly per scenario |
| Versioning | promptVersion + schemaVersion in response + audit log |
| Circuit breaker | CLOSED→OPEN on threshold, HALF_OPEN probe, per-provider isolation |
| Idempotency | identical requestHash → cache hit, no AI call |
| EventType registry | exact match, alias match, similarity match, aiProposed → requiresApproval |
| State machine | invalid transitions rejected (incomplete→deploy blocked) |
| Feature flags | AI_GLOBAL_ENABLED=false → all workspaces systemOnly |
| Audit debug | redacted snapshot stored, expires in 24h |
| Prompt loader | template version mismatch → fail startup, required vars validated |
| Config versioning | version++ on save, history snapshot, rollback via deploy old version |
| Permission split | deploy_ingest_config separate from edit_ingest_template |
```

---

## 16. Idempotency / Request Dedup

**File:** `internal/services/aimappingsvc/service.go`

ก่อน call AI ทุกครั้ง:

```go
requestHash := hash(workspaceId + sourceFamily + samplePayload + existingMappings)
if cached, ok := dedupcache.Get(ctx, requestHash); ok {
    return cached, nil
}
// ... call AI ...
dedupcache.Set(ctx, requestHash, result, 5*time.Minute)
```

Redis key: `ai_suggest_dedup:{workspaceId}:{hash}`
TTL: 5 นาที

ป้องกัน:
- FE retry loop
- user spam click
- network retry ซ้ำ

---

## 17. Circuit Breaker

**File:** `internal/gateways/aiprovider/circuitbreaker.go`

### State Machine

```
CLOSED → OPEN → HALF_OPEN → CLOSED
```

| State | พฤติกรรม |
|---|---|
| `CLOSED` | ปกติ — ส่ง request ไป provider |
| `OPEN` | skip AI → return `mode=systemOnly`, `warning=aiCircuitOpen` |
| `HALF_OPEN` | ส่ง 1 probe request — ถ้าผ่าน → CLOSED, ถ้าพัง → OPEN อีก 60s |

### Trigger Conditions (per provider, ไม่ใช่ global)

```go
type BreakerConfig struct {
    FailureRateThreshold  float64       // default 0.30 (30%)
    P95LatencyThreshold   time.Duration // default 3s
    TimeoutRateThreshold  float64       // default 0.20 (20%)
    WindowSize            time.Duration // default 30s
    OpenCooldown          time.Duration // default 60s
}
```

เปิด OPEN ถ้า condition ใดข้อหนึ่งเป็นจริงภายใน window:
- `failureRate > 30%`
- `p95Latency > 3s`
- `timeoutRate > 20%`

### Per-Provider Isolation

```go
type ProviderCircuitBreakers struct {
    Gemini *CircuitBreaker
    OpenAI *CircuitBreaker
    Claude *CircuitBreaker
}
```

Gemini down ≠ block OpenAI / Claude

### Response เมื่อ circuit OPEN

```json
{
  "mode": "systemOnly",
  "provider": "gemini",
  "warning": "aiCircuitOpen",
  "warnings": ["AI provider temporarily unavailable, using system suggestion"]
}
```

---

## 18. Config Versioning

**File:** `models/workspacemod/aiConfig.go` (เพิ่มเติม)

```go
type ConfigDraft struct {
    DraftID     string
    WorkspaceID string
    Version     int       // bump on every save — ไม่ใช่ UUID random
    Status      string    // ดู §18.1
    // ...
}
```

ทุก save → `Version++` และ immutable snapshot เก็บไว้ (collection: `config_draft_history`)

Rollback path: deploy version เก่าได้จาก history

---

## 19. Draft State Machine

### Allowed Transitions

```
incomplete ──→ ready      (เมื่อ detectMissing() = empty)
ready      ──→ reviewed   (เมื่อ user กด confirm review)
reviewed   ──→ published  (เมื่อ save)
published  ──→ deployed   (เมื่อ deploy explicit)
deployed   ──→ published  (rollback)

blocked:
  incomplete → deploy  ❌
  incomplete → publish ❌
  ready      → deploy  ❌
  any        → skip review ❌ (ถ้า policy enforce review)
```

```go
var allowedTransitions = map[string][]string{
    "incomplete": {"ready"},
    "ready":      {"reviewed", "incomplete"},
    "reviewed":   {"published", "ready"},
    "published":  {"deployed", "reviewed"},
    "deployed":   {"published"},
}

func ValidateTransition(from, to string) error {
    // ถ้า to ไม่อยู่ใน allowed → return ErrInvalidStateTransition
}
```

---

## 20. EventType Registry

**File:** `models/ingestmod/eventTypeRegistry.go`

### Registry Definition

```go
// EventTypeDefinition defines a canonical event type.
// Loaded from config/ingest/eventTypeRegistry.json at startup.
type EventTypeDefinition struct {
    Code        string   `json:"code"`        // "VEHICLE_DETECTION"
    Aliases     []string `json:"aliases"`     // ["car_detect","vehicle_found","plate_detect"]
    Category    string   `json:"category"`    // "traffic" | "security" | "people" | "fire"
    Description string   `json:"description"`
}
```

**File:** `config/ingest/eventTypeRegistry.json` — source of truth, shipped in binary

### AI Prompt Instruction (เปลี่ยนจาก "suggest" เป็น "choose or propose")

```
Choose eventType from the provided registry list.
If none matches with confidence ≥ 0.7, propose a new one.
Set eventTypeSource = "standardMatched" or "aiProposed".
```

### Semantic Matching (ก่อน return)

```go
func matchEventType(aiProposed string, registry []EventTypeDefinition) (code string, confidence float64, source string) {
    // 1. exact match → confidence=1.0, source=standardMatched
    // 2. alias match → confidence=0.95, source=standardMatched
    // 3. levenshtein / token overlap → confidence score
    // if best score > 0.8 → auto-map
    // if 0.5-0.8 → warn + suggest
    // if < 0.5 → aiProposed, requiresApproval=true
}
```

### Response Fields เพิ่ม

```go
type AISuggestResult struct {
    // ...existing fields...
    SuggestedEventType   string `json:"suggestedEventType"`
    EventTypeSource      string `json:"eventTypeSource"`      // "standardMatched" | "aiProposed"
    EventTypeConfidence  float64 `json:"eventTypeConfidence"`
    EventTypeRequiresApproval bool `json:"eventTypeRequiresApproval"`
}
```

ถ้า `aiProposed` → ห้าม auto-create event type → FE ต้องแสดง review badge

---

## 21. Permission Split

แยก permission ให้ชัดสำหรับ enterprise multi-role:

| Action | Permission |
|---|---|
| GET/PUT ai-config, api-key | `manage_ingest_config` |
| POST ai-suggest | `edit_ingest_template` |
| POST from-prompt, refine | `edit_ingest_template` |
| POST dry-run | `edit_ingest_template` |
| POST save | `edit_ingest_template` |
| POST deploy | `deploy_ingest_config` ← แยก explicit |

> `deploy_ingest_config` ควรเป็น permission พิเศษ — assign แยกจาก edit เพื่อ 4-eyes principle

---

## 22. Audit Debug Snapshot

**Mode:** opt-in per workspace, TTL-limited

```go
type AISuggestAuditBlob struct {
    RequestID       string
    WorkspaceID     string
    PromptVersion   string
    SchemaVersion   string
    Mode            string
    Provider        string
    Model           string
    // Redacted payload: paths preserved, values masked for PII
    RedactedPayload map[string]any
    // AI output metadata (no raw text)
    AIOutputFieldCount   int
    AIOutputMatchRules   int
    ParseSuccess         bool
    ValidationSuccess    bool
    ConflictsCount       int
    LatencyMs            int64
    CreatedAt            time.Time
    ExpiresAt            time.Time // TTL 24h
}
```

เก็บใน MongoDB collection `ai_suggest_audit` ถ้า workspace enable debug mode

---

## 23. Prompt Template Storage

**File layout:**

```
config/
  prompts/
    aiSuggest_v1.0.tmpl      ← Feature A prompt template
    configDraft_v1.0.tmpl    ← Feature B prompt template
```

```go
type PromptTemplate struct {
    Version          string
    RequiredVars     []string  // validate ก่อน render
    Content          string
}

type PromptLoader interface {
    Load(name, version string) (*PromptTemplate, error)
    // Hot reload: re-read from embed.FS or configmap
    Reload() error
}
```

Rules:
- template version ต้อง match `AISuggestPromptVersion` constant — ไม่ตรงให้ fail startup
- validate required placeholders on load
- snapshot test: render with fixture input → compare output hash
- ถ้า deploy บน Kubernetes: ใช้ ConfigMap mount → hot reload โดยไม่ restart

---

## 24. Feature Flags (Global Override)

**File:** `internal/app/container.go` — load from env at startup

```go
type AIFeatureFlags struct {
    GlobalEnabled      bool   // AI_GLOBAL_ENABLED (default true)
    ForceSystemOnly    bool   // AI_FORCE_SYSTEM_ONLY (default false)
    ProviderOverride   string // AI_PROVIDER_OVERRIDE (empty = use workspace config)
    DisabledProviders  []string // AI_DISABLED_PROVIDERS (comma-separated)
}
```

Use cases:
- incident → `AI_FORCE_SYSTEM_ONLY=true` → ทุก workspace ได้ system suggestion ทันที
- staging test → `AI_PROVIDER_OVERRIDE=gemini` → ทุก workspace ใช้ Gemini เท่านั้น
- provider breach → `AI_DISABLED_PROVIDERS=openai` → block provider นั้น

Response เมื่อ global flag บังคับ:

```json
{
  "mode": "systemOnly",
  "warning": "aiGloballyDisabled"
}
```

---

## 25. Updated Package Structure (Final)

```
models/
  workspacemod/
    aiConfig.go
  ingestmod/
    eventTypeRegistry.go     ← EventTypeDefinition

config/
  ingest/
    eventTypeRegistry.json   ← source of truth
  prompts/
    aiSuggest_v1.0.tmpl
    configDraft_v1.0.tmpl

internal/
  repo/
    aiconfigrepo/
      repo.go
    aisuggestauditrepo/      ← NEW: audit debug snapshots
      repo.go

  gateways/
    aiprovider/
      provider.go
      capabilities.go
      factory.go
      circuitbreaker.go      ← NEW: per-provider state machine
      gemini.go
      openai.go
      claude.go

  services/
    aimappingsvc/
      service.go
      prompt.go              ← buildPrompt + PromptLoader + EnumContextResolver
      reducer.go
      schema.go
      validate.go
      merge.go
      metrics.go
      eventtype.go           ← NEW: matchEventType + registry lookup
      dedup.go               ← NEW: requestHash cache

    aiconfigdraftsvc/
      service.go
      intentparser.go
      entityresolver.go
      missingdetector.go
      statemachine.go        ← NEW: ValidateTransition
      dryrun.go

controllers/
  aimappingapi/
    suggest.go
    config.go

  aiconfigdraftapi/
    draft.go
    refine.go
    dryrun.go
    save.go
    deploy.go

router/
  aiMapping.go
  aiConfigDraft.go
```
