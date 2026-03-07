# Baseline-Ready Plan: Delivery + Notification + DLQ + Subscription Packages

> อัปเดตจาก baseline เดิม โดยเพิ่ม **subscription packages ใน DB**, **endpoint สำหรับ list plan/packages**, และ **quota/limit ฝั่ง delivery targets** ให้สัมพันธ์กับ feature Delivery/Notification/DLQ โดยตรง

---

## 1) Executive Summary

ตอนนี้ระบบเริ่มชัดแล้วว่า feature ที่กำลังจะทำ **ไม่ใช่แค่ delivery pipeline** แต่เป็น 3 เรื่องที่ต้องออกแบบให้ไปด้วยกัน:

1. **Event Delivery Pipeline**
   - รับ event ที่ผ่าน approve/normalize แล้ว
   - dispatch ไปยัง Webhook / LINE / Discord / Telegram
   - ล้มเหลวแล้วเข้า DLQ

2. **Template-Driven Notification**
   - mappingTemplate ต้องรู้ว่า event นี้ส่งไป target ไหน
   - notification channel ต้อง render ข้อความจาก template
   - field ที่เป็นรหัส เช่น `1,2,3` ต้อง map เป็น label เพื่อให้ operator ใช้งานง่าย

3. **Subscription/Plan Enforcement**
   - แต่ละ plan ต้องกำหนดได้จาก DB ไม่ใช่ hardcode ใน code
   - จำกัดจำนวน organization, members, events/month, webhook per org, notification channels per org
   - UI ต้องมี endpoint สำหรับอ่าน plan catalog ไป render หน้า subscription

ถ้ายังเก็บรายละเอียด package ไว้ใน code ต่อไป จะเจอปัญหาแน่:
- แก้ plan ต้อง deploy backend ใหม่
- หน้าบ้านกับหลังบ้านมีโอกาสไม่ตรงกัน
- เปลี่ยน quota/feature flag ลำบาก
- ทำ promotion / custom enterprise plan / future add-ons ยาก

**ข้อสรุปตรงๆ:**
- `subscriptions` ควรเก็บแค่ “ผู้เช่าใช้อะไรอยู่ตอนนี้”
- รายละเอียดของ plan ต้องแยกไปอยู่ใน collection ใหม่ เช่น `subscription_packages`
- delivery layer ต้อง enforce quota จาก plan package ไม่ใช่จาก constant ใน code

---

## 2) Current vs Target State

### Current State

`subscriptions` ตอนนี้เก็บแค่สถานะ subscription ของ tenant เช่น:

```json
{
  "id": "sub_69a1b50868e410341711beca",
  "tenantId": "aisom",
  "planId": "enterprise",
  "status": "active",
  "billingCycle": "yearly",
  "currentPeriodStart": "2026-02-27T16:03:02.776Z",
  "licenseKeyHash": "..."
}
```

ซึ่ง **ยังไม่พอ** สำหรับ UI และ quota enforcement เพราะไม่มีข้อมูลว่า:
- plan นี้ให้สร้าง org ได้กี่อัน
- ใช้ team member ได้กี่คน
- event/month ได้เท่าไร
- webhook per org ได้กี่ตัว
- LINE / Discord / Telegram ใช้ได้กี่ target ต่อ org
- analytics level / support tier / custom integrations / SSO / on-prem ใช้ได้หรือไม่

### Target State

แยกเป็น 2 ชั้น:

#### A. `subscription_packages`
เก็บ catalog ของทุก plan ที่ระบบรองรับ

#### B. `subscriptions`
เก็บว่า tenant ปัจจุบัน subscribe plan ไหน และ active อยู่หรือไม่

ความสัมพันธ์:

```text
subscription_packages (catalog/master)
        ↑
        │ by planId
        │
subscriptions (tenant active subscription)
```

---

## 3) High-Level Architecture

```text
Device
  └─► POST /events/:orgId
        │
        ├─ NOT approved ──────────────► event_management (statusName: "pending")
        │                                    │
        │                               Operator review:
        │                               PATCH /:id (set eventType)
        │                               POST  /:id/approve
        │                                    │
        ├─ Approved + template match ───┤
        │                               │
        └─ Manual approve (operator) ───┘
                                        │
                                        ▼
                               Kafka: raw.events
                               (CanonicalEvent JSON)
                                        │
                                        ▼
                               Normalizer Consumer
                                   │          │
                                   ▼          ▼
                            event_details      S3
                            (MongoDB)     (binary only)
                                   │
                                   ▼
                          Kafka: normalized.events
                                   │
                                   ▼
                            Delivery Consumer
                                   │
                  ┌────────────────┼────────────────┐
                  ▼                ▼                ▼
               Webhook           Message          DLQ
               dispatch          channels       (per-target)
                          (LINE/Discord/Telegram)
```

