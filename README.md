# KLYNX Backend API

ระบบ Backend สำหรับ KLYNX เขียนด้วย [Go Fiber](https://gofiber.io/) พร้อมระบบ Authentication, Authorization และการจัดการทรัพยากรแบบครบวงจร

---

## 📦 Features

- 🔐 ระบบ Authentication & Authorization
- 📚 Swagger UI สำหรับเอกสาร API
- 📦 MongoDB Integration
- 🗄️ Redis Cache
- 📨 Kafka Message Queue
- � S3 Storage Integration
- 🛡️ Permify Authorization
- � Business Intelligence Integration
- 🗺️ Map และ KML Support
- 📹 Video และ Media Processing
- 🔍 Watchlist Management
- 📱 Device Management
- 👥 User และ Group Management
- 🔄 Real-time Communication (MQTT)
- 📈 OpenTelemetry Integration

---

| Domain name | Package name (แนะนำ)     | หมายเหตุ                             |
| ----------- | ------------------------ | ------------------------------------ |
| `global`	  | `globalmodel, gmod,`     | ✅ ชัดเจนว่ารวม model ส่วนกลาง
| `common`	  | `commonmodel, cmod`      | ✅ เหมาะกับ response/message
| `shared`	  | `sharedmodel, smod`      | ✅ ใช้เมื่อ model ใช้ข้าม domain
| `devices`   | `devmodel`, `devmod`        | ✅ ทั่วไปสุด                          |
| `kcontrol`  | `kctrlmodel`, `kctrlmod`    | ✅ สั้น-จำง่าย                        |
| `users`     | `usermodel`, `usrmod`       | ✅ ไม่ชนกับ Go builtin                |
| `face_cctv` | `faceai`, `facemodel`    | ✅ ชัดว่ามาจาก AI/ภาพ                 |
| `groups`    | `groupmodel`, `grpmod`      | ✅ ย่อให้ไม่ชนกับ Go `sync.WaitGroup` |
| `s3`        | `s3model`, `s3file`      | ✅ เฉพาะเจาะจง                        |
| `auth`      | `authmodel`, `authz`     | ✅ แยกจาก middleware ได้              |
| `token`     | `tokenmodel`, `jwtmod` | ✅ ชัดเจน                             |
| `logs`      | `logmodel`, `auditlog`   | ✅ ใช้ในระบบ log/audit                |
| `camera`    | `cameramodel`, `cammod`     | ✅ ชัดว่าเป็นภาพ                      |
| `alarm`     | `alarmmodel`, `almmod`      | ✅ ตรงกับอุปกรณ์                      |
| `event`     | `eventmodel`, `evtmod`      | ✅ ใช้ร่วมหลาย module ได้             |


| ชื่อ                              | ค่าเลข | ความหมาย                        |
| --------------------------------- | ------ | ------------------------------- |
| `fiber.StatusOK`                  | `200`  | สำเร็จทั่วไป (GET, PATCH)       |
| `fiber.StatusCreated`             | `201`  | สร้างสำเร็จ (POST)              |
| `fiber.StatusBadRequest`          | `400`  | ข้อมูลไม่ถูกต้อง (client error) |
| `fiber.StatusInternalServerError` | `500`  | server error                    |


## ⚙️ การติดตั้ง (Local)

### 1. Clone โปรเจกต์

```bash
git clone https://github.com/your-org/klynx-api.git
cd klynx-api
wget https://go.dev/dl/go1.24.4.linux-amd64.tar.gz
rm -rf /usr/local/go
tar -C /usr/local -xzf go1.24.4.linux-amd64.tar.gz
ln -sf /usr/local/go/bin/go /usr/bin/go
go install github.com/air-verse/air@latest
air -v
# ตั้ง GOPATH และ PATH ให้แน่นอน
export GOPATH=$HOME/go
export PATH=$PATH:/usr/local/go/bin:$GOPATH/bin

# ตรวจสอบ Go version
go version
```

### 2. สร้างไฟล์ `.env` สำหรับ environment ต่างๆ

```env
# Server
PORT=8080
ENV=development

# MongoDB
MONGO_URI=mongodb://localhost:27017
MONGO_DATABASE=klynx

# Redis
REDIS_HOST=localhost
REDIS_PORT=6379
REDIS_PASSWORD=

# Kafka
KAFKA_BROKERS=localhost:9092
KAFKA_GROUP_ID=klynx-group
KAFKA_AUTO_OFFSET_RESET=earliest

# S3
S3_ENDPOINT=
S3_REGION=
S3_ACCESS_KEY=
S3_SECRET_KEY=
S3_BUCKET=
S3_USE_SSL=true

# Permify
PERMIFY_GRPC_ADDR=localhost:3001
PERMIFY_REST_ADDR=http://localhost:3002

# OpenTelemetry
OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4318
OTEL_SERVICE_NAME=klynx-api

# Swagger Docs
SWAGGER_PATH=/docs

# API Base Path
BASE_PATH=/api/v1
```

### 3. ติดตั้ง Dependencies

```bash
go mod tidy
```

### 4. Run ระบบ

```bash
go run main.go --env=dev
```

หรือใช้ Docker:

```bash
docker build -t klynx-api .
docker run --rm -p 3001:3001 --env-file .env_dev klynx-api
```

---

## 🔗 API Endpoints

API แบ่งเป็นหมวดหมู่ดังนี้:

### Authentication & Authorization
- POST `/auth/signin` - เข้าสู่ระบบ
- POST `/auth/refresh-token` - ต่ออายุ token
- POST `/auth/reset-password` - รีเซ็ตรหัสผ่าน
- POST `/authz/resource/grant` - ให้สิทธิ์การเข้าถึงทรัพยากร
- POST `/authz/resource/revoke` - ถอนสิทธิ์การเข้าถึงทรัพยากร

### User & Group Management
- GET `/users` - ดึงรายการผู้ใช้
- POST `/groups` - สร้างกลุ่มใหม่
- GET `/groups/{id}/members` - ดึงรายการสมาชิกในกลุ่ม
- GET `/groups/{id}/roles` - ดึงรายการบทบาทในกลุ่ม

### Device Management
- GET `/devices` - ดึงรายการอุปกรณ์
- POST `/devices` - เพิ่มอุปกรณ์ใหม่
- GET `/devices/{id}` - ดึงข้อมูลอุปกรณ์
- PUT `/devices/{id}` - อัปเดตข้อมูลอุปกรณ์

### Resource Management
- GET `/resources` - ดึงรายการทรัพยากร
- POST `/resources/bulk` - เพิ่มทรัพยากรแบบกลุ่ม

### Media & File Management
- POST `/media/upload` - อัปโหลดไฟล์
- GET `/maps/kml` - ดึงไฟล์ KML
- POST `/maps/kml/upload` - อัปโหลดไฟล์ KML

### Watchlist Management
- GET `/watchlist` - ดึงรายการ watchlist
- POST `/watchlist` - สร้าง watchlist ใหม่
- GET `/watchlist/{id}` - ดึงข้อมูล watchlist
- PUT `/watchlist/{id}` - อัปเดต watchlist

### System Control
- GET `/kcontrol` - ดึงรายการควบคุมระบบ
- POST `/kcontrol/export` - ส่งออกข้อมูลควบคุม
- DELETE `/kcontrol/{id}` - ลบรายการควบคุม

### Documentation
- GET `/docs/index.html` - Swagger UI

> เส้นทาง `/docs/*` และ `/api/v1` สามารถปรับได้ผ่าน ENV: `SWAGGER_PATH`, `BASE_PATH`

---

## 📁 โครงสร้างโปรเจกต์

```txt
.
├── config/              # การตั้งค่าสำหรับ external services
│   ├── kafka.go        # Kafka configuration
│   ├── mongo.go        # MongoDB configuration
│   ├── redis.go        # Redis configuration
│   ├── s3.go           # S3 storage configuration
│   ├── otel.go         # OpenTelemetry configuration
│   └── permify.go      # Permify (authorization) configuration
│
├── controllers/         # HTTP request handlers
│   ├── authapi/        # Authentication endpoints
│   ├── authzapi/       # Authorization endpoints
│   ├── biapi/          # Business Intelligence endpoints
│   ├── devapi/         # Device management
│   ├── grpapi/         # Group management
│   ├── kctrlapi/       # System control
│   ├── kschapi/        # Chat และ video
│   ├── kwatapi/        # Watchlist management
│   ├── mapapi/         # Map และ KML
│   ├── mediapi/        # Media handling
│   ├── optapi/         # Options และ settings
│   ├── rscapi/         # Resource management
│   └── usrapi/         # User management
│
├── internal/           # Internal packages
│   ├── gateways/       # External service integrations
│   ├── kafka/          # Kafka message handling
│   ├── logger/         # Logging implementation
│   ├── middleware/     # HTTP middlewares
│   ├── mqtt/           # MQTT client
│   ├── repo/           # Data repositories
│   └── services/       # Business logic
│
├── models/             # Data models และ types
│   ├── authmod/        # Authentication models
│   ├── devmod/         # Device models
│   ├── grpmod/         # Group models
│   └── ...            # Domain models อื่นๆ
│
├── router/             # API route definitions
├── utils/             # Utility functions
├── docs/              # API documentation
├── main.go            # Entry point
└── go.mod             # Go dependencies
```

---

## 🥪 Swagger Documentation

หลังจากรันแล้ว เปิดได้ที่:

```
http://localhost:3001/docs/index.html
```

---

## 📌 Development Notes

### การจัดการ Response
- ใช้ structured response types จาก `models/gmod` สำหรับ standard responses
- ใช้ `fiber.Map` เฉพาะกรณี dynamic response ที่ไม่ต้องการ strict type checking

### Authorization
- ระบบใช้ Permify สำหรับจัดการสิทธิ์
- สิทธิ์แบ่งเป็น 3 ระดับ: Owner, Editor, Viewer
- ทุก resource operation ต้องผ่านการตรวจสอบสิทธิ์

### Monitoring
- ใช้ OpenTelemetry สำหรับ tracing และ metrics
- Logging ผ่าน structured logger ใน `internal/logger`

### Performance
- ใช้ Redis สำหรับ caching
- Bulk operations ควรใช้ batch size ที่เหมาะสม
- ระวังเรื่อง N+1 queries ใน MongoDB operations

### Security
- ต้องมี JWT token ในทุก protected routes
- ตรวจสอบ input validation ทุกครั้ง
- ใช้ HTTPS ในการ deploy
- ระวังเรื่อง CORS settings

### Testing
- Unit tests อยู่ใน package เดียวกับ code
- Integration tests แยกไว้ใน `tests/` directory
- ใช้ mock สำหรับ external dependencies

---

## 🧑‍💻 Contributors

- @yourname – core developer
- @dev2 – Mongo integration
- @dev3 – Swagger and API security

---

## 📜 License

MIT © 2025 KLYNX Dev Team

