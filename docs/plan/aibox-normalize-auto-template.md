# Plan: AIBOX Normalize Data — Auto Template

**Date:** 2026-04-12
**Scope:** `gateway-api` only

---

## 1. สรุปปัญหาปัจจุบัน

### 1.1 ValueCodes ไม่ถูก propagate ลง DB template

`MappingSuggestion` → `MappingTemplate` (auto-created ใน DB) pipeline มีช่องว่าง:

| Layer | File | ปัญหา |
|---|---|---|
| Model | `models/ingestmod/mappingSuggestion.go:29` | `SuggestionFieldMap` ไม่มี field `ValueCodes` |
| Converter | `internal/services/ingestsvc/suggestion_apply.go:86-88` | `convertSuggestionToTemplate` copy แค่ SourcePath / TargetPath |
| Normalizer | `internal/services/ingestsvc/templateMatch.go:272-276` | ใช้ `fm.ValueCodes` แต่มักจะ empty เพราะถูก drop ไปแล้ว |

**ผล**: `eventAttribute.gender = 1` ไม่มี `eventAttribute.gender_label = "Male"` ใน normalized event

### 1.2 Field mappings ไม่ครบ

`config/ingest/mappingsuggestions/AIBOX/aibox.generalDetect.json` ปัจจุบัน map เฉพาะ Human body attributes บางส่วน ขาด:

| Category | Fields ที่ขาด |
|---|---|
| Alarm | `alarmType` (root field — event category 0-70) |
| Human (เพิ่มเติม) | `glasses` |
| Facial | `name`, `listType`, `similarity`, `featureImageId`, `featureImageUrl`, `idCard`, `glasses` |
| Non-motor vehicle | `helmet`, `number`, `nonVehicleType`, `nonVehiclePlate`, `nonvehicleColor` |
| Vehicle | `plateLicense`, `plateColor`, `carType`, `carColor`, `carBrand`, `speed`, `listType`, `arrowDirection` |
| Other/Stats | `trackTime`, `stayTime`, `direction` (at root eventAttribute), `arrowStartName`, `arrowEndName`, `num` |

### 1.3 Existing enum fields ไม่มี ValueCodes

Fields ที่ map อยู่แล้ว เช่น `age`, `gender`, `mask`, `upper`, `upperColor` ฯลฯ ไม่มี `_label` เพราะ `SuggestionFieldMap` ไม่มี `ValueCodes`

---

## 2. Strategy

**Single comprehensive template** — ใช้ suggestion เดียว (`aibox.generalDetect`) ที่ map ทุก field จากทุก alarm category รวมกัน

เหตุผล:
- AIBOX payload มี `eventAttribute` เป็น nested object เสมอ — match rule `eventAttribute exists` ใช้ได้กับทุก alarm type
- Fields จาก alarm type ต่างกันจะมีแค่บาง fields ใน payload — normalizer skip fields ที่ไม่มีอยู่ใน payload ตามปกติ
- Multi-template approach ต้องการ mutually-exclusive match rules ซึ่งทำได้ยากเพราะ payload ของบาง alarm type overlap กัน (เช่น facial capture ก็มี `gender` เหมือน pedestrian)
- หากในอนาคตต้องการ per-alarm-type template สามารถสร้างผ่าน management API และตั้ง priority ได้

---

## 3. Code Changes

### 3.1 เพิ่ม `ValueCodes` ใน `SuggestionFieldMap`

**File:** `models/ingestmod/mappingSuggestion.go`

```go
// SuggestionFieldMap maps a source payload field path to a target field path.
type SuggestionFieldMap struct {
    SourceField string            `json:"sourceField"`
    TargetField string            `json:"targetField"`
    // ValueCodes maps raw integer/string codes to human-readable labels.
    // Normalizer generates a `<targetField>_label` field for matched codes.
    // Example: {"0": "Unknown", "1": "Male", "2": "Female"}
    ValueCodes  map[string]string `json:"valueCodes,omitempty"`
}
```

### 3.2 Propagate `ValueCodes` ใน converter

**File:** `internal/services/ingestsvc/suggestion_apply.go`

แก้ `convertSuggestionToTemplate`:

```go
for _, fm := range sg.FieldMappings {
    mappings = append(mappings, ingestmod.FieldMapping{
        SourcePath: fm.SourceField,
        TargetPath: fm.TargetField,
        ValueCodes: fm.ValueCodes, // เพิ่มบรรทัดนี้
    })
}
```

> **Note:** Template ที่สร้างไว้ใน DB ก่อนหน้านี้จะยังไม่มี ValueCodes จนกว่าจะ re-create (ลบ auto template ใน DB แล้วให้ event ถัดไป trigger auto-apply ใหม่ หรือ migrate ด้วย script)

---