สำคัญ:
- `Webhook` default ส่ง JSON event ได้เลย
- `LINE/Discord/Telegram` ต้องผ่าน `messageTemplate`
- DLQ ต้องเป็น **ต่อ target** ไม่ใช่ต่อ event ก้อนเดียว
- quota ของ plan ต้องถูก check ตอน **create/update target**, และบางส่วนต้อง check ตอน **dispatch** ด้วย

---

## 4) Subscription Model Design

### 4.1 Collection: `subscription_packages`

ใช้เป็น master data ของแพ็กเกจทั้งหมด

```json
{
  "id": "enterprise",
  "code": "enterprise",
  "name": {
    "th": "Enterprise",
    "en": "Enterprise"
  },
  "description": {
    "th": "สำหรับองค์กรขนาดใหญ่",
    "en": "For large-scale deployments"
  },
  "status": "active",
  "sortOrder": 300,
  "isPublic": true,
  "billing": {
    "supportedCycles": ["monthly", "yearly"],
    "price": {
      "monthly": 0,
      "yearly": 0,
      "currency": "THB",
      "display": {
        "th": "ฟรี",
        "en": "Free"
      }
    }
  },
  "limits": {
    "organizations": -1,
    "teamMembers": -1,
    "eventsPerMonth": -1,
    "webhooksPerOrg": 3,
    "lineTargetsPerOrg": 1,
    "discordTargetsPerOrg": 1,
    "telegramTargetsPerOrg": 1,
    "messageChannelsPerOrg": 3
  },
  "features": {
    "basicAnalytics": true,
    "advancedAnalytics": true,
    "emailSupport": true,
    "priorityEmailSupport": true,
    "dedicatedSupport24x7": true,
    "customIntegrations": true,
    "apiAccess": true,
    "customSla": true,
    "onPremise": true,
    "sso": true,
    "advancedSecurity": true
  },
  "ui": {
    "badge": {
      "th": "แผนปัจจุบัน",
      "en": "Current plan"
    },
    "highlight": true,
    "theme": "success",
    "featureList": [
      { "key": "organizations", "label": { "th": "Unlimited Organizations", "en": "Unlimited Organizations" } },
      { "key": "teamMembers", "label": { "th": "Unlimited Team Members", "en": "Unlimited Team Members" } },
      { "key": "eventsPerMonth", "label": { "th": "Unlimited Events", "en": "Unlimited Events" } },
      { "key": "customSla", "label": { "th": "Custom SLA", "en": "Custom SLA" } },
      { "key": "dedicatedSupport24x7", "label": { "th": "24/7 Dedicated Support", "en": "24/7 Dedicated Support" } },
      { "key": "onPremise", "label": { "th": "On-premise Option", "en": "On-premise Option" } },
      { "key": "ssoAdvancedSecurity", "label": { "th": "SSO & Advanced Security", "en": "SSO & Advanced Security" } },
      { "key": "customIntegrations", "label": { "th": "Custom Integrations", "en": "Custom Integrations" } }
    ]
  },
  "createdAt": "2026-03-07T00:00:00Z",
  "updatedAt": "2026-03-07T00:00:00Z"
}
```

### Rules

- `-1` = unlimited
- `isPublic=true` = แสดงในหน้า pricing/subscription UI
- `status=active` = เลือกสมัครได้
- `ui.featureList` มีไว้ให้ frontend render ได้ตรงกับ backend โดยไม่ hardcode
- `limits.messageChannelsPerOrg` เป็นเพดานรวมของ LINE/Discord/Telegram ต่อ org
- per-type limits เช่น `lineTargetsPerOrg=1` ใช้กัน over-provision และทำ UX ชัด

---

## 5) Recommended Plan Catalog (Initial Seed)

### 5.1 Freemium

