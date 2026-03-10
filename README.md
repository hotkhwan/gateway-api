# KLYNX Gateway API

ระบบ Backend หลักของ KLYNX Platform เขียนด้วย [Go Fiber](https://gofiber.io/)
ทำหน้าที่เป็น Gateway กลางสำหรับ event ingestion จาก devices, operator workflow, canonical event delivery, และการจัดการ users/groups/devices/media

**Module:** `github.com/hotkhwan/gateway-api`

---

## Features

- **Event Ingestion Pipeline** — รับ raw events จาก devices, fingerprint auto-match, operator approval, publish to Kafka
- **Authentication & Authorization** — Keycloak JWT + Permify (gRPC/REST) สิทธิ์ 3 ระดับ (Owner/Editor/Viewer)
- **User & Group Management** — จัดการ users, groups, roles, permissions
- **Device Management** — จัดการอุปกรณ์ CCTV, ATA, KControl, KWatch
- **Media & Storage** — อัปโหลด/ดาวน์โหลดไฟล์ผ่าน MinIO/S3, Map/KML support
- **Real-time Communication** — MQTT สำหรับ device control messages
- **Audit Logging** — บันทึก mutations (POST/PUT/PATCH/DELETE) แบบ async
- **Observability** — OpenTelemetry tracing + zerolog structured logging
- **Swagger UI** — เอกสาร API แบบ interactive

---

## Tech Stack

| Component | Technology |
| --- | --- |
| HTTP Framework | Go Fiber |
| Database | MongoDB |
| Cache | Redis |
| Message Queue | Kafka |
| Storage | MinIO / S3 |
| Authorization | Permify |
| Auth | Keycloak |
| Tracing | OpenTelemetry (OTLP) |
| Logging | zerolog |
| Real-time | MQTT |
| Docs | Swagger (`swag`) |

---

## Project Structure

```text
.
├── config/              # External service initialization
│   ├── mongo.go         # MongoDB client + bootstrap
│   ├── redis.go         # Redis client
│   ├── kafka.go         # Kafka producer/consumer config
│   ├── s3.go            # MinIO/S3 client
│   ├── otel.go          # OpenTelemetry tracer
│   ├── permifyRest.go   # Permify REST client
│   ├── permifygRPC.go   # Permify gRPC client
│   └── masterconf.go    # Master env config loader
│
├── controllers/         # HTTP request handlers (controller layer)
│   ├── authapi/         # Authentication endpoints
│   ├── authzapi/        # Authorization endpoints
│   ├── devapi/          # Device management
│   ├── grpapi/          # Group management
│   ├── ingestctl/       # Event ingestion management
│   ├── kctrlapi/        # System/KControl
│   ├── kwatapi/         # Watchlist management
│   ├── mapapi/          # Map & KML
│   ├── mediapi/         # Media handling
│   ├── usrapi/          # User management
│   └── ...
│
├── internal/
│   ├── app/             # container.go — dependency injection root
│   ├── gateways/        # External service integrations (authgw, authzgw, mediagw, …)
│   ├── kafka/           # Kafka consumers & producer singleton
│   ├── logger/          # zerolog setup + middleware
│   ├── middleware/       # HTTP middlewares (auth, audit, trace)
│   ├── mqtt/            # MQTT client & handlers
│   ├── repo/            # Data repositories (stomongo, stos3minio)
│   │   ├── stomongo/    # MongoDB wrapper
│   │   └── stos3minio/  # S3/MinIO wrapper
│   ├── services/        # Business logic layer
│   └── crypto/          # secretbox encryption
│
├── models/              # Domain data structures (DB shape)
│   ├── gmod/            # Global response types (PaginationResponse, etc.)
│   ├── devmod/          # Device models
│   ├── grpmod/          # Group models
│   ├── authmod/         # Auth models
│   └── ...
│
├── router/              # Route definitions + middleware wiring
├── utils/               # Cross-domain utilities (httputil, authutil, traceutil, …)
├── docs/                # Swagger generated docs
├── tests/               # Integration tests
├── main.go              # Entry point
└── go.mod
```

### Architecture Flow

```text
repo → service → controller → router
```

- `repo` — DB access only
- `service` — business logic, no HTTP imports
- `controller` — parse request, call service, write response
- `router` — route definitions + middleware only

---

## Getting Started

### 1. Install Go 1.24+

```bash
wget https://go.dev/dl/go1.24.4.linux-amd64.tar.gz
rm -rf /usr/local/go
tar -C /usr/local -xzf go1.24.4.linux-amd64.tar.gz
export GOPATH=$HOME/go
export PATH=$PATH:/usr/local/go/bin:$GOPATH/bin
go version
```

### 2. Install air (hot reload)

```bash
go install github.com/air-verse/air@latest
```

### 3. Create `.env` file

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
KAFKA_GROUP_ID=gateway-group
KAFKA_AUTO_OFFSET_RESET=earliest

# S3 / MinIO
S3_ENDPOINT=
S3_REGION=
S3_ACCESS_KEY=
S3_SECRET_KEY=
S3_BUCKET=
S3_USE_SSL=true
S3_EXPIRY=3600

# Keycloak
KEYCLOAK_URL=http://localhost:8080
KEYCLOAK_REALM=klynx

# Permify
PERMIFY_GRPC_ADDR=localhost:3001
PERMIFY_REST_ADDR=http://localhost:3002

# OpenTelemetry
OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4318
OTEL_SERVICE_NAME=gateway-api

# Logging
LOG_LEVEL=info
LOG_PRETTY=false

# Swagger
SWAGGER_PATH=/docs
BASE_PATH=/api/v1
```

### 4. Install dependencies

```bash
go mod tidy
```

### 5. Run

```bash
go run main.go --env=dev
# or with hot reload
air
```

Docker:

```bash
docker build -t gateway-api .
docker run --rm -p 8080:8080 --env-file .env gateway-api
```

---

## API Endpoints

Base path: `/api/v1`

### Authentication

- `POST /auth/signin` — เข้าสู่ระบบ
- `POST /auth/refresh-token` — ต่ออายุ token
- `POST /auth/reset-password` — รีเซ็ตรหัสผ่าน

### Authorization

- `POST /authz/resource/grant` — ให้สิทธิ์ resource
- `POST /authz/resource/revoke` — ถอนสิทธิ์ resource

### Event Ingestion

- `POST /events/:orgId` — รับ raw event จาก device (hot-path, no auth)
- `GET /ingest/management` — ดู pending events
- `POST /ingest/management/:id/approve` — อนุมัติ event
- `POST /ingest/management/bulk/approve` — bulk approve (max 100)
- `POST /ingest/mappingTemplates` — สร้าง mapping template

### User & Group Management

- `GET /users` — ดึงรายการ users
- `POST /groups` — สร้าง group
- `GET /groups/:id/members` — สมาชิกใน group
- `GET /groups/:id/roles` — roles ใน group

### Device Management

- `GET /devices` — ดึงรายการ devices
- `POST /devices` — เพิ่ม device
- `GET /devices/:id` — ดึงข้อมูล device
- `PUT /devices/:id` — อัปเดต device

### Media & Maps

- `POST /media/upload` — อัปโหลดไฟล์
- `GET /maps/kml` — ดึงไฟล์ KML
- `POST /maps/kml/upload` — อัปโหลด KML

### Watchlist

- `GET /watchlist` — ดึง watchlist
- `POST /watchlist` — สร้าง watchlist
- `PUT /watchlist/:id` — อัปเดต watchlist

### System Control (KControl)

- `GET /kcontrol` — ดึงรายการ
- `POST /kcontrol/export` — export
- `DELETE /kcontrol/:id` — ลบ

### Docs

- `GET /docs/index.html` — Swagger UI

---

## Event Pipeline

```text
external device
  → POST /events/:orgId  (stored as "pending")
      ├─ fingerprint match? → YES → auto-approve → Kafka raw.events
      └─ NO → operator reviews
                 ├─ manual approve → Kafka raw.events
                 └─ bulk approve  → Kafka raw.events

  → normalizer service consumes raw.events
  → CanonicalEvent → MongoDB + S3 binary
  → delivery workers → webhook / retry / DLQ
```

---

## Swagger Docs

หลังรันแล้วเปิดที่:

```text
http://localhost:8080/docs/index.html
```

Generate docs:

```bash
swag init -g main.go --output docs/
```

---

## Development Rules

ดูรายละเอียดใน [`.claude/rule/`](.claude/rule/):

- [code-style.md](.claude/rule/code-style.md) — Architecture, package naming, API response, logging, tracing, Swagger
- [security.md](.claude/rule/security.md) — Auth, authorization, audit logging, crypto, input validation
- [test.md](.claude/rule/test.md) — Unit test patterns, mock, integration tests

### Key rules (quick summary)

- Every `.go` file must start with a path comment
- Architecture: `repo → service → controller → router` — no skipping layers
- Service layer never imports `fiber` or `net/http`
- All timestamps RFC3339 UTC
- Always `regexp.QuoteMeta(search)` for MongoDB regex queries
- `logger.Dev` must not exist in any file merged to `main`
- Index creation in bootstrap only (`mongoBootstrap.go`)
- Sensitive fields (passwords, tokens, secrets) encrypted via `secretbox` before storing

---

## License

MIT © 2025 KLYNX Dev Team