## 4. Config Changes — อัปเดต `aibox.generalDetect.json`

**File:** `config/ingest/mappingsuggestions/AIBOX/aibox.generalDetect.json`

### 4.1 Common ValueCodes (ใช้ซ้ำหลาย fields)

```
COLOR_CODES = {
  "0":"Unknown","1":"Black","2":"Brown","3":"Blue","4":"Green",
  "5":"Gray","6":"Orange","7":"Pink","8":"Purple","9":"Red",
  "10":"White","11":"Yellow"
}

AGE_CODES = {
  "0":"Unknown","1":"Child","2":"Young Adult",
  "3":"Middle-Aged Adult","4":"Elderly Adult"
}

GENDER_CODES = { "0":"Unknown","1":"Male","2":"Female" }

UNKNOWN_KNOWN_CODES = { "0":"Unknown","1":"Yes","2":"No" }
  (ใช้กับ: mask, glasses, hat, riding)
```

### 4.2 Field Mappings ทั้งหมด

#### Root fields

| sourceField | targetField | valueCodes |
|---|---|---|
| `address` | `address` | — |
| `channelGb28181Id` | `channelGb28181Id` | — |
| `channelId` | `channelId` | — |
| `device` | `deviceName` | — |
| `deviceGb28181Id` | `deviceGb28181Id` | — |
| `deviceId` | `deviceId` | — |
| `alarmType` | `alarmType` | ALARM_TYPE_CODES (ดู §4.3) |

#### Human body attributes

| sourceField | targetField | valueCodes |
|---|---|---|
| `eventAttribute.age` | `eventAttribute.age` | AGE_CODES |
| `eventAttribute.gender` | `eventAttribute.gender` | GENDER_CODES |
| `eventAttribute.upper` | `eventAttribute.upper` | `{"0":"Unknown","1":"Short Sleeves","2":"Long Sleeves"}` |
| `eventAttribute.upperColor` | `eventAttribute.upperColor` | COLOR_CODES |
| `eventAttribute.upperTexture` | `eventAttribute.upperTexture` | `{"0":"Unknown","1":"Plaid","2":"Floral Print","3":"Solid Color","4":"Stripes"}` |
| `eventAttribute.lower` | `eventAttribute.lower` | `{"0":"Unknown","1":"Shorts","2":"Long Pants"}` |
| `eventAttribute.lowerColor` | `eventAttribute.lowerColor` | COLOR_CODES |
| `eventAttribute.skirt` | `eventAttribute.skirt` | `{"0":"Unknown","1":"Wearing Skirt","2":"Not Wearing Skirt"}` |
| `eventAttribute.hat` | `eventAttribute.hat` | `{"0":"Unknown","1":"Wearing Hat","2":"Not Wearing Hat"}` |
| `eventAttribute.mask` | `eventAttribute.mask` | `{"0":"Unknown","1":"Wearing Mask","2":"Not Wearing Mask"}` |
| `eventAttribute.glasses` | `eventAttribute.glasses` | `{"0":"Unknown","1":"Wearing Glasses","2":"Not Wearing Glasses"}` |
| `eventAttribute.backPack` | `eventAttribute.backPack` | `{"0":"Unknown","1":"Backpack","2":"Shoulder Bag","3":"Handbag","4":"No Bag"}` |
| `eventAttribute.riding` | `eventAttribute.riding` | `{"0":"Unknown","1":"Riding","2":"Not Riding"}` |
| `eventAttribute.direction` | `eventAttribute.direction` | `{"0":"Unknown","1":"Front","2":"Side","3":"Back"}` |
| `eventAttribute.hair` | `eventAttribute.hair` | `{"0":"Unknown","1":"Short Hair","2":"Long Hair","3":"Updo","4":"Bald","5":"Medium-Length Hair"}` |
| `eventAttribute.shoe` | `eventAttribute.shoe` | `{"0":"Unknown","1":"Leather Shoes","2":"Sandals","3":"Casual Shoes","4":"Knee-High Boots"}` |
| `eventAttribute.shoeColor` | `eventAttribute.shoeColor` | COLOR_CODES |

#### Facial attributes

| sourceField | targetField | valueCodes |
|---|---|---|
| `eventAttribute.name` | `eventAttribute.name` | — |
| `eventAttribute.listType` | `eventAttribute.listType` | `{"0":"Stranger","1":"Whitelist","2":"Red List","3":"Blacklist"}` |
| `eventAttribute.similarity` | `eventAttribute.similarity` | — |
| `eventAttribute.unit` | `eventAttribute.unit` | — |
| `eventAttribute.idCard` | `eventAttribute.idCard` | — |
| `eventAttribute.featureImageId` | `eventAttribute.featureImageId` | — |
| `eventAttribute.featureImageUrl` | `eventAttribute.featureImageUrl` | — |