```json
{
  "id": "freemium",
  "code": "freemium",
  "name": { "th": "Freemium", "en": "Freemium" },
  "description": {
    "th": "เริ่มต้นใช้งานด้วยฟีเจอร์พื้นฐาน",
    "en": "Get started with basic features"
  },
  "status": "active",
  "sortOrder": 100,
  "isPublic": true,
  "billing": {
    "supportedCycles": ["monthly", "yearly"],
    "price": {
      "monthly": 0,
      "yearly": 0,
      "currency": "USD",
      "display": { "th": "ฟรี", "en": "Free" }
    }
  },
  "limits": {
    "organizations": 1,
    "teamMembers": 5,
    "eventsPerMonth": 1000,
    "webhooksPerOrg": 1,
    "lineTargetsPerOrg": 1,
    "discordTargetsPerOrg": 1,
    "telegramTargetsPerOrg": 1,
    "messageChannelsPerOrg": 1
  },
  "features": {
    "basicAnalytics": true,
    "advancedAnalytics": false,
    "emailSupport": true,
    "priorityEmailSupport": false,
    "dedicatedSupport24x7": false,
    "customIntegrations": false,
    "apiAccess": false,
    "customSla": false,
    "onPremise": false,
    "sso": false,
    "advancedSecurity": false
  }
}
```

> ความหมายของ `messageChannelsPerOrg = 1` คือใช้ message event ได้ **แค่อย่างใดอย่างหนึ่ง 1 channel** ต่อองค์กร เช่น LINE หรือ Discord หรือ Telegram อย่างใดอย่างหนึ่งเท่านั้น

### 5.2 Pro

```json
{
  "id": "pro",
  "code": "pro",
  "name": { "th": "Pro", "en": "Pro" },
  "description": {
    "th": "สำหรับทีมที่กำลังเติบโต",
    "en": "For growing teams"
  },
  "status": "active",
  "sortOrder": 200,
  "isPublic": true,
  "billing": {
    "supportedCycles": ["monthly", "yearly"],
    "price": {
      "monthly": 49,
      "yearly": 490,
      "currency": "USD",
      "display": { "th": "$49/เดือน", "en": "$49/month" }
    }
  },
  "limits": {
    "organizations": 5,
    "teamMembers": 25,
    "eventsPerMonth": 100000,
    "webhooksPerOrg": 2,
    "lineTargetsPerOrg": 1,
    "discordTargetsPerOrg": 1,
    "telegramTargetsPerOrg": 1,
    "messageChannelsPerOrg": 3
  },
  "features": {
    "basicAnalytics": true,
    "advancedAnalytics": true,
    "emailSupport": true,
    "priorityEmailSupport": true,
    "dedicatedSupport24x7": false,
    "customIntegrations": true,
    "apiAccess": true,
    "customSla": false,
    "onPremise": false,
    "sso": false,
    "advancedSecurity": false
  }
}
```

### 5.3 Enterprise

```json
{
  "id": "enterprise",
  "code": "enterprise",
  "name": { "th": "Enterprise", "en": "Enterprise" },
  "description": {
    "th": "สำหรับองค์กรขนาดใหญ่",
    "en": "For large-scale deployments"
  },
  "status": "active",
  "sortOrder": 300,
  "isPublic": true,
  "billing": {
    "supportedCycles": ["monthly", "yearly"],
    "price": {
      "monthly": 0,
      "yearly": 0,
      "currency": "USD",
      "display": { "th": "ฟรี", "en": "Free" }
    }
  },
  "limits": {
    "organizations": -1,
    "teamMembers": -1,
    "eventsPerMonth": -1,
    "webhooksPerOrg": 3,
    "lineTargetsPerOrg": 1,
    "discordTargetsPerOrg": 1,
    "telegramTargetsPerOrg": 1,
    "messageChannelsPerOrg": 3
  },
  "features": {
    "basicAnalytics": true,
    "advancedAnalytics": true,
    "emailSupport": true,
    "priorityEmailSupport": true,
    "dedicatedSupport24x7": true,
    "customIntegrations": true,
    "apiAccess": true,
    "customSla": true,
    "onPremise": true,
    "sso": true,
    "advancedSecurity": true
  }
}
```

> ตรงนี้ผมไม่ sugarcoat: การตั้ง Enterprise เป็น “ฟรี” อาจใช้ชั่วคราวใน dev/demo ได้ แต่ถ้าไป production จริงโดยไม่มีเหตุผลเชิงธุรกิจชัดเจน มันทำให้ model subscription ของคุณพังเอง เพราะแผนบนสุดกลายเป็น free unlimited. ถ้าเป็น internal/demo ให้แยก `isPublic=false` หรือทำ `custom enterprise contract` ดีกว่า

---

## 6) Changes to `subscriptions` Collection

`subscriptions` ไม่ควรยัดรายละเอียด plan ซ้ำทั้งหมด เพราะจะเกิด data drift

