# Phase B Align Review — ก่อนเริ่ม Implement

**Date:** 2026-04-12  
**Repos:** `gateway-api` (gw), `klynx-api-feature` (klynx)  
**สถานะ:** ⏳ รอ align 5 จุดก่อน implement Phase B

---

## Architectural Decisions ล่าสุด

- **saasKlynx ↔ saasPhibek** → ใช้ **gRPC** (trusted platform-to-platform integration)
- **webhook** → สำหรับ third-party integration เท่านั้น
- **appliance / enterprise** → klynx platform เป็น façade / control plane — user ไม่เห็น config 4 ชั้น

---

## ส่วน 1 — Architectural Decision: Template ↔ Target Binding

### ปัญหาที่พบ

แผนปัจจุบันยังไม่ชัดว่า `ingestTemplates` กับ `deliveryTargets` เชื่อมกันอย่างไร  
ถ้า embed target ลง template โดยตรง จะเกิดปัญหา:

- reuse target ข้ามหลาย template ยาก
- เปลี่ยน endpoint / config ของ target ต้องแก้หลาย template
- audit/trace ว่า target ไหนถูกใช้โดย template ไหน ทำได้ยาก
- realtime/normalize จะไปปนอยู่ใน template logic โดยไม่จำเป็น

### โมเดล 3 Layers ที่ตัดสินใจแล้ว

**A. deliveryTargets** — ปลายทาง / channel เท่านั้น

```json
{ "id": "...", "name": "...", "type": "...", "enabled": true, "config": {} }
```

หน้าที่: destination, channel/protocol, endpoint/config, quota/enable/disable

---

**B. ingestTemplates** — classifier / normalize / extract เท่านั้น

```json
{ "id": "...", "name": "...", "sourceFamily": "...", "matchRules": [], "fieldMapping": {} }
```

หน้าที่: match raw payload, classify event, normalize fields, extract data  
**ห้ามมี dispatch logic อยู่ใน template**

---

**C. templateDeliveryBindings** — ชั้นกลางเชื่อม template ↔ target ↔ stage ↔ messageTemplate

```json
{
  "templateId": "tpl_aibox_intrusion",
  "targetId": "target_line_ops",
  "dispatchStage": "normalize",
  "messageTemplateId": "msg_line_intrusion_th",
  "enabled": true
}
```

ตัวอย่าง multi-stage ต่อ template เดียว:

```json
{ "templateId": "tpl_aibox_intrusion", "targetId": "target_line_ops",      "dispatchStage": "normalize" }
{ "templateId": "tpl_aibox_intrusion", "targetId": "target_webhook_fast",  "dispatchStage": "realtime"  }
```

### กฎที่ต้องล็อก

- `template` = classifier only
- `target` = destination only
- `binding` = ตัวเดียวที่เชื่อม template + target + dispatchStage + messageTemplate

---

## ส่วน 2 — UX ตาม Deployment Profile

### 2.1 saasPhibek — Full Admin UI

```
delivery/targets              → จัดการปลายทาง (destination only)
ingest/templates              → classifier / normalize config
templateDeliveryBindings      → bind template + target + dispatchStage + matchFields
messageTemplates              → render ตาม channel
```

**Runtime flow:**
```
raw event → template match → normalize → lookup bindings → dispatch ตาม dispatchStage
```

### 2.2 saasPublic — Full Admin UI (same as saasPhibek UI)

> ⚠️ **saasPublic ≠ saasPhibek สำหรับ backend** — UI เหมือนกัน แต่ transport ต่างกัน:
> - saasPublic: REST fallback `GET /ingest/details/{eventId}` — **MUST NOT use gRPC**
> - saasPhibek: gRPC (trusted platform-to-platform)

### 2.2 appliance / enterprise — klynx Platform เป็น Façade

**Simple mode (default — user เห็น):**

```
Template นี้ส่งไปไหน   → [Ops LINE] [Security Discord]
Realtime              → On / Off
ข้อความ               → default / custom
```

klynx create/patch binding ใน gw อัตโนมัติ — user ไม่ต้องรู้ว่ามี `deliveryTarget` หรือ `templateDeliveryBinding`