#### Vest / Work Uniform (ที่มีอยู่แล้ว + เพิ่ม)

| sourceField | targetField | valueCodes |
|---|---|---|
| `eventAttribute.vestName` | `eventAttribute.vestName` | — |
| `eventAttribute.vestListType` | `eventAttribute.vestListType` | `{"0":"Stranger","1":"Whitelist","2":"Red List","3":"Blacklist"}` |
| `eventAttribute.vestSimilarity` | `eventAttribute.vestSimilarity` | — |
| `eventAttribute.vestUnit` | `eventAttribute.vestUnit` | — |
| `eventAttribute.vestFeatureImageId` | `eventAttribute.vestFeatureImageId` | — |
| `eventAttribute.vestFeatureImageUrl` | `eventAttribute.vestFeatureImageUrl` | — |

#### Non-motor vehicle attributes

| sourceField | targetField | valueCodes |
|---|---|---|
| `eventAttribute.helmet` | `eventAttribute.helmet` | `{"0":"Unknown","1":"Wearing Helmet","2":"Not Wearing Helmet"}` |
| `eventAttribute.number` | `eventAttribute.number` | `{"0":"Unknown","1":"1 Person","2":"2 Persons","3":"3 or More Persons"}` |
| `eventAttribute.nonVehicleType` | `eventAttribute.nonVehicleType` | `{"0":"Unknown","1":"Motorcycle","2":"Bicycle","3":"Tricycle"}` |
| `eventAttribute.nonVehiclePlate` | `eventAttribute.nonVehiclePlate` | — |
| `eventAttribute.nonvehicleColor` | `eventAttribute.nonvehicleColor` | COLOR_CODES |

#### Vehicle attributes

| sourceField | targetField | valueCodes |
|---|---|---|
| `eventAttribute.plateLicense` | `eventAttribute.plateLicense` | — |
| `eventAttribute.plateColor` | `eventAttribute.plateColor` | `{"0":"Unknown","1":"Black","3":"Blue","4":"Green","10":"White","11":"Yellow"}` |
| `eventAttribute.carType` | `eventAttribute.carType` | CAR_TYPE_CODES (ดู §4.4) |
| `eventAttribute.carColor` | `eventAttribute.carColor` | COLOR_CODES |
| `eventAttribute.carBrand` | `eventAttribute.carBrand` | — |
| `eventAttribute.speed` | `eventAttribute.speed` | — |
| `eventAttribute.arrowDirection` | `eventAttribute.arrowDirection` | `{"0":"Unknown","1":"Forward","2":"Reverse"}` |

> **หมายเหตุ:** `eventAttribute.listType` ใน vehicle context มีความหมายเดียวกับ facial (Whitelist/Blacklist) — ใช้ ValueCodes เดียวกัน

#### Other / Statistics attributes

| sourceField | targetField | valueCodes |
|---|---|---|
| `eventAttribute.trackTime` | `eventAttribute.trackTime` | — |
| `eventAttribute.stayTime` | `eventAttribute.stayTime` | — |
| `eventAttribute.arrowStartName` | `eventAttribute.arrowStartName` | — |
| `eventAttribute.arrowEndName` | `eventAttribute.arrowEndName` | — |
| `eventAttribute.num` | `eventAttribute.num` | — |

### 4.3 ALARM_TYPE_CODES

```json
{
  "0":  "Unknown",
  "1":  "Facial Capture",
  "2":  "Pedestrian Detection",
  "3":  "Non-motor Vehicle Detection",
  "4":  "Vehicle Detection",
  "7":  "Electric Vehicle Entry into Elevator",
  "9":  "Non-Motor Vehicle Illegal Parking",
  "11": "Trash Pile Detection",
  "12": "Trash Bin Overflow",
  "13": "Lingering Detection",
  "14": "Non-motor Vehicle Intrusion",
  "15": "Crowd Gathering",
  "16": "Fall Detection",
  "17": "Smoke Detection",
  "18": "Open Fire Detection",
  "19": "No Helmet Detection",
  "20": "High-Visibility Clothing Non-Wearing",
  "21": "Intrusion Detection",
  "22": "No Mask Detection",
  "24": "Mobile Phone Detection",
  "25": "Smoking Detection",
  "26": "Absenteeism Detection",
  "27": "Non-Motor Vehicle Illegal Parking",
  "28": "Traffic Congestion",
  "29": "Shared Bicycle Illegal Parking",
  "30": "Heavy Vehicle Detection",
  "31": "Prohibited Banner Detection",
  "32": "Electric Vehicle Helmet Non-Wearing",
  "35": "Motor Vehicle Intrusion",
  "36": "Door and Window Status",
  "39": "Electric Vehicle Passenger Detection",
  "40": "Food Delivery Rider Detection",
  "41": "Pedestrian Traffic Statistic",
  "42": "Personnel Stay Duration",
  "44": "Flammable Material Storage",
  "45": "Fire Lane Obstruction",
  "46": "Sleep on Duty Detection",
  "47": "Fire Extinguisher Location",
  "48": "Motor Vehicle Reverse Driving",
  "49": "Vehicle Traffic Statistic",
  "50": "Non-vehicle Wrong-Way Driving",
  "51": "Water Gauge Level Measurement",
  "52": "Large Construction Vehicle Inspection",
  "53": "Floating Object Detection",
  "54": "Hazardous Materials Vehicle Night Restriction",
  "56": "Work Uniform Recognition",
  "57": "Unenclosed Vehicle Cargo Compartment",
  "58": "Face Recognition",
  "59": "License Plate Recognition",
  "60": "Failure To Yield",
  "61": "Solid Line U-Turn",
  "62": "Emergency Vehicle Capture",
  "63": "Electric Vehicle Red Light Violation",
  "64": "Large Bus Nighttime Driving Ban",
  "65": "Fighting Detection",
  "67": "Person Climbing Detection",
  "68": "Unlicensed Street Vendors",
  "69": "Large Model",
  "70": "Third-Party Algorithm"
}
```