### Recommended shape

```json
{
  "id": "sub_69a1b50868e410341711beca",
  "tenantId": "aisom",
  "planId": "enterprise",
  "packageVersion": 1,
  "status": "active",
  "billingCycle": "yearly",
  "createdAt": "2026-02-27T15:15:20.826Z",
  "updatedAt": "2026-03-02T05:44:40.910Z",
  "currentPeriodStart": "2026-02-27T16:03:02.776Z",
  "currentPeriodEnd": null,
  "licenseKeyHash": "0c2aed4eed6e24f5d5657c4c617577db296689a2187cadea73f74e24e6be2b63",
  "overrides": {
    "limits": {
      "webhooksPerOrg": 5
    },
    "features": {
      "dedicatedSupport24x7": true
    }
  }
}
```

### ทำไมต้องมี `overrides`

เพราะ enterprise/custom contract จะมีเคสแบบนี้แน่:
- ลูกค้ารายนี้ได้ webhook เพิ่ม
- ลูกค้ารายนี้ได้ org ไม่จำกัด แต่ event limit ยังมี
- ลูกค้ารายนี้มี SSO แต่ไม่มี on-prem

ถ้าไม่มี `overrides` คุณจะต้องแตก plan ใหม่รัวๆ จนระบบ plan catalog เละ

---

## 7) Effective Subscription Resolution

เวลาจะ enforce quota ห้ามอ่านจาก `subscriptions` ตรงๆ อย่างเดียว ต้อง resolve แบบนี้:

```text
1. load subscription by tenantId (active)
2. load package by planId
3. merge package + subscription.overrides
4. ได้ effectiveSubscription
5. ใช้ effectiveSubscription ในทุก policy/quota check
```

### Effective shape (internal only)

```json
{
  "tenantId": "aisom",
  "planId": "enterprise",
  "status": "active",
  "billingCycle": "yearly",
  "limits": {
    "organizations": -1,
    "teamMembers": -1,
    "eventsPerMonth": -1,
    "webhooksPerOrg": 5,
    "lineTargetsPerOrg": 1,
    "discordTargetsPerOrg": 1,
    "telegramTargetsPerOrg": 1,
    "messageChannelsPerOrg": 3
  },
  "features": {
    "apiAccess": true,
    "customIntegrations": true,
    "advancedAnalytics": true,
    "sso": true,
    "advancedSecurity": true
  }
}
```

---

## 8) Delivery/Notification Quota Enforcement

นี่คือจุดที่หลายระบบออกแบบพัง: ไป enforce แค่ UI แต่ backend ไม่ enforce จริง

ต้อง enforce อย่างน้อย 3 จุด

### 8.1 ตอนสร้าง Delivery Target

เช่น create target type = webhook / line / discord / telegram

ตรวจว่าใน org นี้มี target ชนิดนั้นเกิน plan limit หรือยัง

ตัวอย่าง:
- Freemium: สร้าง webhook ได้ 1
- Freemium: message channel รวมได้แค่ 1 -> ถ้าสร้าง LINE แล้ว จะสร้าง Discord/Telegram เพิ่มไม่ได้
- Pro: webhook 2, LINE 1, Discord 1, Telegram 1, รวม messageChannelsPerOrg=3

### 8.2 ตอน bind target เข้ากับ MappingTemplate

ถึง target จะมีอยู่แล้ว แต่ถ้าจะ bind ใช้งานจริง ควรเช็ค:
- target อยู่ org เดียวกัน
- target ยัง enabled
- target type อยู่ใน feature ที่ plan รองรับ

### 8.3 ตอน Delivery Consumer dispatch

runtime check ควรมีอย่างน้อย:
- tenant subscription active ไหม
- target เกิน quota usage แบบ dynamic หรือไม่
- channel ถูก disable ระดับ tenant/org หรือไม่

> อย่าหวังพึ่งแค่ create-time validation เพราะมีโอกาสที่ plan downgrade แล้ว target เกิน limit อยู่แล้ว

### Downgrade policy ที่แนะนำ

ถ้า downgrade แล้ว target เกิน limit:
- ไม่ควรลบทิ้ง auto
- mark เป็น `overQuota=true`
- dispatch เฉพาะตาม priority/createdAt ตัวแรกก่อน
- หรือ block ทุก target ที่เกิน limit แล้วให้ operator จัดการเอง

**แนะนำ baseline:** block create ใหม่ + แสดง warning list ของ target ที่เกิน limit ก่อน ส่วน runtime ยัง allow ตัวเดิมชั่วคราว 1 release เพื่อไม่ทำของพังทันที