**Advanced mode (admin only):** เปิด full config แบบเดียวกับ saasPhibek

**กฎ:**
- klynx เป็นคน create/patch binding ใน gw อัตโนมัติ
- `mode=klynx` system target ยังคง auto-provision และไม่ expose ให้ user เห็น

---

## ส่วน 3 — Conflicts & Gaps ที่ต้องแก้ก่อน Phase B

### ✅ C-1: payload ของ `mode=normalize` — **RESOLVED**

**ล็อกแล้ว:**
- `mode=normalize` webhook body = **full `NormalizedEvent` snapshot** ผ่าน webhook URL + HMAC
- `{ eventId, workspaceId, canonical: true }` = MQTT **Path B** เท่านั้น — ห้ามใช้เป็น webhook payload

klynx plan Section 5a แก้แล้ว — ทั้งสอง repo ตรงกัน

---

### ✅ C-2: `DeliveryTarget.type` + `mode=realtime` naming — **RESOLVED**

**ล็อกแล้ว:**
- Phase B supported types: `webhook`, `line`, `telegram`, `discord`
- `mqtt` → **NOT supported Phase B** — mark future only; MQTT ใน system นี้ = internal transport ไม่ใช่ delivery channel
- `mode=realtime` → **คงชื่อไว้** (ไม่ rename) แต่ต้อง enforce definition นี้:

```
mode=realtime = pre-Kafka dispatch stage (~0ms)
transport จริงขึ้นกับ deployment:
  appliance  → MQTT
  saasPublic → webhook
ไม่ใช่ transport และไม่ได้การันตี delivery
```

---

### ✅ C-3: klynx Phase C checklist saasPublic — **RESOLVED**

**ล็อกแล้ว:**
- `saasPublic` → REST `GET /ingest/details/{eventId}` เท่านั้น — **MUST NOT use gRPC**
- gRPC ใช้เฉพาะ: `appliance`, `enterprise`, `saasKlynx`, `saasPhibek`

klynx `gw-integration-plan.md` Phase C checklist แก้แล้ว

---

### ✅ G-1: `POST /klynx/entitlement/sync` — **RESOLVED**

**ล็อกแล้ว:**
- Phase B → **ไม่มี** endpoint นี้
- Phase E → เพิ่ม `POST /klynx/entitlement/sync` สำหรับ saasPublic entitlement sync

klynx Phase E checklist อัปเดตแล้ว — ไม่มีใน Phase B

---

## ส่วน 4 — Final Decision Summary

| # | เรื่อง | Decision |
|---|-------|---------|
| C-1 | `mode=normalize` payload | ✅ full NormalizedEvent ผ่าน webhook |
| C-2 | `DeliveryTarget.type=mqtt` | ✅ NOT Phase B — future only |
| C-2b | `mode=realtime` naming | ✅ คงชื่อ แต่ lock definition: pre-Kafka dispatch stage |
| C-3 | saasPublic + gRPC | ✅ REST only — MUST NOT use gRPC |
| G-1 | `/klynx/entitlement/sync` | ✅ Phase E เท่านั้น |
| binding | template ↔ target model | ✅ templateDeliveryBindings layer |
| C-A | `matchFields` location | ✅ moves to `templateDeliveryBinding` — ไม่อยู่บน target |
| C-B | `mode/dispatchStage` location | ✅ `dispatchStage` อยู่บน binding; `mode=klynx` คงอยู่บน target เป็น routing marker เท่านั้น |
| G-A | Phase B checklist | ✅ อัปเดตแล้ว — เพิ่ม CRUD สำหรับ binding, ingestTemplate, messageTemplate |
| G-B/G-C | messageTemplates / ingestTemplates | ✅ เพิ่มใน Phase B checklist แล้ว |
| G-D | Section 2.1 header | ✅ แยก saasPhibek / saasPublic เป็น subsection ต่างกัน |

---

## ✅ GO Phase B

**Condition:** ห้ามแก้ contract (mode, type, payload) ระหว่าง implement  
ถ้าจะแก้ → ต้อง PR ทั้งสอง repo พร้อมกัน + review รอบใหม่ก่อนเสมอ