### 4.4 CAR_TYPE_CODES

```json
{
  "0":  "Unknown",
  "1":  "Bus",
  "2":  "Sedan",
  "3":  "Heavy Truck",
  "4":  "Pickup Truck",
  "5":  "Engineering Vehicle",
  "6":  "Box Truck",
  "7":  "SUV",
  "8":  "Passenger Car",
  "9":  "Coach",
  "10": "Truck",
  "11": "Light Truck",
  "12": "MPV",
  "13": "Van",
  "14": "Off-Road Vehicle",
  "15": "Muck Truck",
  "16": "Concrete Mixer Truck",
  "17": "Crane Truck",
  "18": "Pump Truck",
  "19": "Sanitation Transport Vehicle",
  "20": "Silt Transport Vehicle",
  "21": "Ambulance",
  "22": "Emergency Command Vehicle",
  "23": "Fire Truck",
  "24": "Police Car"
}
```

---

## 5. Migration ของ Templates ที่สร้างไว้แล้วใน DB

Templates ที่ถูก auto-create ก่อน change นี้ (`TemplateId = "auto:aibox.generalDetect.v1"`) จะยังไม่มี ValueCodes

วิธีจัดการ (เลือกทำหลัง deploy):

```
Option A: ลบ document จาก MongoDB แล้วรอ event ถัดไป trigger re-create
  db.mapping_templates.deleteOne({ templateId: "auto:aibox.generalDetect.v1", orgId: "<orgId>" })

Option B: สร้าง one-off migration script (ถ้ามีหลาย org)
  - สำหรับแต่ละ org: ดึง suggestion ใหม่ + สร้าง template ใหม่ด้วย converter ที่อัปเดตแล้ว
  - ไม่แนะนำถ้า org ปรับ template ไปแล้วด้วยมือ (จะ overwrite custom mapping)
```

---

## 6. ลำดับการ Implement

```
Step 1  models/ingestmod/mappingSuggestion.go
        → เพิ่ม ValueCodes field ใน SuggestionFieldMap

Step 2  internal/services/ingestsvc/suggestion_apply.go
        → propagate ValueCodes ใน convertSuggestionToTemplate

Step 3  config/ingest/mappingsuggestions/AIBOX/aibox.generalDetect.json
        → เพิ่ม fieldMappings ที่ขาด + เพิ่ม valueCodes ทุก enum field
        → เพิ่ม alarmType mapping
        → อัปเดต samplePayload ให้ครอบคลุม (ใช้ sample จาก screenshot)

Step 4  ทดสอบ
        → ส่ง test payload ที่มี eventAttribute.gender / eventAttribute.plateLicense / eventAttribute.similarity
        → ตรวจว่า _label fields ปรากฏใน normalized event
        → ตรวจว่า alarmType_label ถูกต้อง

Step 5  Migration (ถ้ามี org ที่ใช้งาน production อยู่แล้ว)
```

---

## 7. Expected Output หลัง Normalize

ตัวอย่าง AIBOX payload (pedestrian):
```json
{
  "eventAttribute": {
    "age": 2,
    "gender": 1,
    "upper": 2,
    "upperColor": 9
  },
  "alarmType": 2
}
```

Normalized event payload:
```json
{
  "eventAttribute": {
    "age": 2,          "age_label": "Young Adult",
    "gender": 1,       "gender_label": "Male",
    "upper": 2,        "upper_label": "Long Sleeves",
    "upperColor": 9,   "upperColor_label": "Red"
  },
  "alarmType": 2,      "alarmType_label": "Pedestrian Detection"
}
```