---

,
```

ผลหลัง normalize:

```json
{
  "payload": {
    "listType": "3",
    "listType_label": "บัญชีดำ"
  }
}
```

แนวนี้สอดคล้องกับ draft เดิมที่ normalizer จะ generate `<field>_label` เพิ่มอัตโนมัติเมื่อ field มี `valueCodes` fileciteturn2file2L20-L24

### 9.2 Add to `MappingTemplate`

```json
{
  "id": "tmpl-lpr-security",
  "name": "LPR Security Alert",
  "defaultLocale": "th",
  "messageTemplates": [
    {
      "channelType": "line",
      "locale": "th",
      "title": "แจ้งเตือน {{.payload.eventType}}",
      "body": "สถานที่: {{.payload.location}}\nอุปกรณ์: {{.payload.deviceName}}\nประเภทเหตุการณ์: {{.payload.eventType}}\nสถานะบัญชี: {{.payload.listType_label}}"
    },
    {
      "channelType": "discord",
      "locale": "en",
      "title": "Alert {{.payload.eventType}}",
      "body": "Location: {{.payload.location}}\nDevice: {{.payload.deviceName}}\nList Type: {{.payload.listType_label}}"
    }
  ],
  "deliveryTargets": [
    {
      "targetId": "webhook-security-main",
      "filter": [
        { "field": "payload.listType", "operator": "eq", "values": ["3"] }
      ]
    },
    {
      "targetId": "line-security-alert",
      "filter": [
        { "field": "payload.listType", "operator": "eq", "values": ["3"] }
      ]
    }
  ]
}
```

การ filter แบบนี้ตรงกับแนวที่คุณคิดไว้ และตรงกับ draft เดิมเรื่อง `payload.listType == "3"` ส่งเฉพาะ blacklist ได้เลย fileciteturn2file2L1-L13

### ข้อเสนอ UX สำคัญ

อย่าให้ frontend/operator พิมพ์ path กับ value มั่วเองทั้งก้อน เพราะจะพังง่ายมาก

ควรมี endpoint:
- `GET /api/v1/ingest/mappingTemplates/filterableFields`
- คืน field พร้อมชนิดข้อมูล + valueCodes ถ้ามี

ตัวอย่าง response:

```json
{
  "code": "SUCCESS",
  "details": [
    {
      "field": "payload.listType",
      "label": "List Type",
      "type": "string",
      "valueCodes": {
        "1": "ไม่ทราบ",
        "2": "บัญชีเฝ้าระวัง",
        "3": "บัญชีดำ"
      }
    },
    {
      "field": "payload.eventType",
      "label": "Event Type",
      "type": "string"
    }
  ],
  "status": true
}
```

Frontend แสดง label แต่ submit code จริงกลับ API

---

## 10) Message Template Design

จากข้อเสนอเดิม `messageTemplates []MessageTemplate` ถูกต้องแล้ว แต่ baseline ควรเพิ่มโครงให้รองรับหลาย channel แบบไม่มั่วกัน

### MessageTemplate model

```go
type MessageTemplate struct {
    channelType string            `json:"channelType" bson:"channelType"`
    locale      string            `json:"locale" bson:"locale"`
    title       string            `json:"title" bson:"title"`
    body        string            `json:"body" bson:"body"`
    extras      map[string]string `json:"extras,omitempty" bson:"extras,omitempty"`
}
```

### หมายเหตุ

- `channelType` สำคัญ เพราะ LINE/Discord/Telegram format ไม่เหมือนกัน
- ถ้ายังไม่รองรับ rich message เต็มรูปแบบ ให้เริ่มจาก text ก่อน
- `extras` เผื่ออนาคต เช่น Discord embed color, Telegram parse mode, LINE altText

### Render context

```json
{
  "eventId": "...",
  "eventType": "LPR_Brand",
  "occurredAt": "...",
  "payload": {
    "location": "ด่านหน้า 1",
    "deviceName": "cam-001",
    "listType": "3",
    "listType_label": "บัญชีดำ"
  },
  "source": {
    "orgId": "...",
    "deviceId": "cam-001"
  }
}
```

### Template example (TH)

```text
สถานที่: {{.payload.location}}
ชื่ออุปกรณ์: {{.payload.deviceName}}
ชนิดเหตุการณ์: {{.eventType}}
สถานะบัญชี: {{.payload.listType_label}}
ภาพ: {{.imageUrl}}
```

### Locale fallback chain

ตาม draft เดิม ควรใช้ fallback chain แบบนี้ fileciteturn2file0L55-L57

```text
TargetConfig.Locale → MappingTemplate.DefaultLocale → "en" → minimal default
```

---

## 11) DLQ Behavior (Final Baseline)

เรื่อง DLQ ที่สรุปไว้ก่อนหน้าใช้ได้ แต่ต้องย้ำให้ชัดว่า **deliver DLQ ต้องเป็น per-target** ตาม key rules เดิม fileciteturn2file0L47-L53

### Final Flow

```text
Event fail
  └─► ตรวจ stage ที่ fail
       ├─ normalize fail  → DLQ stage="normalize"
       ├─ deliver fail    → DLQ stage="deliver" + targetId
       └─ webhook fail    → DLQ stage="webhook" + targetId
```

### DLQ rules

- ถ้า `template.dlq.enabled = true` → insert `dlq_events`
- ถ้า disabled → drop + warn log
- `retry` = republish กลับ topic เดิม พร้อม header `dlqRetry=true`
- `replay` = republish ไป `raw.events` เพื่อเริ่มใหม่ทั้ง pipeline
- `abandon` = mark สถานะ abandoned
- ถ้า retry success และ consumer ตรวจเจอ header `dlqRetry=true` → auto resolve

### DLQ stages & lifecycle

ตรงกับ draft เดิม:
- stages: `normalize`, `deliver`, `webhook` fileciteturn2file3L1-L8
- lifecycle: `pending → retrying → resolved`, หรือ `abandoned` fileciteturn2file3L9-L15

### Idempotency key

draft เดิมใช้ `eventId:stage` ซึ่งยังไม่พอสำหรับ deliver หลาย target fileciteturn2file3L28-L36

**แก้เป็น baseline ใหม่:**

```text
normalize: messageId = {eventId}:normalize
deliver:   messageId = {eventId}:deliver:{targetId}
webhook:   messageId = {eventId}:webhook:{targetId}
```

ไม่งั้น event เดียว fail 2 targets พร้อมกัน จะชน unique key แล้วข้อมูลหาย

---

## 12) Collections / Indexes to Add

### 12.1 `subscription_packages`

Indexes:
- unique: `id`
- unique: `code`
- index: `status`
- index: `isPublic`
- index: `sortOrder`

### 12.2 `subscriptions`

Indexes:
- unique: `tenantId` (ถ้า active ได้ตัวเดียว)
- compound: `tenantId + status`
- index: `planId`

### 12.3 `delivery_targets`

Indexes:
- unique: `orgId + name`
- index: `tenantId + orgId + type`
- index: `tenantId + orgId + enabled`

### 12.4 `dlq_events`

Indexes:
- unique: `messageId`
- index: `tenantId + orgId + stage + status`
- index: `targetId`
- index: `createdAt`

### 12.5 `delivery_logs`

Indexes:
- index: `tenantId + orgId + targetId + status`
- index: `eventId`
- index: `createdAt`

---

## 13) API Plan

## 13.1 Subscription Package APIs

### GET `/api/v1/subscriptions/packages`

ใช้สำหรับหน้า pricing/subscription cards

Query params:
- `publicOnly=true|false` default true
- `status=active`
- `locale=th|en`

Response example:

```json
{
  "code": "SUCCESS",
  "details": [
    {
      "id": "freemium",
      "name": "Freemium",
      "description": "Get started with basic features",
      "billing": {
        "displayPrice": "ฟรี",
        "supportedCycles": ["monthly", "yearly"]
      },
      "limits": {
        "organizations": 1,
        "teamMembers": 5,
        "eventsPerMonth": 1000,
        "webhooksPerOrg": 1,
        "messageChannelsPerOrg": 1
      },
      "features": [
        "1 Organization",
        "5 Team Members",
        "1,000 Events/Month",
        "Basic Analytics",
        "Email Support",
        "Webhook 1 per org",
        "Message event 1 channel per org"
      ],
      "ui": {
        "highlight": false,
        "theme": "default"
      }
    }
  ],
  "status": true
}
```

### GET `/api/v1/subscriptions/current`

ใช้สำหรับแถบ “แผนปัจจุบัน” ด้านบนของ UI

คืน effective subscription ของ tenant ปัจจุบัน

### POST `/api/v1/subscriptions/changePlan`

ใช้ภายหลังเมื่อมี billing จริง

baseline ช่วงแรกอาจยังไม่เปิด public endpoint ถ้ายังไม่มี payment flow

---

## 13.2 Delivery/Template APIs

### MappingTemplate
- `GET /api/v1/ingest/mappingTemplates/filterableFields`
- `POST /api/v1/ingest/mappingTemplates/:id/renderPreview`
- `POST /api/v1/ingest/mappingTemplates/:id/testDelivery`

### DeliveryTarget
- `GET /api/v1/ingest/deliveryTargets`
- `POST /api/v1/ingest/deliveryTargets`
- `PATCH /api/v1/ingest/deliveryTargets/:id`
- `DELETE /api/v1/ingest/deliveryTargets/:id`

### ตอน create/update target ต้อง validate against subscription

ตัวอย่าง error format:

```json
{
  "code": "PLAN_LIMIT_EXCEEDED",
  "message": "Current plan allows only 1 message channel per organization",
  "status": false
}
```

---

## 13.3 DLQ APIs

ยึดตาม draft เดิมเป็นหลัก fileciteturn2file3L17-L26

- `GET /api/v1/ingest/dlq`
- `GET /api/v1/ingest/dlq/stats`
- `GET /api/v1/ingest/dlq/:id`
- `POST /api/v1/ingest/dlq/:id/retry`
- `POST /api/v1/ingest/dlq/:id/replay`
- `POST /api/v1/ingest/dlq/:id/abandon`

---

## 14) Backend Folder / File Plan (Go Fiber Baseline)

```text
models/subscriptionmod/
  package.go
  subscription.go
  effective.go

internal/repo/subscriptionrepo/
  repo.go
  mongo.go
  mongoBootstrap.go

internal/repo/subscriptionpackagerepo/
  repo.go
  mongo.go
  mongoBootstrap.go

internal/services/subscriptionsvc/
  getCurrent.go
  listPackages.go
  resolveEffective.go
  validateQuota.go

controllers/subscriptionapi/
  current.go
  listPackages.go

router/subscription.go

models/deliverymod/
  target.go
  log.go
  filter.go

internal/services/deliverysvc/
  createTarget.go
  updateTarget.go
  deleteTarget.go
  listTargets.go
  quota.go

internal/kafka/deliverycons/
  consumer.go
  dispatch.go
  render.go
  filter.go
  resolveSubscription.go

models/dlqmod/
  dlqEvent.go
  retry.go

internal/services/dlqsvc/
  list.go
  detail.go
  retry.go
  replay.go
  abandon.go

controllers/dlqapi/
  list.go
  detail.go
  retry.go
  replay.go
  abandon.go

router/dlq.go
router/ingest.go
```

---

## 15) Service Responsibilities

### `subscriptionsvc`
รับผิดชอบ:
- load current subscription
- load plan package
- merge overrides
- validate org/member/event/delivery quotas

### `deliverysvc`
รับผิดชอบ:
- CRUD delivery targets
- validate against active plan
- optional test delivery

### `deliverycons`
รับผิดชอบ:
- consume `normalized.events`
- load mapping template
- dispatch to targets
- render message
- create per-target DLQ on failure

### `dlqsvc`
รับผิดชอบ:
- list/detail/retry/replay/abandon
- auto resolve on success

---

## 16) UI/UX Notes

จากภาพหน้า organizations และ subscriptions ที่คุณแนบมา ตอนนี้ UI card ใช้งานได้แล้ว แต่ข้อมูล plan ยังดู hardcoded มาก

### สิ่งที่ควรเปลี่ยน

1. หน้า subscription card ไม่ควร hardcode feature list
2. UI ควรโหลดจาก `GET /subscriptions/packages`
3. แถบ “แผนปัจจุบัน” โหลดจาก `GET /subscriptions/current`
4. ปุ่ม “อัปเกรด” ควรรู้ว่า package ไหน selectable ได้
5. limits ของ webhook/message channel ควรถูกแสดงในหน้า create delivery target ด้วย

### UX แนะนำสำหรับ create target

ตัวอย่าง:

```text
Webhook: used 1/2
LINE: used 1/1
Discord: used 0/1
Telegram: used 0/1
Message channels total: used 1/3
```

แบบนี้ operator เข้าใจทันทีว่าเพิ่มอะไรได้อีก

---

## 17) Validation Rules

### Subscription package validation
- `id`, `code` ต้อง unique
- limit ทุกตัวต้องเป็น `-1` หรือ `>= 0`
- `messageChannelsPerOrg` ต้องไม่น้อยกว่า sum ที่ business ยอมรับ
- `supportedCycles` ต้องไม่ว่าง

### Delivery target validation
- org ต้องอยู่ใน tenant เดียวกัน
- target type ต้องอยู่ใน allowed list
- webhook url ต้อง valid
- channel secret/token ต้อง validate format
- create/update ต้องผ่าน quota check

### MappingTemplate validation
- `deliveryTargets.targetId` ต้องมีจริง
- target ต้องอยู่ org เดียวกัน
- filter field ต้องอยู่ใน filterableFields
- messageTemplate ต้องมีอย่างน้อย 1 locale ถ้าใช้ notification channels

---

## 18) Observability

ต้องมี metrics อย่างน้อย:

```text
deliveryDispatchTotal{targetType,status}
deliveryDispatchLatencyMs{targetType}
dlqInsertTotal{stage,targetType}
dlqRetryTotal{stage,targetType,status}
subscriptionQuotaRejectTotal{resourceType,planId}
```

Logs ที่ต้องมี context:
- tenantId
- orgId
- eventId
- templateId
- targetId
- targetType
- stage
- planId

Tracing:
- span `normalize.consume`
- span `delivery.consume`
- span `delivery.dispatch`
- span `dlq.insert`
- span `subscription.resolve`

---

## 19) Implementation Order (Recommended PRs)

### PR-SUB-1 — Subscription Package Catalog
- add `subscription_packages` model/repo/service/controller/router
- seed freemium/pro/enterprise
- add `GET /api/v1/subscriptions/packages`
- add `GET /api/v1/subscriptions/current`

### PR-SUB-2 — Effective Subscription Resolver + Quota Validator
- `subscriptionsvc.ResolveEffective()`
- `subscriptionsvc.ValidateTargetQuota()`
- wire into delivery target CRUD

### PR-PIPE-4 — Delivery Consumer + Target Dispatch
แนว consumer และ dispatch ตาม draft เดิมใช้ได้ fileciteturn2file1L15-L31 fileciteturn2file4L17-L27
- consume `normalized.events`
- load mapping template
- dispatch per target
- insert DLQ per target

### PR-PIPE-5 — Message Template Render + ValueCodes
- normalizer generate `_label`
- render template ตาม locale/channel
- add preview endpoint

### PR-PIPE-6 — MappingTemplate API Update
- add `deliveryTargets`
- add `messageTemplates`
- add `defaultLocale`
- add `filterableFields`

### PR-PIPE-7 — DLQ Management API
- list/stats/detail/retry/replay/abandon
- auto resolve on success

### PR-PIPE-8 — Observability + Delivery Logs
- metrics
- tracing
- delivery_logs
- admin dashboard hooks

---

## 20) Acceptance Criteria

### Subscription
- UI subscription cards render from API only
- current plan banner render from API only
- create/update delivery target ถูก block เมื่อเกิน plan limits
- downgrade แล้วระบบ detect over-quota ได้

### Delivery
- webhook ส่ง normalized event JSON ได้
- LINE/Discord/Telegram render ข้อความตาม template ได้
- valueCodes generate `_label` ได้จริง
- filter ใช้ code จริง แต่ UI แสดง label ได้

### DLQ
- failure ต่อ target เข้า DLQ แยก record
- retry success แล้ว auto resolve
- replay ส่งกลับ `raw.events` ได้
- duplicate fail ไม่สร้าง record ซ้ำผิดๆ

### Ops
- metrics/traces/logs อ่านปัญหาได้จริง
- admin/operator มองเห็น plan usage และ quota rejection ได้

---

## 21) Final Recommendations

1. **ย้ายรายละเอียด plan ออกจาก code ไป DB ทันที**
   - เรื่องนี้ไม่ใช่ nice-to-have แล้ว มันเป็นโครงหลักของ subscription system

2. **อย่าใช้ DLQ idempotency key แบบ `eventId:deliver` อย่างเดียว**
   - ต้องมี `targetId` ไม่งั้นข้อมูล fail หลาย target จะหาย

3. **อย่าให้ frontend พิมพ์ field path และ code values แบบ free-form ทั้งหมด**
   - ต้องมี `filterableFields` endpoint ไม่งั้น UX แย่และ error สูง

4. **เริ่ม notification จาก text-first ก่อน**
   - อย่าเพิ่ง over-engineer rich cards/flex message ทุก channel พร้อมกัน

5. **Enterprise free เป็น decision ที่เสี่ยง**
   - ถ้าเป็น demo/internal OK
   - ถ้าเป็น public offering จริง business model จะพังเอง
